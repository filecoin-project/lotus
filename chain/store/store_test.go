package store_test

import (
	"context"
	"testing"

	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/gen"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/node/repo"
	blockstore "github.com/ipfs/go-ipfs-blockstore"
)

func init() {
	build.SectorSizes = []uint64{1024}
	build.MinimumMinerPower = 1024
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

	bds, err := lr.Datastore("/blocks")
	if err != nil {
		b.Fatal(err)
	}

	mds, err := lr.Datastore("/metadata")
	if err != nil {
		b.Fatal(err)
	}

	bs := blockstore.NewBlockstore(bds)

	cs := store.NewChainStore(bs, mds, nil)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := cs.GetRandomness(context.TODO(), last.Cids(), 500)
		if err != nil {
			b.Fatal(err)
		}
	}
}
