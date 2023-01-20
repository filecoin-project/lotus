package itests

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"strings"
	"testing"

	"encoding/binary"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	builtintypes "github.com/filecoin-project/go-state-types/builtin"
	"github.com/filecoin-project/go-state-types/manifest"

	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/exitcode"
	"github.com/filecoin-project/lotus/api"
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

func transferValueOrFailTest(ctx context.Context, t *testing.T, client *kit.TestFullNode, fromAddr address.Address, toAddr address.Address, sendAmount big.Int) {
	sendMsg := &types.Message{
		From:  fromAddr,
		To:    toAddr,
		Value: sendAmount,
	}
	signedMsg, err := client.MpoolPushMessage(ctx, sendMsg, nil)
	require.NoError(t, err)
	mLookup, err := client.StateWaitMsg(ctx, signedMsg.Cid(), 3, api.LookbackNoLimit, true)
	require.NoError(t, err)
	require.Equal(t, exitcode.Ok, mLookup.Receipt.ExitCode)
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
	ctx, cancel, client := kit.SetupFEVMTest(t)
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
	ctx, cancel, client := kit.SetupFEVMTest(t)
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
	ctx, cancel, client := kit.SetupFEVMTest(t)
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
	ctx, cancel, client := kit.SetupFEVMTest(t)
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
	//TODO do somethign with events
	_ = events
	//fmt.Println(events)
	//fmt.Println(len(events))

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
	result, _, err := client.EVM().InvokeContractByFuncName(ctx, fromAddr, idAddr, "recursiveCall(uint256)", inputData)
	if err != nil {
		require.Error(t, err)
	} else {
		require.NoError(t, err)

		result, _, err = client.EVM().InvokeContractByFuncName(ctx, fromAddr, idAddr, "totalCalls()", []byte{})
		require.NoError(t, err)

		resultUint, err := decodeOutputToUint64(result)
		require.NoError(t, err)

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
// XXX acts weird has two different types of errors with inconsistent patterns
func TestFEVMRecursiveDelegatecall(t *testing.T) {

	ctx, cancel, client := kit.SetupFEVMTest(t)
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

	ctx, cancel, client := kit.SetupFEVMTest(t)
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
	ctx, cancel, client := kit.SetupFEVMTest(t)
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

	ctx, cancel, client := kit.SetupFEVMTest(t)
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

// TestFEVMDelegateCallRevert makes a delegatecall action and then calls revert.
// the state should not have changed
func TestFEVMDelegateCallRevert(t *testing.T) {
	ctx, cancel, client := kit.SetupFEVMTest(t)
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
	ctx, cancel, client := kit.SetupFEVMTest(t)
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
	ctx, cancel, client := kit.SetupFEVMTest(t)
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

// TestFEVMTestApp deploys a fairly complex app contract and confirms it works as expected
func TestFEVMTestApp(t *testing.T) {
	ctx, cancel, client := kit.SetupFEVMTest(t)
	defer cancel()

	//install contract Actor
	filenameStorage := "contracts/TestApp.hex"
	fromAddr, contractAddr := client.EVM().DeployContractFromFilename(ctx, filenameStorage)

	inputData, err := hex.DecodeString("0000000000000000000000000000000000000000000000000000000000000040000000000000000000000000000000000000000000000000000000000000000700000000000000000000000000000000000000000000000000000000000000066162636465660000000000000000000000000000000000000000000000000000") // sending string "abcdef" and int 7 - constructed using remix
	require.NoError(t, err)
	_, _, err = client.EVM().InvokeContractByFuncName(ctx, fromAddr, contractAddr, "new_Test(string,uint256)", inputData)
	t.Log(err)
	require.NoError(t, err)

	inputData, err = hex.DecodeString("0000000000000000000000000000000000000000000000000000000000000000")
	require.NoError(t, err)

	_, _, err = client.EVM().InvokeContractByFuncName(ctx, fromAddr, contractAddr, "get_Test_N(uint256)", inputData)
	require.NoError(t, err)

}

// TestFEVMTestApp creates a contract that just has a self destruct feature and calls it
func TestFEVMTestConstructor(t *testing.T) {
	ctx, cancel, client := kit.SetupFEVMTest(t)
	defer cancel()

	//install contract Actor
	filenameStorage := "contracts/Constructor.hex"
	fromAddr, contractAddr := client.EVM().DeployContractFromFilename(ctx, filenameStorage)

	//input = uint256{7}. set value and confirm tx success
	inputData, err := hex.DecodeString("0000000000000000000000000000000000000000000000000000000000000007")
	require.NoError(t, err)
	_, _, err = client.EVM().InvokeContractByFuncName(ctx, fromAddr, contractAddr, "new_Test(uint256)", inputData)
	t.Log(err)
	require.NoError(t, err)

}

// TestFEVMAutoSelfDestruct creates a contract that just has a self destruct feature and calls it
// XXX tx w destroy fails
func TestFEVMAutoSelfDestruct(t *testing.T) {
	ctx, cancel, client := kit.SetupFEVMTest(t)
	defer cancel()

	//install contract Actor
	filenameStorage := "contracts/AutoSelfDestruct.hex"
	fromAddr, contractAddr := client.EVM().DeployContractFromFilename(ctx, filenameStorage)

	//call destroy
	_, _, err := client.EVM().InvokeContractByFuncName(ctx, fromAddr, contractAddr, "destroy()", []byte{})
	t.Log(err)
	require.NoError(t, err) // XXX currently returns an error but should be success

}

// TestFEVMTestApp creates a contract that just has a self destruct feature and calls it
// XXX calling self destruct fails
func TestFEVMTestSendToContract(t *testing.T) {

	//TODO not sure how to get actual gas used so for now hard coding these values
	// calculated from running the test and seeing the change
	// next step is to make these come through automatically..

	//gasCostDeploy, _ := big.FromString("508807660917778")
	gasCostTransfer, _ := big.FromString("508807660917778")
	gasCostDestroy, _ := big.FromString("508807660917778")

	_ = gasCostTransfer
	_ = gasCostDestroy

	ctx, cancel, client := kit.SetupFEVMTest(t)
	defer cancel()

	bal, err := client.WalletBalance(ctx, client.DefaultKey.Address)
	require.NoError(t, err)
	t.Log("initial balance- ", bal)

	originalBalance, err := big.FromString("100000000000000000000000000")
	require.NoError(t, err)
	require.Equal(t, originalBalance, bal)

	//install contract TestApp
	filenameStorage := "contracts/SelfDestruct.hex"
	fromAddr, contractAddr := client.EVM().DeployContractFromFilename(ctx, filenameStorage)

	bal, err = client.WalletBalance(ctx, client.DefaultKey.Address)
	require.NoError(t, err)
	t.Log("Deploy cost - ", big.Subtract(originalBalance, bal))

	//transfer 1 wei to contract
	sendAmount := big.NewInt(1)
	sendMsg := &types.Message{
		From:  fromAddr,
		To:    contractAddr,
		Value: sendAmount,
	}
	signedMsg, err := client.MpoolPushMessage(ctx, sendMsg, nil)
	require.NoError(t, err)
	mLookup, err := client.StateWaitMsg(ctx, signedMsg.Cid(), 3, api.LookbackNoLimit, true)
	require.NoError(t, err)
	require.Equal(t, exitcode.Ok, mLookup.Receipt.ExitCode)

	bal, err = client.WalletBalance(ctx, client.DefaultKey.Address)
	require.NoError(t, err)
	t.Log("balance after send - ", bal)

	t.Log("balance change from send - ", big.Subtract(originalBalance, big.Add(bal, sendAmount)))

	//todo confirm half

	//call self destruct which should return balance
	a, b, err := client.EVM().InvokeContractByFuncName(ctx, fromAddr, contractAddr, "destroy()", []byte{})
	t.Log(err)
	t.Log("a - ", a)
	t.Log("b - ", b)
	require.Error(t, err) // XXX currently returns an error but should be success

	bal, err = client.WalletBalance(ctx, client.DefaultKey.Address)
	require.NoError(t, err)
	t.Log("balance after destroy - ", bal)

}

// creates a contract that would fail when tx are sent to it
// on eth but on fevm it succeeds
// example failing on testnet https://goerli.etherscan.io/address/0x2ff1525e060169dbf97b9461758c8f701f107cd2
func TestFEVMTestNotPayable(t *testing.T) {

	ctx, cancel, client := kit.SetupFEVMTest(t)
	defer cancel()

	fromAddr := client.DefaultKey.Address
	t.Log("from - ", fromAddr)

	//create contract A
	filenameStorage := "contracts/NotPayable.hex"
	fromAddr, contractAddr := client.EVM().DeployContractFromFilename(ctx, filenameStorage)
	sendAmount := big.MustFromString("10000000")

	transferValueOrFailTest(ctx, t, client, fromAddr, contractAddr, sendAmount)

}

// creates a contract that would fail when tx are sent to it
// on eth but on fevm it succeeds
// example on goerli of tx failing https://goerli.etherscan.io/address/0xec037bdc9a79420985a53a49fdae3ccf8989909b
func TestFEVMSendGasLimit(t *testing.T) {
	ctx, cancel, client := kit.SetupFEVMTest(t)
	defer cancel()

	//install contract
	filenameActor := "contracts/GasLimitSend.hex"
	fromAddr, contractAddr := client.EVM().DeployContractFromFilename(ctx, filenameActor)

	//send $ to contract
	//transfer 1 wei to contract
	sendAmount := big.MustFromString("10000000")

	transferValueOrFailTest(ctx, t, client, fromAddr, contractAddr, sendAmount)
}

// TestFEVMDelegateCall deploys the two contracts in TestFEVMDelegateCall but instead of A calling B, A calls A which should cause A to cause A in an infinite loop and should give a reasonable error
// XXX should not be fatal errors
func TestFEVMDelegateCallRecursiveFail(t *testing.T) {
	//TODO change the gas limit of this invocation and confirm that the number of errors is different
	//also TODO should we not have fatal error show up here?
	ctx, cancel, client := kit.SetupFEVMTest(t)
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
	if err != nil {
		t.Log(err)
	}

	//assert no fatal errors:
	error1 := "f01002 (method 2) -- fatal error (10)" // showing once
	error2 := "f01002 (method 5) -- fatal error (10)" // showing 256 times
	error3 := "f01002 (method 3) -- fatal error (10)" // showing once
	errorAny := "fatal error"                         // showing once

	require.NotContains(t, err.Error(), error1)
	require.NotContains(t, err.Error(), error2)
	require.NotContains(t, err.Error(), error3)
	require.NotContains(t, err.Error(), errorAny)
	require.NoError(t, err)

}

// XXX Currently fails as self destruct has a bug
// TestFEVMTestSendValueThroughContracts creates A and B contract and exchanges value
// and self destructs and accounts for value sent
func TestFEVMTestSendValueThroughContractsAndDestroy(t *testing.T) {

	ctx, cancel, client := kit.SetupFEVMTest(t)
	defer cancel()

	fromAddr := client.DefaultKey.Address
	t.Log("from - ", fromAddr)

	//create contract A
	filenameStorage := "contracts/ValueSender.hex"
	fromAddr, contractAddr := client.EVM().DeployContractFromFilename(ctx, filenameStorage)

	//create contract B
	ret, _, err := client.EVM().InvokeContractByFuncName(ctx, fromAddr, contractAddr, "createB()", []byte{})
	require.NoError(t, err)

	ethAddr, err := ethtypes.CastEthAddress(ret[12:])
	require.NoError(t, err)
	contractBAddress, err := ethAddr.ToFilecoinAddress()
	require.NoError(t, err)
	t.Log("contractBAddress - ", contractBAddress)

	//self destruct contract B
	_, _, err = client.EVM().InvokeContractByFuncName(ctx, fromAddr, contractBAddress, "selfDestruct()", []byte{})
	require.NoError(t, err)

}

// @mgs possibly irrelevant test
// TestFEVMTestSendGasCost sends $ to yourself and looks at the balance diff
// to determine gas cost of sending
// gas seems to go down each time by a factor of appx 2
// for the first 10 transfers then settles to appx constant ???
// XXX gas costs are weird
func TestFEVMTestSendGasCost(t *testing.T) {

	ctx, cancel, client := kit.SetupFEVMTest(t)
	defer cancel()

	fromAddr := client.DefaultKey.Address

	bal, err := client.WalletBalance(ctx, fromAddr)
	require.NoError(t, err)
	t.Log("initial balance- ", bal)

	originalBalance, err := big.FromString("100000000000000000000000000")
	require.NoError(t, err)
	require.Equal(t, originalBalance, bal)

	//transfer 0 wei to yourself
	sendAmount := big.NewInt(0)
	transferValueOrFailTest(ctx, t, client, fromAddr, fromAddr, sendAmount)

	//calculate gas used to send 0 to yourself
	bal, err = client.WalletBalance(ctx, client.DefaultKey.Address)
	require.NoError(t, err)
	gasUsedFirstSend := big.Subtract(originalBalance, bal)
	t.Log("balance change from send - ", 0, gasUsedFirstSend)

	//send 0 again to yourself 15 times and calculate gas
	for i := 1; i <= 15; i++ {
		bal, err = client.WalletBalance(ctx, client.DefaultKey.Address)
		require.NoError(t, err)
		transferValueOrFailTest(ctx, t, client, fromAddr, fromAddr, sendAmount)
		balAfterTx, err := client.WalletBalance(ctx, client.DefaultKey.Address)
		require.NoError(t, err)
		gasUsed := big.Subtract(bal, balAfterTx)
		t.Log("balance change from send - ", i, gasUsed)

	}
}

func TestEVMRpcDisable(t *testing.T) {
	client, _, _ := kit.EnsembleMinimal(t, kit.MockProofs(), kit.ThroughRPC(), kit.DisableEthRPC())

	_, err := client.EthBlockNumber(context.Background())
	require.ErrorContains(t, err, "module disabled, enable with Fevm.EnableEthRPC")
}
