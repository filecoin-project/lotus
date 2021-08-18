package itests

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/network"
	"github.com/filecoin-project/lotus/chain/wallet"
	"github.com/filecoin-project/lotus/itests/kit"
	verifreg4 "github.com/filecoin-project/specs-actors/v4/actors/builtin/verifreg"
	"github.com/stretchr/testify/require"

	lapi "github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/builtin/verifreg"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/node/impl"
)

func TestVerifiedClientTopUp(t *testing.T) {
	blockTime := 100 * time.Millisecond

	test := func(nv network.Version, shouldWork bool) func(*testing.T) {
		return func(t *testing.T) {
			rootKey, err := wallet.GenerateKey(types.KTSecp256k1)
			require.NoError(t, err)

			verifierKey, err := wallet.GenerateKey(types.KTSecp256k1)
			require.NoError(t, err)

			verifiedClientKey, err := wallet.GenerateKey(types.KTBLS)
			require.NoError(t, err)

			bal, err := types.ParseFIL("100fil")
			require.NoError(t, err)

			node, _, ens := kit.EnsembleMinimal(t, kit.MockProofs(),
				kit.RootVerifier(rootKey, abi.NewTokenAmount(bal.Int64())),
				kit.Account(verifierKey, abi.NewTokenAmount(bal.Int64())), // assign some balance to the verifier so they can send an AddClient message.
				kit.GenesisNetworkVersion(nv))

			ens.InterconnectAll().BeginMining(blockTime)

			api := node.FullNode.(*impl.FullNodeAPI)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			// get VRH
			vrh, err := api.StateVerifiedRegistryRootKey(ctx, types.TipSetKey{})
			fmt.Println(vrh.String())
			require.NoError(t, err)

			// import the root key.
			rootAddr, err := api.WalletImport(ctx, &rootKey.KeyInfo)
			require.NoError(t, err)

			// import the verifier's key.
			verifierAddr, err := api.WalletImport(ctx, &verifierKey.KeyInfo)
			require.NoError(t, err)

			// import the verified client's key.
			verifiedClientAddr, err := api.WalletImport(ctx, &verifiedClientKey.KeyInfo)
			require.NoError(t, err)

			params, err := actors.SerializeParams(&verifreg4.AddVerifierParams{Address: verifierAddr, Allowance: big.NewInt(100000000000)})
			require.NoError(t, err)

			msg := &types.Message{
				From:   rootAddr,
				To:     verifreg.Address,
				Method: verifreg.Methods.AddVerifier,
				Params: params,
				Value:  big.Zero(),
			}

			sm, err := api.MpoolPushMessage(ctx, msg, nil)
			require.NoError(t, err, "AddVerifier failed")

			res, err := api.StateWaitMsg(ctx, sm.Cid(), 1, lapi.LookbackNoLimit, true)
			require.NoError(t, err)
			require.EqualValues(t, 0, res.Receipt.ExitCode)

			// assign datacap to a client
			datacap := big.NewInt(10000)

			params, err = actors.SerializeParams(&verifreg4.AddVerifiedClientParams{Address: verifiedClientAddr, Allowance: datacap})
			require.NoError(t, err)

			msg = &types.Message{
				From:   verifierAddr,
				To:     verifreg.Address,
				Method: verifreg.Methods.AddVerifiedClient,
				Params: params,
				Value:  big.Zero(),
			}

			sm, err = api.MpoolPushMessage(ctx, msg, nil)
			require.NoError(t, err)

			res, err = api.StateWaitMsg(ctx, sm.Cid(), 1, lapi.LookbackNoLimit, true)
			require.NoError(t, err)
			require.EqualValues(t, 0, res.Receipt.ExitCode)

			// check datacap balance
			dcap, err := api.StateVerifiedClientStatus(ctx, verifiedClientAddr, types.EmptyTSK)
			require.NoError(t, err)

			if !dcap.Equals(datacap) {
				t.Fatal("")
			}

			// try to assign datacap to the same client should fail for actor v4 and below
			params, err = actors.SerializeParams(&verifreg4.AddVerifiedClientParams{Address: verifiedClientAddr, Allowance: datacap})
			if err != nil {
				t.Fatal(err)
			}

			msg = &types.Message{
				From:   verifierAddr,
				To:     verifreg.Address,
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
