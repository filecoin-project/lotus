//go:build eip7702_enabled

package itests

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	abi2 "github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	typescrypto "github.com/filecoin-project/go-state-types/crypto"

	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/types"
	ethtypes "github.com/filecoin-project/lotus/chain/types/ethtypes"
	"github.com/filecoin-project/lotus/itests/kit"
)

// TestEth7702_SendRoutesToEVM exercises the send-path for type-0x04 transactions:
// it constructs and signs a minimal 7702 tx with a non-empty authorizationList, sends it via
// eth_sendRawTransaction, and verifies that a Filecoin message targeting the EVM actor's
// ApplyAndCall method is enqueued in the mpool from the recovered f4 sender.
func TestEth7702_SendRoutesToEVM(t *testing.T) {
	// Ensure 7702 feature is enabled and EVM ApplyAndCall actor address configured.
	ethtypes.Eip7702FeatureEnabled = true
	id999, _ := address.NewIDAddress(999)
	ethtypes.EvmApplyAndCallActorAddr = id999

	// Set NV at/after activation to exercise mpool policies consistently.
	ctx, cancel, client := kit.SetupFEVMTest(t)
	defer cancel()

	// Create a new ETH account that we'll use as the tx sender; fund its f4 address.
	senderKey, _, senderFilAddr := client.EVM().NewAccount()
	// Transfer some FIL to cover gas.
	client.EVM().TransferValueOrFail(ctx, client.DefaultKey.Address, senderFilAddr, types.FromFil(10))

	// Build a minimal 7702 tx with one authorization tuple.
	// The tuple contents are not executed in this test; we only validate the send-path routing.
	// Construct a dummy authorization tuple referencing a delegate address.
	var delegate ethtypes.EthAddress
	for i := range delegate {
		delegate[i] = 0xbb
	}
	authz := []ethtypes.EthAuthorization{{
		ChainID: ethtypes.EthUint64(buildconstants.Eip155ChainId),
		Address: delegate,
		Nonce:   0,
		YParity: 0,
		R:       ethtypes.EthBigInt(big.NewInt(1)),
		S:       ethtypes.EthBigInt(big.NewInt(1)),
	}}

	tx := &ethtypes.Eth7702TxArgs{
		ChainID:              buildconstants.Eip155ChainId,
		Nonce:                0,
		To:                   nil,
		Value:                big.Zero(),
		MaxFeePerGas:         types.NewInt(1_000_000_000),
		MaxPriorityFeePerGas: types.NewInt(1_000_000_000),
		GasLimit:             500_000,
		Input:                nil,
		AuthorizationList:    authz,
		V:                    big.Zero(),
		R:                    big.Zero(),
		S:                    big.Zero(),
	}

	// Sign the tx (typed-0x04 hash) using delegated signature over the RLP unsigned preimage.
	preimage, err := tx.ToRlpUnsignedMsg()
	require.NoError(t, err)
	sig, err := kit.SigDelegatedSign(senderKey.PrivateKey, preimage)
	require.NoError(t, err)
	require.Equal(t, typescrypto.SigTypeDelegated, sig.Type)
	require.NoError(t, tx.InitialiseSignature(*sig))

	// Send raw via eth_sendRawTransaction and expect a hash back.
	rawSigned, err := tx.ToRlpSignedMsg()
	require.NoError(t, err)
	_, err = client.EVM().EthSendRawTransaction(ctx, rawSigned)
	require.NoError(t, err)

	// Verify a matching Filecoin message is present in mpool from the recovered f4 sender.
	pending, err := client.MpoolPending(ctx, types.EmptyTSK)
	require.NoError(t, err)

	found := false
	for _, sm := range pending {
		if sm.Message.From == senderFilAddr && sm.Message.Method == abi2.MethodNum(ethtypes.MethodHash("ApplyAndCall")) {
			// Ensure we target the configured EVM ApplyAndCall actor address.
			require.Equal(t, ethtypes.EvmApplyAndCallActorAddr, sm.Message.To)
			found = true
			break
		}
	}
	require.True(t, found, "expected an EVM.ApplyAndCall message in mpool from sender")
}

// TestEth7702_ReceiptFields validates that once a 0x04 transaction is mined, the
// JSON-RPC receipt includes authorizationList and delegatedTo populated.
func TestEth7702_ReceiptFields(t *testing.T) {
	client, _, ens := kit.EnsembleMinimal(t, kit.MockProofs(), kit.ThroughRPC())
	ens.InterconnectAll().BeginMining(100 * time.Millisecond)
	ctx := context.Background()

	// Wait for chain to tick to avoid races with genesis init.
	_ = client.WaitTillChain(context.Background(), kit.HeightAtLeast(1))
	// Sender account.
	senderKey, _, senderFilAddr := client.EVM().NewAccount()
	// Fund sender to create account actor.
	kit.SendFunds(ctx, t, client, senderFilAddr, types.FromFil(10))

	// Enable feature and configure EVM ApplyAndCall actor address only after funding completes.
	ethtypes.Eip7702FeatureEnabled = true
	id999, _ := address.NewIDAddress(999)
	ethtypes.EvmApplyAndCallActorAddr = id999

	// Two delegate addresses to exercise arrays.
	var d1, d2 ethtypes.EthAddress
	for i := range d1 {
		d1[i] = 0x11
	}
	for i := range d2 {
		d2[i] = 0x22
	}
	authz := []ethtypes.EthAuthorization{
		{
			ChainID: ethtypes.EthUint64(buildconstants.Eip155ChainId),
			Address: d1,
			Nonce:   0,
			YParity: 0,
			R:       ethtypes.EthBigInt(big.NewInt(1)),
			S:       ethtypes.EthBigInt(big.NewInt(1)),
		},
		{
			ChainID: ethtypes.EthUint64(buildconstants.Eip155ChainId),
			Address: d2,
			Nonce:   1,
			YParity: 1,
			R:       ethtypes.EthBigInt(big.NewInt(2)),
			S:       ethtypes.EthBigInt(big.NewInt(2)),
		},
	}

	tx := &ethtypes.Eth7702TxArgs{
		ChainID:              buildconstants.Eip155ChainId,
		Nonce:                0,
		To:                   nil,
		Value:                big.Zero(),
		MaxFeePerGas:         types.NewInt(1_000_000_000),
		MaxPriorityFeePerGas: types.NewInt(1_000_000_000),
		GasLimit:             700_000,
		Input:                nil,
		AuthorizationList:    authz,
		V:                    big.Zero(),
		R:                    big.Zero(),
		S:                    big.Zero(),
	}

	preimage, err := tx.ToRlpUnsignedMsg()
	require.NoError(t, err)
	sig, err := kit.SigDelegatedSign(senderKey.PrivateKey, preimage)
	require.NoError(t, err)
	require.NoError(t, tx.InitialiseSignature(*sig))

	rawSigned, err := tx.ToRlpSignedMsg()
	require.NoError(t, err)
	hash, err := client.EVM().EthSendRawTransaction(ctx, rawSigned)
	require.NoError(t, err)

	// Wait for inclusion and fetch receipt.
	receipt, err := client.EVM().WaitTransaction(ctx, hash)
	require.NoError(t, err)
	require.NotNil(t, receipt)
	require.EqualValues(t, ethtypes.EIP7702TxType, receipt.Type)

	// authorizationList should round-trip.
	require.Len(t, receipt.AuthorizationList, 2)
	require.Equal(t, d1, receipt.AuthorizationList[0].Address)
	require.Equal(t, d2, receipt.AuthorizationList[1].Address)

	// delegatedTo should include delegate addresses from tuples even if execution reverts.
	require.GreaterOrEqual(t, len(receipt.DelegatedTo), 2)
	// Order-insensitive check across the two we expect.
	got := map[ethtypes.EthAddress]bool{}
	for _, a := range receipt.DelegatedTo {
		got[a] = true
	}
	require.True(t, got[d1])
	require.True(t, got[d2])
}

// TestEth7702_DelegatedExecute is a scaffold for the full delegated execution flow:
//  1. Apply delegations via a type-0x04 transaction so that an EOA delegates to a contract.
//  2. CALL the EOA; the EVM should execute the delegate via InvokeAsEoa and update storage.
//
// This requires the runtime EVM ApplyAndCall entrypoint and tuple-signing helpers.
// Until those are available in this environment, this test is skipped.
func TestEth7702_DelegatedExecute(t *testing.T) {
	t.Skip("EVM.ApplyAndCall wasm + tuple-signing not wired in this environment; enable when bundled")

	ctx, cancel, client := kit.SetupFEVMTest(t)
	defer cancel()

	// Enable feature and configure EVM ApplyAndCall actor address
	ethtypes.Eip7702FeatureEnabled = true
	id999, _ := address.NewIDAddress(999)
	ethtypes.EvmApplyAndCallActorAddr = id999

	// Deploy a simple delegate contract that writes to storage
	// Note: this call path relies on runtime support; left here to document the expected steps
	_ /*from*/, _ = client.EVM().DeployContractFromFilename(ctx, "contracts/DelegatecallStorage.hex")

	// TODO: Build and send a type-0x04 tx applying delegation from an authority EOA to 'delegate'.
	// This requires generating per-tuple signatures over keccak(rlp(chain_id,address,nonce)).

	// TODO: CALL the authority EOA and assert that storage has been updated by the delegate code.
}
