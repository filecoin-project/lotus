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
	"golang.org/x/crypto/sha3"

	"github.com/filecoin-project/go-address"
	actorstypes "github.com/filecoin-project/go-state-types/actors"
	builtintypes "github.com/filecoin-project/go-state-types/builtin"
	"github.com/filecoin-project/go-state-types/manifest"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/types/ethtypes"
	"github.com/filecoin-project/lotus/itests/kit"
)

func setupFEVMTest(t *testing.T) (context.Context, context.CancelFunc, *kit.TestFullNode) {
	kit.QuietMiningLogs()
	blockTime := 100 * time.Millisecond
	client, _, ens := kit.EnsembleMinimal(t, kit.MockProofs(), kit.ThroughRPC())
	ens.InterconnectAll().BeginMining(blockTime)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	return ctx, cancel, client
}

func installContract(t *testing.T, ctx context.Context, filename string, client *kit.TestFullNode) (address.Address, address.Address) {
	contractHex, err := os.ReadFile(filename)
	require.NoError(t, err)

	contract, err := hex.DecodeString(string(contractHex))
	require.NoError(t, err)

	fromAddr, err := client.WalletDefaultAddress(ctx)
	require.NoError(t, err)

	result := client.EVM().DeployContract(ctx, fromAddr, contract)

	idAddr, err := address.NewIDAddress(result.ActorID)
	require.NoError(t, err)

	return fromAddr, idAddr
}

func invokeContract(t *testing.T, ctx context.Context, client *kit.TestFullNode, fromAddr address.Address, idAddr address.Address, funcSignature string, inputData []byte) []byte {
	entryPoint := calcFuncSignature(funcSignature)
	wait := client.EVM().InvokeSolidity(ctx, fromAddr, idAddr, entryPoint, inputData)
	require.True(t, wait.Receipt.ExitCode.IsSuccess(), "contract execution failed")
	result, err := cbg.ReadByteArray(bytes.NewBuffer(wait.Receipt.Return), uint64(len(wait.Receipt.Return)))
	require.NoError(t, err)
	return result
}

//function signatures are the first 4 bytes of the hash of the function name and types
func calcFuncSignature(funcName string) []byte {
	hasher := sha3.NewLegacyKeccak256()
	hasher.Write([]byte(funcName))
	hash := hasher.Sum(nil)
	return hash[:4]
}

//convert a simple byte array into input data which is a left padded 32 byte array
func inputDataFromArray(input []byte) []byte {
	inputData := make([]byte, 32)
	copy(inputData[32-len(input):], input[:])
	return inputData
}

//convert a "from" address into input data which is a left padded 32 byte array
func inputDataFromFrom(t *testing.T, ctx context.Context, client *kit.TestFullNode, from address.Address) []byte {
	fromId, err := client.StateLookupID(ctx, from, types.EmptyTSK)
	require.NoError(t, err)

	senderEthAddr, err := ethtypes.EthAddressFromFilecoinAddress(fromId)
	require.NoError(t, err)
	inputData := make([]byte, 32)
	copy(inputData[32-len(senderEthAddr):], senderEthAddr[:])
	return inputData
}

// TestFEVMBasic does a basic fevm contract installation and invocation
func TestFEVMBasic(t *testing.T) {

	ctx, cancel, client := setupFEVMTest(t)
	defer cancel()

	filename := "contracts/SimpleCoin.bin"
	// install contract
	fromAddr, idAddr := installContract(t, ctx, filename, client)

	// invoke the contract with owner
	{
		inputData := inputDataFromFrom(t, ctx, client, fromAddr)
		result := invokeContract(t, ctx, client, fromAddr, idAddr, "getBalance(address)", inputData)

		expectedResult, err := hex.DecodeString("0000000000000000000000000000000000000000000000000000000000002710")
		require.NoError(t, err)
		require.Equal(t, result, expectedResult)
	}

	// invoke the contract with non owner
	{
		inputData := inputDataFromFrom(t, ctx, client, fromAddr)
		inputData[31] += 1 // change the pub address to one that has 0 balance by modifying the last byte of the address
		result := invokeContract(t, ctx, client, fromAddr, idAddr, "getBalance(address)", inputData)

		expectedResult, err := hex.DecodeString("0000000000000000000000000000000000000000000000000000000000000000")
		require.NoError(t, err)
		require.Equal(t, result, expectedResult)
	}
}

// TestFEVMETH0 tests that the ETH0 actor is in genesis
func TestFEVMETH0(t *testing.T) {
	ctx, cancel, client := setupFEVMTest(t)
	defer cancel()

	eth0id, err := address.NewIDAddress(1001)
	require.NoError(t, err)

	act, err := client.StateGetActor(ctx, eth0id, types.EmptyTSK)
	require.NoError(t, err)

	nv, err := client.StateNetworkVersion(ctx, types.EmptyTSK)
	require.NoError(t, err)

	av, err := actorstypes.VersionForNetwork(nv)
	require.NoError(t, err)

	evmCodeCid, ok := actors.GetActorCodeID(av, manifest.EvmKey)
	require.True(t, ok, "failed to get EVM code id")
	require.Equal(t, act.Code, evmCodeCid)

	eth0Addr, err := address.NewDelegatedAddress(builtintypes.EthereumAddressManagerActorID, make([]byte, 20))
	require.NoError(t, err)
	require.Equal(t, *act.Address, eth0Addr)
}

// TestFEVMDelegateCall deploys two contracts and makes a delegate call transaction
func TestFEVMDelegateCall(t *testing.T) {

	ctx, cancel, client := setupFEVMTest(t)
	defer cancel()

	//install contract Actor
	filenameActor := "contracts/DelegatecallActor.bin"
	fromAddr, actorAddr := installContract(t, ctx, filenameActor, client)
	//install contract Storage
	filenameStorage := "contracts/DelegatecallStorage.bin"
	fromAddrStorage, storageAddr := installContract(t, ctx, filenameStorage, client)
	require.Equal(t, fromAddr, fromAddrStorage)

	//call Contract Storage which makes a delegatecall to contract Actor

	inputDataContract := inputDataFromFrom(t, ctx, client, actorAddr)
	inputDataValue := inputDataFromArray([]byte{7})
	inputData := append(inputDataContract, inputDataValue...)

	result := invokeContract(t, ctx, client, fromAddr, storageAddr, "setVars(address,uint256)", inputData)
	expectedResult, err := hex.DecodeString("0000000000000000000000000000000000000000000000000000000000000007")
	require.NoError(t, err)
	require.Equal(t, result, expectedResult)

}
