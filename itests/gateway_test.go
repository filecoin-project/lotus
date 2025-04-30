package itests

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/ipfs/go-cid"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-f3"
	"github.com/filecoin-project/go-jsonrpc"
	"github.com/filecoin-project/go-state-types/abi"
	init2 "github.com/filecoin-project/specs-actors/v2/actors/builtin/init"
	multisig2 "github.com/filecoin-project/specs-actors/v2/actors/builtin/multisig"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/api/client"
	"github.com/filecoin-project/lotus/api/v2api"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/types/ethtypes"
	"github.com/filecoin-project/lotus/gateway"
	"github.com/filecoin-project/lotus/itests/kit"
	"github.com/filecoin-project/lotus/itests/multisig"
	res "github.com/filecoin-project/lotus/lib/result"
	"github.com/filecoin-project/lotus/node"
)

const (
	maxLookbackCap           = time.Duration(math.MaxInt64)
	maxMessageLookbackEpochs = stmgr.LookbackNoLimit
)

// TestGatewayWalletMsig tests that API calls to wallet and msig can be made on a lite
// node that is connected through a gateway to a full API node
func TestGatewayWalletMsig(t *testing.T) {

	kit.QuietMiningLogs()

	ctx := context.Background()
	nodes := startNodes(ctx, t)

	lite := nodes.lite
	full := nodes.full

	// The full node starts with a wallet
	fullWalletAddr, err := full.WalletDefaultAddress(ctx)
	require.NoError(t, err)

	// Check the full node's wallet balance from the lite node
	balance, err := lite.WalletBalance(ctx, fullWalletAddr)
	require.NoError(t, err)
	fmt.Println(balance)

	// Create a wallet on the lite node
	liteWalletAddr, err := lite.WalletNew(ctx, types.KTSecp256k1)
	require.NoError(t, err)

	// Send some funds from the full node to the lite node
	err = sendFunds(ctx, full, fullWalletAddr, liteWalletAddr, types.NewInt(1e18))
	require.NoError(t, err)

	// Send some funds from the lite node back to the full node
	err = sendFunds(ctx, lite, liteWalletAddr, fullWalletAddr, types.NewInt(100))
	require.NoError(t, err)

	// Sign some data with the lite node wallet address
	data := []byte("hello")
	sig, err := lite.WalletSign(ctx, liteWalletAddr, data)
	require.NoError(t, err)

	// Verify the signature
	ok, err := lite.WalletVerify(ctx, liteWalletAddr, data, sig)
	require.NoError(t, err)
	require.True(t, ok)

	// Create some wallets on the lite node to use for testing multisig
	var walletAddrs []address.Address
	for i := 0; i < 4; i++ {
		addr, err := lite.WalletNew(ctx, types.KTSecp256k1)
		require.NoError(t, err)

		walletAddrs = append(walletAddrs, addr)

		err = sendFunds(ctx, lite, liteWalletAddr, addr, types.NewInt(1e15))
		require.NoError(t, err)
	}

	// Create an msig with three of the addresses and threshold of two sigs
	msigAddrs := walletAddrs[:3]
	amt := types.NewInt(1000)
	proto, err := lite.MsigCreate(ctx, 2, msigAddrs, abi.ChainEpoch(50), amt, liteWalletAddr, types.NewInt(0))
	require.NoError(t, err)

	doSend := func(proto *api.MessagePrototype) (cid.Cid, error) {
		if proto.ValidNonce {
			sm, err := lite.WalletSignMessage(ctx, proto.Message.From, &proto.Message)
			if err != nil {
				return cid.Undef, err
			}
			return lite.MpoolPush(ctx, sm)
		}

		sm, err := lite.MpoolPushMessage(ctx, &proto.Message, nil)
		if err != nil {
			return cid.Undef, err
		}

		return sm.Cid(), nil
	}

	addProposal, err := doSend(proto)
	require.NoError(t, err)

	res, err := lite.StateWaitMsg(ctx, addProposal, 1, api.LookbackNoLimit, true)
	require.NoError(t, err)
	require.EqualValues(t, 0, res.Receipt.ExitCode)

	var execReturn init2.ExecReturn
	err = execReturn.UnmarshalCBOR(bytes.NewReader(res.Receipt.Return))
	require.NoError(t, err)

	// Get available balance of msig: should be greater than zero and less
	// than initial amount
	msig := execReturn.IDAddress
	msigBalance, err := lite.MsigGetAvailableBalance(ctx, msig, types.EmptyTSK)
	require.NoError(t, err)
	require.GreaterOrEqual(t, msigBalance.Int64(), int64(0))
	require.LessOrEqual(t, msigBalance.Int64(), amt.Int64())

	// Propose to add a new address to the msig
	proto, err = lite.MsigAddPropose(ctx, msig, walletAddrs[0], walletAddrs[3], false)
	require.NoError(t, err)

	addProposal, err = doSend(proto)
	require.NoError(t, err)

	res, err = lite.StateWaitMsg(ctx, addProposal, 1, api.LookbackNoLimit, true)
	require.NoError(t, err)
	require.EqualValues(t, 0, res.Receipt.ExitCode)

	var proposeReturn multisig2.ProposeReturn
	err = proposeReturn.UnmarshalCBOR(bytes.NewReader(res.Receipt.Return))
	require.NoError(t, err)

	// Approve proposal (proposer is first (implicit) signer, approver is
	// second signer
	txnID := uint64(proposeReturn.TxnID)
	proto, err = lite.MsigAddApprove(ctx, msig, walletAddrs[1], txnID, walletAddrs[0], walletAddrs[3], false)
	require.NoError(t, err)

	approval1, err := doSend(proto)
	require.NoError(t, err)

	res, err = lite.StateWaitMsg(ctx, approval1, 1, api.LookbackNoLimit, true)
	require.NoError(t, err)
	require.EqualValues(t, 0, res.Receipt.ExitCode)

	var approveReturn multisig2.ApproveReturn
	err = approveReturn.UnmarshalCBOR(bytes.NewReader(res.Receipt.Return))
	require.NoError(t, err)
	require.True(t, approveReturn.Applied)
}

// TestGatewayMsigCLI tests that msig CLI calls can be made
// on a lite node that is connected through a gateway to a full API node
func TestGatewayMsigCLI(t *testing.T) {
	kit.QuietMiningLogs()

	ctx := context.Background()
	nodes := startNodes(ctx, t, withFunds())

	lite := nodes.lite
	multisig.RunMultisigTests(t, lite)
}

type testNodes struct {
	lite        *kit.TestFullNode
	full        *kit.TestFullNode
	miner       *kit.TestMiner
	gatewayAddr string
	rpcCloser   jsonrpc.ClientCloser
}

type startOptions struct {
	blockTime                   time.Duration
	lookbackCap                 time.Duration
	maxMessageLookbackEpochs    abi.ChainEpoch
	fund                        bool
	perConnectionAPIRateLimit   int
	perHostConnectionsPerMinute int
	nodeOpts                    []kit.NodeOpt
}

type startOption func(*startOptions)

func newStartOptions(opts ...startOption) startOptions {
	o := startOptions{
		blockTime:                5 * time.Millisecond,
		lookbackCap:              maxLookbackCap,
		maxMessageLookbackEpochs: maxMessageLookbackEpochs,
		fund:                     false,
	}
	for _, opt := range opts {
		opt(&o)
	}
	return o
}

func withFunds() startOption {
	return func(opts *startOptions) {
		opts.fund = true
	}
}

func withPerConnectionAPIRateLimit(rateLimit int) startOption {
	return func(opts *startOptions) {
		opts.perConnectionAPIRateLimit = rateLimit
	}
}

func withPerHostRequestsPerMinute(rateLimit int) startOption {
	return func(opts *startOptions) {
		opts.perHostConnectionsPerMinute = rateLimit
	}
}

func withNodeOpts(nodeOpts ...kit.NodeOpt) startOption {
	return func(opts *startOptions) {
		opts.nodeOpts = nodeOpts
	}
}

func startNodes(ctx context.Context, t *testing.T, opts ...startOption) *testNodes {
	options := newStartOptions(opts...)

	var (
		full  *kit.TestFullNode
		miner *kit.TestMiner
		lite  kit.TestFullNode
	)

	// - Create one full node and one lite node
	// - Put a gateway server in front of full node 1
	// - Start full node 2 in lite mode
	// - Connect lite node -> gateway server -> full node

	// create the full node and the miner.
	var ens *kit.Ensemble
	full, miner, ens = kit.EnsembleMinimal(t, kit.MockProofs())
	ens.InterconnectAll().BeginMining(options.blockTime)
	api.RunningNodeType = api.NodeFull

	// Create a gateway server in front of the full node
	v1EthSubHandler := gateway.NewEthSubHandler()
	v2EthSubHandler := gateway.NewEthSubHandler()
	gwapi := gateway.NewNode(
		full,
		full.V2,
		gateway.WithV1EthSubHandler(v1EthSubHandler),
		gateway.WithV2EthSubHandler(v2EthSubHandler),
		gateway.WithMaxLookbackDuration(options.lookbackCap),
		gateway.WithMaxMessageLookbackEpochs(options.maxMessageLookbackEpochs),
	)
	handler, err := gateway.Handler(
		gwapi,
		gateway.WithPerConnectionAPIRateLimit(options.perConnectionAPIRateLimit),
		gateway.WithPerHostConnectionsPerMinute(options.perHostConnectionsPerMinute),
	)
	t.Cleanup(func() { _ = handler.Shutdown(ctx) })
	require.NoError(t, err)

	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	srv, _, _ := kit.CreateRPCServer(t, handler, l)

	// Create a gateway client API that connects to the gateway server
	gapiv1, v1RpcCloser, err := client.NewGatewayRPCV1(ctx, "ws://"+srv.Listener.Addr().String()+"/rpc/v1", nil,
		jsonrpc.WithClientHandler("Filecoin", v1EthSubHandler),
		jsonrpc.WithClientHandlerAlias("eth_subscription", "Filecoin.EthSubscription"),
	)
	require.NoError(t, err)
	gapiv2, v2RpcCloser, err := client.NewGatewayRPCV2(ctx, "ws://"+srv.Listener.Addr().String()+"/rpc/v2", nil,
		jsonrpc.WithClientHandler("Filecoin", v2EthSubHandler),
		jsonrpc.WithClientHandlerAlias("eth_subscription", "Filecoin.EthSubscription"),
	)
	require.NoError(t, err)
	var closeOnce sync.Once
	closer := func() {
		closeOnce.Do(func() {
			v1RpcCloser()
			v2RpcCloser()
		})
	}
	t.Cleanup(closer)

	nodeOpts := append([]kit.NodeOpt{
		kit.LiteNode(),
		kit.ThroughRPC(),
		kit.ConstructorOpts(
			node.Override(new(api.Gateway), gapiv1),
			node.Override(new(v2api.Gateway), gapiv2),
		),
	}, options.nodeOpts...)
	ens.FullNode(&lite, nodeOpts...).Start().InterconnectAll()

	nodes := &testNodes{
		lite:        &lite,
		full:        full,
		miner:       miner,
		gatewayAddr: srv.Listener.Addr().String(),
		rpcCloser:   closer,
	}

	if options.fund {
		// The full node starts with a wallet
		fullWalletAddr, err := nodes.full.WalletDefaultAddress(ctx)
		require.NoError(t, err)

		// Get the lite node default wallet address.
		liteWalletAddr, err := nodes.lite.WalletDefaultAddress(ctx)
		require.NoError(t, err)

		// Send some funds from the full node to the lite node
		err = sendFunds(ctx, nodes.full, fullWalletAddr, liteWalletAddr, types.NewInt(1e18))
		require.NoError(t, err)
	}

	return nodes
}

func sendFunds(ctx context.Context, fromNode *kit.TestFullNode, fromAddr address.Address, toAddr address.Address, amt types.BigInt) error {
	msg := &types.Message{
		From:  fromAddr,
		To:    toAddr,
		Value: amt,
	}

	sm, err := fromNode.MpoolPushMessage(ctx, msg, nil)
	if err != nil {
		return err
	}

	res, err := fromNode.StateWaitMsg(ctx, sm.Cid(), 3, api.LookbackNoLimit, true)
	if err != nil {
		return err
	}
	if res.Receipt.ExitCode != 0 {
		return xerrors.Errorf("send funds failed with exit code %d", res.Receipt.ExitCode)
	}

	return nil
}

func TestGatewayRateLimits(t *testing.T) {
	t.Skip("this test is flaky and needs to be fixed")
	// Fails on the RequireDuration check, e.g. error like:
	// 	Max difference between 2025-02-10 12:05:34.63725116 +0000 UTC m=+30.240446844 and 2025-02-10 12:05:33.519935593 +0000 UTC m=+29.123131278 allowed is 800ms, but difference was 1.117315566s
	// There may be additional calls going through the API that only show up at random and these
	// aren't accounted for. See note below about paymentChannelSettler, which is one such call.
	// Tracking issue: https://github.com/filecoin-project/lotus/issues/12566

	req := require.New(t)

	kit.QuietMiningLogs()
	ctx := context.Background()
	tokensPerSecond := 10
	requestsPerMinute := 30 // http requests
	nodes := startNodes(ctx, t,
		withNodeOpts(kit.DisableEthRPC()),
		withPerConnectionAPIRateLimit(tokensPerSecond),
		withPerHostRequestsPerMinute(requestsPerMinute),
	)

	nodes.full.WaitTillChain(ctx, kit.HeightAtLeast(10)) // let's get going first

	// ChainHead uses chainRateLimitTokens=2.
	// But we're also competing with the paymentChannelSettler which listens to the chain uses
	// ChainGetBlockMessages on each change, which also uses chainRateLimitTokens=2. But because we're
	// rate limiting, it only gets a ChainGetBlockMessages in between our ChainHead calls, not on each
	// 5ms block (which it wants). So each loop should be 4 tokens.
	loops := 25
	tokensPerLoop := 4
	start := time.Now()
	for i := 0; i < loops; i++ {
		_, err := nodes.lite.ChainHead(ctx)
		req.NoError(err)
	}
	tokensUsed := loops * tokensPerLoop
	expectedEnd := start.Add(time.Duration(float64(tokensUsed) / float64(tokensPerSecond) * float64(time.Second)))
	allowPad := time.Duration(float64(tokensPerLoop*2) / float64(tokensPerSecond) * float64(time.Second)) // add padding to account for slow test runs
	t.Logf("expected end: %s, now: %s, allowPad: %s, actual delta: %s", expectedEnd, time.Now(), allowPad, time.Since(expectedEnd))
	req.WithinDuration(expectedEnd, time.Now(), allowPad)

	client := &http.Client{}
	jsonPayload := []byte(`{"method":"Filecoin.ChainHead","params":[],"id":1,"jsonrpc":"2.0"}`)
	var failed bool
	for i := 0; i < requestsPerMinute*2 && !failed; i++ {
		status, body := makeManualRpcCall(t, 1, client, nodes.gatewayAddr, string(jsonPayload))
		if http.StatusOK == status {
			result := map[string]interface{}{}
			req.NoError(json.Unmarshal([]byte(body), &result))
			req.NotNil(result["result"])
			height, ok := result["result"].(map[string]interface{})["Height"].(float64)
			req.True(ok)
			req.Greater(int(height), 0)
		} else {
			req.Equal(http.StatusTooManyRequests, status)
			req.LessOrEqual(i, requestsPerMinute+1)
			failed = true
		}
	}
	req.True(failed, "expected requests to fail due to rate limiting")
}

func makeManualRpcCall(t *testing.T, version int, client *http.Client, gatewayAddr, payload string) (int, string) {
	// not available over plain http
	url := fmt.Sprintf("http://%s/rpc/v%d", gatewayAddr, version)
	request, err := http.NewRequest("POST", url, bytes.NewBuffer([]byte(payload)))
	require.NoError(t, err)
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	require.NoError(t, err)
	defer func() { _ = response.Body.Close() }()
	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	return response.StatusCode, string(body)
}

func TestStatefulCallHandling(t *testing.T) {
	req := require.New(t)

	kit.QuietMiningLogs()
	ctx := context.Background()
	nodes := startNodes(ctx, t)

	t.Logf("Testing stateful call handling rejection via plain http")
	for version := range 3 {
		for _, typ := range []string{
			"EthNewBlockFilter",
			"EthNewPendingTransactionFilter",
			"EthNewFilter",
			"EthGetFilterChanges",
			"EthGetFilterLogs",
			"EthUninstallFilter",
			"EthSubscribe",
			"EthUnsubscribe",
		} {
			params := ""
			expErr := typ + " not supported: stateful methods are only available on websocket connections"

			switch typ {
			case "EthNewFilter":
				params = "{}"
			case "EthGetFilterChanges", "EthGetFilterLogs", "EthUninstallFilter", "EthUnsubscribe":
				params = `"0x0000000000000000000000000000000000000000000000000000000000000000"`
			case "EthSubscribe":
				params = `"newHeads"`
				expErr = "EthSubscribe not supported: connection doesn't support callbacks"
			}

			status, body := makeManualRpcCall(
				t,
				version,
				&http.Client{},
				nodes.gatewayAddr,
				`{"method":"Filecoin.`+typ+`","params":[`+params+`],"id":1,"jsonrpc":"2.0"}`,
			)

			req.Equal(http.StatusOK, status, "not ok for "+typ)
			req.Contains(body, `{"error":{"code":1,"message":"`+expErr+`"},"id":1,"jsonrpc":"2.0"}`, "unexpected response for "+typ)
		}
	}

	t.Logf("Testing subscriptions")
	// subscribe twice, so we can unsub one over ws to check unsub works, then unsub after ws close to
	// check that auto-cleanup happened
	subId1, err := nodes.lite.EthSubscribe(ctx, res.Wrap[jsonrpc.RawParams](json.Marshal(ethtypes.EthSubscribeParams{EventType: "newHeads"})).Assert(req.NoError))
	req.NoError(err)
	err = nodes.lite.EthSubRouter.AddSub(ctx, subId1, func(ctx context.Context, resp *ethtypes.EthSubscriptionResponse) error {
		t.Logf("Received subscription response (sub1): %v", resp)
		return nil
	})
	req.NoError(err)
	subId2, err := nodes.lite.EthSubscribe(ctx, res.Wrap[jsonrpc.RawParams](json.Marshal(ethtypes.EthSubscribeParams{EventType: "newHeads"})).Assert(req.NoError))
	req.NoError(err)
	err = nodes.lite.EthSubRouter.AddSub(ctx, subId2, func(ctx context.Context, resp *ethtypes.EthSubscriptionResponse) error {
		t.Logf("Received subscription response (sub2): %v", resp)
		return nil
	})
	req.NoError(err)

	ok, err := nodes.lite.EthUnsubscribe(ctx, subId1) // unsub on lite node, should work
	req.NoError(err)
	req.True(ok)
	ok, err = nodes.full.EthUnsubscribe(ctx, subId1) // unsub on full node, already done
	req.NoError(err)
	req.False(ok)

	t.Logf("Installing a stateful filters via ws")
	// install the variety of stateful filters we have, but only up to the max total
	var (
		blockFilterIds   = make([]ethtypes.EthFilterID, gateway.DefaultEthMaxFiltersPerConn/3)
		pendingFilterIds = make([]ethtypes.EthFilterID, gateway.DefaultEthMaxFiltersPerConn/3)
		// matchFilterIds takes up the remainder, minus 1 because we still have 1 live subscription that counts
		matchFilterIds = make([]ethtypes.EthFilterID, gateway.DefaultEthMaxFiltersPerConn-len(blockFilterIds)-len(pendingFilterIds)-1)
	)
	for i := 0; i < len(blockFilterIds); i++ {
		fid, err := nodes.lite.EthNewBlockFilter(ctx)
		req.NoError(err)
		blockFilterIds[i] = fid
	}
	for i := 0; i < len(pendingFilterIds); i++ {
		fid, err := nodes.lite.EthNewPendingTransactionFilter(ctx)
		req.NoError(err)
		pendingFilterIds[i] = fid
	}
	for i := 0; i < len(matchFilterIds); i++ {
		fid, err := nodes.lite.EthNewFilter(ctx, &ethtypes.EthFilterSpec{})
		req.NoError(err)
		matchFilterIds[i] = fid
	}

	// sanity check we're actually doing something
	req.Greater(len(blockFilterIds), 0)
	req.Greater(len(pendingFilterIds), 0)
	req.Greater(len(matchFilterIds), 0)

	t.Logf("Testing 'too many filters' rejection")
	_, err = nodes.lite.EthNewBlockFilter(ctx)
	require.ErrorContains(t, err, "too many subscriptions and filters per connection")
	_, err = nodes.lite.EthNewPendingTransactionFilter(ctx)
	require.ErrorContains(t, err, "too many subscriptions and filters per connection")
	_, err = nodes.lite.EthNewFilter(ctx, &ethtypes.EthFilterSpec{})
	require.ErrorContains(t, err, "too many subscriptions and filters per connection")
	_, err = nodes.lite.EthSubscribe(ctx, res.Wrap[jsonrpc.RawParams](json.Marshal(ethtypes.EthSubscribeParams{EventType: "newHeads"})).Assert(req.NoError))
	require.ErrorContains(t, err, "too many subscriptions and filters per connection")

	t.Logf("Shutting down the lite node")
	req.NoError(nodes.lite.Stop(ctx))

	nodes.rpcCloser()
	// Once the websocket connection is closed, the server should clean up the filters for us.
	// Unfortunately we have no other way to check for completeness of shutdown and cleanup.
	// * Asynchronously the rpcCloser() call will end the client websockets connection to the gateway.
	// * When the gateway recognises the end of the HTTP connection, it will asynchronously make calls
	//   to the fullnode to clean up the filters.
	// * The fullnode will then uninstall the filters and we can finally move on to check it directly
	//   that this has happened.
	// This should happen quickly, but we have no way to synchronously check for it. So we just wait a bit.
	time.Sleep(time.Second)

	t.Logf("Checking that all filters and subs were cleared up by directly talking to full node")

	ok, err = nodes.full.EthUnsubscribe(ctx, subId2) // unsub on full node, already done
	req.NoError(err)
	req.False(ok) // already unsubscribed because of auto-cleanup

	for _, fid := range blockFilterIds {
		_, err = nodes.full.EthGetFilterChanges(ctx, fid)
		req.ErrorContains(err, "filter not found")
	}
	for _, fid := range pendingFilterIds {
		_, err = nodes.full.EthGetFilterChanges(ctx, fid)
		req.ErrorContains(err, "filter not found")
	}
	for _, fid := range matchFilterIds {
		_, err = nodes.full.EthGetFilterChanges(ctx, fid)
		req.ErrorContains(err, "filter not found")
	}
}

func TestGatewayF3(t *testing.T) {
	// Test that disabled & not-running F3 calls properly error

	kit.QuietMiningLogs()

	t.Run("not running", func(t *testing.T) {
		ctx := context.Background()
		nodes := startNodes(ctx, t)

		cert, err := nodes.lite.F3GetLatestCertificate(ctx)
		require.ErrorContains(t, err, f3.ErrF3NotRunning.Error())
		require.Nil(t, cert)

		cert, err = nodes.lite.F3GetCertificate(ctx, 2)
		require.ErrorContains(t, err, f3.ErrF3NotRunning.Error())
		require.Nil(t, cert)
	})

	t.Run("disabled", func(t *testing.T) {
		ctx := context.Background()
		nodes := startNodes(ctx, t, withNodeOpts(kit.F3Disabled()))

		cert, err := nodes.lite.F3GetLatestCertificate(ctx)
		require.ErrorIs(t, err, api.ErrF3Disabled)
		require.Nil(t, cert)

		cert, err = nodes.lite.F3GetCertificate(ctx, 2)
		require.ErrorIs(t, err, api.ErrF3Disabled)
		require.Nil(t, cert)
	})
}
