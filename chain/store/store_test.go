package store_test

import (
	"bytes"
	"context"
	"testing"

	datastore "github.com/ipfs/go-datastore"
	blockstore "github.com/ipfs/go-ipfs-blockstore"

	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/abi/big"
	"github.com/filecoin-project/specs-actors/actors/builtin/miner"
	"github.com/filecoin-project/specs-actors/actors/builtin/power"
	"github.com/filecoin-project/specs-actors/actors/builtin/verifreg"
	"github.com/filecoin-project/specs-actors/actors/crypto"

	"github.com/filecoin-project/lotus/chain/gen"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/node/repo"
)

func init() {
	miner.SupportedProofTypes = map[abi.RegisteredProof]struct{}{
		abi.RegisteredProof_StackedDRG2KiBSeal: {},
	}
	power.ConsensusMinerMinPower = big.NewInt(2048)
	verifreg.MinVerifiedDealSize = big.NewInt(256)
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
		_, err := cs.GetRandomness(context.TODO(), last.Cids(), crypto.DomainSeparationTag_SealRandomness, 500, nil)
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
	if err := cg.ChainStore().Export(context.TODO(), last, buf); err != nil {
		t.Fatal(err)
	}

	nbs := blockstore.NewBlockstore(datastore.NewMapDatastore())
	cs := store.NewChainStore(nbs, datastore.NewMapDatastore(), nil)

	root, err := cs.Import(buf)
	if err != nil {
		t.Fatal(err)
	}

	if !root.Equals(last) {
		t.Fatal("imported chain differed from exported chain")
	}
}
