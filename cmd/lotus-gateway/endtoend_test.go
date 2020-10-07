package main

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"

	"github.com/filecoin-project/lotus/api"

	"github.com/filecoin-project/lotus/node"

	"github.com/filecoin-project/lotus/api/client"

	"github.com/filecoin-project/go-jsonrpc"

	"github.com/filecoin-project/lotus/chain/wallet"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-state-types/abi"
	builder "github.com/filecoin-project/lotus/node/test"

	"github.com/filecoin-project/lotus/api/test"
	"github.com/filecoin-project/lotus/chain/actors/policy"
	"github.com/filecoin-project/lotus/chain/types"
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
	liteWalletAddr, err := lite.WalletNew(ctx, wallet.ActSigType("secp256k1"))
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
		func(nodes []test.TestNode) node.Option {
			fullNode := nodes[0]

			// Create a gateway server in front of the full node
			_, addr, err := builder.CreateRPCServer(&GatewayAPI{api: fullNode})
			require.NoError(t, err)

			// Create a gateway client API that connects to the gateway server
			var gapi api.GatewayAPI
			gapi, closer, err = client.NewGatewayRPC(ctx, addr, nil)
			require.NoError(t, err)

			// Override this node with lite-mode options
			return node.LiteModeOverrides(gapi)
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
