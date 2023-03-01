package itests

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/big"
	builtintypes "github.com/filecoin-project/go-state-types/builtin"
	"github.com/filecoin-project/go-state-types/exitcode"
	"github.com/filecoin-project/go-state-types/manifest"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
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

// recursive delegate calls that fail due to gas limits are currently getting to 229 iterations
// before running out of gas
func recursiveDelegatecallFail(ctx context.Context, t *testing.T, client *kit.TestFullNode, filename string, count uint64) {
	expectedIterationsBeforeFailing := int(220)
	fromAddr, idAddr := client.EVM().DeployContractFromFilename(ctx, filename)
	t.Log("recursion count - ", count)
	inputData := buildInputFromuint64(count)
	_, _, err := client.EVM().InvokeContractByFuncName(ctx, fromAddr, idAddr, "recursiveCall(uint256)", inputData)

	require.NoError(t, err)

	result, _, err := client.EVM().InvokeContractByFuncName(ctx, fromAddr, idAddr, "totalCalls()", []byte{})
	require.NoError(t, err)

	resultUint, err := decodeOutputToUint64(result)
	require.NoError(t, err)

	require.NotEqual(t, int(resultUint), int(count))
	require.Equal(t, expectedIterationsBeforeFailing, int(resultUint))
}
func recursiveDelegatecallSuccess(ctx context.Context, t *testing.T, client *kit.TestFullNode, filename string, count uint64) {
	t.Log("Count - ", count)

	fromAddr, idAddr := client.EVM().DeployContractFromFilename(ctx, filename)
	inputData := buildInputFromuint64(count)
	_, _, err := client.EVM().InvokeContractByFuncName(ctx, fromAddr, idAddr, "recursiveCall(uint256)", inputData)
	require.NoError(t, err)

	result, _, err := client.EVM().InvokeContractByFuncName(ctx, fromAddr, idAddr, "totalCalls()", []byte{})
	require.NoError(t, err)

	resultUint, err := decodeOutputToUint64(result)
	require.NoError(t, err)

	require.Equal(t, int(count), int(resultUint))
}

// TestFEVMRecursive does a basic fevm contract installation and invocation
func TestFEVMRecursive(t *testing.T) {
	callCounts := []uint64{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 100, 230, 330}
	ctx, cancel, client := kit.SetupFEVMTest(t)
	defer cancel()
	filename := "contracts/Recursive.hex"
	fromAddr, idAddr := client.EVM().DeployContractFromFilename(ctx, filename)

	// Successful calls
	for _, callCount := range callCounts {
		callCount := callCount // linter unhappy unless callCount is local to loop
		t.Run(fmt.Sprintf("TestFEVMRecursive%d", callCount), func(t *testing.T) {
			_, _, err := client.EVM().InvokeContractByFuncName(ctx, fromAddr, idAddr, "recursiveCall(uint256)", buildInputFromuint64(callCount))
			require.NoError(t, err)
		})
	}

}

func TestFEVMRecursiveFail(t *testing.T) {
	ctx, cancel, client := kit.SetupFEVMTest(t)
	defer cancel()
	filename := "contracts/Recursive.hex"
	fromAddr, idAddr := client.EVM().DeployContractFromFilename(ctx, filename)

	// Unsuccessful calls
	failCallCounts := []uint64{340, 400, 600, 850, 1000}
	for _, failCallCount := range failCallCounts {
		failCallCount := failCallCount // linter unhappy unless callCount is local to loop
		t.Run(fmt.Sprintf("TestFEVMRecursiveFail%d", failCallCount), func(t *testing.T) {
			_, wait, err := client.EVM().InvokeContractByFuncName(ctx, fromAddr, idAddr, "recursiveCall(uint256)", buildInputFromuint64(failCallCount))
			require.Error(t, err)
			require.Equal(t, exitcode.ExitCode(37), wait.Receipt.ExitCode)
		})
	}
}

func TestFEVMRecursive1(t *testing.T) {
	callCount := 1
	ctx, cancel, client := kit.SetupFEVMTest(t)
	defer cancel()
	filename := "contracts/Recursive.hex"
	fromAddr, idAddr := client.EVM().DeployContractFromFilename(ctx, filename)
	_, ret, err := client.EVM().InvokeContractByFuncName(ctx, fromAddr, idAddr, "recursive1()", []byte{})
	require.NoError(t, err)
	events := client.EVM().LoadEvents(ctx, *ret.Receipt.EventsRoot)
	require.Equal(t, callCount, len(events))
}
func TestFEVMRecursive2(t *testing.T) {
	ctx, cancel, client := kit.SetupFEVMTest(t)
	defer cancel()
	filename := "contracts/Recursive.hex"
	fromAddr, idAddr := client.EVM().DeployContractFromFilename(ctx, filename)
	_, ret, err := client.EVM().InvokeContractByFuncName(ctx, fromAddr, idAddr, "recursive2()", []byte{})
	require.NoError(t, err)
	events := client.EVM().LoadEvents(ctx, *ret.Receipt.EventsRoot)
	require.Equal(t, 2, len(events))
}

// TestFEVMRecursiveDelegatecallCount tests the maximum delegatecall recursion depth. It currently
// succeeds succeeds up to 237 times.
func TestFEVMRecursiveDelegatecallCount(t *testing.T) {

	ctx, cancel, client := kit.SetupFEVMTest(t)
	defer cancel()

	highestSuccessCount := uint64(225)

	filename := "contracts/RecursiveDelegeatecall.hex"
	recursiveDelegatecallSuccess(ctx, t, client, filename, uint64(1))
	recursiveDelegatecallSuccess(ctx, t, client, filename, uint64(2))
	recursiveDelegatecallSuccess(ctx, t, client, filename, uint64(10))
	recursiveDelegatecallSuccess(ctx, t, client, filename, uint64(100))
	recursiveDelegatecallSuccess(ctx, t, client, filename, highestSuccessCount)

	recursiveDelegatecallFail(ctx, t, client, filename, highestSuccessCount+1)
	recursiveDelegatecallFail(ctx, t, client, filename, uint64(1000))
	recursiveDelegatecallFail(ctx, t, client, filename, uint64(10000000))

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

	//test the value is 7 a second way by calling the getter
	result, _, err = client.EVM().InvokeContractByFuncName(ctx, fromAddr, storageAddr, "getCounter()", []byte{})
	require.NoError(t, err)
	require.Equal(t, result, expectedResult)

	//test the value is 0 via calling the getter on the Actor contract
	result, _, err = client.EVM().InvokeContractByFuncName(ctx, fromAddr, actorAddr, "getCounter()", []byte{})
	require.NoError(t, err)
	expectedResultActor, err := hex.DecodeString("0000000000000000000000000000000000000000000000000000000000000000")
	require.NoError(t, err)
	require.Equal(t, result, expectedResultActor)

	// The implementation's storage should not have been updated.
	actorAddrEth, err := ethtypes.EthAddressFromFilecoinAddress(actorAddr)
	require.NoError(t, err)
	value, err := client.EVM().EthGetStorageAt(ctx, actorAddrEth, nil, "latest")
	require.NoError(t, err)
	require.Equal(t, ethtypes.EthBytes(make([]byte, 32)), value)

	// The storage actor's storage _should_ have been updated
	storageAddrEth, err := ethtypes.EthAddressFromFilecoinAddress(storageAddr)
	require.NoError(t, err)
	value, err = client.EVM().EthGetStorageAt(ctx, storageAddrEth, nil, "latest")
	require.NoError(t, err)
	require.Equal(t, ethtypes.EthBytes(expectedResult), value)
}

// TestFEVMDelegateCallRevert makes a delegatecall action and then calls revert.
// the state should not have changed because of the revert
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
	_, wait, err := client.EVM().InvokeContractByFuncName(ctx, fromAddr, storageAddr, "setVarsRevert(address,uint256)", inputData)
	require.Error(t, err)
	require.Equal(t, exitcode.ExitCode(33), wait.Receipt.ExitCode)

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

// TestFEVMSimpleRevert makes a call that is a simple revert
func TestFEVMSimpleRevert(t *testing.T) {
	ctx, cancel, client := kit.SetupFEVMTest(t)
	defer cancel()

	//install contract Actor
	filenameStorage := "contracts/DelegatecallStorage.hex"
	fromAddr, contractAddr := client.EVM().DeployContractFromFilename(ctx, filenameStorage)

	//call revert
	_, wait, err := client.EVM().InvokeContractByFuncName(ctx, fromAddr, contractAddr, "revert()", []byte{})

	require.Equal(t, wait.Receipt.ExitCode, exitcode.ExitCode(33))
	require.Error(t, err)
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
	require.NoError(t, err)

	//call destroy a second time and also no error
	_, _, err = client.EVM().InvokeContractByFuncName(ctx, fromAddr, contractAddr, "destroy()", []byte{})
	require.NoError(t, err)
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
	require.NoError(t, err)

}

// TestFEVMAutoSelfDestruct creates a contract that just has a self destruct feature and calls it
func TestFEVMAutoSelfDestruct(t *testing.T) {
	ctx, cancel, client := kit.SetupFEVMTest(t)
	defer cancel()

	//install contract Actor
	filenameStorage := "contracts/AutoSelfDestruct.hex"
	fromAddr, contractAddr := client.EVM().DeployContractFromFilename(ctx, filenameStorage)

	//call destroy
	_, _, err := client.EVM().InvokeContractByFuncName(ctx, fromAddr, contractAddr, "destroy()", []byte{})
	require.NoError(t, err)
}

// TestFEVMTestApp creates a contract that just has a self destruct feature and calls it
func TestFEVMTestSendToContract(t *testing.T) {
	ctx, cancel, client := kit.SetupFEVMTest(t)
	defer cancel()

	bal, err := client.WalletBalance(ctx, client.DefaultKey.Address)
	require.NoError(t, err)

	//install contract TestApp
	filenameStorage := "contracts/SelfDestruct.hex"
	fromAddr, contractAddr := client.EVM().DeployContractFromFilename(ctx, filenameStorage)

	//transfer half balance to contract

	sendAmount := big.Div(bal, big.NewInt(2))
	client.EVM().TransferValueOrFail(ctx, fromAddr, contractAddr, sendAmount)

	//call self destruct which should return balance
	_, _, err = client.EVM().InvokeContractByFuncName(ctx, fromAddr, contractAddr, "destroy()", []byte{})
	require.NoError(t, err)

	finalBalanceMinimum := types.FromFil(uint64(99_999_999)) // 100 million FIL - 1 FIL for gas upper bounds
	finalBal, err := client.WalletBalance(ctx, client.DefaultKey.Address)
	require.NoError(t, err)
	require.Equal(t, true, finalBal.GreaterThan(finalBalanceMinimum))
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
	sendAmount := big.NewInt(10_000_000)

	client.EVM().TransferValueOrFail(ctx, fromAddr, contractAddr, sendAmount)

}

// tx to non function succeeds
func TestFEVMSendCall(t *testing.T) {
	ctx, cancel, client := kit.SetupFEVMTest(t)
	defer cancel()

	//install contract
	filenameActor := "contracts/GasSendTest.hex"
	fromAddr, contractAddr := client.EVM().DeployContractFromFilename(ctx, filenameActor)

	_, _, err := client.EVM().InvokeContractByFuncName(ctx, fromAddr, contractAddr, "x()", []byte{})
	require.NoError(t, err)
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
	//transfer 1 attoFIL to contract
	sendAmount := big.MustFromString("1")

	client.EVM().TransferValueOrFail(ctx, fromAddr, contractAddr, sendAmount)
	_, _, err := client.EVM().InvokeContractByFuncName(ctx, fromAddr, contractAddr, "getDataLength()", []byte{})
	require.NoError(t, err)

}

// TestFEVMDelegateCall deploys the two contracts in TestFEVMDelegateCall but instead of A calling B, A calls A which should cause A to cause A in an infinite loop and should give a reasonable error
func TestFEVMDelegateCallRecursiveFail(t *testing.T) {
	//TODO change the gas limit of this invocation and confirm that the number of errors is
	// different
	ctx, cancel, client := kit.SetupFEVMTest(t)
	defer cancel()

	//install contract Actor
	filenameActor := "contracts/DelegatecallStorage.hex"
	fromAddr, actorAddr := client.EVM().DeployContractFromFilename(ctx, filenameActor)

	//any data will do for this test that fails
	inputDataContract := inputDataFromFrom(ctx, t, client, actorAddr)
	inputDataValue := inputDataFromArray([]byte{7})
	inputData := append(inputDataContract, inputDataValue...)

	//verify that we run out of gas then revert.
	_, wait, err := client.EVM().InvokeContractByFuncName(ctx, fromAddr, actorAddr, "setVarsSelf(address,uint256)", inputData)
	require.Error(t, err)
	require.Equal(t, exitcode.ExitCode(33), wait.Receipt.ExitCode)

	//assert no fatal errors but still there are errors::
	errorAny := "fatal error"
	require.NotContains(t, err.Error(), errorAny)
}

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

func TestEVMRpcDisable(t *testing.T) {
	client, _, _ := kit.EnsembleMinimal(t, kit.MockProofs(), kit.ThroughRPC(), kit.DisableEthRPC())

	_, err := client.EthBlockNumber(context.Background())
	require.ErrorContains(t, err, "module disabled, enable with Fevm.EnableEthRPC")
}

// TestFEVMRecursiveFuncCall deploys a contract and makes a recursive function calls
func TestFEVMRecursiveFuncCall(t *testing.T) {
	ctx, cancel, client := kit.SetupFEVMTest(t)
	defer cancel()

	//install contract Actor
	filenameActor := "contracts/StackFunc.hex"
	fromAddr, actorAddr := client.EVM().DeployContractFromFilename(ctx, filenameActor)

	testN := func(n int, ex exitcode.ExitCode) func(t *testing.T) {
		return func(t *testing.T) {
			inputData := make([]byte, 32)
			binary.BigEndian.PutUint64(inputData[24:], uint64(n))
			client.EVM().InvokeContractByFuncNameExpectExit(ctx, fromAddr, actorAddr, "exec1(uint256)", inputData, ex)
		}
	}

	t.Run("n=0", testN(0, exitcode.Ok))
	t.Run("n=1", testN(1, exitcode.Ok))
	t.Run("n=20", testN(20, exitcode.Ok))
	t.Run("n=200", testN(200, exitcode.Ok))
	t.Run("n=507", testN(507, exitcode.Ok))
	t.Run("n=508", testN(508, exitcode.ExitCode(37))) // 37 means stack overflow
}

// TestFEVMRecursiveActorCall deploys a contract and makes a recursive actor calls
func TestFEVMRecursiveActorCall(t *testing.T) {
	ctx, cancel, client := kit.SetupFEVMTest(t)
	defer cancel()

	//install contract Actor
	filenameActor := "contracts/RecCall.hex"
	fromAddr, actorAddr := client.EVM().DeployContractFromFilename(ctx, filenameActor)

	testN := func(n, r int, ex exitcode.ExitCode) func(t *testing.T) {
		return func(t *testing.T) {
			inputData := make([]byte, 32*3)
			binary.BigEndian.PutUint64(inputData[24:], uint64(n))
			binary.BigEndian.PutUint64(inputData[32+24:], uint64(n))
			binary.BigEndian.PutUint64(inputData[32+32+24:], uint64(r))

			client.EVM().InvokeContractByFuncNameExpectExit(ctx, fromAddr, actorAddr, "exec1(uint256,uint256,uint256)", inputData, ex)
		}
	}

	t.Run("n=0,r=1", testN(0, 1, exitcode.Ok))
	t.Run("n=1,r=1", testN(1, 1, exitcode.Ok))
	t.Run("n=20,r=1", testN(20, 1, exitcode.Ok))
	t.Run("n=200,r=1", testN(200, 1, exitcode.Ok))
	t.Run("n=251,r=1", testN(251, 1, exitcode.Ok))

	t.Run("n=252,r=1-fails", testN(252, 1, exitcode.ExitCode(37))) // 37 means stack overflow

	t.Run("n=0,r=10", testN(0, 10, exitcode.Ok))
	t.Run("n=1,r=10", testN(1, 10, exitcode.Ok))
	t.Run("n=20,r=10", testN(20, 10, exitcode.Ok))
	t.Run("n=200,r=10", testN(200, 10, exitcode.Ok))
	t.Run("n=251,r=10", testN(251, 10, exitcode.Ok))

	t.Run("n=252,r=10-fails", testN(252, 10, exitcode.ExitCode(37)))

	t.Run("n=0,r=32", testN(0, 32, exitcode.Ok))
	t.Run("n=1,r=32", testN(1, 32, exitcode.Ok))
	t.Run("n=20,r=32", testN(20, 32, exitcode.Ok))
	t.Run("n=200,r=32", testN(200, 32, exitcode.Ok))
	t.Run("n=251,r=32", testN(251, 32, exitcode.Ok))

	t.Run("n=0,r=252", testN(0, 252, exitcode.Ok))
	t.Run("n=251,r=166", testN(251, 166, exitcode.Ok))

	t.Run("n=0,r=253-fails", testN(0, 253, exitcode.ExitCode(33))) // 33 means transaction reverted
	t.Run("n=251,r=167-fails", testN(251, 167, exitcode.ExitCode(33)))
}

// TestFEVMRecursiveActorCallEstimate
func TestFEVMRecursiveActorCallEstimate(t *testing.T) {
	ctx, cancel, client := kit.SetupFEVMTest(t)
	defer cancel()

	//install contract Actor
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
			gaslimit, err := client.EthEstimateGas(ctx, ethtypes.EthCall{
				From: &ethAddr,
				To:   &contractAddr,
				Data: params,
			})
			require.NoError(t, err)
			require.LessOrEqual(t, int64(gaslimit), build.BlockGasLimit)

			t.Logf("EthEstimateGas GasLimit=%d", gaslimit)

			maxPriorityFeePerGas, err := client.EthMaxPriorityFeePerGas(ctx)
			require.NoError(t, err)

			nonce, err := client.MpoolGetNonce(ctx, ethFilAddr)
			require.NoError(t, err)

			tx := &ethtypes.EthTxArgs{
				ChainID:              build.Eip155ChainId,
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

			smsg, err := tx.ToSignedMessage()
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

// TestFEVM deploys a contract while sending value to it
func TestFEVMDeployWithValue(t *testing.T) {
	ctx, cancel, client := kit.SetupFEVMTest(t)
	defer cancel()

	//testValue is the amount sent when the contract is created
	//at the end we check that the new contract has a balance of testValue
	testValue := big.NewInt(20)

	// deploy DeployValueTest which creates NewContract
	// testValue is sent to DeployValueTest and that amount is
	// also sent to NewContract
	filenameActor := "contracts/DeployValueTest.hex"
	fromAddr, idAddr := client.EVM().DeployContractFromFilenameWithValue(ctx, filenameActor, testValue)

	//call getNewContractBalance to find the value of NewContract
	ret, _, err := client.EVM().InvokeContractByFuncName(ctx, fromAddr, idAddr, "getNewContractBalance()", []byte{})
	require.NoError(t, err)

	contractBalance, err := decodeOutputToUint64(ret)
	require.NoError(t, err)

	//require balance of NewContract is testValue
	require.Equal(t, testValue.Uint64(), contractBalance)
}

func TestFEVMDestroyCreate2(t *testing.T) {
	ctx, cancel, client := kit.SetupFEVMTest(t)
	defer cancel()

	//deploy create2 factory contract
	filename := "contracts/Create2Factory.hex"
	fromAddr, idAddr := client.EVM().DeployContractFromFilename(ctx, filename)

	//construct salt for create2
	salt := make([]byte, 32)
	_, err := rand.Read(salt)
	require.NoError(t, err)

	//deploy contract using create2 factory
	selfDestructAddress, _, err := client.EVM().InvokeContractByFuncName(ctx, fromAddr, idAddr, "deploy(bytes32)", salt)
	require.NoError(t, err)

	//convert to filecoin actor address so we can call InvokeContractByFuncName
	ea, err := ethtypes.CastEthAddress(selfDestructAddress[12:])
	require.NoError(t, err)
	selfDestructAddressActor, err := ea.ToFilecoinAddress()
	require.NoError(t, err)

	//read sender property from contract
	ret, _, err := client.EVM().InvokeContractByFuncName(ctx, fromAddr, selfDestructAddressActor, "sender()", []byte{})
	require.NoError(t, err)

	//assert contract has correct data
	ethFromAddr := inputDataFromFrom(ctx, t, client, fromAddr)
	require.Equal(t, ethFromAddr, ret)

	//run test() which 1.calls sefldestruct 2. verifies sender() is the correct value 3. attempts and fails to deploy via create2
	testSenderAddress, _, err := client.EVM().InvokeContractByFuncName(ctx, fromAddr, idAddr, "test(address)", selfDestructAddress)
	require.NoError(t, err)
	require.Equal(t, testSenderAddress, ethFromAddr)

	//read sender() but get response of 0x0 because of self destruct
	senderAfterDestroy, _, err := client.EVM().InvokeContractByFuncName(ctx, fromAddr, selfDestructAddressActor, "sender()", []byte{})
	require.NoError(t, err)
	require.Equal(t, []byte{}, senderAfterDestroy)

	// deploy new contract at same address usign same salt
	newAddressSelfDestruct, _, err := client.EVM().InvokeContractByFuncName(ctx, fromAddr, idAddr, "deploy(bytes32)", salt)
	require.NoError(t, err)
	require.Equal(t, newAddressSelfDestruct, selfDestructAddress)

	//verify sender() property is correct
	senderSecondCall, _, err := client.EVM().InvokeContractByFuncName(ctx, fromAddr, selfDestructAddressActor, "sender()", []byte{})
	require.NoError(t, err)

	//assert contract has correct data
	require.Equal(t, ethFromAddr, senderSecondCall)

}

func TestFEVMBareTransferTriggersSmartContractLogic(t *testing.T) {
	ctx, cancel, client := kit.SetupFEVMTest(t)
	defer cancel()

	// This contract emits an event on receiving value.
	filename := "contracts/ValueSender.hex"
	_, contractAddr := client.EVM().DeployContractFromFilename(ctx, filename)

	accctKey, accntEth, accntFil := client.EVM().NewAccount()
	kit.SendFunds(ctx, t, client, accntFil, types.FromFil(10))

	contractEth, err := ethtypes.EthAddressFromFilecoinAddress(contractAddr)
	require.NoError(t, err)

	gaslimit, err := client.EthEstimateGas(ctx, ethtypes.EthCall{
		From:  &accntEth,
		To:    &contractEth,
		Value: ethtypes.EthBigInt(big.NewInt(100)),
	})
	require.NoError(t, err)

	maxPriorityFeePerGas, err := client.EthMaxPriorityFeePerGas(ctx)
	require.NoError(t, err)

	tx := ethtypes.EthTxArgs{
		ChainID:              build.Eip155ChainId,
		Value:                big.NewInt(100),
		Nonce:                0,
		To:                   &contractEth,
		MaxFeePerGas:         types.NanoFil,
		MaxPriorityFeePerGas: big.Int(maxPriorityFeePerGas),
		GasLimit:             int(gaslimit),
		V:                    big.Zero(),
		R:                    big.Zero(),
		S:                    big.Zero(),
	}

	client.EVM().SignTransaction(&tx, accctKey.PrivateKey)

	hash := client.EVM().SubmitTransaction(ctx, &tx)

	var receipt *api.EthTxReceipt
	for i := 0; i < 1000; i++ {
		receipt, err = client.EthGetTransactionReceipt(ctx, hash)
		require.NoError(t, err)
		if receipt != nil {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}

	// The receive() function emits one log, that's how we know we hit it.
	require.Len(t, receipt.Logs, 1)
}

// This test ensures that we can deploy new contracts from a solidity call to `transfer` without
// exceeding the 10M gas limit.
func TestFEVMTestDeployOnTransfer(t *testing.T) {
	ctx, cancel, client := kit.SetupFEVMTest(t)
	defer cancel()

	fromAddr := client.DefaultKey.Address
	t.Log("from - ", fromAddr)

	//create contract A
	filenameStorage := "contracts/ValueSender.hex"
	fromAddr, contractAddr := client.EVM().DeployContractFromFilename(ctx, filenameStorage)

	//send to some random address.
	params := [32]byte{}
	params[30] = 0xff
	randomAddr, err := ethtypes.CastEthAddress(params[12:])
	value := big.NewInt(100)
	entryPoint := kit.CalcFuncSignature("sendEthToB(address)")
	require.NoError(t, err)
	ret, err := client.EVM().InvokeSolidityWithValue(ctx, fromAddr, contractAddr, entryPoint, params[:], value)
	require.NoError(t, err)
	require.True(t, ret.Receipt.ExitCode.IsSuccess())

	balance, err := client.EVM().EthGetBalance(ctx, randomAddr, "latest")
	require.NoError(t, err)
	require.Equal(t, value.Int, balance.Int)

	filAddr, err := randomAddr.ToFilecoinAddress()
	require.NoError(t, err)
	client.AssertActorType(ctx, filAddr, manifest.PlaceholderKey)
}

func TestFEVMProxyUpgradeable(t *testing.T) {
	ctx, cancel, client := kit.SetupFEVMTest(t)
	defer cancel()

	//install transparently upgradeable proxy
	proxyFilename := "contracts/TransparentUpgradeableProxy.hex"
	fromAddr, contractAddr := client.EVM().DeployContractFromFilename(ctx, proxyFilename)

	_, _, err := client.EVM().InvokeContractByFuncName(ctx, fromAddr, contractAddr, "test()", []byte{})
	require.NoError(t, err)
}

func TestFEVMGetBlockDifficulty(t *testing.T) {
	ctx, cancel, client := kit.SetupFEVMTest(t)
	defer cancel()

	//install contract
	filenameActor := "contracts/GetDifficulty.hex"
	fromAddr, contractAddr := client.EVM().DeployContractFromFilename(ctx, filenameActor)

	ret, _, err := client.EVM().InvokeContractByFuncName(ctx, fromAddr, contractAddr, "getDifficulty()", []byte{})
	require.NoError(t, err)
	require.Equal(t, len(ret), 32)
}

func TestFEVMTestCorrectChainID(t *testing.T) {
	ctx, cancel, client := kit.SetupFEVMTest(t)
	defer cancel()

	//install contract
	filenameActor := "contracts/Blocktest.hex"
	fromAddr, contractAddr := client.EVM().DeployContractFromFilename(ctx, filenameActor)

	//run test
	_, _, err := client.EVM().InvokeContractByFuncName(ctx, fromAddr, contractAddr, "testChainID()", []byte{})
	require.NoError(t, err)
}

func TestFEVMGetChainPropertiesBlockTimestamp(t *testing.T) {
	ctx, cancel, client := kit.SetupFEVMTest(t)
	defer cancel()

	//install contract
	filenameActor := "contracts/Blocktest.hex"
	fromAddr, contractAddr := client.EVM().DeployContractFromFilename(ctx, filenameActor)

	// block number check
	ret, wait, err := client.EVM().InvokeContractByFuncName(ctx, fromAddr, contractAddr, "getTimestamp()", []byte{})
	require.NoError(t, err)

	timestampFromSolidity, err := decodeOutputToUint64(ret)
	require.NoError(t, err)

	ethBlock := client.EVM().GetEthBlockFromWait(ctx, wait)

	require.Equal(t, ethBlock.Timestamp, ethtypes.EthUint64(timestampFromSolidity))
}

func TestFEVMGetChainPropertiesBlockNumber(t *testing.T) {
	ctx, cancel, client := kit.SetupFEVMTest(t)
	defer cancel()

	//install contract
	filenameActor := "contracts/Blocktest.hex"
	fromAddr, contractAddr := client.EVM().DeployContractFromFilename(ctx, filenameActor)

	// block number check
	ret, wait, err := client.EVM().InvokeContractByFuncName(ctx, fromAddr, contractAddr, "getBlockNumber()", []byte{})
	require.NoError(t, err)

	blockHeightFromSolidity, err := decodeOutputToUint64(ret)
	require.NoError(t, err)

	ethBlock := client.EVM().GetEthBlockFromWait(ctx, wait)

	require.Equal(t, ethBlock.Number, ethtypes.EthUint64(blockHeightFromSolidity))
}

func TestFEVMGetChainPropertiesBlockHash(t *testing.T) {
	ctx, cancel, client := kit.SetupFEVMTest(t)
	defer cancel()

	//install contract
	filenameActor := "contracts/Blocktest.hex"
	fromAddr, contractAddr := client.EVM().DeployContractFromFilename(ctx, filenameActor)

	//block hash check
	ret, wait, err := client.EVM().InvokeContractByFuncName(ctx, fromAddr, contractAddr, "getBlockhashPrevious()", []byte{})
	expectedBlockHash := hex.EncodeToString(ret)
	require.NoError(t, err)

	ethBlock := client.EVM().GetEthBlockFromWait(ctx, wait)
	//in solidity we get the parent block hash because the current block hash doesnt exist at that execution context yet
	//so we compare the parent hash here in the test
	require.Equal(t, "0x"+expectedBlockHash, ethBlock.ParentHash.String())
}

func TestFEVMGetChainPropertiesBaseFee(t *testing.T) {
	ctx, cancel, client := kit.SetupFEVMTest(t)
	defer cancel()

	//install contract
	filenameActor := "contracts/Blocktest.hex"
	fromAddr, contractAddr := client.EVM().DeployContractFromFilename(ctx, filenameActor)

	ret, wait, err := client.EVM().InvokeContractByFuncName(ctx, fromAddr, contractAddr, "getBasefee()", []byte{})
	require.NoError(t, err)
	baseFeeRet, err := decodeOutputToUint64(ret)
	require.NoError(t, err)

	ethBlock := client.EVM().GetEthBlockFromWait(ctx, wait)

	require.Equal(t, ethBlock.BaseFeePerGas, ethtypes.EthBigInt(big.NewInt(int64(baseFeeRet))))
}

func TestFEVMErrorParsing(t *testing.T) {
	ctx, cancel, client := kit.SetupFEVMTest(t)
	defer cancel()

	e := client.EVM()

	_, contractAddr := e.DeployContractFromFilename(ctx, "contracts/Errors.hex")
	contractAddrEth, err := ethtypes.EthAddressFromFilecoinAddress(contractAddr)
	require.NoError(t, err)
	customError := ethtypes.EthBytes(kit.CalcFuncSignature("CustomError()")).String()
	for sig, expected := range map[string]string{
		"failRevertEmpty()":  "none",
		"failRevertReason()": "Error(my reason)",
		"failAssert()":       "Assert()",
		"failDivZero()":      "DivideByZero()",
		"failCustom()":       customError,
	} {
		sig := sig
		expected := fmt.Sprintf("exit 33, revert reason: %s, vm error", expected)
		t.Run(sig, func(t *testing.T) {
			entryPoint := kit.CalcFuncSignature(sig)
			t.Run("EthCall", func(t *testing.T) {
				_, err := e.EthCall(ctx, ethtypes.EthCall{
					To:   &contractAddrEth,
					Data: entryPoint,
				}, "latest")
				require.ErrorContains(t, err, expected)
			})
			t.Run("EthEstimateGas", func(t *testing.T) {
				_, err := e.EthEstimateGas(ctx, ethtypes.EthCall{
					To:   &contractAddrEth,
					Data: entryPoint,
				})
				require.ErrorContains(t, err, expected)
			})
		})
	}
}
