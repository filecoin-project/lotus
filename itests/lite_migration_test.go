package itests

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/filecoin-project/lotus/chain/actors/builtin/system"

	"github.com/filecoin-project/lotus/chain/state"

	"github.com/ipfs/go-cid"
	mh "github.com/multiformats/go-multihash"

	"github.com/filecoin-project/lotus/blockstore"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-state-types/manifest"
	"github.com/filecoin-project/go-state-types/network"
	"github.com/filecoin-project/specs-actors/v8/actors/util/adt"

	"github.com/filecoin-project/lotus/chain/consensus/filcns"
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
	ctxStore := adt.WrapBlockStore(ctx, bs)

	ts, err := client16.ChainHead(ctx)
	require.NoError(t, err)

	stateRoot := ts.ParentState()
	newManifestCid := makeTestManifest(t, ctxStore)

	newStateRoot, err := filcns.LiteMigration(ctx, bs, newManifestCid, stateRoot)
	require.NoError(t, err)

	stateTree, err := state.LoadStateTree(ctxStore, newStateRoot)
	require.NoError(t, err)

	systemActor, err := stateTree.GetActor(system.Address)

	var newManifest manifest.Manifest
	err = ctxStore.Get(ctx, newManifestCid, &newManifest)
	require.NoError(t, err)
	err = newManifest.Load(ctx, ctxStore)
	require.NoError(t, err)
	manifestSystemCodeCid, ok := newManifest.Get("system")
	require.True(t, ok)

	require.Equal(t, systemActor.Code, manifestSystemCodeCid)

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
