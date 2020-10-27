package store_test

import (
	"bytes"
	"context"
	"testing"

	datastore "github.com/ipfs/go-datastore"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/crypto"

	"github.com/filecoin-project/lotus/chain/actors/policy"
	"github.com/filecoin-project/lotus/chain/gen"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/lib/blockstore"
	"github.com/filecoin-project/lotus/node/repo"
)

func init() {
	policy.SetSupportedProofTypes(abi.RegisteredSealProof_StackedDrg2KiBV1)
	policy.SetConsensusMinerMinPower(abi.NewStoragePower(2048))
	policy.SetMinVerifiedDealSize(abi.NewStoragePower(256))
}

func BenchmarkGetRandomness(b *testing.B) {
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

	bds, err := lr.Datastore("/chain")
	if err != nil {
		b.Fatal(err)
	}

	mds, err := lr.Datastore("/metadata")
	if err != nil {
		b.Fatal(err)
	}

	bs := blockstore.NewBlockstore(bds)

	cs := store.NewChainStore(bs, mds, nil, nil)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := cs.GetChainRandomness(context.TODO(), last.Cids(), crypto.DomainSeparationTag_SealRandomness, 500, nil)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func TestChainExportImport(t *testing.T) {
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

	nbs := blockstore.NewTemporary()
	cs := store.NewChainStore(nbs, datastore.NewMapDatastore(), nil, nil)

	root, err := cs.Import(buf)
	if err != nil {
		t.Fatal(err)
	}

	if !root.Equals(last) {
		t.Fatal("imported chain differed from exported chain")
	}
}

func TestChainExportImportFull(t *testing.T) {
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

	nbs := blockstore.NewTemporary()
	cs := store.NewChainStore(nbs, datastore.NewMapDatastore(), nil, nil)
	root, err := cs.Import(buf)
	if err != nil {
		t.Fatal(err)
	}

	err = cs.SetHead(last)
	if err != nil {
		t.Fatal(err)
	}

	if !root.Equals(last) {
		t.Fatal("imported chain differed from exported chain")
	}

	sm := stmgr.NewStateManager(cs)
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
