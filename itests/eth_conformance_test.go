package itests

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"os"
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

	"github.com/filecoin-project/lotus/chain/types/ethtypes"
	"github.com/filecoin-project/lotus/itests/kit"
)

// TODO generate this using reflection
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
	err = json.Unmarshal([]byte(specJSON), specParsed)
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

	contractHex, err := os.ReadFile(EventMatrixContract.Filename)
	require.NoError(t, err)

	// strip any trailing newlines from the file
	contractHex = bytes.TrimRight(contractHex, "\n")

	contractBin, err := hex.DecodeString(string(contractHex))
	require.NoError(t, err)

	senderAddr, err := client.EVM().WalletDefaultAddress(ctx)
	require.NoError(t, err)
	// senderEthAddr := getContractEthAddress(ctx, t, client, senderAddr)

	result := client.EVM().DeployContract(ctx, senderAddr, contractBin)

	contractAddr, err := address.NewIDAddress(result.ActorID)
	require.NoError(t, err)

	// send a message that exercises event logs
	messages := invokeAndWaitUntilAllOnChain(t, client, []Invocation{
		{
			Sender:   senderAddr,
			Target:   contractAddr,
			Selector: EventMatrixContract.Fn["logEventThreeIndexedWithData"],
			Data:     packUint64Values(44, 27, 19, 12),
		},
	})

	require.NotEmpty(t, messages)

	var messageWithEvents ethtypes.EthHash
	var blockHashWithMessage ethtypes.EthHash
	var blockNumberWithMessage ethtypes.EthUint64

	for k, mts := range messages {
		messageWithEvents = k
		ts := mts.ts
		blockNumberWithMessage = ethtypes.EthUint64(ts.Height())

		tsCid, err := ts.Key().Cid()
		require.NoError(t, err)

		blockHashWithMessage, err = ethtypes.EthHashFromCid(tsCid)
		require.NoError(t, err)
		break
	}

	// create a json-rpc client that returns raw json responses
	var ethapi ethAPIRaw

	netAddr, err := manet.ToNetAddr(client.ListenAddr)
	require.NoError(t, err)
	rpcAddr := "ws://" + netAddr.String() + "/rpc/v1"

	closer, err := jsonrpc.NewClient(ctx, rpcAddr, "Filecoin", &ethapi, nil)
	require.NoError(t, err)
	defer closer()

	// maxPriorityFeePerGas, err := client.EthMaxPriorityFeePerGas(ctx)

	testCases := []struct {
		method  string
		variant string // suffix applied to the test name to distinguish different variants of a method call
		call    func(*ethAPIRaw) (json.RawMessage, error)
	}{
		// Simple no-argument calls first

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
			method: "eth_chainId",
			call: func(a *ethAPIRaw) (json.RawMessage, error) {
				return ethapi.EthChainId(context.Background())
			},
		},

		{
			method: "eth_gasPrice",
			call: func(a *ethAPIRaw) (json.RawMessage, error) {
				return ethapi.EthGasPrice(context.Background())
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
			method: "eth_newPendingTransactionFilter",
			call: func(a *ethAPIRaw) (json.RawMessage, error) {
				return ethapi.EthNewPendingTransactionFilter(context.Background())
			},
		},

		{
			method: "eth_getTransactionReceipt",
			call: func(a *ethAPIRaw) (json.RawMessage, error) {
				return ethapi.EthGetTransactionReceipt(context.Background(), messageWithEvents)
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
		},

		{
			method:  "eth_getBlockByNumber",
			variant: "pending",
			call: func(a *ethAPIRaw) (json.RawMessage, error) {
				return ethapi.EthGetBlockByNumber(context.Background(), "pending", true)
			},
		},

		{
			method:  "eth_getBlockByNumber",
			variant: "latest",
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
			method: "eth_getTransactionByBlockHashAndIndex",
			call: func(a *ethAPIRaw) (json.RawMessage, error) {
				return ethapi.EthGetTransactionByBlockHashAndIndex(context.Background(), blockHashWithMessage, ethtypes.EthUint64(0))
			},
		},

		{
			method: "eth_getTransactionByBlockNumberAndIndex",
			call: func(a *ethAPIRaw) (json.RawMessage, error) {
				return ethapi.EthGetTransactionByBlockNumberAndIndex(context.Background(), blockNumberWithMessage, ethtypes.EthUint64(0))
			},
		},

		{
			method: "eth_getTransactionByHash",
			call: func(a *ethAPIRaw) (json.RawMessage, error) {
				return ethapi.EthGetTransactionByHash(context.Background(), &messageWithEvents)
			},
		},

		// {
		// 	method: "eth_call",
		// 	call: func(a *ethAPIRaw) (json.RawMessage, error) {
		// 		return ethapi.EthCall(context.Background(), ethtypes.EthCall{
		// 			From: &senderEthAddr,
		// 			Data: contractBin,
		// 		}, "logEventZeroData")
		// 	},
		// },

		// {
		// 	method: "eth_estimateGas",
		// 	call: func(a *ethAPIRaw) (json.RawMessage, error) {
		// 		return ethapi.EthEstimateGas(context.Background(), ethtypes.EthCall{
		// 			From: &senderEthAddr,
		// 			Data: contractBin,
		// 		})
		// 	},
		// },
	}

	for _, tc := range testCases {
		name := tc.method
		if tc.variant != "" {
			name += "_" + tc.variant
		}
		t.Run(name, func(t *testing.T) {
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
