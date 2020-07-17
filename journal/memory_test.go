package journal

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/types"

	"github.com/filecoin-project/specs-actors/actors/abi"

	"github.com/raulk/clock"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx/fxtest"
)

func TestMemJournal_AddEntry(t *testing.T) {
	lc := fxtest.NewLifecycle(t)
	defer lc.RequireStop()

	clk := clock.NewMock()
	build.Clock = clk

	journal := NewMemoryJournal(lc, nil)
	addEntries(journal, 100)

	require.Eventually(t, func() bool { return len(journal.Entries()) == 100 }, 1*time.Second, 100*time.Millisecond)

	entries := journal.Entries()
	cnt := make(map[string]int, 10)
	for i, e := range entries {
		require.EqualValues(t, "spaceship", e.System)
		require.Equal(t, HeadChangeEvt{
			From:        types.TipSetKey{},
			FromHeight:  abi.ChainEpoch(i),
			To:          types.TipSetKey{},
			ToHeight:    abi.ChainEpoch(i),
			RevertCount: i,
			ApplyCount:  i,
		}, e.Data)
		require.Equal(t, build.Clock.Now(), e.Timestamp)
		cnt[e.Event]++
	}

	// we received 10 entries of each event type.
	for _, c := range cnt {
		require.Equal(t, 10, c)
	}
}

func TestMemJournal_Close(t *testing.T) {
	lc := fxtest.NewLifecycle(t)
	defer lc.RequireStop()

	journal := NewMemoryJournal(lc, nil)
	addEntries(journal, 100)

	require.Eventually(t, func() bool { return len(journal.Entries()) == 100 }, 1*time.Second, 100*time.Millisecond)

	o1 := journal.Observe(context.TODO(), false)
	o2 := journal.Observe(context.TODO(), false)
	o3 := journal.Observe(context.TODO(), false)

	time.Sleep(500 * time.Millisecond)

	// Close the journal.
	require.NoError(t, journal.Close())

	time.Sleep(500 * time.Millisecond)

NextChannel:
	for _, ch := range []<-chan *Entry{o1, o2, o3} {
		for {
			select {
			case _, more := <-ch:
				if more {
					// keep consuming
				} else {
					continue NextChannel
				}
			default:
				t.Fatal("nothing more to consume, and channel is not closed")
			}
		}
	}
}

func TestMemJournal_Clear(t *testing.T) {
	lc := fxtest.NewLifecycle(t)
	defer lc.RequireStop()

	journal := NewMemoryJournal(lc, nil)
	addEntries(journal, 100)

	require.Eventually(t, func() bool { return len(journal.Entries()) == 100 }, 1*time.Second, 100*time.Millisecond)

	journal.Clear()
	require.Empty(t, journal.Entries())
	require.Empty(t, journal.Entries())
	require.Empty(t, journal.Entries())
}

func TestMemJournal_Observe(t *testing.T) {
	lc := fxtest.NewLifecycle(t)
	defer lc.RequireStop()

	journal := NewMemoryJournal(lc, nil)
	addEntries(journal, 100)

	require.Eventually(t, func() bool { return len(journal.Entries()) == 100 }, 1*time.Second, 100*time.Millisecond)

	o1 := journal.Observe(context.TODO(), false, EventType{"spaceship", "wheezing-1"})
	o2 := journal.Observe(context.TODO(), true, EventType{"spaceship", "wheezing-1"}, EventType{"spaceship", "wheezing-2"})
	o3 := journal.Observe(context.TODO(), true)

	time.Sleep(1 * time.Second)

	require.Len(t, o1, 0)   // no replay
	require.Len(t, o2, 20)  // replay with include set
	require.Len(t, o3, 100) // replay with no include set (all entries)

	// add another 100 entries and assert what the observers have seen.
	addEntries(journal, 100)

	require.Eventually(t, func() bool { return len(journal.Entries()) == 200 }, 1*time.Second, 100*time.Millisecond)

	// note: we're able to queue items because the observer channel buffer size is 256.
	require.Len(t, o1, 10)  // should have 0 old entries + 10 new entries
	require.Len(t, o2, 40)  // should have 20 old entries + 20 new entries
	require.Len(t, o3, 200) // should have 100 old entries + 100 new entries
}

func TestMemJournal_ObserverCancellation(t *testing.T) {
	lc := fxtest.NewLifecycle(t)
	defer lc.RequireStop()

	journal := NewMemoryJournal(lc, nil)

	ctx, cancel := context.WithCancel(context.TODO())
	o1 := journal.Observe(ctx, false)
	o2 := journal.Observe(context.TODO(), false)
	addEntries(journal, 100)

	require.Eventually(t, func() bool { return len(journal.Entries()) == 100 }, 1*time.Second, 100*time.Millisecond)

	// all observers have received the 100 entries.
	require.Len(t, o1, 100)
	require.Len(t, o2, 100)

	// cancel o1's context.
	cancel()
	time.Sleep(500 * time.Millisecond)

	// add 50 new entries
	addEntries(journal, 50)

	require.Eventually(t, func() bool { return len(journal.Entries()) == 150 }, 1*time.Second, 100*time.Millisecond)

	require.Len(t, o1, 100) // has not moved.
	require.Len(t, o2, 150) // should have 100 old entries + 50 new entries

}

func addEntries(journal *MemJournal, count int) {
	for i := 0; i < count; i++ {
		eventIdx := i % 10
		journal.AddEntry(EventType{"spaceship", fmt.Sprintf("wheezing-%d", eventIdx)}, HeadChangeEvt{
			From:        types.TipSetKey{},
			FromHeight:  abi.ChainEpoch(i),
			To:          types.TipSetKey{},
			ToHeight:    abi.ChainEpoch(i),
			RevertCount: i,
			ApplyCount:  i,
		})
	}
}
