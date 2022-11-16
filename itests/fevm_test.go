package itests

import (
	"bytes"
	"context"
	"encoding/hex"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	cbg "github.com/whyrusleeping/cbor-gen"

	"github.com/filecoin-project/go-address"
	actorstypes "github.com/filecoin-project/go-state-types/actors"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/itests/kit"
)

// TestFEVMBasic does a basic fevm contract installation and invocation
func TestFEVMBasic(t *testing.T) {
	// TODO the contract installation and invocation can be lifted into utility methods
	// He who writes the second test, shall do that.
	kit.QuietMiningLogs()

	blockTime := 100 * time.Millisecond
	client, _, ens := kit.EnsembleMinimal(t, kit.MockProofs(), kit.ThroughRPC())
	ens.InterconnectAll().BeginMining(blockTime)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	// install contract
	contractHex, err := os.ReadFile("contracts/SimpleCoin.bin")
	require.NoError(t, err)

	contract, err := hex.DecodeString(string(contractHex))
	require.NoError(t, err)

	fromAddr, err := client.WalletDefaultAddress(ctx)
	require.NoError(t, err)

	result := client.EVM().DeployContract(ctx, fromAddr, contract)

	idAddr, err := address.NewIDAddress(result.ActorID)
	require.NoError(t, err)
	t.Logf("actor ID address is %s", idAddr)

	// invoke the contract with owner
	{
		entryPoint, err := hex.DecodeString("f8b2cb4f")
		require.NoError(t, err)

		inputData, err := hex.DecodeString("000000000000000000000000ff00000000000000000000000000000000000064")
		require.NoError(t, err)

		wait := client.EVM().InvokeSolidity(ctx, fromAddr, idAddr, entryPoint, inputData)

		require.True(t, wait.Receipt.ExitCode.IsSuccess(), "contract execution failed")

		result, err := cbg.ReadByteArray(bytes.NewBuffer(wait.Receipt.Return), uint64(len(wait.Receipt.Return)))
		require.NoError(t, err)

		expectedResult, err := hex.DecodeString("0000000000000000000000000000000000000000000000000000000000002710")
		require.NoError(t, err)
		require.Equal(t, result, expectedResult)
	}

	// invoke the contract with non owner
	{
		entryPoint, err := hex.DecodeString("f8b2cb4f")
		require.NoError(t, err)

		inputData, err := hex.DecodeString("000000000000000000000000ff00000000000000000000000000000000000065")
		require.NoError(t, err)

		wait := client.EVM().InvokeSolidity(ctx, fromAddr, idAddr, entryPoint, inputData)

		require.True(t, wait.Receipt.ExitCode.IsSuccess(), "contract execution failed")

		result, err := cbg.ReadByteArray(bytes.NewBuffer(wait.Receipt.Return), uint64(len(wait.Receipt.Return)))
		require.NoError(t, err)

		expectedResult, err := hex.DecodeString("0000000000000000000000000000000000000000000000000000000000000000")
		require.NoError(t, err)
		require.Equal(t, result, expectedResult)

	}
}

// TestFEVMETH0 tests that the ETH0 actor is in genesis
func TestFEVMETH0(t *testing.T) {
	kit.QuietMiningLogs()

	blockTime := 100 * time.Millisecond
	client, _, ens := kit.EnsembleMinimal(t, kit.MockProofs(), kit.ThroughRPC())
	ens.InterconnectAll().BeginMining(blockTime)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	eth0id, err := address.NewIDAddress(1001)
	require.NoError(t, err)

	act, err := client.StateGetActor(ctx, eth0id, types.EmptyTSK)
	require.NoError(t, err)

	nv, err := client.StateNetworkVersion(ctx, types.EmptyTSK)
	require.NoError(t, err)

	av, err := actorstypes.VersionForNetwork(nv)
	require.NoError(t, err)

	evmCodeCid, ok := actors.GetActorCodeID(av, actors.EvmKey)
	require.True(t, ok, "failed to get EVM code id")
	require.Equal(t, act.Code, evmCodeCid)
}
