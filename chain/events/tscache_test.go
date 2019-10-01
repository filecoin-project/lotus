package events

import (
	"context"
	"fmt"
	"testing"

	"github.com/filecoin-project/go-lotus/chain/types"
)

func TestTsCache(t *testing.T) {
	tsc := newTSCache(50, func(context.Context, uint64, *types.TipSet) (*types.TipSet, error) {
		t.Fatal("storage call")
		return &types.TipSet{}, nil
	})

	h := uint64(75)

	add := func() {
		ts, err := types.NewTipSet([]*types.BlockHeader{{
			Height:                h,
			ParentStateRoot:       dummyCid,
			Messages:              dummyCid,
			ParentMessageReceipts: dummyCid,
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
		fmt.Printf("i=%d; tl=%d; tcl=%d\n", i, tsc.len, len(tsc.cache))

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
