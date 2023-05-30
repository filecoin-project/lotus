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
	actorstypes "github.com/filecoin-project/go-state-types/actors"
	system8 "github.com/filecoin-project/go-state-types/builtin/v8/system"
	"github.com/filecoin-project/go-state-types/manifest"
	"github.com/filecoin-project/go-state-types/network"
	gstStore "github.com/filecoin-project/go-state-types/store"
	"github.com/filecoin-project/specs-actors/v8/actors/util/adt"

	"github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/chain/actors/builtin/system"
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

	oldManifestData, err := stmgr.GetManifestData(ctx, oldStateTree)
	require.NoError(t, err)
	newManifestCid := makeTestManifest(t, ctxStore, actorstypes.Version9)
	// Use the Cid we generated to get the new manifest instead of loading it from the store, so as to confirm it's in the store
	var newManifest manifest.Manifest
	require.NoError(t, ctxStore.Get(ctx, newManifestCid, &newManifest), "error getting new manifest")

	// populate the entries field of the manifest
	require.NoError(t, newManifest.Load(ctx, ctxStore), "error loading new manifest")

	newStateRoot, err := filcns.LiteMigration(ctx, bs, newManifestCid, stateRoot, actorstypes.Version8, actorstypes.Version9, types.StateTreeVersion4, types.StateTreeVersion4)
	require.NoError(t, err)

	newStateTree, err := state.LoadStateTree(ctxStore, newStateRoot)
	require.NoError(t, err)

	migrations := make(map[cid.Cid]cid.Cid)
	for _, entry := range oldManifestData.Entries {
		newCodeCid, ok := newManifest.Get(entry.Name)
		require.True(t, ok)
		migrations[entry.Code] = newCodeCid
	}

	err = newStateTree.ForEach(func(addr address.Address, newActorState *types.Actor) error {
		oldActor, err := oldStateTree.GetActor(addr)
		require.NoError(t, err)
		newCodeCid, ok := migrations[oldActor.Code]
		require.True(t, ok)
		require.Equal(t, newCodeCid, newActorState.Code)

		if addr == system.Address {
			var systemSt system8.State
			require.NoError(t, ctxStore.Get(ctx, newActorState.Head, &systemSt))
			require.Equal(t, systemSt.BuiltinActors, newManifest.Data)
		}

		return nil
	})
	require.NoError(t, err)
}

func makeTestManifest(t *testing.T, ctxStore adt.Store, av actorstypes.Version) cid.Cid {
	builder := cid.V1Builder{Codec: cid.Raw, MhType: mh.IDENTITY}

	manifestData := manifest.ManifestData{}
	for _, name := range manifest.GetBuiltinActorsKeys(av) {
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
