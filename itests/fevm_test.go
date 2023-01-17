package itests

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"strings"
	"testing"
	"time"

	"encoding/binary"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	builtintypes "github.com/filecoin-project/go-state-types/builtin"
	"github.com/filecoin-project/go-state-types/manifest"

	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/types/ethtypes"
	"github.com/filecoin-project/lotus/itests/kit"
)

// convert a simple byte array into input data which is a left padded 32 byte array
func inputDataFromArray(input []byte) []byte {
	inputData := make([]byte, 32)
	copy(inputData[32-len(input):], input[:])
	return inputData
}

// convert a "from" address into input data which is a left padded 32 byte array
func inputDataFromFrom(ctx context.Context, t *testing.T, client *kit.TestFullNode, from address.Address) []byte {
	fromId, err := client.StateLookupID(ctx, from, types.EmptyTSK)
	require.NoError(t, err)

	senderEthAddr, err := ethtypes.EthAddressFromFilecoinAddress(fromId)
	require.NoError(t, err)
	inputData := make([]byte, 32)
	copy(inputData[32-len(senderEthAddr):], senderEthAddr[:])
	return inputData
}

func setupFEVMTest(t *testing.T) (context.Context, context.CancelFunc, *kit.TestFullNode) {
	kit.QuietMiningLogs()
	blockTime := 100 * time.Millisecond
	client, _, ens := kit.EnsembleMinimal(t, kit.MockProofs(), kit.ThroughRPC())
	ens.InterconnectAll().BeginMining(blockTime)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	return ctx, cancel, client
}

func decodeOutputToUint64(output []byte) (uint64, error) {
	var result uint64
	buf := bytes.NewReader(output[len(output)-8:])
	err := binary.Read(buf, binary.BigEndian, &result)
	return result, err
}
func buildInputFromuint64(number uint64) []byte {
	// Convert the number to a binary uint64 array
	binaryNumber := make([]byte, 8)
	binary.BigEndian.PutUint64(binaryNumber, number)
	return inputDataFromArray(binaryNumber)
}

// math.Max is weird needs float
func max(x int, y int) int {
	max := x
	if y > max {
		max = y
	}
	return max
}

// TestFEVMRecursive does a basic fevm contract installation and invocation
func TestFEVMRecursive(t *testing.T) {
	callcounts := []uint64{0, 1}
	ctx, cancel, client := setupFEVMTest(t)
	defer cancel()
	filename := "contracts/Recursive.hex"
	fromAddr, idAddr := client.EVM().DeployContractFromFilename(ctx, filename)

	// Iterate over the numbers array
	for _, _callcount := range callcounts {
		callcount := _callcount // create local copy in loop to mollify golint
		t.Run(fmt.Sprintf("TestFEVMRecursive%d", callcount), func(t *testing.T) {
			_, ret, err := client.EVM().InvokeContractByFuncName(ctx, fromAddr, idAddr, "recursiveCall(uint256)", buildInputFromuint64(callcount))
			if err != nil {
				fmt.Printf("error - %+v", err)
			}
			require.NoError(t, err)
			events := client.EVM().LoadEvents(ctx, *ret.Receipt.EventsRoot)
			//passing in 0 still means there's 1 event
			require.Equal(t, max(1, int(callcount)), len(events))
		})
	}
}

func TestFEVMRecursiveFail(t *testing.T) {
	callcounts := []uint64{2, 10, 1000, 100000}
	ctx, cancel, client := setupFEVMTest(t)
	defer cancel()
	filename := "contracts/Recursive.hex"
	fromAddr, idAddr := client.EVM().DeployContractFromFilename(ctx, filename)

	// Iterate over the numbers array
	for _, _callcount := range callcounts {
		callcount := _callcount // create local copy in loop to mollify golint
		t.Run(fmt.Sprintf("TestFEVMRecursive%d", callcount), func(t *testing.T) {
			_, _, err := client.EVM().InvokeContractByFuncName(ctx, fromAddr, idAddr, "recursiveCall(uint256)", buildInputFromuint64(callcount))
			require.Error(t, err)
			require.True(t, strings.HasPrefix(err.Error(), "GasEstimateMessageGas"))
		})
	}
}
func TestFEVMRecursive1(t *testing.T) {
	callcount := 1
	ctx, cancel, client := setupFEVMTest(t)
	defer cancel()
	filename := "contracts/Recursive.hex"
	fromAddr, idAddr := client.EVM().DeployContractFromFilename(ctx, filename)
	_, ret, err := client.EVM().InvokeContractByFuncName(ctx, fromAddr, idAddr, "recursive1()", []byte{})
	require.NoError(t, err)
	events := client.EVM().LoadEvents(ctx, *ret.Receipt.EventsRoot)
	//passing in 0 still means there's 1 event
	require.Equal(t, max(1, callcount), len(events))
}
func TestFEVMRecursive2(t *testing.T) {
	ctx, cancel, client := setupFEVMTest(t)
	defer cancel()
	filename := "contracts/Recursive.hex"
	fromAddr, idAddr := client.EVM().DeployContractFromFilename(ctx, filename)
	_, _, err := client.EVM().InvokeContractByFuncName(ctx, fromAddr, idAddr, "recursive2()", []byte{})
	require.Error(t, err)
	require.True(t, strings.HasPrefix(err.Error(), "GasEstimateMessageGas"))
}

func recursiveDelegatecallNotEqual(ctx context.Context, t *testing.T, client *kit.TestFullNode, filename string, count uint64) {

	fromAddr, idAddr := client.EVM().DeployContractFromFilename(ctx, filename)
	t.Log("recursion count - ", count)
	inputData := buildInputFromuint64(count)
	result, ret, err := client.EVM().InvokeContractByFuncName(ctx, fromAddr, idAddr, "recursiveCall(uint256)", inputData)
	require.NoError(t, err)
	fmt.Println(result)
	events := client.EVM().LoadEvents(ctx, *ret.Receipt.EventsRoot)
	fmt.Println(events)
	fmt.Println(len(events))

	result, _, err = client.EVM().InvokeContractByFuncName(ctx, fromAddr, idAddr, "totalCalls()", []byte{})
	require.NoError(t, err)
	t.Log("result - ", result)

	resultUint, err := decodeOutputToUint64(result)
	require.NoError(t, err)
	t.Log("result - ", resultUint)

	require.NotEqual(t, int(resultUint), int(count))
}
func recursiveDelegatecallError(ctx context.Context, t *testing.T, client *kit.TestFullNode, filename string, count uint64) {

	fromAddr, idAddr := client.EVM().DeployContractFromFilename(ctx, filename)
	t.Log("recursion count - ", count)
	inputData := buildInputFromuint64(count)
	_, _, err := client.EVM().InvokeContractByFuncName(ctx, fromAddr, idAddr, "recursiveCall(uint256)", inputData)
	require.Error(t, err)

	//result, _, err = client.EVM().InvokeContractByFuncName(ctx, fromAddr, idAddr, "totalCalls()", []byte{})
	//require.Error(t, err)
}

func recursiveDelegatecallFail(ctx context.Context, t *testing.T, client *kit.TestFullNode, filename string, count uint64) {

	fromAddr, idAddr := client.EVM().DeployContractFromFilename(ctx, filename)
	t.Log("recursion count - ", count)
	inputData := buildInputFromuint64(count)
	result, ret, err := client.EVM().InvokeContractByFuncName(ctx, fromAddr, idAddr, "recursiveCall(uint256)", inputData)
	if err != nil {
		require.Error(t, err)
	} else {
		require.NoError(t, err)
		fmt.Println(result)
		events := client.EVM().LoadEvents(ctx, *ret.Receipt.EventsRoot)
		fmt.Println(events)
		fmt.Println(len(events))

		result, _, err = client.EVM().InvokeContractByFuncName(ctx, fromAddr, idAddr, "totalCalls()", []byte{})
		require.NoError(t, err)
		t.Log("result - ", result)

		resultUint, err := decodeOutputToUint64(result)
		require.NoError(t, err)
		t.Log("result - ", resultUint)

		require.NotEqual(t, int(resultUint), int(count))
	}
}
func recursiveDelegatecallSuccess(ctx context.Context, t *testing.T, client *kit.TestFullNode, filename string, count uint64) {

	fromAddr, idAddr := client.EVM().DeployContractFromFilename(ctx, filename)
	t.Log("recursion count - ", count)
	inputData := buildInputFromuint64(count)
	result, ret, err := client.EVM().InvokeContractByFuncName(ctx, fromAddr, idAddr, "recursiveCall(uint256)", inputData)
	require.NoError(t, err)
	fmt.Println(result)
	events := client.EVM().LoadEvents(ctx, *ret.Receipt.EventsRoot)
	fmt.Println(events)
	fmt.Println(len(events))

	result, _, err = client.EVM().InvokeContractByFuncName(ctx, fromAddr, idAddr, "totalCalls()", []byte{})
	require.NoError(t, err)
	t.Log("result - ", result)

	resultUint, err := decodeOutputToUint64(result)
	require.NoError(t, err)
	t.Log("result - ", resultUint)

	require.Equal(t, int(resultUint), int(count))
}

// TestFEVMBasic does a basic fevm contract installation and invocation
func TestFEVMRecursiveDelegatecall(t *testing.T) {

	ctx, cancel, client := setupFEVMTest(t)
	defer cancel()

	filename := "contracts/RecursiveDelegeatecall.hex"

	//success with 44 or fewer calls
	for i := uint64(1); i <= 44; i++ { // 0-10 linearly then 10-120 by 10s
		recursiveDelegatecallSuccess(ctx, t, client, filename, i)
	}

	//these fail but the evm doesn't error, just the return value is not count
	for i := uint64(46); i <= 58; i++ {
		recursiveDelegatecallNotEqual(ctx, t, client, filename, i)
	}

	//from 59 to 62 it fails in an oscillating way between count not being correct and EVM error
	for i := uint64(59); i <= 62; i++ {
		if i%2 == 0 {
			recursiveDelegatecallNotEqual(ctx, t, client, filename, i)
		} else {
			recursiveDelegatecallError(ctx, t, client, filename, i)
		}
	}
	//63 and beyond it fails either w evm error or an incorrect result:
	for i := uint64(63); i <= 800; i += 40 {
		recursiveDelegatecallFail(ctx, t, client, filename, i)
	}
}

// TestFEVMBasic does a basic fevm contract installation and invocation
func TestFEVMBasic(t *testing.T) {

	ctx, cancel, client := setupFEVMTest(t)
	defer cancel()

	filename := "contracts/SimpleCoin.hex"
	// install contract
	fromAddr, idAddr := client.EVM().DeployContractFromFilename(ctx, filename)

	// invoke the contract with owner
	{
		inputData := inputDataFromFrom(ctx, t, client, fromAddr)
		result, _, err := client.EVM().InvokeContractByFuncName(ctx, fromAddr, idAddr, "getBalance(address)", inputData)
		require.NoError(t, err)

		expectedResult, err := hex.DecodeString("0000000000000000000000000000000000000000000000000000000000002710")
		require.NoError(t, err)
		require.Equal(t, result, expectedResult)
	}

	// invoke the contract with non owner
	{
		inputData := inputDataFromFrom(ctx, t, client, fromAddr)
		inputData[31]++ // change the pub address to one that has 0 balance by modifying the last byte of the address
		result, _, err := client.EVM().InvokeContractByFuncName(ctx, fromAddr, idAddr, "getBalance(address)", inputData)
		require.NoError(t, err)

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

	client.AssertActorType(ctx, eth0id, manifest.EthAccountKey)

	act, err := client.StateGetActor(ctx, eth0id, types.EmptyTSK)
	require.NoError(t, err)

	eth0Addr, err := address.NewDelegatedAddress(builtintypes.EthereumAddressManagerActorID, make([]byte, 20))
	require.NoError(t, err)
	require.Equal(t, *act.Address, eth0Addr)
}

// TestFEVMDelegateCall deploys two contracts and makes a delegate call transaction
func TestFEVMDelegateCall(t *testing.T) {

	ctx, cancel, client := setupFEVMTest(t)
	defer cancel()

	//install contract Actor
	filenameActor := "contracts/DelegatecallActor.hex"
	fromAddr, actorAddr := client.EVM().DeployContractFromFilename(ctx, filenameActor)
	//install contract Storage
	filenameStorage := "contracts/DelegatecallStorage.hex"
	fromAddrStorage, storageAddr := client.EVM().DeployContractFromFilename(ctx, filenameStorage)
	require.Equal(t, fromAddr, fromAddrStorage)

	//call Contract Storage which makes a delegatecall to contract Actor
	//this contract call sets the "counter" variable to 7, from default value 0

	inputDataContract := inputDataFromFrom(ctx, t, client, actorAddr)
	inputDataValue := inputDataFromArray([]byte{7})
	inputData := append(inputDataContract, inputDataValue...)

	//verify that the returned value of the call to setvars is 7
	result, _, err := client.EVM().InvokeContractByFuncName(ctx, fromAddr, storageAddr, "setVars(address,uint256)", inputData)
	require.NoError(t, err)
	expectedResult, err := hex.DecodeString("0000000000000000000000000000000000000000000000000000000000000007")
	require.NoError(t, err)
	require.Equal(t, result, expectedResult)

	//test the value is 7 via calling the getter
	result, _, err = client.EVM().InvokeContractByFuncName(ctx, fromAddr, storageAddr, "getCounter()", []byte{})
	require.NoError(t, err)
	require.Equal(t, result, expectedResult)

	//test the value is 0 via calling the getter on the Actor contract
	result, _, err = client.EVM().InvokeContractByFuncName(ctx, fromAddr, actorAddr, "getCounter()", []byte{})
	require.NoError(t, err)
	expectedResultActor, err := hex.DecodeString("0000000000000000000000000000000000000000000000000000000000000000")
	require.NoError(t, err)
	require.Equal(t, result, expectedResultActor)
}

// TestFEVMDelegateCall deploys the two contracts in TestFEVMDelegateCall but instead of A calling B, A calls A which should cause A to cause A in an infinite loop and should give a reasonable error
func TestFEVMDelegateCallRecursiveFail(t *testing.T) {
	//TODO change the gas limit of this invocation and confirm that the number of errors is different
	//also TODO should we not have fatal error show up here?
	ctx, cancel, client := setupFEVMTest(t)
	defer cancel()

	//install contract Actor
	filenameActor := "contracts/DelegatecallStorage.hex"
	fromAddr, actorAddr := client.EVM().DeployContractFromFilename(ctx, filenameActor)

	//any data will do for this test that fails
	inputDataContract := inputDataFromFrom(ctx, t, client, actorAddr)
	inputDataValue := inputDataFromArray([]byte{7})
	inputData := append(inputDataContract, inputDataValue...)

	//verify that the returned value of the call to setvars is 7
	_, _, err := client.EVM().InvokeContractByFuncName(ctx, fromAddr, actorAddr, "setVarsSelf(address,uint256)", inputData)
	require.Error(t, err)

	t.Log(err)
	//the error message is a big string, the handling is going to be a little messy
	//the failure is that we recursively call setVarsSelf
	//so check the counts of the specific sub call error messages are correct:
	error1 := "f01002 (method 2) -- fatal error (10)" // expect once
	error2 := "f01002 (method 5) -- fatal error (10)" // expected 256 times
	error3 := "f01002 (method 3) -- fatal error (10)" // expect once
	require.Equal(t, 1, strings.Count(err.Error(), error1))
	require.Equal(t, 256, strings.Count(err.Error(), error2))
	require.Equal(t, 1, strings.Count(err.Error(), error3))
}

// TestFEVMDelegateCallRevert makes a delegatecall action and then calls revert.
// the state should not have changed
func TestFEVMDelegateCallRevert(t *testing.T) {
	ctx, cancel, client := setupFEVMTest(t)
	defer cancel()

	//install contract Actor
	filenameActor := "contracts/DelegatecallActor.hex"
	fromAddr, actorAddr := client.EVM().DeployContractFromFilename(ctx, filenameActor)
	//install contract Storage
	filenameStorage := "contracts/DelegatecallStorage.hex"
	fromAddrStorage, storageAddr := client.EVM().DeployContractFromFilename(ctx, filenameStorage)
	require.Equal(t, fromAddr, fromAddrStorage)

	//call Contract Storage which makes a delegatecall to contract Actor
	//this contract call sets the "counter" variable to 7, from default value 0

	inputDataContract := inputDataFromFrom(ctx, t, client, actorAddr)
	inputDataValue := inputDataFromArray([]byte{7})
	inputData := append(inputDataContract, inputDataValue...)

	//verify that the returned value of the call to setvars is 7
	_, _, err := client.EVM().InvokeContractByFuncName(ctx, fromAddr, storageAddr, "setVarsRevert(address,uint256)", inputData)
	require.Error(t, err)
	t.Log(err)

	//test the value is 0 via calling the getter and was not set to 7
	expectedResult, err := hex.DecodeString("0000000000000000000000000000000000000000000000000000000000000000")
	require.NoError(t, err)
	result, _, err := client.EVM().InvokeContractByFuncName(ctx, fromAddr, storageAddr, "getCounter()", []byte{})
	require.NoError(t, err)
	require.Equal(t, result, expectedResult)

	//test the value is 0 via calling the getter on the Actor contract
	result, _, err = client.EVM().InvokeContractByFuncName(ctx, fromAddr, actorAddr, "getCounter()", []byte{})
	require.NoError(t, err)
	require.Equal(t, result, expectedResult)
}

// TestFEVMDelegateCallRevert makes a call that is a simple revert
func TestFEVMSimpleRevert(t *testing.T) {
	ctx, cancel, client := setupFEVMTest(t)
	defer cancel()

	//install contract Actor
	filenameStorage := "contracts/DelegatecallStorage.hex"
	fromAddr, contractAddr := client.EVM().DeployContractFromFilename(ctx, filenameStorage)

	//call revert
	_, _, err := client.EVM().InvokeContractByFuncName(ctx, fromAddr, contractAddr, "revert()", []byte{})

	require.Error(t, err)
	t.Log(err)
}

// TestFEVMSelfDestruct creates a contract that just has a self destruct feature and calls it
func TestFEVMSelfDestruct(t *testing.T) {
	ctx, cancel, client := setupFEVMTest(t)
	defer cancel()

	//install contract Actor
	filenameStorage := "contracts/SelfDestruct.hex"
	fromAddr, contractAddr := client.EVM().DeployContractFromFilename(ctx, filenameStorage)

	//call destroy
	_, _, err := client.EVM().InvokeContractByFuncName(ctx, fromAddr, contractAddr, "destroy()", []byte{})
	t.Log(err)
	require.Error(t, err) // XXX currently returns an error but should be success

	//call destroy a second time and get error
	_, _, err = client.EVM().InvokeContractByFuncName(ctx, fromAddr, contractAddr, "destroy()", []byte{})
	t.Log(err)
	require.Error(t, err)
}

// TestFEVMAutoSelfDestruct creates a contract that just has a self destruct feature and calls it
func TestFEVMAutoSelfDestruct(t *testing.T) {
	ctx, cancel, client := setupFEVMTest(t)
	defer cancel()

	//install contract Actor
	filenameStorage := "contracts/AutoSelfDestruct.hex"
	fromAddr, contractAddr := client.EVM().DeployContractFromFilename(ctx, filenameStorage)

	//call destroy
	_, _, err := client.EVM().InvokeContractByFuncName(ctx, fromAddr, contractAddr, "destroy()", []byte{})
	t.Log(err)
	require.Error(t, err) // XXX currently returns an error but should be success

}
