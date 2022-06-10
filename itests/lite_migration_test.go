package itests

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/ipfs/go-cid"
	mh "github.com/multiformats/go-multihash"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/manifest"
	"github.com/filecoin-project/go-state-types/network"
	gstStore "github.com/filecoin-project/go-state-types/store"
	"github.com/filecoin-project/specs-actors/v8/actors/util/adt"

	"github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/consensus/filcns"
	"github.com/filecoin-project/lotus/chain/state"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/itests/kit"
)

func TestLiteMigration(t *testing.T) {
	ctx := context.Background()

	kit.QuietMiningLogs()

	client16, _, ens := kit.EnsembleMinimal(t, kit.MockProofs(), kit.GenesisNetworkVersion(network.Version16))
	ens.InterconnectAll().BeginMining(10 * time.Millisecond)

	client16.WaitTillChain(ctx, func(set *types.TipSet) bool {
		return set.Height() > 100
	})

	bs := blockstore.NewAPIBlockstore(client16)
	ctxStore := gstStore.WrapBlockStore(ctx, bs)

	ts, err := client16.ChainHead(ctx)
	require.NoError(t, err)

	stateRoot := ts.ParentState()
	oldStateTree, err := state.LoadStateTree(ctxStore, stateRoot)
	require.NoError(t, err)

	oldManifest, err := stmgr.GetManifest(ctx, oldStateTree)
	require.NoError(t, err)
	newManifestCid := makeTestManifest(t, ctxStore)
	// Use the Cid we generated to get the new manifest instead of loading it from the state tree, because that would not test that we have the correct manifest in the state
	var newManifest manifest.Manifest
	err = ctxStore.Get(ctx, newManifestCid, &newManifest)
	require.NoError(t, err)
	err = newManifest.Load(ctx, ctxStore)
	require.NoError(t, err)
	newManifestData := manifest.ManifestData{}
	err = ctxStore.Get(ctx, newManifest.Data, &newManifestData)
	require.NoError(t, err)

	newStateRoot, err := filcns.LiteMigration(ctx, bs, newManifestCid, stateRoot, actors.Version8, types.StateTreeVersion4, types.StateTreeVersion4)
	require.NoError(t, err)

	newStateTree, err := state.LoadStateTree(ctxStore, newStateRoot)
	require.NoError(t, err)

	migrations := make(map[cid.Cid]cid.Cid)
	for _, entry := range newManifestData.Entries {
		oldCodeCid, ok := oldManifest.Get(entry.Name)
		require.True(t, ok)
		migrations[oldCodeCid] = entry.Code
	}

	err = newStateTree.ForEach(func(addr address.Address, newActorState *types.Actor) error {
		oldActor, err := oldStateTree.GetActor(addr)
		require.NoError(t, err)
		newCodeCid, ok := migrations[oldActor.Code]
		require.True(t, ok)
		require.Equal(t, newCodeCid, newActorState.Code)
		return nil
	})
	require.NoError(t, err)
}

func makeTestManifest(t *testing.T, ctxStore adt.Store) cid.Cid {
	builder := cid.V1Builder{Codec: cid.Raw, MhType: mh.IDENTITY}

	manifestData := manifest.ManifestData{}
	for _, name := range []string{"system", "init", "cron", "account", "storagepower", "storageminer", "storagemarket", "paymentchannel", "multisig", "reward", "verifiedregistry"} {
		codeCid, err := builder.Sum([]byte(fmt.Sprintf("fil/8/%s", name)))
		if err != nil {
			t.Fatal(err)
		}

		manifestData.Entries = append(manifestData.Entries,
			manifest.ManifestEntry{
				Name: name,
				Code: codeCid,
			})
	}

	manifestDataCid, err := ctxStore.Put(ctxStore.Context(), &manifestData)
	if err != nil {
		t.Fatal(err)
	}

	mf := manifest.Manifest{
		Version: 1,
		Data:    manifestDataCid,
	}

	manifestCid, err := ctxStore.Put(ctxStore.Context(), &mf)
	if err != nil {
		t.Fatal(err)
	}

	return manifestCid
}
