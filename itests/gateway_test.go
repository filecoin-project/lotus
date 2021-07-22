package itests

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"net"
	"testing"
	"time"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-jsonrpc"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/api/client"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/cli"
	"github.com/filecoin-project/lotus/gateway"
	"github.com/filecoin-project/lotus/itests/kit"
	"github.com/filecoin-project/lotus/itests/multisig"
	"github.com/filecoin-project/lotus/node"

	init2 "github.com/filecoin-project/specs-actors/v2/actors/builtin/init"
	multisig2 "github.com/filecoin-project/specs-actors/v2/actors/builtin/multisig"

	"github.com/ipfs/go-cid"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"
)

const (
	maxLookbackCap            = time.Duration(math.MaxInt64)
	maxStateWaitLookbackLimit = stmgr.LookbackNoLimit
)

// TestGatewayWalletMsig tests that API calls to wallet and msig can be made on a lite
// node that is connected through a gateway to a full API node
func TestGatewayWalletMsig(t *testing.T) {
	kit.QuietMiningLogs()

	blocktime := 5 * time.Millisecond
	ctx := context.Background()
	nodes := startNodes(ctx, t, blocktime, maxLookbackCap, maxStateWaitLookbackLimit)

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
	require.Greater(t, msigBalance.Int64(), int64(0))
	require.Less(t, msigBalance.Int64(), amt.Int64())

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

	blocktime := 5 * time.Millisecond
	ctx := context.Background()
	nodes := startNodesWithFunds(ctx, t, blocktime, maxLookbackCap, maxStateWaitLookbackLimit)

	lite := nodes.lite
	multisig.RunMultisigTests(t, lite)
}

func TestGatewayDealFlow(t *testing.T) {
	kit.QuietMiningLogs()

	blocktime := 5 * time.Millisecond
	ctx := context.Background()
	nodes := startNodesWithFunds(ctx, t, blocktime, maxLookbackCap, maxStateWaitLookbackLimit)

	time.Sleep(5 * time.Second)

	// For these tests where the block time is artificially short, just use
	// a deal start epoch that is guaranteed to be far enough in the future
	// so that the deal starts sealing in time
	dealStartEpoch := abi.ChainEpoch(2 << 12)

	dh := kit.NewDealHarness(t, nodes.lite, nodes.miner, nodes.miner)
	dealCid, res, _ := dh.MakeOnlineDeal(context.Background(), kit.MakeFullDealParams{
		Rseed:      6,
		StartEpoch: dealStartEpoch,
	})
	dh.PerformRetrieval(ctx, dealCid, res.Root, false)
}

func TestGatewayCLIDealFlow(t *testing.T) {
	kit.QuietMiningLogs()

	blocktime := 5 * time.Millisecond
	ctx := context.Background()
	nodes := startNodesWithFunds(ctx, t, blocktime, maxLookbackCap, maxStateWaitLookbackLimit)

	kit.RunClientTest(t, cli.Commands, nodes.lite)
}

type testNodes struct {
	lite  *kit.TestFullNode
	full  *kit.TestFullNode
	miner *kit.TestMiner
}

func startNodesWithFunds(
	ctx context.Context,
	t *testing.T,
	blocktime time.Duration,
	lookbackCap time.Duration,
	stateWaitLookbackLimit abi.ChainEpoch,
) *testNodes {
	nodes := startNodes(ctx, t, blocktime, lookbackCap, stateWaitLookbackLimit)

	// The full node starts with a wallet
	fullWalletAddr, err := nodes.full.WalletDefaultAddress(ctx)
	require.NoError(t, err)

	// Get the lite node default wallet address.
	liteWalletAddr, err := nodes.lite.WalletDefaultAddress(ctx)
	require.NoError(t, err)

	// Send some funds from the full node to the lite node
	err = sendFunds(ctx, nodes.full, fullWalletAddr, liteWalletAddr, types.NewInt(1e18))
	require.NoError(t, err)

	return nodes
}

func startNodes(
	ctx context.Context,
	t *testing.T,
	blocktime time.Duration,
	lookbackCap time.Duration,
	stateWaitLookbackLimit abi.ChainEpoch,
) *testNodes {
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
	ens.InterconnectAll().BeginMining(blocktime)

	// Create a gateway server in front of the full node
	gwapi := gateway.NewNode(full, lookbackCap, stateWaitLookbackLimit)
	handler, err := gateway.Handler(gwapi)
	require.NoError(t, err)

	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	srv, _ := kit.CreateRPCServer(t, handler, l)

	// Create a gateway client API that connects to the gateway server
	var gapi api.Gateway
	gapi, closer, err = client.NewGatewayRPCV1(ctx, "ws://"+srv.Listener.Addr().String()+"/rpc/v1", nil)
	require.NoError(t, err)
	t.Cleanup(closer)

	ens.FullNode(&lite,
		kit.LiteNode(),
		kit.ThroughRPC(),
		kit.ConstructorOpts(
			node.Override(new(api.Gateway), gapi),
		),
	).Start().InterconnectAll()

	return &testNodes{lite: &lite, full: full, miner: miner}
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
