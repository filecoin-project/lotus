package itests

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/go-openapi/spec"
	"github.com/gregdhill/go-openrpc/parse"
	orpctypes "github.com/gregdhill/go-openrpc/types"
	manet "github.com/multiformats/go-multiaddr/net"
	"github.com/stretchr/testify/require"
	"github.com/xeipuuv/gojsonschema"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-jsonrpc"
	"github.com/filecoin-project/go-state-types/big"

	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/types/ethtypes"
	"github.com/filecoin-project/lotus/chain/wallet/key"
	"github.com/filecoin-project/lotus/itests/kit"
)

// TODO generate this using reflection. It's the same as the EthAPI except every return value is a json.RawMessage
type ethAPIRaw struct {
	EthAccounts                            func(context.Context) (json.RawMessage, error)
	EthBlockNumber                         func(context.Context) (json.RawMessage, error)
	EthCall                                func(context.Context, ethtypes.EthCall, string) (json.RawMessage, error)
	EthChainId                             func(context.Context) (json.RawMessage, error)
	EthEstimateGas                         func(context.Context, ethtypes.EthCall) (json.RawMessage, error)
	EthFeeHistory                          func(context.Context, ethtypes.EthUint64, string, []float64) (json.RawMessage, error)
	EthGasPrice                            func(context.Context) (json.RawMessage, error)
	EthGetBalance                          func(context.Context, ethtypes.EthAddress, string) (json.RawMessage, error)
	EthGetBlockByHash                      func(context.Context, ethtypes.EthHash, bool) (json.RawMessage, error)
	EthGetBlockByNumber                    func(context.Context, string, bool) (json.RawMessage, error)
	EthGetBlockTransactionCountByHash      func(context.Context, ethtypes.EthHash) (json.RawMessage, error)
	EthGetBlockTransactionCountByNumber    func(context.Context, ethtypes.EthUint64) (json.RawMessage, error)
	EthGetCode                             func(context.Context, ethtypes.EthAddress, string) (json.RawMessage, error)
	EthGetFilterChanges                    func(context.Context, ethtypes.EthFilterID) (json.RawMessage, error)
	EthGetFilterLogs                       func(context.Context, ethtypes.EthFilterID) (json.RawMessage, error)
	EthGetLogs                             func(context.Context, *ethtypes.EthFilterSpec) (json.RawMessage, error)
	EthGetStorageAt                        func(context.Context, ethtypes.EthAddress, ethtypes.EthBytes, string) (json.RawMessage, error)
	EthGetTransactionByBlockHashAndIndex   func(context.Context, ethtypes.EthHash, ethtypes.EthUint64) (json.RawMessage, error)
	EthGetTransactionByBlockNumberAndIndex func(context.Context, ethtypes.EthUint64, ethtypes.EthUint64) (json.RawMessage, error)
	EthGetTransactionByHash                func(context.Context, *ethtypes.EthHash) (json.RawMessage, error)
	EthGetTransactionCount                 func(context.Context, ethtypes.EthAddress, string) (json.RawMessage, error)
	EthGetTransactionReceipt               func(context.Context, ethtypes.EthHash) (json.RawMessage, error)
	EthMaxPriorityFeePerGas                func(context.Context) (json.RawMessage, error)
	EthNewBlockFilter                      func(context.Context) (json.RawMessage, error)
	EthNewFilter                           func(context.Context, *ethtypes.EthFilterSpec) (json.RawMessage, error)
	EthNewPendingTransactionFilter         func(context.Context) (json.RawMessage, error)
	EthSendRawTransaction                  func(context.Context, ethtypes.EthBytes) (json.RawMessage, error)
	EthSubscribe                           func(context.Context, string, *ethtypes.EthSubscriptionParams) (json.RawMessage, error)
	EthUninstallFilter                     func(context.Context, ethtypes.EthFilterID) (json.RawMessage, error)
	EthUnsubscribe                         func(context.Context, ethtypes.EthSubscriptionID) (json.RawMessage, error)
}

func TestEthOpenRPCConformance(t *testing.T) {
	kit.QuietAllLogsExcept("events", "messagepool")

	// specs/eth_openrpc.json is built from https://github.com/ethereum/execution-apis
	specJSON, err := os.ReadFile("specs/eth_openrpc.json")
	require.NoError(t, err)

	specParsed := orpctypes.NewOpenRPCSpec1()
	err = json.Unmarshal(specJSON, specParsed)
	require.NoError(t, err)
	parse.GetTypes(specParsed, specParsed.Objects)

	schemas := make(map[string]spec.Schema)
	for _, method := range specParsed.Methods {
		if method.Result != nil {
			schemas[method.Name] = method.Result.Schema
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	client, _, ens := kit.EnsembleMinimal(t, kit.MockProofs(), kit.ThroughRPC(), kit.WithEthRPC())
	ens.InterconnectAll().BeginMining(10 * time.Millisecond)

	contractHex, err := os.ReadFile("contracts/EventMatrix.hex")
	require.NoError(t, err)

	// strip any trailing newlines from the file
	contractHex = bytes.TrimRight(contractHex, "\n")

	contractBin, err := hex.DecodeString(string(contractHex))
	require.NoError(t, err)

	senderKey, senderEthAddr, senderFilAddr := client.EVM().NewAccount()
	_, receiverEthAddr, _ := client.EVM().NewAccount()
	kit.SendFunds(ctx, t, client, senderFilAddr, types.FromFil(1000))

	deployerAddr, err := client.EVM().WalletDefaultAddress(ctx)
	require.NoError(t, err)

	pendingTransactionFilterID, err := client.EthNewPendingTransactionFilter(ctx)
	require.NoError(t, err)

	blockFilterID, err := client.EthNewBlockFilter(ctx)
	require.NoError(t, err)

	filterAllLogs := kit.NewEthFilterBuilder().FromBlockEpoch(0).Filter()

	logFilterID, err := client.EthNewFilter(ctx, filterAllLogs)
	require.NoError(t, err)

	uninstallableFilterID, err := client.EthNewFilter(ctx, filterAllLogs)
	require.NoError(t, err)

	rawSignedEthTx := createRawSignedEthTx(ctx, t, client, senderEthAddr, receiverEthAddr, senderKey, contractBin)

	result := client.EVM().DeployContract(ctx, deployerAddr, contractBin)
	contractAddr, err := address.NewIDAddress(result.ActorID)
	require.NoError(t, err)

	contractEthAddr := ethtypes.EthAddress(result.EthAddress)

	messageWithEvents, blockHashWithMessage, blockNumberWithMessage := waitForMessageWithEvents(ctx, t, client, deployerAddr, contractAddr)

	// create a json-rpc client that returns raw json responses
	var ethapi ethAPIRaw

	netAddr, err := manet.ToNetAddr(client.ListenAddr)
	require.NoError(t, err)
	rpcAddr := "ws://" + netAddr.String() + "/rpc/v1"

	closer, err := jsonrpc.NewClient(ctx, rpcAddr, "Filecoin", &ethapi, nil)
	require.NoError(t, err)
	defer closer()

	testCases := []struct {
		method     string
		variant    string // suffix applied to the test name to distinguish different variants of a method call
		call       func(*ethAPIRaw) (json.RawMessage, error)
		skipReason string
	}{
		// Alphabetical order

		{
			method: "eth_accounts",
			call: func(a *ethAPIRaw) (json.RawMessage, error) {
				return ethapi.EthAccounts(context.Background())
			},
		},

		{
			method: "eth_blockNumber",
			call: func(a *ethAPIRaw) (json.RawMessage, error) {
				return ethapi.EthBlockNumber(context.Background())
			},
		},

		{
			method:  "eth_call",
			variant: "latest",
			call: func(a *ethAPIRaw) (json.RawMessage, error) {
				return ethapi.EthCall(context.Background(), ethtypes.EthCall{
					From: &senderEthAddr,
					Data: contractBin,
				}, "latest")
			},
		},

		{
			method: "eth_chainId",
			call: func(a *ethAPIRaw) (json.RawMessage, error) {
				return ethapi.EthChainId(context.Background())
			},
		},

		{
			method: "eth_estimateGas",
			call: func(a *ethAPIRaw) (json.RawMessage, error) {
				return ethapi.EthEstimateGas(context.Background(), ethtypes.EthCall{
					From: &senderEthAddr,
					Data: contractBin,
				})
			},
		},

		{
			method: "eth_feeHistory",
			call: func(a *ethAPIRaw) (json.RawMessage, error) {
				return ethapi.EthFeeHistory(context.Background(), ethtypes.EthUint64(2), "latest", nil)
			},
		},

		{
			method: "eth_gasPrice",
			call: func(a *ethAPIRaw) (json.RawMessage, error) {
				return ethapi.EthGasPrice(context.Background())
			},
		},

		{
			method:  "eth_getBalance",
			variant: "blocknumber",
			call: func(a *ethAPIRaw) (json.RawMessage, error) {
				return ethapi.EthGetBalance(context.Background(), contractEthAddr, "0x0")
			},
		},

		{
			method:  "eth_getBlockByHash",
			variant: "txhashes",
			call: func(a *ethAPIRaw) (json.RawMessage, error) {
				return ethapi.EthGetBlockByHash(context.Background(), blockHashWithMessage, false)
			},
		},

		{
			method:  "eth_getBlockByHash",
			variant: "txfull",
			call: func(a *ethAPIRaw) (json.RawMessage, error) {
				return ethapi.EthGetBlockByHash(context.Background(), blockHashWithMessage, true)
			},
		},

		{
			method:  "eth_getBlockByNumber",
			variant: "earliest",
			call: func(a *ethAPIRaw) (json.RawMessage, error) {
				return ethapi.EthGetBlockByNumber(context.Background(), "earliest", true)
			},
			skipReason: "earliest block is not supported",
		},

		{
			method: "eth_getBlockByNumber",
			call: func(a *ethAPIRaw) (json.RawMessage, error) {
				return ethapi.EthGetBlockByNumber(context.Background(), blockNumberWithMessage.Hex(), true)
			},
		},

		{
			method: "eth_getBlockTransactionCountByHash",
			call: func(a *ethAPIRaw) (json.RawMessage, error) {
				return ethapi.EthGetBlockTransactionCountByHash(context.Background(), blockHashWithMessage)
			},
		},

		{
			method: "eth_getBlockTransactionCountByNumber",
			call: func(a *ethAPIRaw) (json.RawMessage, error) {
				return ethapi.EthGetBlockTransactionCountByNumber(context.Background(), blockNumberWithMessage)
			},
		},

		{
			method:  "eth_getCode",
			variant: "blocknumber",
			call: func(a *ethAPIRaw) (json.RawMessage, error) {
				return ethapi.EthGetCode(context.Background(), contractEthAddr, blockNumberWithMessage.Hex())
			},
		},

		{
			method:  "eth_getFilterChanges",
			variant: "pendingtransaction",
			call: func(a *ethAPIRaw) (json.RawMessage, error) {
				return a.EthGetFilterChanges(ctx, pendingTransactionFilterID)
			},
		},

		{
			method:  "eth_getFilterChanges",
			variant: "block",
			call: func(a *ethAPIRaw) (json.RawMessage, error) {
				return a.EthGetFilterChanges(ctx, blockFilterID)
			},
		},

		{
			method:  "eth_getFilterChanges",
			variant: "logs",
			call: func(a *ethAPIRaw) (json.RawMessage, error) {
				return a.EthGetFilterChanges(ctx, logFilterID)
			},
		},

		{
			method: "eth_getFilterLogs",
			call: func(a *ethAPIRaw) (json.RawMessage, error) {
				return a.EthGetFilterLogs(ctx, logFilterID)
			},
		},

		{
			method: "eth_getLogs",
			call: func(a *ethAPIRaw) (json.RawMessage, error) {
				return ethapi.EthGetLogs(context.Background(), filterAllLogs)
			},
		},

		{
			method:  "eth_getStorageAt",
			variant: "blocknumber",
			call: func(a *ethAPIRaw) (json.RawMessage, error) {
				return ethapi.EthGetStorageAt(context.Background(), contractEthAddr, ethtypes.EthBytes{0}, "0x0")
			},
		},

		{
			method: "eth_getTransactionByBlockHashAndIndex",
			call: func(a *ethAPIRaw) (json.RawMessage, error) {
				return ethapi.EthGetTransactionByBlockHashAndIndex(context.Background(), blockHashWithMessage, ethtypes.EthUint64(0))
			},
			skipReason: "unimplemented",
		},

		{
			method: "eth_getTransactionByBlockNumberAndIndex",
			call: func(a *ethAPIRaw) (json.RawMessage, error) {
				return ethapi.EthGetTransactionByBlockNumberAndIndex(context.Background(), blockNumberWithMessage, ethtypes.EthUint64(0))
			},
			skipReason: "unimplemented",
		},

		{
			method: "eth_getTransactionByHash",
			call: func(a *ethAPIRaw) (json.RawMessage, error) {
				return ethapi.EthGetTransactionByHash(context.Background(), &messageWithEvents)
			},
		},

		{
			method:  "eth_getTransactionCount",
			variant: "blocknumber",
			call: func(a *ethAPIRaw) (json.RawMessage, error) {
				return ethapi.EthGetTransactionCount(context.Background(), senderEthAddr, blockNumberWithMessage.Hex())
			},
		},

		{
			method: "eth_getTransactionReceipt",
			call: func(a *ethAPIRaw) (json.RawMessage, error) {
				return ethapi.EthGetTransactionReceipt(context.Background(), messageWithEvents)
			},
		},

		{
			method: "eth_maxPriorityFeePerGas",
			call: func(a *ethAPIRaw) (json.RawMessage, error) {
				return ethapi.EthMaxPriorityFeePerGas(context.Background())
			},
		},

		{
			method: "eth_newBlockFilter",
			call: func(a *ethAPIRaw) (json.RawMessage, error) {
				return ethapi.EthNewBlockFilter(context.Background())
			},
		},

		{
			method: "eth_newFilter",
			call: func(a *ethAPIRaw) (json.RawMessage, error) {
				return ethapi.EthNewFilter(context.Background(), filterAllLogs)
			},
		},

		{
			method: "eth_newPendingTransactionFilter",
			call: func(a *ethAPIRaw) (json.RawMessage, error) {
				return ethapi.EthNewPendingTransactionFilter(context.Background())
			},
		},

		{
			method: "eth_sendRawTransaction",
			call: func(a *ethAPIRaw) (json.RawMessage, error) {
				return ethapi.EthSendRawTransaction(context.Background(), rawSignedEthTx)
			},
		},
		{
			method: "eth_uninstallFilter",
			call: func(a *ethAPIRaw) (json.RawMessage, error) {
				return a.EthUninstallFilter(ctx, uninstallableFilterID)
			},
		},
	}

	for _, tc := range testCases {
		tc := tc
		name := tc.method
		if tc.variant != "" {
			name += "_" + tc.variant
		}
		t.Run(name, func(t *testing.T) {
			if tc.skipReason != "" {
				t.Skipf(tc.skipReason)
			}

			schema, ok := schemas[tc.method]
			require.True(t, ok, "method not found in openrpc spec")

			resp, err := tc.call(&ethapi)
			require.NoError(t, err)

			respJson, err := json.Marshal(resp)
			require.NoError(t, err)

			loader := gojsonschema.NewGoLoader(schema)
			resploader := gojsonschema.NewBytesLoader(respJson)
			result, err := gojsonschema.Validate(loader, resploader)
			require.NoError(t, err)

			if !result.Valid() {
				if len(result.Errors()) == 1 && strings.Contains(result.Errors()[0].String(), "Must validate one and only one schema (oneOf)") {
					// Ignore this error, since it seems the openrpc spec can't handle it
					// New transaction and block filters have the same schema: an array of 32 byte hashes
					return
				}

				niceRespJson, err := json.MarshalIndent(resp, "", "  ")
				if err == nil {
					t.Logf("response was %s", niceRespJson)
				}

				schemaJson, err := json.MarshalIndent(schema, "", "  ")
				if err == nil {
					t.Logf("schema was %s", schemaJson)
				}

				// check against https://www.jsonschemavalidator.net/

				for _, desc := range result.Errors() {
					t.Logf("- %s\n", desc)
				}

				t.Errorf("response did not validate")
			}
		})
	}
}

func createRawSignedEthTx(ctx context.Context, t *testing.T, client *kit.TestFullNode, senderEthAddr ethtypes.EthAddress, receiverEthAddr ethtypes.EthAddress, senderKey *key.Key, contractBin []byte) []byte {
	gaslimit, err := client.EthEstimateGas(ctx, ethtypes.EthCall{
		From: &senderEthAddr,
		Data: contractBin,
	})
	require.NoError(t, err)

	maxPriorityFeePerGas, err := client.EthMaxPriorityFeePerGas(ctx)
	require.NoError(t, err)

	tx := ethtypes.EthTxArgs{
		ChainID:              build.Eip155ChainId,
		Value:                big.NewInt(100),
		Nonce:                0,
		To:                   &receiverEthAddr,
		MaxFeePerGas:         types.NanoFil,
		MaxPriorityFeePerGas: big.Int(maxPriorityFeePerGas),
		GasLimit:             int(gaslimit),
		V:                    big.Zero(),
		R:                    big.Zero(),
		S:                    big.Zero(),
	}

	client.EVM().SignTransaction(&tx, senderKey.PrivateKey)
	signed, err := tx.ToRlpSignedMsg()
	require.NoError(t, err)
	return signed
}

func waitForMessageWithEvents(ctx context.Context, t *testing.T, client *kit.TestFullNode, sender address.Address, target address.Address) (ethtypes.EthHash, ethtypes.EthHash, ethtypes.EthUint64) {
	vals := []uint64{44, 27, 19, 12}
	inputData := []byte{}
	for _, v := range vals {
		buf := make([]byte, 32)
		binary.BigEndian.PutUint64(buf[24:], v)
		inputData = append(inputData, buf...)
	}

	// send a message that exercises event logs
	ret, err := client.EVM().InvokeSolidity(ctx, sender, target, kit.EventMatrixContract.Fn["logEventThreeIndexedWithData"], inputData)
	require.NoError(t, err)
	require.True(t, ret.Receipt.ExitCode.IsSuccess(), "contract execution failed")

	msgHash, err := client.EthGetTransactionHashByCid(ctx, ret.Message)
	require.NoError(t, err)
	require.NotNil(t, msgHash)

	ts, err := client.ChainGetTipSet(ctx, ret.TipSet)
	require.NoError(t, err)

	blockNumber := ethtypes.EthUint64(ts.Height())

	tsCid, err := ts.Key().Cid()
	require.NoError(t, err)

	blockHash, err := ethtypes.EthHashFromCid(tsCid)
	require.NoError(t, err)
	return *msgHash, blockHash, blockNumber
}
