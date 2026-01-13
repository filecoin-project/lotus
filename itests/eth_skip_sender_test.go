package itests

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/lotus/api"
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
	ctx          context.Context
	cancel       context.CancelFunc
	client       *kit.TestFullNode
	contractAddr ethtypes.EthAddress
	eoaAddr      ethtypes.EthAddress
	eoaAddr2     ethtypes.EthAddress
}

// setupSkipSenderTest creates a test environment with a deployed contract and funded EOAs.
func setupSkipSenderTest(t *testing.T) *skipSenderEnv {
	ctx, cancel, client := kit.SetupFEVMTest(t)

	// Create and fund two EOAs
	_, eoaAddr, filAddr := client.EVM().NewAccount()
	kit.SendFunds(ctx, t, client, filAddr, types.FromFil(10))

	_, eoaAddr2, _ := client.EVM().NewAccount()

	// Deploy SimpleCoin contract
	_, contractFilAddr := client.EVM().DeployContractFromFilename(ctx, "contracts/SimpleCoin.hex")
	actor, err := client.StateGetActor(ctx, contractFilAddr, types.EmptyTSK)
	require.NoError(t, err)
	require.NotNil(t, actor.DelegatedAddress)

	contractAddr, err := ethtypes.EthAddressFromFilecoinAddress(*actor.DelegatedAddress)
	require.NoError(t, err)

	return &skipSenderEnv{ctx: ctx, cancel: cancel, client: client, contractAddr: contractAddr, eoaAddr: eoaAddr, eoaAddr2: eoaAddr2}
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

	tests := []struct {
		name    string
		call    ethtypes.EthCall
		wantErr bool
		check   func(*testing.T, ethtypes.EthBytes, error)
	}{
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

	tests := []struct {
		name    string
		call    ethtypes.EthCall
		wantErr bool
		check   func(*testing.T, ethtypes.EthUint64, error)
	}{
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

	// First call creates synthetic actor in buffered store
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
}
