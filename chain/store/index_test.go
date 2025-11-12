package store_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/ipfs/go-datastore"
	syncds "github.com/ipfs/go-datastore/sync"
	"github.com/stretchr/testify/assert"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/chain/consensus/filcns"
	"github.com/filecoin-project/lotus/chain/gen"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/types/mock"
)

func TestIndexSeeks(t *testing.T) {
	cg, err := gen.NewGenerator()
	if err != nil {
		t.Fatal(err)
	}

	gencar, err := cg.GenesisCar()
	if err != nil {
		t.Fatal(err)
	}

	gen := cg.Genesis()

	ctx := context.TODO()

	nbs := blockstore.NewMemorySync()
	cs := store.NewChainStore(nbs, nbs, syncds.MutexWrap(datastore.NewMapDatastore()), filcns.Weight, nil)
	defer cs.Close() //nolint:errcheck

	_, _, err = cs.Import(ctx, nil, bytes.NewReader(gencar))
	if err != nil {
		t.Fatal(err)
	}

	cur := mock.TipSet(gen)

	assert.NoError(t, cs.SetGenesis(ctx, gen))

	// Put 113 blocks from genesis
	for i := 0; i < 113; i++ {
		nextBlk := mock.MkBlock(cur, 1, 1)
		nextts := mock.TipSet(nextBlk)
		assert.NoError(t, cs.PersistTipsets(ctx, []*types.TipSet{nextts}))
		assert.NoError(t, cs.AddToTipSetTracker(ctx, nextBlk))
		cur = nextts
	}

	assert.NoError(t, cs.RefreshHeaviestTipSet(ctx, cur.Height()))

	// Put 50 null epochs + 1 block
	skip := mock.MkBlock(cur, 1, 1)
	skip.Height += 50
	skipts := mock.TipSet(skip)

	assert.NoError(t, cs.PersistTipsets(ctx, []*types.TipSet{skipts}))
	assert.NoError(t, cs.AddToTipSetTracker(ctx, skip))

	if err := cs.RefreshHeaviestTipSet(ctx, skip.Height); err != nil {
		t.Fatal(err)
	}

	ts, err := cs.GetTipsetByHeight(ctx, skip.Height-10, skipts, false)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, abi.ChainEpoch(164), ts.Height())

	for i := 0; i <= 113; i++ {
		ts3, err := cs.GetTipsetByHeight(ctx, abi.ChainEpoch(i), skipts, false)
		if err != nil {
			t.Fatal(err)
		}
		assert.Equal(t, abi.ChainEpoch(i), ts3.Height())
	}
}
