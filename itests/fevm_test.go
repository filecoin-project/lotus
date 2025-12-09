package itests

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-jsonrpc"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	builtintypes "github.com/filecoin-project/go-state-types/builtin"
	"github.com/filecoin-project/go-state-types/exitcode"
	"github.com/filecoin-project/go-state-types/manifest"
	"github.com/filecoin-project/go-state-types/network"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/consensus/filcns"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/store"
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
func buildInputFromUint64(number uint64) []byte {
	// Convert the number to a binary uint64 array
	binaryNumber := make([]byte, 8)
	binary.BigEndian.PutUint64(binaryNumber, number)
	return inputDataFromArray(binaryNumber)
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
			_, _, err := client.EVM().InvokeContractByFuncName(ctx, fromAddr, idAddr, "recursiveCall(uint256)", buildInputFromUint64(callCount))
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
			_, wait, err := client.EVM().InvokeContractByFuncName(ctx, fromAddr, idAddr, "recursiveCall(uint256)", buildInputFromUint64(failCallCount))
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

// TestFEVMRecursiveDelegatecallCount tests the maximum delegatecall recursion depth.
func TestFEVMRecursiveDelegatecallCount(t *testing.T) {
	ctx, cancel, client := kit.SetupFEVMTest(t)
	defer cancel()

	// these depend on the actors bundle, may need to be adjusted with a network upgrade
	const highestSuccessCount = 228
	const expectedIterationsBeforeFailing = 222

	filename := "contracts/RecursiveDelegeatecall.hex"

	testCases := []struct {
		recursionCount uint64
		expectSuccess  bool
	}{
		// success
		{1, true},
		{2, true},
		{10, true},
		{100, true},
		{highestSuccessCount, true},
		// failure
		{highestSuccessCount + 1, false},
		{1000, false},
		{10000000, false},
	}
	for _, tc := range testCases {
		t.Run(fmt.Sprintf("recursionCount=%d,expectSuccess=%t", tc.recursionCount, tc.expectSuccess), func(t *testing.T) {
			fromAddr, idAddr := client.EVM().DeployContractFromFilename(ctx, filename)
			inputData := buildInputFromUint64(tc.recursionCount)
			_, _, err := client.EVM().InvokeContractByFuncName(ctx, fromAddr, idAddr, "recursiveCall(uint256)", inputData)
			require.NoError(t, err)

			result, _, err := client.EVM().InvokeContractByFuncName(ctx, fromAddr, idAddr, "totalCalls()", []byte{})
			require.NoError(t, err)

			resultUint, err := decodeOutputToUint64(result)
			require.NoError(t, err)

			if tc.expectSuccess {
				require.Equal(t, int(tc.recursionCount), int(resultUint))
			} else {
				require.NotEqual(t, int(resultUint), int(tc.recursionCount), "unexpected recursion count, if the actors bundle has changed, this test may need to be adjusted")
				require.Equal(t, int(expectedIterationsBeforeFailing), int(resultUint))
			}
		})
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
	require.Equal(t, *act.DelegatedAddress, eth0Addr)
}

// TestFEVMDelegateCall deploys two contracts and makes a delegate call transaction
func TestFEVMDelegateCall(t *testing.T) {

	ctx, cancel, client := kit.SetupFEVMTest(t)
	defer cancel()

	// install contract Actor
	filenameActor := "contracts/DelegatecallActor.hex"
	fromAddr, actorAddr := client.EVM().DeployContractFromFilename(ctx, filenameActor)
	// install contract Storage
	filenameStorage := "contracts/DelegatecallStorage.hex"
	fromAddrStorage, storageAddr := client.EVM().DeployContractFromFilename(ctx, filenameStorage)
	require.Equal(t, fromAddr, fromAddrStorage)

	// call Contract Storage which makes a delegatecall to contract Actor
	// this contract call sets the "counter" variable to 7, from default value 0
	inputDataContract := inputDataFromFrom(ctx, t, client, actorAddr)
	inputDataValue := inputDataFromArray([]byte{7})
	inputData := append(inputDataContract, inputDataValue...)

	// verify that the returned value of the call to setvars is 7
	result, _, err := client.EVM().InvokeContractByFuncName(ctx, fromAddr, storageAddr, "setVars(address,uint256)", inputData)
	require.NoError(t, err)
	expectedResult, err := hex.DecodeString("0000000000000000000000000000000000000000000000000000000000000007")
	require.NoError(t, err)
	require.Equal(t, result, expectedResult)

	// test the value is 7 a second way by calling the getter
	result, _, err = client.EVM().InvokeContractByFuncName(ctx, fromAddr, storageAddr, "getCounter()", []byte{})
	require.NoError(t, err)
	require.Equal(t, result, expectedResult)

	// test the value is 0 via calling the getter on the Actor contract
	result, _, err = client.EVM().InvokeContractByFuncName(ctx, fromAddr, actorAddr, "getCounter()", []byte{})
	require.NoError(t, err)
	expectedResultActor, err := hex.DecodeString("0000000000000000000000000000000000000000000000000000000000000000")
	require.NoError(t, err)
	require.Equal(t, result, expectedResultActor)

	// The implementation's storage should not have been updated.
	actorAddrEth, err := ethtypes.EthAddressFromFilecoinAddress(actorAddr)
	require.NoError(t, err)
	value, err := client.EVM().EthGetStorageAt(ctx, actorAddrEth, nil, ethtypes.NewEthBlockNumberOrHashFromPredefined("latest"))
	require.NoError(t, err)
	require.Equal(t, ethtypes.EthBytes(make([]byte, 32)), value)

	// The storage actor's storage _should_ have been updated
	storageAddrEth, err := ethtypes.EthAddressFromFilecoinAddress(storageAddr)
	require.NoError(t, err)
	value, err = client.EVM().EthGetStorageAt(ctx, storageAddrEth, nil, ethtypes.NewEthBlockNumberOrHashFromPredefined("latest"))
	require.NoError(t, err)
	require.Equal(t, ethtypes.EthBytes(expectedResult), value)
}

// TestFEVMDelegateCallRevert makes a delegatecall action and then calls revert.
// the state should not have changed because of the revert
func TestFEVMDelegateCallRevert(t *testing.T) {
	ctx, cancel, client := kit.SetupFEVMTest(t)
	defer cancel()

	// install contract Actor
	filenameActor := "contracts/DelegatecallActor.hex"
	fromAddr, actorAddr := client.EVM().DeployContractFromFilename(ctx, filenameActor)
	// install contract Storage
	filenameStorage := "contracts/DelegatecallStorage.hex"
	fromAddrStorage, storageAddr := client.EVM().DeployContractFromFilename(ctx, filenameStorage)
	require.Equal(t, fromAddr, fromAddrStorage)

	// call Contract Storage which makes a delegatecall to contract Actor
	// this contract call sets the "counter" variable to 7, from default value 0

	inputDataContract := inputDataFromFrom(ctx, t, client, actorAddr)
	inputDataValue := inputDataFromArray([]byte{7})
	inputData := append(inputDataContract, inputDataValue...)

	// verify that the returned value of the call to setvars is 7
	_, wait, err := client.EVM().InvokeContractByFuncName(ctx, fromAddr, storageAddr, "setVarsRevert(address,uint256)", inputData)
	require.Error(t, err)
	require.Equal(t, exitcode.ExitCode(33), wait.Receipt.ExitCode)

	// test the value is 0 via calling the getter and was not set to 7
	expectedResult, err := hex.DecodeString("0000000000000000000000000000000000000000000000000000000000000000")
	require.NoError(t, err)
	result, _, err := client.EVM().InvokeContractByFuncName(ctx, fromAddr, storageAddr, "getCounter()", []byte{})
	require.NoError(t, err)
	require.Equal(t, result, expectedResult)

	// test the value is 0 via calling the getter on the Actor contract
	result, _, err = client.EVM().InvokeContractByFuncName(ctx, fromAddr, actorAddr, "getCounter()", []byte{})
	require.NoError(t, err)
	require.Equal(t, result, expectedResult)
}

// TestFEVMSimpleRevert makes a call that is a simple revert
func TestFEVMSimpleRevert(t *testing.T) {
	ctx, cancel, client := kit.SetupFEVMTest(t)
	defer cancel()

	// install contract Actor
	filenameStorage := "contracts/DelegatecallStorage.hex"
	fromAddr, contractAddr := client.EVM().DeployContractFromFilename(ctx, filenameStorage)

	// call revert
	_, wait, err := client.EVM().InvokeContractByFuncName(ctx, fromAddr, contractAddr, "revert()", []byte{})

	require.Equal(t, wait.Receipt.ExitCode, exitcode.ExitCode(33))
	require.Error(t, err)
}

// TestFEVMSelfDestruct creates a contract that just has a self destruct feature and calls it
func TestFEVMSelfDestruct(t *testing.T) {
	ctx, cancel, client := kit.SetupFEVMTest(t)
	defer cancel()

	// install contract Actor
	filenameStorage := "contracts/SelfDestruct.hex"
	fromAddr, contractAddr := client.EVM().DeployContractFromFilename(ctx, filenameStorage)

	// call destroy
	_, _, err := client.EVM().InvokeContractByFuncName(ctx, fromAddr, contractAddr, "destroy()", []byte{})
	require.NoError(t, err)

	// call destroy a second time and also no error
	_, _, err = client.EVM().InvokeContractByFuncName(ctx, fromAddr, contractAddr, "destroy()", []byte{})
	require.NoError(t, err)
}

// TestFEVMTestApp deploys a fairly complex app contract and confirms it works as expected
func TestFEVMTestApp(t *testing.T) {
	ctx, cancel, client := kit.SetupFEVMTest(t)
	defer cancel()

	// install contract Actor
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

// TestFEVMTestConstructor creates a contract that just has a self destruct feature and calls it
func TestFEVMTestConstructor(t *testing.T) {
	ctx, cancel, client := kit.SetupFEVMTest(t)
	defer cancel()

	// install contract Actor
	filenameStorage := "contracts/Constructor.hex"
	fromAddr, contractAddr := client.EVM().DeployContractFromFilename(ctx, filenameStorage)

	// input = uint256{7}. set value and confirm tx success
	inputData, err := hex.DecodeString("0000000000000000000000000000000000000000000000000000000000000007")
	require.NoError(t, err)
	_, _, err = client.EVM().InvokeContractByFuncName(ctx, fromAddr, contractAddr, "new_Test(uint256)", inputData)
	require.NoError(t, err)

}

// TestFEVMAutoSelfDestruct creates a contract that just has a self destruct feature and calls it
func TestFEVMAutoSelfDestruct(t *testing.T) {
	ctx, cancel, client := kit.SetupFEVMTest(t)
	defer cancel()

	// install contract Actor
	filenameStorage := "contracts/AutoSelfDestruct.hex"
	fromAddr, contractAddr := client.EVM().DeployContractFromFilename(ctx, filenameStorage)

	// call destroy
	_, _, err := client.EVM().InvokeContractByFuncName(ctx, fromAddr, contractAddr, "destroy()", []byte{})
	require.NoError(t, err)
}

// TestFEVMTestSendToContract creates a contract that just has a self destruct feature and calls it
func TestFEVMTestSendToContract(t *testing.T) {
	ctx, cancel, client := kit.SetupFEVMTest(t)
	defer cancel()

	bal, err := client.WalletBalance(ctx, client.DefaultKey.Address)
	require.NoError(t, err)

	// install contract TestApp
	filenameStorage := "contracts/SelfDestruct.hex"
	fromAddr, contractAddr := client.EVM().DeployContractFromFilename(ctx, filenameStorage)

	// transfer half balance to contract

	sendAmount := big.Div(bal, big.NewInt(2))
	client.EVM().TransferValueOrFail(ctx, fromAddr, contractAddr, sendAmount)

	// call self destruct which should return balance
	_, _, err = client.EVM().InvokeContractByFuncName(ctx, fromAddr, contractAddr, "destroy()", []byte{})
	require.NoError(t, err)

	finalBalanceMinimum := types.FromFil(uint64(9_999_999)) // 10 million FIL - 1 FIL for gas upper bounds
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

	// create contract A
	filenameStorage := "contracts/NotPayable.hex"
	fromAddr, contractAddr := client.EVM().DeployContractFromFilename(ctx, filenameStorage)
	sendAmount := big.NewInt(10_000_000)

	client.EVM().TransferValueOrFail(ctx, fromAddr, contractAddr, sendAmount)

}

// tx to non function succeeds
func TestFEVMSendCall(t *testing.T) {
	ctx, cancel, client := kit.SetupFEVMTest(t)
	defer cancel()

	// install contract
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

	// install contract
	filenameActor := "contracts/GasLimitSend.hex"
	fromAddr, contractAddr := client.EVM().DeployContractFromFilename(ctx, filenameActor)

	// send $ to contract
	// transfer 1 attoFIL to contract
	sendAmount := big.MustFromString("1")

	client.EVM().TransferValueOrFail(ctx, fromAddr, contractAddr, sendAmount)
	_, _, err := client.EVM().InvokeContractByFuncName(ctx, fromAddr, contractAddr, "getDataLength()", []byte{})
	require.NoError(t, err)

}

// TestFEVMDelegateCallRecursiveFail deploys the two contracts in TestFEVMDelegateCall but instead of A calling B, A calls A which should cause A to cause A in an infinite loop and should give a reasonable error
func TestFEVMDelegateCallRecursiveFail(t *testing.T) {
	// TODO change the gas limit of this invocation and confirm that the number of errors is
	// different
	ctx, cancel, client := kit.SetupFEVMTest(t)
	defer cancel()

	// install contract Actor
	filenameActor := "contracts/DelegatecallStorage.hex"
	fromAddr, actorAddr := client.EVM().DeployContractFromFilename(ctx, filenameActor)

	// any data will do for this test that fails
	inputDataContract := inputDataFromFrom(ctx, t, client, actorAddr)
	inputDataValue := inputDataFromArray([]byte{7})
	inputData := append(inputDataContract, inputDataValue...)

	// verify that we run out of gas then revert.
	_, wait, err := client.EVM().InvokeContractByFuncName(ctx, fromAddr, actorAddr, "setVarsSelf(address,uint256)", inputData)
	require.Error(t, err)
	require.Equal(t, exitcode.ExitCode(33), wait.Receipt.ExitCode)

	// assert no fatal errors but still there are errors::
	errorAny := "fatal error"
	require.NotContains(t, err.Error(), errorAny)
}

// TestFEVMTestSendValueThroughContractsAndDestroy creates A and B contract and exchanges value
// and self destructs and accounts for value sent
func TestFEVMTestSendValueThroughContractsAndDestroy(t *testing.T) {

	ctx, cancel, client := kit.SetupFEVMTest(t)
	defer cancel()

	fromAddr := client.DefaultKey.Address
	t.Log("from - ", fromAddr)

	// create contract A
	filenameStorage := "contracts/ValueSender.hex"
	fromAddr, contractAddr := client.EVM().DeployContractFromFilename(ctx, filenameStorage)

	// create contract B
	ret, _, err := client.EVM().InvokeContractByFuncName(ctx, fromAddr, contractAddr, "createB()", []byte{})
	require.NoError(t, err)

	ethAddr, err := ethtypes.CastEthAddress(ret[12:])
	require.NoError(t, err)
	contractBAddress, err := ethAddr.ToFilecoinAddress()
	require.NoError(t, err)
	t.Log("contractBAddress - ", contractBAddress)

	// self destruct contract B
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

	// install contract Actor
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

	// install contract Actor
	filenameActor := "contracts/RecCall.hex"
	fromAddr, actorAddr := client.EVM().DeployContractFromFilename(ctx, filenameActor)

	exitCodeStackOverflow := exitcode.ExitCode(37)
	exitCodeTransactionReverted := exitcode.ExitCode(33)

	testCases := []struct {
		stackDepth     int
		recursionLimit int
		exitCode       exitcode.ExitCode
	}{
		{0, 1, exitcode.Ok},
		{1, 1, exitcode.Ok},
		{20, 1, exitcode.Ok},
		{200, 1, exitcode.Ok},
		{251, 1, exitcode.Ok},
		{252, 1, exitCodeStackOverflow},
		{0, 10, exitcode.Ok},
		{1, 10, exitcode.Ok},
		{20, 10, exitcode.Ok},
		{200, 10, exitcode.Ok},
		{251, 10, exitcode.Ok},
		{252, 10, exitCodeStackOverflow},
		{0, 32, exitcode.Ok},
		{1, 32, exitcode.Ok},
		{20, 32, exitcode.Ok},
		{200, 32, exitcode.Ok},
		{251, 32, exitcode.Ok},
		{252, 32, exitCodeStackOverflow},
		// the following are actors bundle dependent and may need to be tweaked with a network upgrade
		{0, 255, exitcode.Ok},
		{251, 164, exitcode.Ok},
		{0, 261, exitCodeTransactionReverted},
		{251, 173, exitCodeTransactionReverted},
	}
	for _, tc := range testCases {
		var fail string
		if tc.exitCode != exitcode.Ok {
			fail = "-fails"
		}
		t.Run(fmt.Sprintf("stackDepth=%d,recursionLimit=%d%s", tc.stackDepth, tc.recursionLimit, fail), func(t *testing.T) {
			inputData := make([]byte, 32*3)
			binary.BigEndian.PutUint64(inputData[24:], uint64(tc.stackDepth))
			binary.BigEndian.PutUint64(inputData[32+24:], uint64(tc.stackDepth))
			binary.BigEndian.PutUint64(inputData[32+32+24:], uint64(tc.recursionLimit))

			client.EVM().InvokeContractByFuncNameExpectExit(ctx, fromAddr, actorAddr, "exec1(uint256,uint256,uint256)", inputData, tc.exitCode)
		})
	}
}

// TestFEVMRecursiveActorCallEstimate
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

// TestFEVMDeployWithValue deploys a contract while sending value to it
func TestFEVMDeployWithValue(t *testing.T) {
	ctx, cancel, client := kit.SetupFEVMTest(t)
	defer cancel()

	// testValue is the amount sent when the contract is created
	// at the end we check that the new contract has a balance of testValue
	testValue := big.NewInt(20)

	// deploy DeployValueTest which creates NewContract
	// testValue is sent to DeployValueTest and that amount is
	// also sent to NewContract
	filenameActor := "contracts/DeployValueTest.hex"
	fromAddr, idAddr := client.EVM().DeployContractFromFilenameWithValue(ctx, filenameActor, testValue)

	// call getNewContractBalance to find the value of NewContract
	ret, _, err := client.EVM().InvokeContractByFuncName(ctx, fromAddr, idAddr, "getNewContractBalance()", []byte{})
	require.NoError(t, err)

	contractBalance, err := decodeOutputToUint64(ret)
	require.NoError(t, err)

	// require balance of NewContract is testValue
	require.Equal(t, testValue.Uint64(), contractBalance)
}

func TestFEVMDestroyCreate2(t *testing.T) {
	ctx, cancel, client := kit.SetupFEVMTest(t)
	defer cancel()

	// deploy create2 factory contract
	filename := "contracts/Create2Factory.hex"
	fromAddr, idAddr := client.EVM().DeployContractFromFilename(ctx, filename)

	// construct salt for create2
	salt := make([]byte, 32)
	_, err := rand.Read(salt)
	require.NoError(t, err)

	// deploy contract using create2 factory
	selfDestructAddress, _, err := client.EVM().InvokeContractByFuncName(ctx, fromAddr, idAddr, "deploy(bytes32)", salt)
	require.NoError(t, err)

	// convert to filecoin actor address so we can call InvokeContractByFuncName
	ea, err := ethtypes.CastEthAddress(selfDestructAddress[12:])
	require.NoError(t, err)
	selfDestructAddressActor, err := ea.ToFilecoinAddress()
	require.NoError(t, err)

	// read sender property from contract
	ret, _, err := client.EVM().InvokeContractByFuncName(ctx, fromAddr, selfDestructAddressActor, "sender()", []byte{})
	require.NoError(t, err)

	// assert contract has correct data
	ethFromAddr := inputDataFromFrom(ctx, t, client, fromAddr)
	require.Equal(t, ethFromAddr, ret)

	// run test() which 1.calls sefldestruct 2. verifies sender() is the correct value 3. attempts and fails to deploy via create2
	testSenderAddress, _, err := client.EVM().InvokeContractByFuncName(ctx, fromAddr, idAddr, "test(address)", selfDestructAddress)
	require.NoError(t, err)
	require.Equal(t, testSenderAddress, ethFromAddr)

	// read sender() but get response of 0x0 because of self destruct
	senderAfterDestroy, _, err := client.EVM().InvokeContractByFuncName(ctx, fromAddr, selfDestructAddressActor, "sender()", []byte{})
	require.NoError(t, err)
	require.Equal(t, []byte{}, senderAfterDestroy)

	// deploy new contract at same address using same salt
	newAddressSelfDestruct, _, err := client.EVM().InvokeContractByFuncName(ctx, fromAddr, idAddr, "deploy(bytes32)", salt)
	require.NoError(t, err)
	require.Equal(t, newAddressSelfDestruct, selfDestructAddress)

	// verify sender() property is correct
	senderSecondCall, _, err := client.EVM().InvokeContractByFuncName(ctx, fromAddr, selfDestructAddressActor, "sender()", []byte{})
	require.NoError(t, err)

	// assert contract has correct data
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

	gasParams, err := json.Marshal(ethtypes.EthEstimateGasParams{Tx: ethtypes.EthCall{
		From:  &accntEth,
		To:    &contractEth,
		Value: ethtypes.EthBigInt(big.NewInt(100)),
	}})
	require.NoError(t, err)

	gaslimit, err := client.EthEstimateGas(ctx, gasParams)
	require.NoError(t, err)

	maxPriorityFeePerGas, err := client.EthMaxPriorityFeePerGas(ctx)
	require.NoError(t, err)

	tx := ethtypes.Eth1559TxArgs{
		ChainID:              buildconstants.Eip155ChainId,
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

	var receipt *ethtypes.EthTxReceipt
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

	// create contract A
	filenameStorage := "contracts/ValueSender.hex"
	fromAddr, contractAddr := client.EVM().DeployContractFromFilename(ctx, filenameStorage)

	// send to some random address.
	params := [32]byte{}
	params[30] = 0xff
	randomAddr, err := ethtypes.CastEthAddress(params[12:])
	value := big.NewInt(100)
	entryPoint := kit.CalcFuncSignature("sendEthToB(address)")
	require.NoError(t, err)
	ret, err := client.EVM().InvokeSolidityWithValue(ctx, fromAddr, contractAddr, entryPoint, params[:], value)
	require.NoError(t, err)
	require.True(t, ret.Receipt.ExitCode.IsSuccess())

	balance, err := client.EVM().EthGetBalance(ctx, randomAddr, ethtypes.NewEthBlockNumberOrHashFromPredefined("latest"))
	require.NoError(t, err)
	require.Equal(t, value.Int, balance.Int)

	filAddr, err := randomAddr.ToFilecoinAddress()
	require.NoError(t, err)
	client.AssertActorType(ctx, filAddr, manifest.PlaceholderKey)
}

func TestFEVMProxyUpgradeable(t *testing.T) {
	ctx, cancel, client := kit.SetupFEVMTest(t)
	defer cancel()

	// install transparently upgradeable proxy
	proxyFilename := "contracts/TransparentUpgradeableProxy.hex"
	fromAddr, contractAddr := client.EVM().DeployContractFromFilename(ctx, proxyFilename)

	_, _, err := client.EVM().InvokeContractByFuncName(ctx, fromAddr, contractAddr, "test()", []byte{})
	require.NoError(t, err)
}

func TestFEVMGetBlockDifficulty(t *testing.T) {
	ctx, cancel, client := kit.SetupFEVMTest(t)
	defer cancel()

	// install contract
	filenameActor := "contracts/GetDifficulty.hex"
	fromAddr, contractAddr := client.EVM().DeployContractFromFilename(ctx, filenameActor)

	ret, _, err := client.EVM().InvokeContractByFuncName(ctx, fromAddr, contractAddr, "getDifficulty()", []byte{})
	require.NoError(t, err)
	require.Equal(t, len(ret), 32)
}

func TestFEVMTestCorrectChainID(t *testing.T) {
	ctx, cancel, client := kit.SetupFEVMTest(t)
	defer cancel()

	// install contract
	filenameActor := "contracts/Blocktest.hex"
	fromAddr, contractAddr := client.EVM().DeployContractFromFilename(ctx, filenameActor)

	// run test
	_, _, err := client.EVM().InvokeContractByFuncName(ctx, fromAddr, contractAddr, "testChainID()", []byte{})
	require.NoError(t, err)
}

func TestFEVMGetChainPropertiesBlockTimestamp(t *testing.T) {
	ctx, cancel, client := kit.SetupFEVMTest(t)
	defer cancel()

	// install contract
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

	// install contract
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

	// install contract
	filenameActor := "contracts/Blocktest.hex"
	fromAddr, contractAddr := client.EVM().DeployContractFromFilename(ctx, filenameActor)

	// block hash check
	ret, wait, err := client.EVM().InvokeContractByFuncName(ctx, fromAddr, contractAddr, "getBlockhashPrevious()", []byte{})
	expectedBlockHash := hex.EncodeToString(ret)
	require.NoError(t, err)

	ethBlock := client.EVM().GetEthBlockFromWait(ctx, wait)
	// in solidity we get the parent block hash because the current block hash doesn't exist at that execution context yet
	// so we compare the parent hash here in the test
	require.Equal(t, "0x"+expectedBlockHash, ethBlock.ParentHash.String())
}

func TestFEVMGetChainPropertiesBaseFee(t *testing.T) {
	ctx, cancel, client := kit.SetupFEVMTest(t)
	defer cancel()

	// install contract
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
		"failRevertEmpty()":  "0x",
		"failRevertReason()": fmt.Sprintf("%x", []byte("my reason")),
		"failAssert()":       "0x4e487b710000000000000000000000000000000000000000000000000000000000000001", // Assert()
		"failDivZero()":      "0x4e487b710000000000000000000000000000000000000000000000000000000000000012", // DivideByZero()
		"failCustom()":       customError,
	} {
		sig := sig
		expected := expected
		t.Run(sig, func(t *testing.T) {
			entryPoint := kit.CalcFuncSignature(sig)
			t.Run("EthCall", func(t *testing.T) {
				_, err = e.EthCall(ctx, ethtypes.EthCall{
					To:   &contractAddrEth,
					Data: entryPoint,
				}, ethtypes.NewEthBlockNumberOrHashFromPredefined("latest"))
				require.Error(t, err)

				var dataErr *api.ErrExecutionReverted
				require.ErrorAs(t, err, &dataErr, "Expected error to be ErrExecutionReverted")
				require.Contains(t, dataErr.Data, expected, "Error data should contain the expected error")
			})
			t.Run("EthEstimateGas", func(t *testing.T) {
				gasParams, err := json.Marshal(ethtypes.EthEstimateGasParams{Tx: ethtypes.EthCall{
					To:   &contractAddrEth,
					Data: entryPoint,
				}})
				require.NoError(t, err)

				_, err = e.EthEstimateGas(ctx, gasParams)
				require.Error(t, err)
				var dataErr *api.ErrExecutionReverted
				require.ErrorAs(t, err, &dataErr, "Expected error to be ErrExecutionReverted")
				require.Contains(t, dataErr.Data, expected, "Error data should contain the expected error")
			})
		})
	}
}

// TestEthGetBlockReceipts tests retrieving block receipts after invoking a contract
func TestEthGetBlockReceipts(t *testing.T) {
	blockTime := 500 * time.Millisecond
	client, _, ens := kit.EnsembleMinimal(t, kit.MockProofs(), kit.ThroughRPC())

	ens.InterconnectAll().BeginMining(blockTime)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	// Create a new Ethereum account
	key, ethAddr, deployer := client.EVM().NewAccount()
	// Send some funds to the f410 address
	kit.SendFunds(ctx, t, client, deployer, types.FromFil(10))

	// Deploy MultipleEvents contract
	tx := deployContractWithEth(ctx, t, client, ethAddr, "./contracts/MultipleEvents.hex")

	client.EVM().SignTransaction(tx, key.PrivateKey)
	hash := client.EVM().SubmitTransaction(ctx, tx)

	receipt, err := client.EVM().WaitTransaction(ctx, hash)
	require.NoError(t, err)
	require.NotNil(t, receipt)
	require.EqualValues(t, ethtypes.EthUint64(0x1), receipt.Status)

	contractAddr := client.EVM().ComputeContractAddress(ethAddr, 0)

	// Prepare function call data
	params4Events, err := hex.DecodeString("98e8da00")
	require.NoError(t, err)
	params3Events, err := hex.DecodeString("05734db70000000000000000000000001cd1eeecac00fe01f9d11803e48c15c478fe1d22000000000000000000000000000000000000000000000000000000000000000b")
	require.NoError(t, err)

	// Estimate gas
	gasParams, err := json.Marshal(ethtypes.EthEstimateGasParams{Tx: ethtypes.EthCall{
		From: &ethAddr,
		To:   &contractAddr,
		Data: params4Events,
	}})
	require.NoError(t, err)

	gaslimit, err := client.EthEstimateGas(ctx, gasParams)
	require.NoError(t, err)

	maxPriorityFeePerGas, err := client.EthMaxPriorityFeePerGas(ctx)
	require.NoError(t, err)

	paramsArray := [][]byte{params4Events, params3Events, params3Events}

	var hashes []ethtypes.EthHash

	nonce := 1
	for _, params := range paramsArray {
		invokeTx := ethtypes.Eth1559TxArgs{
			ChainID:              build.Eip155ChainId,
			To:                   &contractAddr,
			Value:                big.Zero(),
			Nonce:                nonce,
			MaxFeePerGas:         types.NanoFil,
			MaxPriorityFeePerGas: big.Int(maxPriorityFeePerGas),
			GasLimit:             int(gaslimit),
			Input:                params,
			V:                    big.Zero(),
			R:                    big.Zero(),
			S:                    big.Zero(),
		}

		client.EVM().SignTransaction(&invokeTx, key.PrivateKey)
		hash := client.EVM().SubmitTransaction(ctx, &invokeTx)
		hashes = append(hashes, hash)
		nonce++
	}

	// Wait for the transactions to be mined
	var lastReceipt *ethtypes.EthTxReceipt
	for _, hash := range hashes {
		receipt, err := client.EVM().WaitTransaction(ctx, hash)
		require.NoError(t, err)
		require.NotNil(t, receipt)
		lastReceipt = receipt
	}

	// Get block receipts
	blockReceipts, err := client.EthGetBlockReceipts(ctx, ethtypes.EthBlockNumberOrHash{BlockHash: &lastReceipt.BlockHash})
	require.NoError(t, err)
	require.NotEmpty(t, blockReceipts)

	// Verify receipts
	require.Len(t, blockReceipts, 3) // Ensure there are three receipts

	expectedLogCounts := []int{4, 3, 3}

	for i, receipt := range blockReceipts {
		require.Equal(t, &contractAddr, receipt.To)
		require.Equal(t, ethtypes.EthUint64(1), receipt.Status)
		require.NotEmpty(t, receipt.BlockHash, "Block hash should not be empty")
		require.Equal(t, expectedLogCounts[i], len(receipt.Logs), fmt.Sprintf("Transaction %d should have %d event logs", i+1, expectedLogCounts[i]))
		if i > 0 {
			require.Equal(t, blockReceipts[i-1].BlockHash, receipt.BlockHash, "All receipts should have the same block hash")
		}

		txReceipt, err := client.EthGetTransactionReceipt(ctx, receipt.TransactionHash)
		require.NoError(t, err)
		require.Equal(t, txReceipt, receipt)
	}

	// try with the geth request format for `EthBlockNumberOrHash`
	var req ethtypes.EthBlockNumberOrHash
	reqStr := fmt.Sprintf(`"%s"`, lastReceipt.BlockHash.String())
	err = req.UnmarshalJSON([]byte(reqStr))
	require.NoError(t, err)

	gethBlockReceipts, err := client.EthGetBlockReceipts(ctx, req)
	require.NoError(t, err)
	require.Len(t, gethBlockReceipts, 3)

	t.Run("EthGetBlockReceiptsLimited", func(t *testing.T) {
		// just to be sure we're far enough in the chain for the limit to work
		client.WaitTillChain(ctx, kit.HeightAtLeast(10))
		// request epoch 2
		bn := ethtypes.EthUint64(2)
		// limit to 5 epochs lookback
		blockReceipts, err := client.EthGetBlockReceiptsLimited(ctx, ethtypes.EthBlockNumberOrHash{BlockNumber: &bn}, 5)
		require.ErrorContains(t, err, "older than the allowed")
		require.Nil(t, blockReceipts, "should not return any receipts")
	})
}

func deployContractWithEth(ctx context.Context, t *testing.T, client *kit.TestFullNode, ethAddr ethtypes.EthAddress,
	contractPath string) *ethtypes.Eth1559TxArgs {
	// install contract
	contractHex, err := os.ReadFile(contractPath)
	require.NoError(t, err)

	contract, err := hex.DecodeString(string(contractHex))
	require.NoError(t, err)

	gasParams, err := json.Marshal(ethtypes.EthEstimateGasParams{Tx: ethtypes.EthCall{
		From: &ethAddr,
		Data: contract,
	}})
	require.NoError(t, err)

	gaslimit, err := client.EthEstimateGas(ctx, gasParams)
	require.NoError(t, err)

	maxPriorityFeePerGas, err := client.EthMaxPriorityFeePerGas(ctx)
	require.NoError(t, err)

	// now deploy a contract from the embryo, and validate it went well
	return &ethtypes.Eth1559TxArgs{
		ChainID:              build.Eip155ChainId,
		Value:                big.Zero(),
		Nonce:                0,
		MaxFeePerGas:         types.NanoFil,
		MaxPriorityFeePerGas: big.Int(maxPriorityFeePerGas),
		GasLimit:             int(gaslimit),
		Input:                contract,
		V:                    big.Zero(),
		R:                    big.Zero(),
		S:                    big.Zero(),
	}
}

func TestEthGetTransactionCount(t *testing.T) {
	ctx, cancel, client := kit.SetupFEVMTest(t)
	defer cancel()

	// Create a new Ethereum account
	key, ethAddr, filAddr := client.EVM().NewAccount()

	// Test initial state (should be zero)
	initialCount, err := client.EVM().EthGetTransactionCount(ctx, ethAddr, ethtypes.NewEthBlockNumberOrHashFromPredefined("latest"))
	require.NoError(t, err)
	require.Zero(t, initialCount)

	// Send some funds to the new account (this shouldn't change the nonce)
	kit.SendFunds(ctx, t, client, filAddr, types.FromFil(10))

	// Check nonce again (should still be zero)
	count, err := client.EVM().EthGetTransactionCount(ctx, ethAddr, ethtypes.NewEthBlockNumberOrHashFromPredefined("latest"))
	require.NoError(t, err)
	require.Zero(t, count)

	// Prepare and send multiple transactions
	numTx := 5
	var lastHash ethtypes.EthHash

	contractHex, err := os.ReadFile("./contracts/SelfDestruct.hex")
	require.NoError(t, err)
	contract, err := hex.DecodeString(string(contractHex))
	require.NoError(t, err)

	gasParams, err := json.Marshal(ethtypes.EthEstimateGasParams{Tx: ethtypes.EthCall{
		From: &ethAddr,
		Data: contract,
	}})
	require.NoError(t, err)
	gaslimit, err := client.EthEstimateGas(ctx, gasParams)
	require.NoError(t, err)
	for i := 0; i < numTx; i++ {
		tx := &ethtypes.Eth1559TxArgs{
			ChainID:              buildconstants.Eip155ChainId,
			To:                   &ethAddr, // sending to self
			Value:                big.NewInt(1000),
			Nonce:                i,
			MaxFeePerGas:         types.NanoFil,
			MaxPriorityFeePerGas: types.NanoFil,
			GasLimit:             int(gaslimit),
		}
		client.EVM().SignTransaction(tx, key.PrivateKey)
		lastHash = client.EVM().SubmitTransaction(ctx, tx)

		// Check counts for "earliest", "latest", and "pending"
		_, err = client.EVM().EthGetTransactionCount(ctx, ethAddr, ethtypes.NewEthBlockNumberOrHashFromPredefined("earliest"))
		require.Error(t, err) // earliest is not supported

		latestCount, err := client.EVM().EthGetTransactionCount(ctx, ethAddr, ethtypes.NewEthBlockNumberOrHashFromPredefined("latest"))
		require.NoError(t, err)
		require.Equal(t, ethtypes.EthUint64(i), latestCount, "Latest transaction count should be equal to the number of mined transactions")

		pendingCount, err := client.EVM().EthGetTransactionCount(ctx, ethAddr, ethtypes.NewEthBlockNumberOrHashFromPredefined("pending"))
		require.NoError(t, err)
		require.True(t, int(pendingCount) == i || int(pendingCount) == i+1,
			fmt.Sprintf("Pending transaction count should be either %d or %d, but got %d", i, i+1, pendingCount))

		// Wait for the transaction to be mined
		_, err = client.EVM().WaitTransaction(ctx, lastHash)
		require.NoError(t, err)
	}

	// Get the final counts for "earliest", "latest", and "pending"
	_, err = client.EVM().EthGetTransactionCount(ctx, ethAddr, ethtypes.NewEthBlockNumberOrHashFromPredefined("earliest"))
	require.Error(t, err) // earliest is not supported

	finalLatestCount, err := client.EVM().EthGetTransactionCount(ctx, ethAddr, ethtypes.NewEthBlockNumberOrHashFromPredefined("latest"))
	require.NoError(t, err)
	require.Equal(t, ethtypes.EthUint64(numTx), finalLatestCount, "Final latest transaction count should equal the number of transactions sent")

	finalPendingCount, err := client.EVM().EthGetTransactionCount(ctx, ethAddr, ethtypes.NewEthBlockNumberOrHashFromPredefined("pending"))
	require.NoError(t, err)
	require.Equal(t, ethtypes.EthUint64(numTx), finalPendingCount, "Final pending transaction count should equal the number of transactions sent")

	// Test with a contract
	createReturn := client.EVM().DeployContract(ctx, client.DefaultKey.Address, contract)
	contractAddr := createReturn.EthAddress
	contractFilAddr := *createReturn.RobustAddress

	// Check contract nonce (should be 1 after deployment)
	contractNonce, err := client.EVM().EthGetTransactionCount(ctx, contractAddr, ethtypes.NewEthBlockNumberOrHashFromPredefined("latest"))
	require.NoError(t, err)
	require.Equal(t, ethtypes.EthUint64(1), contractNonce)

	// Destroy the contract
	_, _, err = client.EVM().InvokeContractByFuncName(ctx, client.DefaultKey.Address, contractFilAddr, "destroy()", nil)
	require.NoError(t, err)

	// Check contract nonce after destruction (should be 0)
	contractNonceAfterDestroy, err := client.EVM().EthGetTransactionCount(ctx, contractAddr, ethtypes.NewEthBlockNumberOrHashFromPredefined("latest"))
	require.NoError(t, err)
	require.Zero(t, contractNonceAfterDestroy)
}

func TestMcopy(t *testing.T) {
	// MCOPY introduced in nv24, start the test on nv23 to check the error, then upgrade at epoch 100
	// and check that an MCOPY contract can be deployed and run.
	nv24epoch := abi.ChainEpoch(100)
	upgradeSchedule := kit.UpgradeSchedule(
		stmgr.Upgrade{
			Network: network.Version23,
			Height:  -1,
		},
		stmgr.Upgrade{
			Network:   network.Version24,
			Height:    nv24epoch,
			Migration: filcns.UpgradeActorsV15,
		},
	)

	ctx, cancel, client := kit.SetupFEVMTest(t, upgradeSchedule)
	defer cancel()

	// try to deploy the contract before the upgrade, expect an error somewhere' in deploy or in call,
	// if the error is in deploy we may need to implement DeployContractFromFilename here where we can
	// assert an error

	// 0000000000000000000000000000000000000000000000000000000000000020: The offset for the bytes argument (32 bytes).
	// 0000000000000000000000000000000000000000000000000000000000000008: The length of the bytes data (8 bytes for "testdata").
	// 7465737464617461000000000000000000000000000000000000000000000000: The hexadecimal representation of "testdata", padded to 32 bytes.
	hexString := "000000000000000000000000000000000000000000000000000000000000002000000000000000000000000000000000000000000000000000000000000000087465737464617461000000000000000000000000000000000000000000000000"

	// Decode the hex string into a byte slice
	inputArgument, err := hex.DecodeString(hexString)
	require.NoError(t, err)

	filenameActor := "contracts/mcopy/MCOPYTest.hex"
	fromAddr, contractAddr := client.EVM().DeployContractFromFilename(ctx, filenameActor)
	_, _, err = client.EVM().InvokeContractByFuncName(ctx, fromAddr, contractAddr, "optimizedCopy(bytes)", inputArgument)
	// We expect an error here due to MCOPY not being available in this network version
	require.ErrorContains(t, err, "undefined instruction (35)")

	// wait for the upgrade
	client.WaitTillChain(ctx, kit.HeightAtLeast(nv24epoch+5))

	// should be able to deploy and call the contract now
	fromAddr, contractAddr = client.EVM().DeployContractFromFilename(ctx, filenameActor)
	result, _, err := client.EVM().InvokeContractByFuncName(ctx, fromAddr, contractAddr, "optimizedCopy(bytes)", inputArgument)
	require.NoError(t, err)
	require.Equal(t, inputArgument, result)
}

func TestEthGetBlockByNumber(t *testing.T) {
	blockTime := 100 * time.Millisecond
	client, _, ens := kit.EnsembleMinimal(t, kit.MockProofs(), kit.ThroughRPC())

	bms := ens.InterconnectAll().BeginMining(blockTime)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	// Create a new Ethereum account
	_, ethAddr, filAddr := client.EVM().NewAccount()
	// Send some funds to the f410 address
	kit.SendFunds(ctx, t, client, filAddr, types.FromFil(10))

	// Test getting the latest block
	latest, err := client.EthBlockNumber(ctx)
	require.NoError(t, err)
	latestBlock, err := client.EthGetBlockByNumber(ctx, "latest", true)
	require.NoError(t, err)
	require.NotNil(t, latestBlock)

	// Test getting a specific block by number
	specificBlock, err := client.EthGetBlockByNumber(ctx, latest.Hex(), true)
	require.NoError(t, err)
	require.NotNil(t, specificBlock)

	// Test getting a future block (should fail)
	_, err = client.EthGetBlockByNumber(ctx, (latest + 10000).Hex(), true)
	require.Error(t, err)

	// Inject 10 null rounds
	bms[0].InjectNulls(10)

	// Wait until we produce blocks again
	tctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	ch, err := client.ChainNotify(tctx)
	require.NoError(t, err)
	<-ch       // current
	hc := <-ch // wait for next block
	require.Equal(t, store.HCApply, hc[0].Type)

	afterNullHeight := hc[0].Val.Height()

	// Find the first null round
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

	// Test getting a block for a null round
	_, err = client.EthGetBlockByNumber(ctx, (ethtypes.EthUint64(nullHeight)).Hex(), true)
	require.ErrorContains(t, err, "requested epoch was a null round")

	// Test getting balance on a null round
	bal, err := client.EthGetBalance(ctx, ethAddr, ethtypes.NewEthBlockNumberOrHashFromNumber(ethtypes.EthUint64(nullHeight)))
	require.NoError(t, err)
	require.NotEqual(t, big.Zero(), bal)
	require.Equal(t, types.FromFil(10).Int, bal.Int)

	// Test getting block by pending
	pendingBlock, err := client.EthGetBlockByNumber(ctx, "pending", true)
	require.NoError(t, err)
	require.NotNil(t, pendingBlock)
	require.True(t, pendingBlock.Number >= latest)
}

func TestEthGetTransactionByBlockHashAndIndexAndNumber(t *testing.T) {
	ctx, cancel, client := kit.SetupFEVMTest(t)
	defer cancel()

	ethKey, ethAddr, ethFilAddr := client.EVM().NewAccount()
	kit.SendFunds(ctx, t, client, ethFilAddr, types.FromFil(10))

	var txHashes []ethtypes.EthHash
	var receipts []*ethtypes.EthTxReceipt
	numTx := 3

	contractHex, err := os.ReadFile("./contracts/MultipleEvents.hex")
	require.NoError(t, err)
	contract, err := hex.DecodeString(string(contractHex))
	require.NoError(t, err)

	gasParams, err := json.Marshal(ethtypes.EthEstimateGasParams{Tx: ethtypes.EthCall{
		From: &ethAddr,
		Data: contract,
	}})
	require.NoError(t, err)
	gaslimit, err := client.EthEstimateGas(ctx, gasParams)
	require.NoError(t, err)

	maxPriorityFeePerGas, err := client.EthMaxPriorityFeePerGas(ctx)
	require.NoError(t, err)

	for {
		txHashes = nil
		receipts = nil
		nonce, err := client.MpoolGetNonce(ctx, ethFilAddr)
		require.NoError(t, err)

		for i := 0; i < numTx; i++ {
			tx := &ethtypes.Eth1559TxArgs{
				ChainID:              buildconstants.Eip155ChainId,
				Value:                big.Zero(),
				Nonce:                int(nonce) + i,
				MaxFeePerGas:         types.NanoFil,
				MaxPriorityFeePerGas: big.Int(maxPriorityFeePerGas),
				GasLimit:             int(gaslimit),
				Input:                contract,
				V:                    big.Zero(),
				R:                    big.Zero(),
				S:                    big.Zero(),
			}
			client.EVM().SignTransaction(tx, ethKey.PrivateKey)
			hash := client.EVM().SubmitTransaction(ctx, tx)
			txHashes = append(txHashes, hash)
		}

		for _, hash := range txHashes {
			receipt, err := client.EVM().WaitTransaction(ctx, hash)
			require.NoError(t, err)
			require.NotNil(t, receipt)
			receipts = append(receipts, receipt)
		}

		allInSameTipset := true
		for i := 1; i < len(receipts); i++ {
			if receipts[i].BlockHash != receipts[0].BlockHash {
				allInSameTipset = false
				break
			}
		}

		if allInSameTipset {
			break
		}
		t.Logf("Retrying because transactions didn't land in the same tipset")
	}

	require.NotEmpty(t, receipts, "No transactions were mined")
	blockHash := receipts[0].BlockHash
	blockNumber := receipts[0].BlockNumber

	for _, receipt := range receipts {
		t.Logf("transaction index: %d", receipt.TransactionIndex)
		ethTx, err := client.EthGetTransactionByBlockHashAndIndex(ctx, blockHash, receipt.TransactionIndex)
		require.NoError(t, err)
		require.NotNil(t, ethTx)
		require.Equal(t, receipt.TransactionHash.String(), ethTx.Hash.String())
		require.Equal(t, ethtypes.EthUint64(2), ethTx.Type)
		require.NotEmpty(t, ethTx.Input, "Contract deployment should have input data")
		require.Equal(t, ethAddr.String(), ethTx.From.String())
	}

	for _, receipt := range receipts {
		ethTx, err := client.EthGetTransactionByBlockNumberAndIndex(ctx, blockNumber.Hex(), receipt.TransactionIndex)
		require.NoError(t, err)
		require.NotNil(t, ethTx)
		require.Equal(t, receipt.TransactionHash.String(), ethTx.Hash.String())
	}

	t.Run("Error cases", func(t *testing.T) {
		// 1. Invalid block hash
		invalidBlockHash := ethtypes.EthHash{1}
		_, err = client.EthGetTransactionByBlockHashAndIndex(ctx, invalidBlockHash, ethtypes.EthUint64(0))
		require.Error(t, err)
		require.ErrorContains(t, err, "failed to get tipset by hash")

		// 2. Invalid block number
		_, err = client.EthGetTransactionByBlockNumberAndIndex(ctx, (blockNumber + 1000).Hex(), ethtypes.EthUint64(0))
		require.Error(t, err)
		require.ErrorContains(t, err, "requested a future epoch")

		// 3. Index out of range
		_, err = client.EthGetTransactionByBlockHashAndIndex(ctx, blockHash, ethtypes.EthUint64(100))
		require.Error(t, err)
		require.ErrorContains(t, err, "out of range")

		// 4. Empty block
		emptyBlock, err := client.EthGetBlockByNumber(ctx, "latest", false)
		require.NoError(t, err)
		require.NotNil(t, emptyBlock)

		_, err = client.EthGetTransactionByBlockHashAndIndex(ctx, emptyBlock.Hash, ethtypes.EthUint64(0))
		require.Error(t, err)
		require.ErrorContains(t, err, "out of range")
	})
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
			name: "EthFeeHistory",
			testFunc: func() error {
				_, err := client.EthFeeHistory(ctx, jsonrpc.RawParams([]byte(`[1,"`+nullBlockHex+`",[]]`)))
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
			if err == nil {
				return
			}
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

func TestFEVMEamCreateTwiceFail(t *testing.T) {
	// See https://github.com/filecoin-project/lotus/issues/12731
	// EAM create errors were not being properly decoded for traces, we should be able to fail a
	// transaction and get a trace with the error message.
	// This test uses a contract that performs two create operations, the second one should fail with
	// an ErrForbidden error because it derives the same address as the first one.

	req := require.New(t)

	ctx, cancel, client := kit.SetupFEVMTest(t)
	defer cancel()

	filenameActor := "contracts/DuplicateCreate2Error.hex"
	fromAddr, actorAddr := client.EVM().DeployContractFromFilename(ctx, filenameActor)

	_, wait, err := client.EVM().InvokeContractByFuncName(ctx, fromAddr, actorAddr, "deployTwice()", nil)
	req.Error(err)
	req.Equal(exitcode.ExitCode(33), wait.Receipt.ExitCode)
	req.NotContains(err.Error(), "fatal error")

	t.Logf("Failed as expected: %s", err)

	traces, err := client.EthTraceBlock(ctx, fmt.Sprintf("0x%x", int(wait.Height-1)))
	req.NoError(err)

	req.Len(traces, 3)
	req.EqualValues(wait.Height-1, traces[0].BlockNumber)
	req.Equal("call", traces[0].EthTrace.Type)
	req.Contains("Reverted", traces[0].EthTrace.Error)
	req.EqualValues(wait.Height-1, traces[1].BlockNumber)
	req.Equal("create", traces[1].EthTrace.Type)
	req.Equal("", traces[1].EthTrace.Error)
	req.EqualValues(wait.Height-1, traces[2].BlockNumber)
	req.Equal("create", traces[2].EthTrace.Type)
	req.Contains(traces[2].EthTrace.Error, "ErrForbidden")
}

func TestTstore(t *testing.T) {
	nv25epoch := abi.ChainEpoch(100)
	upgradeSchedule := kit.UpgradeSchedule(
		stmgr.Upgrade{
			Network: network.Version24,
			Height:  -1,
		},
		stmgr.Upgrade{
			Network:   network.Version25,
			Height:    nv25epoch,
			Migration: filcns.UpgradeActorsV16,
		},
	)

	ctx, cancel, client := kit.SetupFEVMTest(t, upgradeSchedule)
	defer cancel()

	// try to deploy the contract before the upgrade, expect an error somewhere' in deploy or in call,
	// if the error is in deploy we may need to implement DeployContractFromFilename here where we can
	// assert an error

	filenameActor := "contracts/TransientStorageTest.hex"
	fromAddr, contractAddr := client.EVM().DeployContractFromFilename(ctx, filenameActor)

	inputData := make([]byte, 0)
	_, _, err := client.EVM().InvokeContractByFuncName(ctx, fromAddr, contractAddr, "runTests()", inputData)
	// We expect an error here due to TSTORE not being available in this network version
	require.ErrorContains(t, err, "undefined instruction (35)")

	client.WaitTillChain(ctx, kit.HeightAtLeast(nv25epoch+5))

	// wait for the upgrade

	//Step 1 initial reentry test
	// should be able to deploy and call the contract now
	_, _, err = client.EVM().InvokeContractByFuncName(ctx, fromAddr, contractAddr, "runTests()", inputData)
	require.NoError(t, err)

	//Step 2 subsequent transaction to confirm the transient data was reset
	_, _, err = client.EVM().InvokeContractByFuncName(ctx, fromAddr, contractAddr, "testLifecycleValidationSubsequentTransaction()", inputData)
	require.NoError(t, err)

	fromAddr, contractAddr2 := client.EVM().DeployContractFromFilename(ctx, filenameActor)
	inputDataContract := inputDataFromFrom(ctx, t, client, contractAddr2)

	//Step 3 test reentry from multiple contracts in a transaction
	_, _, err = client.EVM().InvokeContractByFuncName(ctx, fromAddr, contractAddr, "testReentry(address)", inputDataContract)
	require.NoError(t, err)

	//Step 4 test tranisent data from nested contract calls
	_, _, err = client.EVM().InvokeContractByFuncName(ctx, fromAddr, contractAddr, "testNestedContracts(address)", inputDataContract)
	require.NoError(t, err)
}

func TestFEVMTestBLS(t *testing.T) {
	ctx, cancel, client := kit.SetupFEVMTest(t)
	defer cancel()

	tests := []string{
		"G1AddTest",
		"G1MsmTest",
		"G2AddTest",
		"G2MsmTest",
		"MapFpToG1Test",
		"MapFp2ToG2Test",
		"PairingTest",
	}

	for _, name := range tests {
		name := name
		t.Run(name, func(t *testing.T) {
			filename := fmt.Sprintf("contracts/bls12/%s.hex", name)
			fromAddr, contractAddr := client.EVM().DeployContractFromFilename(ctx, filename)
			_, _, err := client.EVM().InvokeContractByFuncName(ctx, fromAddr, contractAddr, "runTests()", []byte{})
			require.NoError(t, err)
		})
	}
}
