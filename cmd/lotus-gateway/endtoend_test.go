package main

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"os"
	"testing"
	"time"

	"github.com/filecoin-project/lotus/cli"
	clitest "github.com/filecoin-project/lotus/cli/test"

	init2 "github.com/filecoin-project/specs-actors/v2/actors/builtin/init"
	multisig2 "github.com/filecoin-project/specs-actors/v2/actors/builtin/multisig"

	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-jsonrpc"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/api/client"
	"github.com/filecoin-project/lotus/api/test"
	"github.com/filecoin-project/lotus/api/v0api"
	"github.com/filecoin-project/lotus/api/v1api"
	"github.com/filecoin-project/lotus/chain/actors/policy"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/node"
	builder "github.com/filecoin-project/lotus/node/test"
)

const maxLookbackCap = time.Duration(math.MaxInt64)
const maxStateWaitLookbackLimit = stmgr.LookbackNoLimit

func init() {
	policy.SetSupportedProofTypes(abi.RegisteredSealProof_StackedDrg2KiBV1)
	policy.SetConsensusMinerMinPower(abi.NewStoragePower(2048))
	policy.SetMinVerifiedDealSize(abi.NewStoragePower(256))
}

// TestWalletMsig tests that API calls to wallet and msig can be made on a lite
// node that is connected through a gateway to a full API node
func TestWalletMsig(t *testing.T) {
	_ = os.Setenv("BELLMAN_NO_GPU", "1")
	clitest.QuietMiningLogs()

	blocktime := 5 * time.Millisecond
	ctx := context.Background()
	nodes := startNodes(ctx, t, blocktime, maxLookbackCap, maxStateWaitLookbackLimit)
	defer nodes.closer()

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
	addProposal, err := lite.MsigCreate(ctx, 2, msigAddrs, abi.ChainEpoch(50), amt, liteWalletAddr, types.NewInt(0))
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
	addProposal, err = lite.MsigAddPropose(ctx, msig, walletAddrs[0], walletAddrs[3], false)
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
	approval1, err := lite.MsigAddApprove(ctx, msig, walletAddrs[1], txnID, walletAddrs[0], walletAddrs[3], false)
	require.NoError(t, err)

	res, err = lite.StateWaitMsg(ctx, approval1, 1, api.LookbackNoLimit, true)
	require.NoError(t, err)
	require.EqualValues(t, 0, res.Receipt.ExitCode)

	var approveReturn multisig2.ApproveReturn
	err = approveReturn.UnmarshalCBOR(bytes.NewReader(res.Receipt.Return))
	require.NoError(t, err)
	require.True(t, approveReturn.Applied)
}

// TestMsigCLI tests that msig CLI calls can be made
// on a lite node that is connected through a gateway to a full API node
func TestMsigCLI(t *testing.T) {
	_ = os.Setenv("BELLMAN_NO_GPU", "1")
	clitest.QuietMiningLogs()

	blocktime := 5 * time.Millisecond
	ctx := context.Background()
	nodes := startNodesWithFunds(ctx, t, blocktime, maxLookbackCap, maxStateWaitLookbackLimit)
	defer nodes.closer()

	lite := nodes.lite
	clitest.RunMultisigTest(t, cli.Commands, lite)
}

func TestDealFlow(t *testing.T) {
	_ = os.Setenv("BELLMAN_NO_GPU", "1")
	clitest.QuietMiningLogs()

	blocktime := 5 * time.Millisecond
	ctx := context.Background()
	nodes := startNodesWithFunds(ctx, t, blocktime, maxLookbackCap, maxStateWaitLookbackLimit)
	defer nodes.closer()

	// For these tests where the block time is artificially short, just use
	// a deal start epoch that is guaranteed to be far enough in the future
	// so that the deal starts sealing in time
	dealStartEpoch := abi.ChainEpoch(2 << 12)
	test.MakeDeal(t, ctx, 6, nodes.lite, nodes.miner, false, false, dealStartEpoch)
}

func TestCLIDealFlow(t *testing.T) {
	_ = os.Setenv("BELLMAN_NO_GPU", "1")
	clitest.QuietMiningLogs()

	blocktime := 5 * time.Millisecond
	ctx := context.Background()
	nodes := startNodesWithFunds(ctx, t, blocktime, maxLookbackCap, maxStateWaitLookbackLimit)
	defer nodes.closer()

	clitest.RunClientTest(t, cli.Commands, nodes.lite)
}

type testNodes struct {
	lite   test.TestNode
	full   test.TestNode
	miner  test.TestStorageNode
	closer jsonrpc.ClientCloser
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

	// Create a wallet on the lite node
	liteWalletAddr, err := nodes.lite.WalletNew(ctx, types.KTSecp256k1)
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

	// Create one miner and two full nodes.
	// - Put a gateway server in front of full node 1
	// - Start full node 2 in lite mode
	// - Connect lite node -> gateway server -> full node
	opts := append(
		// Full node
		test.OneFull,
		// Lite node
		test.FullNodeOpts{
			Lite: true,
			Opts: func(nodes []test.TestNode) node.Option {
				fullNode := nodes[0]

				// Create a gateway server in front of the full node
				gapiImpl := newGatewayAPI(fullNode, lookbackCap, stateWaitLookbackLimit)
				_, addr, err := builder.CreateRPCServer(t, map[string]interface{}{
					"/rpc/v1": gapiImpl,
					"/rpc/v0": api.Wrap(new(v1api.FullNodeStruct), new(v0api.WrapperV1Full), gapiImpl),
				})
				require.NoError(t, err)

				// Create a gateway client API that connects to the gateway server
				var gapi api.Gateway
				gapi, closer, err = client.NewGatewayRPCV1(ctx, addr+"/rpc/v1", nil)
				require.NoError(t, err)

				// Provide the gateway API to dependency injection
				return node.Override(new(api.Gateway), gapi)
			},
		},
	)
	n, sn := builder.RPCMockSbBuilder(t, opts, test.OneMiner)

	full := n[0]
	lite := n[1]
	miner := sn[0]

	// Get the listener address for the full node
	fullAddr, err := full.NetAddrsListen(ctx)
	require.NoError(t, err)

	// Connect the miner and the full node
	err = miner.NetConnect(ctx, fullAddr)
	require.NoError(t, err)

	// Connect the miner and the lite node (so that the lite node can send
	// data to the miner)
	liteAddr, err := lite.NetAddrsListen(ctx)
	require.NoError(t, err)
	err = miner.NetConnect(ctx, liteAddr)
	require.NoError(t, err)

	// Start mining blocks
	bm := test.NewBlockMiner(ctx, t, miner, blocktime)
	bm.MineBlocks()
	t.Cleanup(bm.Stop)

	return &testNodes{lite: lite, full: full, miner: miner, closer: closer}
}

func sendFunds(ctx context.Context, fromNode test.TestNode, fromAddr address.Address, toAddr address.Address, amt types.BigInt) error {
	msg := &types.Message{
		From:  fromAddr,
		To:    toAddr,
		Value: amt,
	}

	sm, err := fromNode.MpoolPushMessage(ctx, msg, nil)
	if err != nil {
		return err
	}

	res, err := fromNode.StateWaitMsg(ctx, sm.Cid(), 1, api.LookbackNoLimit, true)
	if err != nil {
		return err
	}
	if res.Receipt.ExitCode != 0 {
		return xerrors.Errorf("send funds failed with exit code %d", res.Receipt.ExitCode)
	}

	return nil
}
