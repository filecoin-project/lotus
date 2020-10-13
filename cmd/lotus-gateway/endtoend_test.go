package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	init0 "github.com/filecoin-project/specs-actors/actors/builtin/init"
	"github.com/filecoin-project/specs-actors/actors/builtin/multisig"

	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-jsonrpc"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/api/client"
	"github.com/filecoin-project/lotus/api/test"
	"github.com/filecoin-project/lotus/chain/actors/policy"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/node"
	builder "github.com/filecoin-project/lotus/node/test"
)

func init() {
	policy.SetSupportedProofTypes(abi.RegisteredSealProof_StackedDrg2KiBV1)
	policy.SetConsensusMinerMinPower(abi.NewStoragePower(2048))
	policy.SetMinVerifiedDealSize(abi.NewStoragePower(256))
}

// TestEndToEnd tests that API calls can be made on a lite node that is
// connected through a gateway to a full API node
func TestEndToEnd(t *testing.T) {
	_ = os.Setenv("BELLMAN_NO_GPU", "1")

	blocktime := 5 * time.Millisecond
	ctx := context.Background()
	full, lite, closer := startNodes(ctx, t, blocktime)
	defer closer()

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
	err = sendFunds(ctx, t, full, fullWalletAddr, liteWalletAddr, types.NewInt(1e18))
	require.NoError(t, err)

	// Send some funds from the lite node back to the full node
	err = sendFunds(ctx, t, lite, liteWalletAddr, fullWalletAddr, types.NewInt(100))
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

		err = sendFunds(ctx, t, lite, liteWalletAddr, addr, types.NewInt(1e15))
		require.NoError(t, err)
	}

	// Create an msig with three of the addresses and threshold of two sigs
	msigAddrs := walletAddrs[:3]
	amt := types.NewInt(1000)
	addProposal, err := lite.MsigCreate(ctx, 2, msigAddrs, abi.ChainEpoch(50), amt, liteWalletAddr, types.NewInt(0))
	require.NoError(t, err)

	res, err := lite.StateWaitMsg(ctx, addProposal, 1)
	require.NoError(t, err)
	require.EqualValues(t, 0, res.Receipt.ExitCode)

	var execReturn init0.ExecReturn
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

	res, err = lite.StateWaitMsg(ctx, addProposal, 1)
	require.NoError(t, err)
	require.EqualValues(t, 0, res.Receipt.ExitCode)

	var proposeReturn multisig.ProposeReturn
	err = proposeReturn.UnmarshalCBOR(bytes.NewReader(res.Receipt.Return))
	require.NoError(t, err)

	// Approve proposal (proposer is first (implicit) signer, approver is
	// second signer
	txnID := uint64(proposeReturn.TxnID)
	approval1, err := lite.MsigAddApprove(ctx, msig, walletAddrs[1], txnID, walletAddrs[0], walletAddrs[3], false)
	require.NoError(t, err)

	res, err = lite.StateWaitMsg(ctx, approval1, 1)
	require.NoError(t, err)
	require.EqualValues(t, 0, res.Receipt.ExitCode)

	var approveReturn multisig.ApproveReturn
	err = approveReturn.UnmarshalCBOR(bytes.NewReader(res.Receipt.Return))
	require.NoError(t, err)
	require.True(t, approveReturn.Applied)
}

func sendFunds(ctx context.Context, t *testing.T, fromNode test.TestNode, fromAddr address.Address, toAddr address.Address, amt types.BigInt) error {
	msg := &types.Message{
		From:  fromAddr,
		To:    toAddr,
		Value: amt,
	}

	sm, err := fromNode.MpoolPushMessage(ctx, msg, nil)
	if err != nil {
		return err
	}

	res, err := fromNode.StateWaitMsg(ctx, sm.Cid(), 1)
	if err != nil {
		return err
	}
	if res.Receipt.ExitCode != 0 {
		return xerrors.Errorf("send funds failed with exit code %d", res.Receipt.ExitCode)
	}

	return nil
}

func startNodes(ctx context.Context, t *testing.T, blocktime time.Duration) (test.TestNode, test.TestNode, jsonrpc.ClientCloser) {
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
				_, addr, err := builder.CreateRPCServer(&GatewayAPI{api: fullNode})
				require.NoError(t, err)

				// Create a gateway client API that connects to the gateway server
				var gapi api.GatewayAPI
				gapi, closer, err = client.NewGatewayRPC(ctx, addr, nil)
				require.NoError(t, err)

				// Provide the gateway API to dependency injection
				return node.Override(new(api.GatewayAPI), gapi)
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

	// Start mining blocks
	bm := test.NewBlockMiner(ctx, t, miner, blocktime)
	bm.MineBlocks()

	return full, lite, closer
}
