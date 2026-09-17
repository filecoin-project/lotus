// Tests for the simulation endpoints, eth_call and eth_estimateGas: sender handling
// (contract, non-existent, placeholder and EOA senders), revert data propagation, gas
// estimation accuracy, block parameter handling, and simulation state isolation.
package itests

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/big"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/types/ethtypes"
	"github.com/filecoin-project/lotus/itests/kit"
)

// nonExistentAddr creates a deterministic non-existent Ethereum address from a seed byte.
func nonExistentAddr(seed byte) ethtypes.EthAddress {
	return ethtypes.EthAddress{0xde, 0xad, 0xbe, 0xef, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, seed}
}

// panicSelector returns the hex-encoded selector for Panic(uint256) errors with the given code.
// Solidity uses Panic(uint256) for assert failures (0x01), division by zero (0x12), etc.
func panicSelector(code byte) string {
	selector := hex.EncodeToString(kit.CalcFuncSignature("Panic(uint256)"))
	return selector + "00000000000000000000000000000000000000000000000000000000000000" + hex.EncodeToString([]byte{code})
}

// errorSelector returns the hex-encoded 4-byte selector for a custom error signature.
func errorSelector(sig string) string {
	return hex.EncodeToString(kit.CalcFuncSignature(sig))
}

// skipSenderEnv holds the common test environment.
type skipSenderEnv struct {
	ctx             context.Context
	cancel          context.CancelFunc
	client          *kit.TestFullNode
	contractAddr    ethtypes.EthAddress
	contractFilAddr address.Address
	deployerFilAddr address.Address
	eoaAddr         ethtypes.EthAddress
	eoaAddr2        ethtypes.EthAddress
}

// setupSkipSenderTest creates a test environment with a deployed contract and funded EOAs.
func setupSkipSenderTest(t *testing.T) *skipSenderEnv {
	ctx, cancel, client := kit.SetupFEVMTest(t)

	// Create and fund two EOAs
	_, eoaAddr, filAddr := client.EVM().NewAccount()
	kit.SendFunds(ctx, t, client, filAddr, types.FromFil(10))

	_, eoaAddr2, _ := client.EVM().NewAccount()

	// Deploy SimpleCoin contract
	deployerFilAddr, contractFilAddr := client.EVM().DeployContractFromFilename(ctx, "contracts/SimpleCoin.hex")
	actor, err := client.StateGetActor(ctx, contractFilAddr, types.EmptyTSK)
	require.NoError(t, err)
	require.NotNil(t, actor.DelegatedAddress)

	contractAddr, err := ethtypes.EthAddressFromFilecoinAddress(*actor.DelegatedAddress)
	require.NoError(t, err)

	return &skipSenderEnv{
		ctx: ctx, cancel: cancel, client: client,
		contractAddr: contractAddr, contractFilAddr: contractFilAddr, deployerFilAddr: deployerFilAddr,
		eoaAddr: eoaAddr, eoaAddr2: eoaAddr2,
	}
}

// loadInitcode reads and decodes a hex contract fixture for use as creation calldata.
func loadInitcode(t *testing.T, path string) ethtypes.EthBytes {
	contractHex, err := os.ReadFile(path)
	require.NoError(t, err)
	initcode, err := hex.DecodeString(string(contractHex))
	require.NoError(t, err)
	return initcode
}

func TestEthCallSkipSender(t *testing.T) {
	env := setupSkipSenderTest(t)
	defer env.cancel()

	nonExistent := nonExistentAddr(0x01)
	blkParam := ethtypes.NewEthBlockNumberOrHashFromPredefined("latest")

	gasPrice := ethtypes.EthBigInt(types.NewInt(1000000000))

	// Deploy Errors contract for revert test
	_, errorsFilAddr := env.client.EVM().DeployContractFromFilename(env.ctx, "contracts/Errors.hex")
	errorsActor, err := env.client.StateGetActor(env.ctx, errorsFilAddr, types.EmptyTSK)
	require.NoError(t, err)
	errorsAddr, err := ethtypes.EthAddressFromFilecoinAddress(*errorsActor.DelegatedAddress)
	require.NoError(t, err)

	initcode := loadInitcode(t, "contracts/SimpleCoin.hex")

	tests := []struct {
		name    string
		call    ethtypes.EthCall
		wantErr bool
		check   func(*testing.T, ethtypes.EthBytes, error)
	}{
		{
			// KNOWN GETH COMPATIBILITY GAP: Geth simulates creation from a contract
			// sender; we reject it because the EAM's CreateExternal refuses non-account
			// callers in actor code, beyond the reach of sender-check skipping. This
			// case documents the current scope of the feature; a future rewrite to EAM
			// Create (with the nonce from the EVM actor state) should flip it to
			// expect success.
			name:    "CreateFromContract",
			call:    ethtypes.EthCall{From: &env.contractAddr, To: nil, Data: initcode},
			wantErr: true,
			check: func(t *testing.T, _ ethtypes.EthBytes, err error) {
				require.Contains(t, err.Error(), "disallowed caller")
			},
		},
		{
			// A non-existent sender is promoted to EthAccount on the explicit path, a
			// caller type the EAM accepts, so creation simulation works.
			name:    "CreateFromNonExistent",
			call:    ethtypes.EthCall{From: &nonExistent, To: nil, Data: initcode},
			wantErr: false,
		},
		{
			name:    "FromContract",
			call:    ethtypes.EthCall{From: &env.contractAddr, To: &env.eoaAddr},
			wantErr: false,
		},
		{
			name:    "FromContractWithGasPrice",
			call:    ethtypes.EthCall{From: &env.contractAddr, To: &env.eoaAddr, GasPrice: gasPrice},
			wantErr: false,
		},
		{
			// The Safe SDK pattern (EIP-1271 signature checks): a contract calling itself.
			name:    "FromContractToSelf",
			call:    ethtypes.EthCall{From: &env.contractAddr, To: &env.contractAddr, Data: kit.EvmCalldata("getBalance(address)", kit.EvmWordBytes(env.contractAddr[:]))},
			wantErr: false,
		},
		{
			name:    "FromContractWithValue",
			call:    ethtypes.EthCall{From: &env.contractAddr, To: &env.eoaAddr, Value: ethtypes.EthBigInt(types.FromFil(1))},
			wantErr: true,
			check: func(t *testing.T, _ ethtypes.EthBytes, err error) {
				require.Contains(t, strings.ToLower(err.Error()), "insufficient")
			},
		},
		{
			name:    "FromNonExistent",
			call:    ethtypes.EthCall{From: &nonExistent, To: &env.eoaAddr},
			wantErr: false,
		},
		{
			name:    "FromNonExistentWithGasPrice",
			call:    ethtypes.EthCall{From: &nonExistent, To: &env.eoaAddr, GasPrice: gasPrice},
			wantErr: false,
		},
		{
			name:    "FromNonExistentToContractWithData",
			call:    ethtypes.EthCall{From: &nonExistent, To: &errorsAddr, Data: kit.CalcFuncSignature("failRevertEmpty()")},
			wantErr: true,
			check: func(t *testing.T, _ ethtypes.EthBytes, err error) {
				var execErr *api.ErrExecutionReverted
				require.True(t, errors.As(err, &execErr), "expected ErrExecutionReverted, got %T: %v", err, err)
				require.Contains(t, execErr.Message, "none")
			},
		},
		{
			name:    "FromNonExistentWithValue",
			call:    ethtypes.EthCall{From: &nonExistent, To: &env.eoaAddr, Value: ethtypes.EthBigInt(types.FromFil(1))},
			wantErr: true,
			check: func(t *testing.T, _ ethtypes.EthBytes, err error) {
				require.Contains(t, strings.ToLower(err.Error()), "insufficient")
			},
		},
		{
			name:    "FromEOA",
			call:    ethtypes.EthCall{From: &env.eoaAddr, To: &env.eoaAddr2},
			wantErr: false,
		},
		{
			name:    "FromNil",
			call:    ethtypes.EthCall{From: nil, To: &env.eoaAddr},
			wantErr: false,
		},
		{
			name:    "ValueOverBalance",
			call:    ethtypes.EthCall{From: &env.eoaAddr, To: &nonExistent, Value: ethtypes.EthBigInt(types.FromFil(11))},
			wantErr: true,
			check: func(t *testing.T, _ ethtypes.EthBytes, err error) {
				require.Contains(t, strings.ToLower(err.Error()), "insufficient")
			},
		},
		{
			name:    "RevertDivideByZero",
			call:    ethtypes.EthCall{From: &env.eoaAddr, To: &errorsAddr, Data: kit.CalcFuncSignature("failDivZero()")},
			wantErr: true,
			check: func(t *testing.T, _ ethtypes.EthBytes, err error) {
				var execErr *api.ErrExecutionReverted
				require.True(t, errors.As(err, &execErr), "expected ErrExecutionReverted, got %T: %v", err, err)
				require.Contains(t, execErr.Message, "DivideByZero")
				require.Contains(t, execErr.Data, panicSelector(0x12))
			},
		},
		{
			name:    "RevertAssert",
			call:    ethtypes.EthCall{From: &env.eoaAddr, To: &errorsAddr, Data: kit.CalcFuncSignature("failAssert()")},
			wantErr: true,
			check: func(t *testing.T, _ ethtypes.EthBytes, err error) {
				var execErr *api.ErrExecutionReverted
				require.True(t, errors.As(err, &execErr), "expected ErrExecutionReverted, got %T: %v", err, err)
				require.Contains(t, execErr.Message, "Assert")
				require.Contains(t, execErr.Data, panicSelector(0x01))
			},
		},
		{
			name:    "RevertWithReason",
			call:    ethtypes.EthCall{From: &env.eoaAddr, To: &errorsAddr, Data: kit.CalcFuncSignature("failRevertReason()")},
			wantErr: true,
			check: func(t *testing.T, _ ethtypes.EthBytes, err error) {
				var execErr *api.ErrExecutionReverted
				require.True(t, errors.As(err, &execErr), "expected ErrExecutionReverted, got %T: %v", err, err)
				require.Contains(t, execErr.Message, "my reason")
			},
		},
		{
			name:    "RevertEmpty",
			call:    ethtypes.EthCall{From: &env.eoaAddr, To: &errorsAddr, Data: kit.CalcFuncSignature("failRevertEmpty()")},
			wantErr: true,
			check: func(t *testing.T, _ ethtypes.EthBytes, err error) {
				var execErr *api.ErrExecutionReverted
				require.True(t, errors.As(err, &execErr), "expected ErrExecutionReverted, got %T: %v", err, err)
				require.Contains(t, execErr.Message, "none")
				require.Equal(t, "0x", execErr.Data)
			},
		},
		{
			name:    "RevertCustomError",
			call:    ethtypes.EthCall{From: &env.eoaAddr, To: &errorsAddr, Data: kit.CalcFuncSignature("failCustom()")},
			wantErr: true,
			check: func(t *testing.T, _ ethtypes.EthBytes, err error) {
				var execErr *api.ErrExecutionReverted
				require.True(t, errors.As(err, &execErr), "expected ErrExecutionReverted, got %T: %v", err, err)
				require.Contains(t, execErr.Data, errorSelector("CustomError()"))
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := env.client.EthCall(env.ctx, tc.call, blkParam)
			if tc.wantErr {
				require.Error(t, err)
				if tc.check != nil {
					tc.check(t, result, err)
				}
			} else {
				require.NoError(t, err, "eth_call should succeed")
			}
		})
	}
}

func TestEthEstimateGasSkipSender(t *testing.T) {
	env := setupSkipSenderTest(t)
	defer env.cancel()

	nonExistent := nonExistentAddr(0x02)
	blkParam := ethtypes.NewEthBlockNumberOrHashFromPredefined("latest")
	gasPrice := ethtypes.EthBigInt(types.NewInt(1000000000))

	// Deploy Errors contract for revert test
	_, errorsFilAddr := env.client.EVM().DeployContractFromFilename(env.ctx, "contracts/Errors.hex")
	errorsActor, err := env.client.StateGetActor(env.ctx, errorsFilAddr, types.EmptyTSK)
	require.NoError(t, err)
	errorsAddr, err := ethtypes.EthAddressFromFilecoinAddress(*errorsActor.DelegatedAddress)
	require.NoError(t, err)

	// Gas bounds for sanity checks
	const (
		minGas = uint64(21_000)         // Minimum gas for a simple transfer
		maxGas = uint64(10_000_000_000) // Upper bound to detect overflow
	)

	initcode := loadInitcode(t, "contracts/SimpleCoin.hex")

	tests := []struct {
		name    string
		call    ethtypes.EthCall
		wantErr bool
		check   func(*testing.T, ethtypes.EthUint64, error)
	}{
		{
			// KNOWN GETH COMPATIBILITY GAP: see the matching case in
			// TestEthCallSkipSender; a future rewrite to EAM Create should flip this
			// to expect a successful estimate.
			name:    "CreateFromContract",
			call:    ethtypes.EthCall{From: &env.contractAddr, To: nil, Data: initcode},
			wantErr: true,
			check: func(t *testing.T, _ ethtypes.EthUint64, err error) {
				require.Contains(t, err.Error(), "disallowed caller")
			},
		},
		{
			name:    "CreateFromNonExistent",
			call:    ethtypes.EthCall{From: &nonExistent, To: nil, Data: initcode},
			wantErr: false,
			check: func(t *testing.T, gas ethtypes.EthUint64, _ error) {
				require.GreaterOrEqual(t, uint64(gas), minGas, "gas should be at least minimum transfer gas")
				require.Less(t, uint64(gas), maxGas, "gas should not overflow")
			},
		},
		{
			name:    "FromContract",
			call:    ethtypes.EthCall{From: &env.contractAddr, To: &env.eoaAddr},
			wantErr: false,
			check: func(t *testing.T, gas ethtypes.EthUint64, _ error) {
				require.GreaterOrEqual(t, uint64(gas), minGas, "gas should be at least minimum transfer gas")
				require.Less(t, uint64(gas), maxGas, "gas should not overflow")
			},
		},
		{
			name:    "FromContractWithGasPrice",
			call:    ethtypes.EthCall{From: &env.contractAddr, To: &env.eoaAddr, GasPrice: gasPrice},
			wantErr: false,
			check: func(t *testing.T, gas ethtypes.EthUint64, _ error) {
				require.GreaterOrEqual(t, uint64(gas), minGas, "gas should be at least minimum transfer gas")
				require.Less(t, uint64(gas), maxGas, "gas should not overflow")
			},
		},
		{
			// The Safe SDK pattern (EIP-1271 signature checks): a contract calling itself.
			name:    "FromContractToSelf",
			call:    ethtypes.EthCall{From: &env.contractAddr, To: &env.contractAddr, Data: kit.EvmCalldata("getBalance(address)", kit.EvmWordBytes(env.contractAddr[:]))},
			wantErr: false,
			check: func(t *testing.T, gas ethtypes.EthUint64, _ error) {
				require.GreaterOrEqual(t, uint64(gas), minGas, "gas should be at least minimum transfer gas")
				require.Less(t, uint64(gas), maxGas, "gas should not overflow")
			},
		},
		{
			name:    "FromContractWithValue",
			call:    ethtypes.EthCall{From: &env.contractAddr, To: &env.eoaAddr, Value: ethtypes.EthBigInt(types.FromFil(1))},
			wantErr: true,
			check: func(t *testing.T, _ ethtypes.EthUint64, err error) {
				require.Contains(t, strings.ToLower(err.Error()), "insufficient")
			},
		},
		{
			name:    "FromNonExistent",
			call:    ethtypes.EthCall{From: &nonExistent, To: &env.eoaAddr},
			wantErr: false,
			check: func(t *testing.T, gas ethtypes.EthUint64, _ error) {
				require.GreaterOrEqual(t, uint64(gas), minGas, "gas should be at least minimum transfer gas")
				require.Less(t, uint64(gas), maxGas, "gas should not overflow")
			},
		},
		{
			name:    "FromNonExistentWithGasPrice",
			call:    ethtypes.EthCall{From: &nonExistent, To: &env.eoaAddr, GasPrice: gasPrice},
			wantErr: false,
			check: func(t *testing.T, gas ethtypes.EthUint64, _ error) {
				require.GreaterOrEqual(t, uint64(gas), minGas, "gas should be at least minimum transfer gas")
				require.Less(t, uint64(gas), maxGas, "gas should not overflow")
			},
		},
		{
			name:    "FromNonExistentToContractWithData",
			call:    ethtypes.EthCall{From: &nonExistent, To: &errorsAddr, Data: kit.CalcFuncSignature("failRevertEmpty()")},
			wantErr: true,
			check: func(t *testing.T, _ ethtypes.EthUint64, err error) {
				var execErr *api.ErrExecutionReverted
				require.True(t, errors.As(err, &execErr), "expected ErrExecutionReverted, got %T: %v", err, err)
				require.Contains(t, execErr.Message, "none")
			},
		},
		{
			name:    "FromEOA",
			call:    ethtypes.EthCall{From: &env.eoaAddr, To: &env.eoaAddr2},
			wantErr: false,
			check: func(t *testing.T, gas ethtypes.EthUint64, _ error) {
				require.GreaterOrEqual(t, uint64(gas), minGas, "gas should be at least minimum transfer gas")
				require.Less(t, uint64(gas), maxGas, "gas should not overflow")
			},
		},
		{
			name:    "RevertDivideByZero",
			call:    ethtypes.EthCall{From: &env.eoaAddr, To: &errorsAddr, Data: kit.CalcFuncSignature("failDivZero()")},
			wantErr: true,
			check: func(t *testing.T, _ ethtypes.EthUint64, err error) {
				var execErr *api.ErrExecutionReverted
				require.True(t, errors.As(err, &execErr), "expected ErrExecutionReverted, got %T: %v", err, err)
				require.Contains(t, execErr.Message, "DivideByZero")
				require.Contains(t, execErr.Data, panicSelector(0x12))
			},
		},
		{
			name:    "RevertAssert",
			call:    ethtypes.EthCall{From: &env.eoaAddr, To: &errorsAddr, Data: kit.CalcFuncSignature("failAssert()")},
			wantErr: true,
			check: func(t *testing.T, _ ethtypes.EthUint64, err error) {
				var execErr *api.ErrExecutionReverted
				require.True(t, errors.As(err, &execErr), "expected ErrExecutionReverted, got %T: %v", err, err)
				require.Contains(t, execErr.Message, "Assert")
				require.Contains(t, execErr.Data, panicSelector(0x01))
			},
		},
		{
			name:    "RevertWithReason",
			call:    ethtypes.EthCall{From: &env.eoaAddr, To: &errorsAddr, Data: kit.CalcFuncSignature("failRevertReason()")},
			wantErr: true,
			check: func(t *testing.T, _ ethtypes.EthUint64, err error) {
				var execErr *api.ErrExecutionReverted
				require.True(t, errors.As(err, &execErr), "expected ErrExecutionReverted, got %T: %v", err, err)
				require.Contains(t, execErr.Message, "my reason")
			},
		},
		{
			name:    "RevertEmpty",
			call:    ethtypes.EthCall{From: &env.eoaAddr, To: &errorsAddr, Data: kit.CalcFuncSignature("failRevertEmpty()")},
			wantErr: true,
			check: func(t *testing.T, _ ethtypes.EthUint64, err error) {
				var execErr *api.ErrExecutionReverted
				require.True(t, errors.As(err, &execErr), "expected ErrExecutionReverted, got %T: %v", err, err)
				require.Contains(t, execErr.Message, "none")
				require.Equal(t, "0x", execErr.Data)
			},
		},
		{
			name:    "RevertCustomError",
			call:    ethtypes.EthCall{From: &env.eoaAddr, To: &errorsAddr, Data: kit.CalcFuncSignature("failCustom()")},
			wantErr: true,
			check: func(t *testing.T, _ ethtypes.EthUint64, err error) {
				var execErr *api.ErrExecutionReverted
				require.True(t, errors.As(err, &execErr), "expected ErrExecutionReverted, got %T: %v", err, err)
				require.Contains(t, execErr.Data, errorSelector("CustomError()"))
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gasParams, err := json.Marshal(ethtypes.EthEstimateGasParams{Tx: tc.call, BlkParam: &blkParam})
			require.NoError(t, err)

			gas, err := env.client.EthEstimateGas(env.ctx, gasParams)
			if tc.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err, "gas estimation should succeed")
			}
			if tc.check != nil {
				tc.check(t, gas, err)
			}
		})
	}
}

func TestSkipSenderStateIsolation(t *testing.T) {
	blockTime := 100 * time.Millisecond
	client, _, ens := kit.EnsembleMinimal(t, kit.MockProofs(), kit.ThroughRPC())
	ens.InterconnectAll().BeginMining(blockTime)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	// Create target EOA
	_, eoaAddr, filAddr := client.EVM().NewAccount()
	kit.SendFunds(ctx, t, client, filAddr, types.FromFil(10))

	nonExistent := nonExistentAddr(0x03)
	blkParam := ethtypes.NewEthBlockNumberOrHashFromPredefined("latest")
	call := ethtypes.EthCall{From: &nonExistent, To: &eoaAddr}

	// First call creates an ephemeral placeholder actor in the buffered store
	result1, err := client.EthCall(ctx, call, blkParam)
	require.NoError(t, err)

	// Second call should produce identical result (state not persisted)
	result2, err := client.EthCall(ctx, call, blkParam)
	require.NoError(t, err)
	require.Equal(t, result1, result2, "repeated calls should produce same result")

	// Verify address doesn't exist on chain
	filNonExistent, err := nonExistent.ToFilecoinAddress()
	require.NoError(t, err)
	_, err = client.StateGetActor(ctx, filNonExistent, types.EmptyTSK)
	require.Error(t, err)
	require.Contains(t, err.Error(), "actor not found")

	// Concurrent skip-path calls each get their own ephemeral state and must not interfere.
	type callResult struct {
		result ethtypes.EthBytes
		err    error
	}
	results := make(chan callResult, 8)
	for i := 0; i < 8; i++ {
		go func() {
			r, err := client.EthCall(ctx, call, blkParam)
			results <- callResult{r, err}
		}()
	}
	for i := 0; i < 8; i++ {
		res := <-results
		require.NoError(t, res.err)
		require.Equal(t, result1, res.result)
	}

	// The fallback must respect an explicit historical block parameter.
	head, err := client.EthBlockNumber(ctx)
	require.NoError(t, err)
	require.Greater(t, uint64(head), uint64(2))
	histParam := ethtypes.NewEthBlockNumberOrHashFromNumber(head - 2)
	_, err = client.EthCall(ctx, call, histParam)
	require.NoError(t, err)
}

// TestCrossContractCallbackIsolation tests that eth_call works correctly when:
// 1. Calling from a contract address (skip sender validation)
// 2. ContractA calls ContractB
// 3. ContractB calls back to ContractA's view function
// This validates that cross-contract callbacks work during simulated calls.
func TestCrossContractCallbackIsolation(t *testing.T) {
	ctx, cancel, client := kit.SetupFEVMTest(t)
	defer cancel()

	// Deploy ContractB first (no dependencies)
	_, contractBFilAddr := client.EVM().DeployContractFromFilename(ctx, "contracts/ContractB.hex")
	contractBActor, err := client.StateGetActor(ctx, contractBFilAddr, types.EmptyTSK)
	require.NoError(t, err)
	require.NotNil(t, contractBActor.DelegatedAddress)
	contractBAddr, err := ethtypes.EthAddressFromFilecoinAddress(*contractBActor.DelegatedAddress)
	require.NoError(t, err)

	// Deploy ContractA
	fromAddr, contractAFilAddr := client.EVM().DeployContractFromFilename(ctx, "contracts/ContractA.hex")
	contractAActor, err := client.StateGetActor(ctx, contractAFilAddr, types.EmptyTSK)
	require.NoError(t, err)
	require.NotNil(t, contractAActor.DelegatedAddress)
	contractAAddr, err := ethtypes.EthAddressFromFilecoinAddress(*contractAActor.DelegatedAddress)
	require.NoError(t, err)

	// Link ContractA to ContractB by calling setContractB(address)
	// Address parameter is ABI-encoded as 32 bytes (left-padded)
	var contractBParam [32]byte
	copy(contractBParam[12:], contractBAddr[:])
	_, _, err = client.EVM().InvokeContractByFuncName(ctx, fromAddr, contractAFilAddr, "setContractB(address)", contractBParam[:])
	require.NoError(t, err)

	blkParam := ethtypes.NewEthBlockNumberOrHashFromPredefined("latest")

	t.Run("FromContractAddress", func(t *testing.T) {
		// Call callBAndReadBack() from ContractB's address (a contract calling another contract)
		// This tests: ContractB (sender) -> ContractA.callBAndReadBack() -> ContractB.callBackAndRead() -> ContractA.getValue()
		call := ethtypes.EthCall{
			From: &contractBAddr,
			To:   &contractAAddr,
			Data: kit.CalcFuncSignature("callBAndReadBack()"),
		}
		result, err := client.EthCall(ctx, call, blkParam)
		require.NoError(t, err, "cross-contract callback should succeed when calling from contract address")

		// Result should be 42 (the storedValue in ContractA)
		// ABI-encoded uint256 is 32 bytes
		require.Len(t, result, 32, "result should be 32 bytes (uint256)")
		value := result[31] // Last byte contains the value for small numbers
		require.Equal(t, byte(42), value, "should return storedValue (42)")
	})

	t.Run("FromNonExistentAddress", func(t *testing.T) {
		// Call from a non-existent address
		nonExistent := nonExistentAddr(0x10)
		call := ethtypes.EthCall{
			From: &nonExistent,
			To:   &contractAAddr,
			Data: kit.CalcFuncSignature("callBAndReadBack()"),
		}
		result, err := client.EthCall(ctx, call, blkParam)
		require.NoError(t, err, "cross-contract callback should succeed when calling from non-existent address")

		require.Len(t, result, 32)
		value := result[31]
		require.Equal(t, byte(42), value, "should return storedValue (42)")
	})

	t.Run("FromEOA", func(t *testing.T) {
		// Create and fund an EOA
		_, eoaAddr, filAddr := client.EVM().NewAccount()
		kit.SendFunds(ctx, t, client, filAddr, types.FromFil(1))

		call := ethtypes.EthCall{
			From: &eoaAddr,
			To:   &contractAAddr,
			Data: kit.CalcFuncSignature("callBAndReadBack()"),
		}
		result, err := client.EthCall(ctx, call, blkParam)
		require.NoError(t, err, "cross-contract callback should succeed when calling from EOA")

		require.Len(t, result, 32)
		value := result[31]
		require.Equal(t, byte(42), value, "should return storedValue (42)")
	})

	t.Run("DoubleCallback", func(t *testing.T) {
		// Test the callBAndDouble function which does callback and computation
		nonExistent := nonExistentAddr(0x11)
		call := ethtypes.EthCall{
			From: &nonExistent,
			To:   &contractAAddr,
			Data: kit.CalcFuncSignature("callBAndDouble()"),
		}
		result, err := client.EthCall(ctx, call, blkParam)
		require.NoError(t, err, "double callback should succeed")

		require.Len(t, result, 32)
		value := result[31]
		require.Equal(t, byte(84), value, "should return storedValue * 2 (84)")
	})
}

// TestEthSkipSenderGasAccuracy exercises the gas-accounting properties of estimates made
// with senders that don't exist on chain.
func TestEthSkipSenderGasAccuracy(t *testing.T) {
	env := setupSkipSenderTest(t)
	defer env.cancel()

	blkParam := ethtypes.NewEthBlockNumberOrHashFromPredefined("latest")
	sendCoinData := kit.EvmCalldata("sendCoin(address,uint256)", kit.EvmWordBytes(env.eoaAddr[:]), kit.EvmWordUint64(0))

	estimate := func(from, to ethtypes.EthAddress, data ethtypes.EthBytes) (ethtypes.EthUint64, error) {
		gasParams, err := json.Marshal(ethtypes.EthEstimateGasParams{
			Tx:       ethtypes.EthCall{From: &from, To: &to, Data: data},
			BlkParam: &blkParam,
		})
		require.NoError(t, err)
		return env.client.EthEstimateGas(env.ctx, gasParams)
	}

	t.Run("RoundTripFromUnfunded", func(t *testing.T) {
		key, ethAddr, filAddr := env.client.EVM().NewAccount()

		// The address has no actor on chain; the estimate must still be executable later.
		gasLimit, err := estimate(ethAddr, env.contractAddr, sendCoinData)
		require.NoError(t, err)

		// Fund the address (creating a placeholder actor) and submit the same call as a
		// real transaction with the estimated gas limit. This proves the estimate covers
		// inclusion cost and placeholder promotion, and that the ephemeral placeholder
		// used during estimation leaked no nonce.
		kit.SendFunds(env.ctx, t, env.client, filAddr, types.FromFil(1))

		maxPriorityFee, err := env.client.EthMaxPriorityFeePerGas(env.ctx)
		require.NoError(t, err)
		tx := ethtypes.Eth1559TxArgs{
			ChainID:              buildconstants.Eip155ChainId,
			Nonce:                0,
			To:                   &env.contractAddr,
			Value:                big.Zero(),
			MaxFeePerGas:         types.NanoFil,
			MaxPriorityFeePerGas: big.Int(maxPriorityFee),
			GasLimit:             int(gasLimit),
			Input:                sendCoinData,
			V:                    big.Zero(),
			R:                    big.Zero(),
			S:                    big.Zero(),
		}
		env.client.EVM().SignTransaction(&tx, key.PrivateKey)
		hash := env.client.EVM().SubmitTransaction(env.ctx, &tx)
		receipt, err := env.client.EVM().WaitTransaction(env.ctx, hash)
		require.NoError(t, err)
		require.EqualValues(t, 1, receipt.Status, "estimated gas limit must be sufficient for the real transaction")
	})

	t.Run("ParityWithExistingSender", func(t *testing.T) {
		// A funded, never-sent account (a placeholder actor) estimates via the normal
		// path; a non-existent account estimates via the fallback with an ephemeral
		// placeholder. The two must substantially agree: a large gap means the fallback
		// is dropping gas components (e.g. inclusion cost).
		_, fundedAddr, fundedFil := env.client.EVM().NewAccount()
		kit.SendFunds(env.ctx, t, env.client, fundedFil, types.FromFil(1))

		fundedGas, err := estimate(fundedAddr, env.contractAddr, sendCoinData)
		require.NoError(t, err)
		nonExistentGas, err := estimate(nonExistentAddr(0x42), env.contractAddr, sendCoinData)
		require.NoError(t, err)
		require.InEpsilon(t, uint64(fundedGas), uint64(nonExistentGas), 0.1,
			"estimates from placeholder and non-existent senders should substantially agree")
	})

	t.Run("RoundTripRecursive", func(t *testing.T) {
		// Regression for the fallback path skipping the gas search: recursive calls need
		// a higher limit than the gas they use (the 63/64 stipend rule), so an estimate
		// of GasUsed times the overestimation multiplier reverts on submission.
		_, recursiveFilAddr := env.client.EVM().DeployContractFromFilename(env.ctx, "contracts/ExternalRecursiveCallSimple.hex")
		recursiveActor, err := env.client.StateGetActor(env.ctx, recursiveFilAddr, types.EmptyTSK)
		require.NoError(t, err)
		require.NotNil(t, recursiveActor.DelegatedAddress)
		recursiveAddr, err := ethtypes.EthAddressFromFilecoinAddress(*recursiveActor.DelegatedAddress)
		require.NoError(t, err)

		recurseData := kit.EvmCalldata("exec1(uint256)", kit.EvmWordUint64(100))

		key, ethAddr, filAddr := env.client.EVM().NewAccount()
		gasLimit, err := estimate(ethAddr, recursiveAddr, recurseData)
		require.NoError(t, err)

		// The fallback estimate must agree with the normal path for the same call.
		_, fundedAddr, fundedFil := env.client.EVM().NewAccount()
		kit.SendFunds(env.ctx, t, env.client, fundedFil, types.FromFil(2))
		fundedGas, err := estimate(fundedAddr, recursiveAddr, recurseData)
		require.NoError(t, err)
		require.InEpsilon(t, uint64(fundedGas), uint64(gasLimit), 0.1,
			"recursive estimates from placeholder and non-existent senders should substantially agree")

		kit.SendFunds(env.ctx, t, env.client, filAddr, types.FromFil(2))
		maxPriorityFee, err := env.client.EthMaxPriorityFeePerGas(env.ctx)
		require.NoError(t, err)
		tx := ethtypes.Eth1559TxArgs{
			ChainID:              buildconstants.Eip155ChainId,
			Nonce:                0,
			To:                   &recursiveAddr,
			Value:                big.Zero(),
			MaxFeePerGas:         types.NanoFil,
			MaxPriorityFeePerGas: big.Int(maxPriorityFee),
			GasLimit:             int(gasLimit),
			Input:                recurseData,
			V:                    big.Zero(),
			R:                    big.Zero(),
			S:                    big.Zero(),
		}
		env.client.EVM().SignTransaction(&tx, key.PrivateKey)
		hash := env.client.EVM().SubmitTransaction(env.ctx, &tx)
		receipt, err := env.client.EVM().WaitTransaction(env.ctx, hash)
		require.NoError(t, err)
		require.EqualValues(t, 1, receipt.Status, "estimated gas limit must be sufficient for the recursive transaction")
	})

	t.Run("FundedPlaceholderSender", func(t *testing.T) {
		// An address that has received funds but never sent is a placeholder actor and
		// already a valid sender: value-bearing calls from it succeed on the normal path.
		_, pAddr, pFil := env.client.EVM().NewAccount()
		kit.SendFunds(env.ctx, t, env.client, pFil, types.FromFil(2))

		_, err := env.client.EthCall(env.ctx,
			ethtypes.EthCall{From: &pAddr, To: &env.eoaAddr, Value: ethtypes.EthBigInt(types.FromFil(1))}, blkParam)
		require.NoError(t, err, "value-bearing eth_call from a funded placeholder should succeed")
	})
}

// TestEthCallSenderIdentity verifies that the callee observes the requested from address as
// msg.sender on the skip-sender paths. SimpleCoin's sendCoin returns false when
// balances[msg.sender] is insufficient, so crediting coin balances to chosen identities lets
// the callee report whether the simulated sender identity was preserved.
func TestEthCallSenderIdentity(t *testing.T) {
	env := setupSkipSenderTest(t)
	defer env.cancel()

	blkParam := ethtypes.NewEthBlockNumberOrHashFromPredefined("latest")

	// Deploy a contract to act as a contract-typed sender identity.
	_, contractBFil := env.client.EVM().DeployContractFromFilename(env.ctx, "contracts/ContractB.hex")
	bActor, err := env.client.StateGetActor(env.ctx, contractBFil, types.EmptyTSK)
	require.NoError(t, err)
	require.NotNil(t, bActor.DelegatedAddress)
	senderContract, err := ethtypes.EthAddressFromFilecoinAddress(*bActor.DelegatedAddress)
	require.NoError(t, err)

	fundedNonExistent := nonExistentAddr(0x21)
	unfundedNonExistent := nonExistentAddr(0x22)

	// Credit coins to the contract and to a non-existent address; coin balances are just
	// mapping entries in SimpleCoin, no actor is required at the credited address.
	for _, to := range []ethtypes.EthAddress{senderContract, fundedNonExistent} {
		_, _, err := env.client.EVM().InvokeContractByFuncName(env.ctx, env.deployerFilAddr, env.contractFilAddr,
			"sendCoin(address,uint256)", append(kit.EvmWordBytes(to[:]), kit.EvmWordUint64(100)...))
		require.NoError(t, err)
	}

	// sendCoin(recipient, 10) returns true iff balances[msg.sender] >= 10.
	spendData := kit.EvmCalldata("sendCoin(address,uint256)", kit.EvmWordBytes(env.eoaAddr[:]), kit.EvmWordUint64(10))

	for _, tc := range []struct {
		name string
		from ethtypes.EthAddress
		want byte
	}{
		{"FromContract", senderContract, 1},
		{"FromNonExistentWithCoins", fundedNonExistent, 1},
		{"FromNonExistentWithoutCoins", unfundedNonExistent, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := env.client.EthCall(env.ctx,
				ethtypes.EthCall{From: &tc.from, To: &env.contractAddr, Data: spendData}, blkParam)
			require.NoError(t, err)
			require.Len(t, result, 32)
			require.Equal(t, tc.want, result[31],
				"callee must observe the requested from address as msg.sender")
		})
	}
}

func TestFEVMRecursiveActorCallEstimate(t *testing.T) {
	ctx, cancel, client := kit.SetupFEVMTest(t)
	defer cancel()

	// install contract Actor
	filenameActor := "contracts/ExternalRecursiveCallSimple.hex"
	_, actorAddr := client.EVM().DeployContractFromFilename(ctx, filenameActor)

	contractAddr, err := ethtypes.EthAddressFromFilecoinAddress(actorAddr)
	require.NoError(t, err)

	// create a new Ethereum account
	key, ethAddr, ethFilAddr := client.EVM().NewAccount()
	kit.SendFunds(ctx, t, client, ethFilAddr, types.FromFil(1000))

	makeParams := func(r int) []byte {
		funcSignature := "exec1(uint256)"
		entryPoint := kit.CalcFuncSignature(funcSignature)

		inputData := make([]byte, 32)
		binary.BigEndian.PutUint64(inputData[24:], uint64(r))

		params := append(entryPoint, inputData...)

		return params
	}

	testN := func(r int) func(t *testing.T) {
		return func(t *testing.T) {
			t.Logf("running with %d recursive calls", r)

			params := makeParams(r)

			gasParams, err := json.Marshal(ethtypes.EthEstimateGasParams{Tx: ethtypes.EthCall{
				From: &ethAddr,
				To:   &contractAddr,
				Data: params,
			}})
			require.NoError(t, err)

			gaslimit, err := client.EthEstimateGas(ctx, gasParams)
			require.NoError(t, err)
			require.LessOrEqual(t, int64(gaslimit), buildconstants.BlockGasLimit)

			t.Logf("EthEstimateGas GasLimit=%d", gaslimit)

			maxPriorityFeePerGas, err := client.EthMaxPriorityFeePerGas(ctx)
			require.NoError(t, err)

			nonce, err := client.MpoolGetNonce(ctx, ethFilAddr)
			require.NoError(t, err)

			tx := &ethtypes.Eth1559TxArgs{
				ChainID:              buildconstants.Eip155ChainId,
				To:                   &contractAddr,
				Value:                big.Zero(),
				Nonce:                int(nonce),
				MaxFeePerGas:         types.NanoFil,
				MaxPriorityFeePerGas: big.Int(maxPriorityFeePerGas),
				GasLimit:             int(gaslimit),
				Input:                params,
				V:                    big.Zero(),
				R:                    big.Zero(),
				S:                    big.Zero(),
			}

			client.EVM().SignTransaction(tx, key.PrivateKey)
			hash := client.EVM().SubmitTransaction(ctx, tx)

			smsg, err := ethtypes.ToSignedFilecoinMessage(tx)
			require.NoError(t, err)

			_, err = client.StateWaitMsg(ctx, smsg.Cid(), 0, 0, false)
			require.NoError(t, err)

			receipt, err := client.EthGetTransactionReceipt(ctx, hash)
			require.NoError(t, err)
			require.NotNil(t, receipt)

			t.Logf("Receipt GasUsed=%d", receipt.GasUsed)
			t.Logf("Ratio %0.2f", float64(receipt.GasUsed)/float64(gaslimit))
			t.Logf("Overestimate %0.2f", ((float64(gaslimit)/float64(receipt.GasUsed))-1)*100)

			require.EqualValues(t, ethtypes.EthUint64(1), receipt.Status)
		}
	}

	t.Run("n=1", testN(1))
	t.Run("n=2", testN(2))
	t.Run("n=3", testN(3))
	t.Run("n=4", testN(4))
	t.Run("n=5", testN(5))
	t.Run("n=10", testN(10))
	t.Run("n=20", testN(20))
	t.Run("n=30", testN(30))
	t.Run("n=40", testN(40))
	t.Run("n=50", testN(50))
	t.Run("n=100", testN(100))
}
func TestEthCall(t *testing.T) {
	ctx, cancel, client := kit.SetupFEVMTest(t)
	defer cancel()

	filename := "contracts/Errors.hex"
	fromAddr, contractAddr := client.EVM().DeployContractFromFilename(ctx, filename)

	divideByZeroSignature := kit.CalcFuncSignature("failDivZero()")

	_, _, err := client.EVM().InvokeContractByFuncName(ctx, fromAddr, contractAddr, "failDivZero()", []byte{})
	require.Error(t, err)

	latestBlock, err := client.EthBlockNumber(ctx)
	require.NoError(t, err)

	contractAddrEth, err := ethtypes.EthAddressFromFilecoinAddress(contractAddr)
	require.NoError(t, err)

	callParams := ethtypes.EthCall{
		From: nil,
		To:   &contractAddrEth,
		Data: divideByZeroSignature,
	}

	t.Run("FailedToProcessBlockParam", func(t *testing.T) {
		invalidBlockNumber := latestBlock + 1000
		_, err = client.EthCall(ctx, callParams, ethtypes.NewEthBlockNumberOrHashFromNumber(invalidBlockNumber))
		require.Error(t, err)
		require.Contains(t, err.Error(), "requested a future epoch (beyond 'latest')")
	})

	t.Run("DivideByZeroError", func(t *testing.T) {
		_, err = client.EthCall(ctx, callParams, ethtypes.NewEthBlockNumberOrHashFromNumber(latestBlock))
		require.Error(t, err)

		var dataErr *api.ErrExecutionReverted
		require.ErrorAs(t, err, &dataErr, "Expected error to be ErrExecutionReverted")
		require.Regexp(t, `message execution failed [\s\S]+\[DivideByZero\(\)\]`, dataErr.Message)

		// Get the error data
		require.Equal(t, dataErr.Data, "0x4e487b710000000000000000000000000000000000000000000000000000000000000012", "Expected error data to contain 'DivideByZero()'")
	})
}
func TestEthEstimateGas(t *testing.T) {
	ctx, cancel, client := kit.SetupFEVMTest(t)
	defer cancel()

	_, ethAddr, filAddr := client.EVM().NewAccount()

	kit.SendFunds(ctx, t, client, filAddr, types.FromFil(10))

	filename := "contracts/Errors.hex"
	_, contractAddr := client.EVM().DeployContractFromFilename(ctx, filename)

	contractAddrEth, err := ethtypes.EthAddressFromFilecoinAddress(contractAddr)
	require.NoError(t, err)

	testCases := []struct {
		name           string
		function       string
		expectedError  string
		expectedErrMsg interface{}
	}{
		{"DivideByZero", "failDivZero()", "0x4e487b710000000000000000000000000000000000000000000000000000000000000012", `message execution failed [\s\S]+\[DivideByZero\(\)\]`},
		{"Assert", "failAssert()", "0x4e487b710000000000000000000000000000000000000000000000000000000000000001", `message execution failed [\s\S]+\[Assert\(\)\]`},
		{"RevertWithReason", "failRevertReason()", fmt.Sprintf("%x", []byte("my reason")), `message execution failed [\s\S]+\[Error\(my reason\)\]`},
		{"RevertEmpty", "failRevertEmpty()", "0x", `message execution failed [\s\S]+\[none\]`},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			signature := kit.CalcFuncSignature(tc.function)

			callParams := ethtypes.EthCall{
				From: &ethAddr,
				To:   &contractAddrEth,
				Data: signature,
			}

			gasParams, err := json.Marshal(ethtypes.EthEstimateGasParams{Tx: callParams})
			require.NoError(t, err, "Error marshaling gas params")

			_, err = client.EthEstimateGas(ctx, gasParams)

			if tc.expectedError != "" {
				require.Error(t, err)
				var dataErr *api.ErrExecutionReverted
				require.ErrorAs(t, err, &dataErr, "Expected error to be ErrExecutionReverted")
				require.Regexp(t, tc.expectedErrMsg, dataErr.Message)
				require.Contains(t, dataErr.Data, tc.expectedError)
			}
		})
	}
}
func TestEthNullRoundHandling(t *testing.T) {
	blockTime := 100 * time.Millisecond
	client, _, ens := kit.EnsembleMinimal(t, kit.MockProofs(), kit.ThroughRPC())

	bms := ens.InterconnectAll().BeginMining(blockTime)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	client.WaitTillChain(ctx, kit.HeightAtLeast(10))

	bms[0].InjectNulls(10)

	tctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	ch, err := client.ChainNotify(tctx)
	require.NoError(t, err)
	<-ch
	hc := <-ch
	require.Equal(t, store.HCApply, hc[0].Type)

	afterNullHeight := hc[0].Val.Height()

	nullHeight := afterNullHeight - 1
	for nullHeight > 0 {
		ts, err := client.ChainGetTipSetByHeight(ctx, nullHeight, types.EmptyTSK)
		require.NoError(t, err)
		if ts.Height() == nullHeight {
			nullHeight--
		} else {
			break
		}
	}

	nullBlockHex := fmt.Sprintf("0x%x", int(nullHeight))
	nullBlockParam := ethtypes.NewEthBlockNumberOrHashFromNumber(ethtypes.EthUint64(nullHeight))
	client.WaitTillChain(ctx, kit.HeightAtLeast(nullHeight+2))
	testCases := []struct {
		name     string
		testFunc func() error
	}{
		{
			name: "EthGetBlockByNumber",
			testFunc: func() error {
				_, err := client.EthGetBlockByNumber(ctx, nullBlockHex, true)
				return err
			},
		},
		{
			name: "EthGetBlockTransactionCountByNumber",
			testFunc: func() error {
				_, err := client.EthGetBlockTransactionCountByNumber(ctx, nullBlockHex)
				return err
			},
		},
		{
			name: "EthGetTransactionByBlockNumberAndIndex",
			testFunc: func() error {
				_, err := client.EthGetTransactionByBlockNumberAndIndex(ctx, nullBlockHex, ethtypes.EthUint64(0))
				return err
			},
		},
		{
			name: "EthGetBlockReceipts",
			testFunc: func() error {
				_, err := client.EthGetBlockReceipts(ctx, nullBlockParam)
				return err
			},
		},
		{
			name: "EthGetBlockReceiptsLimited",
			testFunc: func() error {
				_, err := client.EthGetBlockReceiptsLimited(ctx, nullBlockParam, api.LookbackNoLimit)
				return err
			},
		},
		{
			name: "EthTraceBlock",
			testFunc: func() error {
				_, err := client.EthTraceBlock(ctx, nullBlockHex)
				return err
			},
		},
		{
			name: "EthTraceReplayBlockTransactions",
			testFunc: func() error {
				_, err := client.EthTraceReplayBlockTransactions(ctx, nullBlockHex, []string{"trace"})
				return err
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.testFunc()
			require.Error(t, err)

			// Test errors.Is
			require.ErrorIs(t, err, new(api.ErrNullRound), "error should be or wrap ErrNullRound")

			// Test errors.As and verify message
			var nullRoundErr *api.ErrNullRound
			require.ErrorAs(t, err, &nullRoundErr, "error should be convertible to ErrNullRound")

			expectedMsg := fmt.Sprintf("requested epoch was a null round (%d)", nullHeight)
			require.Equal(t, expectedMsg, nullRoundErr.Error())
			require.Equal(t, nullHeight, nullRoundErr.Epoch)
		})
	}
}
