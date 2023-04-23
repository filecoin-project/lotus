// stm: #unit
package store_test

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/ipfs/go-datastore"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/crypto"

	"github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/chain/actors/policy"
	"github.com/filecoin-project/lotus/chain/consensus"
	"github.com/filecoin-project/lotus/chain/consensus/filcns"
	"github.com/filecoin-project/lotus/chain/gen"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/node/repo"
)

func init() {
	policy.SetSupportedProofTypes(abi.RegisteredSealProof_StackedDrg2KiBV1)
	policy.SetConsensusMinerMinPower(abi.NewStoragePower(2048))
	policy.SetMinVerifiedDealSize(abi.NewStoragePower(256))
}

func BenchmarkGetRandomness(b *testing.B) {
	//stm: @CHAIN_GEN_NEXT_TIPSET_001
	//stm: @CHAIN_STATE_GET_RANDOMNESS_FROM_TICKETS_001
	cg, err := gen.NewGenerator()
	if err != nil {
		b.Fatal(err)
	}

	var last *types.TipSet
	for i := 0; i < 2000; i++ {
		ts, err := cg.NextTipSet()
		if err != nil {
			b.Fatal(err)
		}

		last = ts.TipSet.TipSet()
	}

	r, err := cg.YieldRepo()
	if err != nil {
		b.Fatal(err)
	}

	lr, err := r.Lock(repo.FullNode)
	if err != nil {
		b.Fatal(err)
	}

	bs, err := lr.Blockstore(context.TODO(), repo.UniversalBlockstore)
	if err != nil {
		b.Fatal(err)
	}

	defer func() {
		if c, ok := bs.(io.Closer); ok {
			if err := c.Close(); err != nil {
				b.Logf("WARN: failed to close blockstore: %s", err)
			}
		}
	}()

	mds, err := lr.Datastore(context.Background(), "/metadata")
	if err != nil {
		b.Fatal(err)
	}

	cs := store.NewChainStore(bs, bs, mds, filcns.Weight, nil)
	defer cs.Close() //nolint:errcheck

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := cg.StateManager().GetRandomnessFromTickets(context.TODO(), crypto.DomainSeparationTag_SealRandomness, 500, nil, last.Key())
		if err != nil {
			b.Fatal(err)
		}
	}
}

func TestChainExportImport(t *testing.T) {
	//stm: @CHAIN_GEN_NEXT_TIPSET_001
	//stm: @CHAIN_STORE_IMPORT_001
	cg, err := gen.NewGenerator()
	if err != nil {
		t.Fatal(err)
	}

	var last *types.TipSet
	for i := 0; i < 100; i++ {
		ts, err := cg.NextTipSet()
		if err != nil {
			t.Fatal(err)
		}

		last = ts.TipSet.TipSet()
	}

	buf := new(bytes.Buffer)
	if err := cg.ChainStore().Export(context.TODO(), last, 0, false, buf); err != nil {
		t.Fatal(err)
	}

	nbs := blockstore.NewMemorySync()
	cs := store.NewChainStore(nbs, nbs, datastore.NewMapDatastore(), filcns.Weight, nil)
	defer cs.Close() //nolint:errcheck

	root, err := cs.Import(context.TODO(), buf)
	if err != nil {
		t.Fatal(err)
	}

	if !root.Equals(last) {
		t.Fatal("imported chain differed from exported chain")
	}
}

// Test to check if tipset key cids are being stored on snapshot
func TestChainImportTipsetKeyCid(t *testing.T) {

	ctx := context.Background()
	cg, err := gen.NewGenerator()
	require.NoError(t, err)

	buf := new(bytes.Buffer)
	var last *types.TipSet
	var tsKeys []types.TipSetKey
	for i := 0; i < 10; i++ {
		ts, err := cg.NextTipSet()
		require.NoError(t, err)
		last = ts.TipSet.TipSet()
		tsKeys = append(tsKeys, last.Key())
	}

	if err := cg.ChainStore().Export(ctx, last, last.Height(), false, buf); err != nil {
		t.Fatal(err)
	}

	nbs := blockstore.NewMemorySync()
	cs := store.NewChainStore(nbs, nbs, datastore.NewMapDatastore(), filcns.Weight, nil)
	defer cs.Close() //nolint:errcheck

	root, err := cs.Import(ctx, buf)
	require.NoError(t, err)

	require.Truef(t, root.Equals(last), "imported chain differed from exported chain")

	err = cs.SetHead(ctx, last)
	require.NoError(t, err)

	for _, tsKey := range tsKeys {
		_, err := cs.LoadTipSet(ctx, tsKey)
		require.NoError(t, err)

		tsCid, err := tsKey.Cid()
		require.NoError(t, err)
		_, err = cs.ChainLocalBlockstore().Get(ctx, tsCid)
		require.NoError(t, err)

	}
}

func TestChainExportImportFull(t *testing.T) {
	//stm: @CHAIN_GEN_NEXT_TIPSET_001
	//stm: @CHAIN_STORE_IMPORT_001, @CHAIN_STORE_EXPORT_001, @CHAIN_STORE_SET_HEAD_001
	//stm: @CHAIN_STORE_GET_TIPSET_BY_HEIGHT_001
	cg, err := gen.NewGenerator()
	if err != nil {
		t.Fatal(err)
	}

	var last *types.TipSet
	for i := 0; i < 100; i++ {
		ts, err := cg.NextTipSet()
		if err != nil {
			t.Fatal(err)
		}

		last = ts.TipSet.TipSet()
	}

	buf := new(bytes.Buffer)
	if err := cg.ChainStore().Export(context.TODO(), last, last.Height(), false, buf); err != nil {
		t.Fatal(err)
	}

	nbs := blockstore.NewMemorySync()
	ds := datastore.NewMapDatastore()
	cs := store.NewChainStore(nbs, nbs, ds, filcns.Weight, nil)
	defer cs.Close() //nolint:errcheck

	root, err := cs.Import(context.TODO(), buf)
	if err != nil {
		t.Fatal(err)
	}

	err = cs.SetHead(context.Background(), last)
	if err != nil {
		t.Fatal(err)
	}

	if !root.Equals(last) {
		t.Fatal("imported chain differed from exported chain")
	}

	sm, err := stmgr.NewStateManager(cs, consensus.NewTipSetExecutor(filcns.RewardFunc), nil, filcns.DefaultUpgradeSchedule(), cg.BeaconSchedule(), ds)
	if err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 100; i++ {
		ts, err := cs.GetTipsetByHeight(context.TODO(), abi.ChainEpoch(i), nil, false)
		if err != nil {
			t.Fatal(err)
		}

		st, err := sm.ParentState(ts)
		if err != nil {
			t.Fatal(err)
		}

		// touches a bunch of actors
		_, err = sm.GetCirculatingSupply(context.TODO(), abi.ChainEpoch(i), st)
		if err != nil {
			t.Fatal(err)
		}
	}
}
