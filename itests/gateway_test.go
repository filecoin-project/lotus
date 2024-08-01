// stm: #integration
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
	"github.com/filecoin-project/go-jsonrpc"
	"github.com/filecoin-project/go-state-types/abi"
	init2 "github.com/filecoin-project/specs-actors/v2/actors/builtin/init"
	multisig2 "github.com/filecoin-project/specs-actors/v2/actors/builtin/multisig"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/api/client"
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
	maxLookbackCap            = time.Duration(math.MaxInt64)
	maxStateWaitLookbackLimit = stmgr.LookbackNoLimit
)

// TestGatewayWalletMsig tests that API calls to wallet and msig can be made on a lite
// node that is connected through a gateway to a full API node
func TestGatewayWalletMsig(t *testing.T) {
	//stm: @CHAIN_SYNCER_LOAD_GENESIS_001, @CHAIN_SYNCER_FETCH_TIPSET_001,
	//stm: @CHAIN_SYNCER_START_001, @CHAIN_SYNCER_SYNC_001, @BLOCKCHAIN_BEACON_VALIDATE_BLOCK_VALUES_01
	//stm: @CHAIN_SYNCER_COLLECT_CHAIN_001, @CHAIN_SYNCER_COLLECT_HEADERS_001, @CHAIN_SYNCER_VALIDATE_TIPSET_001
	//stm: @CHAIN_SYNCER_NEW_PEER_HEAD_001, @CHAIN_SYNCER_VALIDATE_MESSAGE_META_001, @CHAIN_SYNCER_STOP_001

	//stm: @CHAIN_INCOMING_HANDLE_INCOMING_BLOCKS_001, @CHAIN_INCOMING_VALIDATE_BLOCK_PUBSUB_001, @CHAIN_INCOMING_VALIDATE_MESSAGE_PUBSUB_001
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

	//stm: @CHAIN_STATE_WAIT_MSG_001
	res, err := lite.StateWaitMsg(ctx, addProposal, 1, api.LookbackNoLimit, true)
	require.NoError(t, err)
	require.EqualValues(t, 0, res.Receipt.ExitCode)

	var execReturn init2.ExecReturn
	err = execReturn.UnmarshalCBOR(bytes.NewReader(res.Receipt.Return))
	require.NoError(t, err)

	// Get available balance of msig: should be greater than zero and less
	// than initial amount
	msig := execReturn.IDAddress
	//stm: @CHAIN_STATE_MINER_AVAILABLE_BALANCE_001
	msigBalance, err := lite.MsigGetAvailableBalance(ctx, msig, types.EmptyTSK)
	require.NoError(t, err)
	require.GreaterOrEqual(t, msigBalance.Int64(), int64(0))
	require.LessOrEqual(t, msigBalance.Int64(), amt.Int64())

	// Propose to add a new address to the msig
	proto, err = lite.MsigAddPropose(ctx, msig, walletAddrs[0], walletAddrs[3], false)
	require.NoError(t, err)

	addProposal, err = doSend(proto)
	require.NoError(t, err)

	//stm: @CHAIN_STATE_WAIT_MSG_001
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

	//stm: @CHAIN_STATE_WAIT_MSG_001
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
	//stm: @CHAIN_SYNCER_LOAD_GENESIS_001, @CHAIN_SYNCER_FETCH_TIPSET_001,
	//stm: @CHAIN_SYNCER_START_001, @CHAIN_SYNCER_SYNC_001, @BLOCKCHAIN_BEACON_VALIDATE_BLOCK_VALUES_01
	//stm: @CHAIN_SYNCER_COLLECT_CHAIN_001, @CHAIN_SYNCER_COLLECT_HEADERS_001, @CHAIN_SYNCER_VALIDATE_TIPSET_001
	//stm: @CHAIN_SYNCER_NEW_PEER_HEAD_001, @CHAIN_SYNCER_VALIDATE_MESSAGE_META_001, @CHAIN_SYNCER_STOP_001
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
	blocktime                 time.Duration
	lookbackCap               time.Duration
	stateWaitLookbackLimit    abi.ChainEpoch
	fund                      bool
	perConnectionAPIRateLimit int
	perHostRequestsPerMinute  int
	nodeOpts                  []kit.NodeOpt
}

type startOption func(*startOptions)

func applyStartOptions(opts ...startOption) startOptions {
	o := startOptions{
		blocktime:              5 * time.Millisecond,
		lookbackCap:            maxLookbackCap,
		stateWaitLookbackLimit: maxStateWaitLookbackLimit,
		fund:                   false,
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
		opts.perHostRequestsPerMinute = rateLimit
	}
}

func withNodeOpts(nodeOpts ...kit.NodeOpt) startOption {
	return func(opts *startOptions) {
		opts.nodeOpts = nodeOpts
	}
}

func startNodes(ctx context.Context, t *testing.T, opts ...startOption) *testNodes {
	options := applyStartOptions(opts...)

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
	ens.InterconnectAll().BeginMining(options.blocktime)
	api.RunningNodeType = api.NodeFull

	// Create a gateway server in front of the full node
	ethSubHandler := gateway.NewEthSubHandler()
	gwapi := gateway.NewNode(full, ethSubHandler, options.lookbackCap, options.stateWaitLookbackLimit, 0, time.Minute)
	handler, err := gateway.Handler(gwapi, full, options.perConnectionAPIRateLimit, options.perHostRequestsPerMinute)
	t.Cleanup(func() { _ = handler.Shutdown(ctx) })
	require.NoError(t, err)

	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	srv, _, _ := kit.CreateRPCServer(t, handler, l)

	// Create a gateway client API that connects to the gateway server
	var gapi api.Gateway
	var _closer jsonrpc.ClientCloser
	gapi, _closer, err = client.NewGatewayRPCV1(ctx, "ws://"+srv.Listener.Addr().String()+"/rpc/v1", nil,
		jsonrpc.WithClientHandler("Filecoin", ethSubHandler),
		jsonrpc.WithClientHandlerAlias("eth_subscription", "Filecoin.EthSubscription"),
	)
	require.NoError(t, err)
	var closeOnce sync.Once
	closer := func() { closeOnce.Do(_closer) }
	t.Cleanup(closer)

	nodeOpts := append([]kit.NodeOpt{
		kit.LiteNode(),
		kit.ThroughRPC(),
		kit.ConstructorOpts(
			node.Override(new(api.Gateway), gapi),
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

	time.Sleep(time.Second)

	// ChainHead uses chainRateLimitTokens=2.
	// But we're also competing with the paymentChannelSettler which listens to the chain uses
	// ChainGetBlockMessages on each change, which also uses chainRateLimitTokens=2.
	// So each loop should be 4 tokens.
	loops := 10
	tokensPerLoop := 4
	start := time.Now()
	for i := 0; i < loops; i++ {
		_, err := nodes.lite.ChainHead(ctx)
		req.NoError(err)
	}
	tokensUsed := loops * tokensPerLoop
	expectedEnd := start.Add(time.Duration(float64(tokensUsed) / float64(tokensPerSecond) * float64(time.Second)))
	allowPad := time.Duration(float64(tokensPerLoop) / float64(tokensPerSecond) * float64(time.Second)) // add padding to account for slow test runs
	t.Logf("expected end: %s, now: %s, allowPad: %s, actual delta: %s", expectedEnd, time.Now(), allowPad, time.Since(expectedEnd))
	req.WithinDuration(expectedEnd, time.Now(), allowPad)

	client := &http.Client{}
	url := fmt.Sprintf("http://%s/rpc/v1", nodes.gatewayAddr)
	jsonPayload := []byte(`{"method":"Filecoin.ChainHead","params":[],"id":1,"jsonrpc":"2.0"}`)
	var failed bool
	for i := 0; i < requestsPerMinute*2 && !failed; i++ {
		func() {
			request, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonPayload))
			req.NoError(err)
			request.Header.Set("Content-Type", "application/json")
			response, err := client.Do(request)
			req.NoError(err)
			defer func() { _ = response.Body.Close() }()
			if http.StatusOK == response.StatusCode {
				body, err := io.ReadAll(response.Body)
				req.NoError(err)
				result := map[string]interface{}{}
				req.NoError(json.Unmarshal(body, &result))
				req.NoError(err)
				req.NotNil(result["result"])
				height, ok := result["result"].(map[string]interface{})["Height"].(float64)
				req.True(ok)
				req.Greater(int(height), 0)
			} else {
				req.Equal(http.StatusTooManyRequests, response.StatusCode)
				req.LessOrEqual(i, requestsPerMinute+1)
				failed = true
			}
		}()
	}
	req.True(failed, "expected requests to fail due to rate limiting")
}

func TestStatefulCallHandling(t *testing.T) {
	req := require.New(t)

	kit.QuietMiningLogs()
	ctx := context.Background()
	nodes := startNodes(ctx, t)

	httpReq := func(payload string) (int, string) {
		// not available over plain http
		client := &http.Client{}
		url := fmt.Sprintf("http://%s/rpc/v1", nodes.gatewayAddr)
		request, err := http.NewRequest("POST", url, bytes.NewBuffer([]byte(payload)))
		req.NoError(err)
		request.Header.Set("Content-Type", "application/json")
		response, err := client.Do(request)
		req.NoError(err)
		defer func() { _ = response.Body.Close() }()
		body, err := io.ReadAll(response.Body)
		req.NoError(err)
		return response.StatusCode, string(body)
	}

	t.Logf("Testing stateful call handling rejection via plain http")
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
		expErr := typ + " not supported: stateful methods are only available on websockets connections"

		switch typ {
		case "EthNewFilter":
			params = "{}"
		case "EthGetFilterChanges", "EthGetFilterLogs", "EthUninstallFilter", "EthUnsubscribe":
			params = `"0x0000000000000000000000000000000000000000000000000000000000000000"`
		case "EthSubscribe":
			params = `"newHeads"`
			expErr = "EthSubscribe not supported: connection doesn't support callbacks"
		}

		status, body := httpReq(`{"method":"Filecoin.` + typ + `","params":[` + params + `],"id":1,"jsonrpc":"2.0"}`)

		req.Equal(http.StatusOK, status, "not ok for "+typ)
		req.Contains(body, `{"error":{"code":1,"message":"`+expErr+`"},"id":1,"jsonrpc":"2.0"}`, "unexpected response for "+typ)
	}

	t.Logf("Installing a stateful filters via ws")
	// install the variety of stateful filters we have, but only up to the max total
	var (
		blockFilterIds   = make([]ethtypes.EthFilterID, gateway.EthMaxFiltersPerConn/3)
		pendingFilterIds = make([]ethtypes.EthFilterID, gateway.EthMaxFiltersPerConn/3)
		matchFilterIds   = make([]ethtypes.EthFilterID, gateway.EthMaxFiltersPerConn-len(blockFilterIds)-len(pendingFilterIds))
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
	_, err := nodes.lite.EthNewBlockFilter(ctx)
	require.ErrorContains(t, err, "too many filters")
	_, err = nodes.lite.EthNewPendingTransactionFilter(ctx)
	require.ErrorContains(t, err, "too many filters")
	_, err = nodes.lite.EthNewFilter(ctx, &ethtypes.EthFilterSpec{})
	require.ErrorContains(t, err, "too many filters")

	t.Logf("Testing subscriptions")
	// subscribe twice, so we can unsub one over ws to check unsub works, then unsub after ws close to
	// check that auto-cleanup happned
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

	t.Logf("Shutting down the lite node")
	req.NoError(nodes.lite.Stop(ctx))
	nodes.rpcCloser() // once the websocket connection is closed, the server should clean up the filters for us

	time.Sleep(time.Second) // unfortunately we have no other way to check for completeness of shutdown and cleanup

	t.Logf("Checking that all filters and subs were cleared up by directly talking to full node")

	ok, err = nodes.full.EthUnsubscribe(ctx, subId2) // unsub on full node, already done
	req.NoError(err)
	req.False(ok) // already unsubbed because of auto-cleanup

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
