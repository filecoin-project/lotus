package events

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/chain/types"
)

func TestTsCache(t *testing.T) {
	tsc := newTSCache(50, func(context.Context, uint64, *types.TipSet) (*types.TipSet, error) {
		t.Fatal("storage call")
		return &types.TipSet{}, nil
	})

	h := uint64(75)

	a, _ := address.NewFromString("t00")

	add := func() {
		ts, err := types.NewTipSet([]*types.BlockHeader{{
			Miner:                 a,
			Height:                h,
			ParentStateRoot:       dummyCid,
			Messages:              dummyCid,
			ParentMessageReceipts: dummyCid,
			BlockSig:              &types.Signature{Type: types.KTBLS},
			BLSAggregate:          types.Signature{Type: types.KTBLS},
		}})
		if err != nil {
			t.Fatal(err)
		}
		if err := tsc.add(ts); err != nil {
			t.Fatal(err)
		}
		h++
	}

	for i := 0; i < 9000; i++ {
		if i%90 > 60 {
			if err := tsc.revert(tsc.best()); err != nil {
				t.Fatal(err, "; i:", i)
				return
			}
			h--
		} else {
			add()
		}
	}

}

func TestTsCacheNulls(t *testing.T) {
	tsc := newTSCache(50, func(context.Context, uint64, *types.TipSet) (*types.TipSet, error) {
		t.Fatal("storage call")
		return &types.TipSet{}, nil
	})

	h := uint64(75)

	a, _ := address.NewFromString("t00")
	add := func() {
		ts, err := types.NewTipSet([]*types.BlockHeader{{
			Miner:                 a,
			Height:                h,
			ParentStateRoot:       dummyCid,
			Messages:              dummyCid,
			ParentMessageReceipts: dummyCid,
			BlockSig:              &types.Signature{Type: types.KTBLS},
			BLSAggregate:          types.Signature{Type: types.KTBLS},
		}})
		if err != nil {
			t.Fatal(err)
		}
		if err := tsc.add(ts); err != nil {
			t.Fatal(err)
		}
		h++
	}

	add()
	add()
	add()
	h += 5

	add()
	add()

	require.Equal(t, h-1, tsc.best().Height())

	ts, err := tsc.get(h - 1)
	require.NoError(t, err)
	require.Equal(t, h-1, ts.Height())

	ts, err = tsc.get(h - 2)
	require.NoError(t, err)
	require.Equal(t, h-2, ts.Height())

	ts, err = tsc.get(h - 3)
	require.NoError(t, err)
	require.Nil(t, ts)

	ts, err = tsc.get(h - 8)
	require.NoError(t, err)
	require.Equal(t, h-8, ts.Height())

	require.NoError(t, tsc.revert(tsc.best()))
	require.NoError(t, tsc.revert(tsc.best()))
	require.Equal(t, h-8, tsc.best().Height())

	h += 50
	add()

	ts, err = tsc.get(h - 1)
	require.NoError(t, err)
	require.Equal(t, h-1, ts.Height())
}
