package itests

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path"
	"testing"
	"time"

	"github.com/filecoin-project/go-jsonrpc"
	"github.com/filecoin-project/lotus/chain/types/ethtypes"
	"github.com/filecoin-project/lotus/itests/kit"
	oaerrors "github.com/go-openapi/errors"
	"github.com/go-openapi/spec"
	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/validate"
	"github.com/ipfs/go-cid"
	manet "github.com/multiformats/go-multiaddr/net"
	"github.com/stretchr/testify/require"

	"github.com/gregdhill/go-openrpc/parse"
	"github.com/gregdhill/go-openrpc/types"
	"github.com/gregdhill/go-openrpc/util"
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
	EthGetTransactionHashByCid             func(context.Context, cid.Cid) (json.RawMessage, error)
	EthGetTransactionReceipt               func(context.Context, ethtypes.EthHash) (json.RawMessage, error)
	EthMaxPriorityFeePerGas                func(context.Context) (json.RawMessage, error)
	EthNewBlockFilter                      func(context.Context) (json.RawMessage, error)
	EthNewFilter                           func(context.Context, *ethtypes.EthFilterSpec) (json.RawMessage, error)
	EthNewPendingTransactionFilter         func(context.Context) (json.RawMessage, error)
	EthProtocolVersion                     func(context.Context) (json.RawMessage, error)
	EthSendRawTransaction                  func(context.Context, ethtypes.EthBytes) (json.RawMessage, error)
	EthSubscribe                           func(context.Context, string, *ethtypes.EthSubscriptionParams) (json.RawMessage, error)
	EthUninstallFilter                     func(context.Context, ethtypes.EthFilterID) (json.RawMessage, error)
	EthUnsubscribe                         func(context.Context, ethtypes.EthSubscriptionID) (json.RawMessage, error)
}

func TestEthOpenRPC(t *testing.T) {
	kit.QuietAllLogsExcept("events", "messagepool")

	specJSON, err := os.ReadFile("specs/eth_openrpc.json")
	require.NoError(t, err)

	specParsed := types.NewOpenRPCSpec1()
	err = json.Unmarshal([]byte(specJSON), specParsed)
	require.NoError(t, err)
	parse.GetTypes(specParsed, specParsed.Objects)

	schemas := make(map[string]spec.Schema)
	for _, method := range specParsed.Methods {
		if method.Result != nil {
			schema := resolveSchema(specParsed, method.Result.Schema)
			schemas[method.Name] = schema
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	client, _, ens := kit.EnsembleMinimal(t, kit.MockProofs(), kit.ThroughRPC(), kit.WithEthRPC())
	ens.InterconnectAll().BeginMining(10 * time.Millisecond)

	// send a message that exercises event logs
	sender, contract := client.EVM().DeployContractFromFilename(ctx, EventMatrixContract.Filename)
	messages := invokeAndWaitUntilAllOnChain(t, client, []Invocation{
		{
			Sender:   sender,
			Target:   contract,
			Selector: EventMatrixContract.Fn["logEventThreeIndexedWithData"],
			Data:     packUint64Values(44, 27, 19, 12),
		},
	})

	require.NotEmpty(t, messages)

	var messageWithEvents ethtypes.EthHash
	for k := range messages {
		messageWithEvents = k
		break
	}

	netAddr, err := manet.ToNetAddr(client.ListenAddr)
	require.NoError(t, err)
	rpcAddr := "ws://" + netAddr.String() + "/rpc/v1"

	var ethapi ethAPIRaw
	closer, err := jsonrpc.NewClient(ctx, rpcAddr, "Filecoin", &ethapi, nil)
	require.NoError(t, err)
	defer closer()

	testCases := []struct {
		method string
		call   func(*ethAPIRaw) (json.RawMessage, error)
	}{
		{
			method: "eth_blockNumber",
			call: func(a *ethAPIRaw) (json.RawMessage, error) {
				return ethapi.EthBlockNumber(context.Background())
			},
		},

		{
			method: "eth_getTransactionReceipt",
			call: func(a *ethAPIRaw) (json.RawMessage, error) {
				return ethapi.EthGetTransactionReceipt(context.Background(), messageWithEvents)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.method, func(t *testing.T) {
			schema, ok := schemas[tc.method]
			require.True(t, ok, "method not found in openrpc spec")

			resp, err := tc.call(&ethapi)
			require.NoError(t, err)

			respJson, err := json.MarshalIndent(resp, "", "  ")
			require.NoError(t, err)

			err = validate.AgainstSchema(&schema, respJson, strfmt.Default)
			if err != nil {
				t.Logf("response was %s", respJson)

				ce := &oaerrors.CompositeError{}
				if errors.As(err, &ce) {
					for _, e := range ce.Errors {
						t.Logf("error: %v", e)
					}
				}
			}
			require.NoError(t, err)
		})
	}
}

func resolveSchema(openrpc *types.OpenRPCSpec1, sch spec.Schema) spec.Schema {
	doc, _, _ := sch.Ref.GetPointer().Get(openrpc)

	if s, ok := doc.(spec.Schema); ok {
		sch = persistFields(sch, s)
	} else if cd, ok := doc.(*types.ContentDescriptor); ok {
		sch = persistFields(sch, cd.Schema)
	}

	for i := range sch.OneOf {
		sch.OneOf[i] = resolveSchema(openrpc, sch.OneOf[i])
	}

	for i := range sch.AllOf {
		sch.AllOf[i] = resolveSchema(openrpc, sch.AllOf[i])
	}

	for i := range sch.AnyOf {
		sch.AnyOf[i] = resolveSchema(openrpc, sch.AnyOf[i])
	}

	if sch.Not != nil {
		s := resolveSchema(openrpc, *sch.Not)
		sch.Not = &s
	}

	if sch.Ref.GetURL() != nil {
		return resolveSchema(openrpc, sch)
	}
	return sch
}

func persistFields(prev, next spec.Schema) spec.Schema {
	next.Title = util.FirstOf(next.Title, prev.Title, path.Base(prev.Ref.String()))
	next.Description = util.FirstOf(next.Description, prev.Description)
	if next.Items == nil {
		next.Items = prev.Items
	}
	return next
}
