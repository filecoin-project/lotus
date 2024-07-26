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
	"github.com/filecoin-project/lotus/gateway"
	"github.com/filecoin-project/lotus/itests/kit"
	"github.com/filecoin-project/lotus/itests/multisig"
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

	var closer jsonrpc.ClientCloser

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
	gwapi := gateway.NewNode(full, nil, options.lookbackCap, options.stateWaitLookbackLimit, 0, time.Minute)
	handler, err := gateway.Handler(gwapi, full, options.perConnectionAPIRateLimit, options.perHostRequestsPerMinute)
	t.Cleanup(func() { _ = handler.Shutdown(ctx) })
	require.NoError(t, err)

	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	srv, _, _ := kit.CreateRPCServer(t, handler, l)

	// Create a gateway client API that connects to the gateway server
	var gapi api.Gateway
	gapi, closer, err = client.NewGatewayRPCV1(ctx, "ws://"+srv.Listener.Addr().String()+"/rpc/v1", nil)
	require.NoError(t, err)
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
			req.NoError(err)
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
