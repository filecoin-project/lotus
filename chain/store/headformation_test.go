package store

import (
	"context"
	"fmt"
	"math/rand"
	"testing"

	"github.com/ipfs/go-datastore"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/types/mock"
)

// countWeight weighs a tipset by its block count, enough to rank candidates.
func countWeight(_ context.Context, _ blockstore.Blockstore, ts *types.TipSet) (types.BigInt, error) {
	return types.NewInt(uint64(len(ts.Blocks()))), nil
}

// siblings returns n blocks from distinct miners sharing one parent.
func siblings(n int) []*types.BlockHeader {
	parent := mock.TipSet(mock.MkBlock(nil, 1, 1))
	blks := make([]*types.BlockHeader, n)
	for i := range blks {
		blks[i] = mock.MkBlock(parent, 1, uint64(i+2))
		blks[i].Miner = mock.Address(uint64(1000 + i))
	}
	return blks
}

func formHead(t *testing.T, blks []*types.BlockHeader) *types.TipSet {
	t.Helper()
	ctx := context.Background()
	bs := blockstore.NewMemory()
	cs := NewChainStore(bs, bs, datastore.NewMapDatastore(), countWeight, nil)
	defer cs.Close() //nolint:errcheck

	require.NoError(t, cs.persistBlockHeaders(ctx, blks...))
	for _, b := range blks {
		require.NoError(t, cs.AddToTipSetTracker(ctx, b))
	}
	ts, weight, err := cs.FormHeaviestTipSetForHeight(ctx, blks[0].Height)
	require.NoError(t, err)
	require.Equal(t, types.NewInt(uint64(len(ts.Blocks()))), weight, "weight is of the returned tipset")
	require.LessOrEqual(t, len(ts.Blocks()), types.MaxTipSetSize)
	return ts
}

func TestFormHeaviestTipSetCapsWidth(t *testing.T) {
	blks := siblings(types.MaxTipSetSize + 1)
	canonical := mock.TipSet(append([]*types.BlockHeader(nil), blks...)...).Cids()

	orders := map[string][]*types.BlockHeader{
		"forward": blks,
		"reverse": make([]*types.BlockHeader, len(blks)),
	}
	for i, b := range blks {
		orders["reverse"][len(blks)-1-i] = b
	}
	for seed := int64(0); seed < 3; seed++ {
		shuffled := append([]*types.BlockHeader(nil), blks...)
		rand.New(rand.NewSource(seed)).Shuffle(len(shuffled), func(i, j int) {
			shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
		})
		orders[fmt.Sprintf("shuffled %d", seed)] = shuffled
	}

	for name, order := range orders {
		t.Run(name, func(t *testing.T) {
			ts := formHead(t, append([]*types.BlockHeader(nil), order...))
			require.Equal(t, canonical[:types.MaxTipSetSize], ts.Cids())
		})
	}
}

func TestFormHeaviestTipSetWithinCap(t *testing.T) {
	blks := siblings(types.MaxTipSetSize)
	want := mock.TipSet(append([]*types.BlockHeader(nil), blks...)...).Cids()
	require.Equal(t, want, formHead(t, blks).Cids())
}
