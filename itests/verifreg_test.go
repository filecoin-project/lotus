package itests

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/filecoin-project/go-state-types/network"
	"github.com/filecoin-project/lotus/itests/kit"

	lapi "github.com/filecoin-project/lotus/api"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/builtin/verifreg"
	"github.com/filecoin-project/lotus/node/impl"
	verifreg4 "github.com/filecoin-project/specs-actors/v4/actors/builtin/verifreg"

	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/lotus/chain/types"
)

func TestVerifiedClientTopUp(t *testing.T) {
	test := func(nv network.Version, shouldWork bool) func(*testing.T) {
		return func(t *testing.T) {
			nodes, miners := kit.MockMinerBuilder(t, []kit.FullNodeOpts{kit.FullNodeWithNetworkUpgradeAt(nv, -1)}, kit.OneMiner)
			api := nodes[0].FullNode.(*impl.FullNodeAPI)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			//Get VRH
			vrh, err := api.StateVerifiedRegistryRootKey(ctx, types.TipSetKey{})
			if err != nil {
				t.Fatal(err)
			}

			//Add verifier
			verifier, err := api.WalletDefaultAddress(ctx)
			if err != nil {
				t.Fatal(err)
			}

			params, err := actors.SerializeParams(&verifreg4.AddVerifierParams{Address: verifier, Allowance: big.NewInt(100000000000)})
			if err != nil {
				t.Fatal(err)
			}
			msg := &types.Message{
				To:     verifreg.Address,
				From:   vrh,
				Method: verifreg.Methods.AddVerifier,
				Params: params,
				Value:  big.Zero(),
			}

			bm := kit.NewBlockMiner(t, miners[0])
			bm.MineBlocks(ctx, 100*time.Millisecond)
			t.Cleanup(bm.Stop)

			sm, err := api.MpoolPushMessage(ctx, msg, nil)
			if err != nil {
				t.Fatal("AddVerifier failed: ", err)
			}
			res, err := api.StateWaitMsg(ctx, sm.Cid(), 1, lapi.LookbackNoLimit, true)
			if err != nil {
				t.Fatal(err)
			}
			if res.Receipt.ExitCode != 0 {
				t.Fatal("did not successfully send message")
			}

			//Assign datacap to a client
			datacap := big.NewInt(10000)
			clientAddress, err := api.WalletNew(ctx, types.KTBLS)
			if err != nil {
				t.Fatal(err)
			}

			params, err = actors.SerializeParams(&verifreg4.AddVerifiedClientParams{Address: clientAddress, Allowance: datacap})
			if err != nil {
				t.Fatal(err)
			}

			msg = &types.Message{
				To:     verifreg.Address,
				From:   verifier,
				Method: verifreg.Methods.AddVerifiedClient,
				Params: params,
				Value:  big.Zero(),
			}

			sm, err = api.MpoolPushMessage(ctx, msg, nil)
			if err != nil {
				t.Fatal("AddVerifiedClient faield: ", err)
			}
			res, err = api.StateWaitMsg(ctx, sm.Cid(), 1, lapi.LookbackNoLimit, true)
			if err != nil {
				t.Fatal(err)
			}
			if res.Receipt.ExitCode != 0 {
				t.Fatal("did not successfully send message")
			}

			//check datacap balance
			dcap, err := api.StateVerifiedClientStatus(ctx, clientAddress, types.EmptyTSK)
			if err != nil {
				t.Fatal(err)
			}
			if !dcap.Equals(datacap) {
				t.Fatal("")
			}

			//try to assign datacap to the same client should fail for actor v4 and below
			params, err = actors.SerializeParams(&verifreg4.AddVerifiedClientParams{Address: clientAddress, Allowance: datacap})
			if err != nil {
				t.Fatal(err)
			}

			msg = &types.Message{
				To:     verifreg.Address,
				From:   verifier,
				Method: verifreg.Methods.AddVerifiedClient,
				Params: params,
				Value:  big.Zero(),
			}

			_, err = api.MpoolPushMessage(ctx, msg, nil)
			if shouldWork && err != nil {
				t.Fatal("expected nil err", err)
			}

			if !shouldWork && (err == nil || !strings.Contains(err.Error(), "verified client already exists")) {
				t.Fatal("Add datacap to an existing verified client should fail")
			}
		}
	}

	t.Run("nv12", test(network.Version12, false))
	t.Run("nv13", test(network.Version13, true))
}
