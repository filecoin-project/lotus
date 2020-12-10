package chain

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/types/mock"
)

func init() {
	BootstrapPeerThreshold = 1
}

var genTs = mock.TipSet(mock.MkBlock(nil, 0, 0))

type syncOp struct {
	ts   *types.TipSet
	done func()
}

func runSyncMgrTest(t *testing.T, tname string, thresh int, tf func(*testing.T, *syncManager, chan *syncOp)) {
	syncTargets := make(chan *syncOp)
	sm := NewSyncManager(func(ctx context.Context, ts *types.TipSet) error {
		ch := make(chan struct{})
		syncTargets <- &syncOp{
			ts:   ts,
			done: func() { close(ch) },
		}
		<-ch
		return nil
	}).(*syncManager)

	oldBootstrapPeerThreshold := BootstrapPeerThreshold
	BootstrapPeerThreshold = thresh
	defer func() {
		BootstrapPeerThreshold = oldBootstrapPeerThreshold
	}()

	sm.Start()
	defer sm.Stop()
	t.Run(tname+fmt.Sprintf("-%d", thresh), func(t *testing.T) {
		tf(t, sm, syncTargets)
	})
}

func assertTsEqual(t *testing.T, actual, expected *types.TipSet) {
	t.Helper()
	if !actual.Equals(expected) {
		t.Fatalf("got unexpected tipset %s (expected: %s)", actual.Cids(), expected.Cids())
	}
}

func assertNoOp(t *testing.T, c chan *syncOp) {
	t.Helper()
	select {
	case <-time.After(time.Millisecond * 20):
	case <-c:
		t.Fatal("shouldnt have gotten any sync operations yet")
	}
}

func assertGetSyncOp(t *testing.T, c chan *syncOp, ts *types.TipSet) {
	t.Helper()

	select {
	case <-time.After(time.Millisecond * 100):
		t.Fatal("expected sync manager to try and sync to our target")
	case op := <-c:
		op.done()
		if !op.ts.Equals(ts) {
			t.Fatalf("somehow got wrong tipset from syncer (got %s, expected %s)", op.ts.Cids(), ts.Cids())
		}
	}
}

func TestSyncManagerEdgeCase(t *testing.T) {
	ctx := context.Background()

	a := mock.TipSet(mock.MkBlock(genTs, 1, 1))
	t.Logf("a: %s", a)
	b1 := mock.TipSet(mock.MkBlock(a, 1, 2))
	t.Logf("b1: %s", b1)
	b2 := mock.TipSet(mock.MkBlock(a, 2, 3))
	t.Logf("b2: %s", b2)
	c1 := mock.TipSet(mock.MkBlock(b1, 2, 4))
	t.Logf("c1: %s", c1)
	c2 := mock.TipSet(mock.MkBlock(b2, 1, 5))
	t.Logf("c2: %s", c2)
	d1 := mock.TipSet(mock.MkBlock(c1, 1, 6))
	t.Logf("d1: %s", d1)
	e1 := mock.TipSet(mock.MkBlock(d1, 1, 7))
	t.Logf("e1: %s", e1)

	runSyncMgrTest(t, "edgeCase", 1, func(t *testing.T, sm *syncManager, stc chan *syncOp) {
		sm.SetPeerHead(ctx, "peer1", a)

		sm.SetPeerHead(ctx, "peer1", b1)
		sm.SetPeerHead(ctx, "peer1", b2)

		assertGetSyncOp(t, stc, a)

		// b1 and b2 are in queue after a; the sync manager should pick the heaviest one which is b2
		bop := <-stc
		if !bop.ts.Equals(b2) {
			t.Fatalf("Expected tipset %s to sync, but got %s", b2, bop.ts)
		}

		sm.SetPeerHead(ctx, "peer2", c2)
		sm.SetPeerHead(ctx, "peer2", c1)
		sm.SetPeerHead(ctx, "peer3", b2)
		sm.SetPeerHead(ctx, "peer1", a)

		bop.done()

		// get the next sync target; it should be c1 as the heaviest tipset but added last (same weight as c2)
		bop = <-stc
		if !bop.ts.Equals(c1) {
			t.Fatalf("Expected tipset %s to sync, but got %s", c1, bop.ts)
		}

		sm.SetPeerHead(ctx, "peer4", d1)
		sm.SetPeerHead(ctx, "peer5", e1)
		bop.done()

		// get the last sync target; it should be e1
		var last *types.TipSet
		for i := 0; i < 10; {
			select {
			case bop = <-stc:
				bop.done()
				if last == nil || bop.ts.Height() > last.Height() {
					last = bop.ts
				}
			default:
				i++
				time.Sleep(10 * time.Millisecond)
			}
		}
		if !last.Equals(e1) {
			t.Fatalf("Expected tipset %s to sync, but got %s", e1, last)
		}

		if len(sm.state) != 0 {
			t.Errorf("active syncs expected empty but got: %d", len(sm.state))
		}
	})
}

func TestSyncManager(t *testing.T) {
	ctx := context.Background()

	a := mock.TipSet(mock.MkBlock(genTs, 1, 1))
	b := mock.TipSet(mock.MkBlock(a, 1, 2))
	c1 := mock.TipSet(mock.MkBlock(b, 1, 3))
	c2 := mock.TipSet(mock.MkBlock(b, 2, 4))
	c3 := mock.TipSet(mock.MkBlock(b, 3, 5))
	d := mock.TipSet(mock.MkBlock(c1, 4, 5))

	runSyncMgrTest(t, "testBootstrap", 1, func(t *testing.T, sm *syncManager, stc chan *syncOp) {
		sm.SetPeerHead(ctx, "peer1", c1)
		assertGetSyncOp(t, stc, c1)
	})

	runSyncMgrTest(t, "testBootstrap", 2, func(t *testing.T, sm *syncManager, stc chan *syncOp) {
		sm.SetPeerHead(ctx, "peer1", c1)
		assertNoOp(t, stc)

		sm.SetPeerHead(ctx, "peer2", c1)
		assertGetSyncOp(t, stc, c1)
	})

	runSyncMgrTest(t, "testSyncAfterBootstrap", 1, func(t *testing.T, sm *syncManager, stc chan *syncOp) {
		sm.SetPeerHead(ctx, "peer1", b)
		assertGetSyncOp(t, stc, b)

		sm.SetPeerHead(ctx, "peer2", c1)
		assertGetSyncOp(t, stc, c1)

		sm.SetPeerHead(ctx, "peer2", c2)
		assertGetSyncOp(t, stc, c2)
	})

	runSyncMgrTest(t, "testCoalescing", 1, func(t *testing.T, sm *syncManager, stc chan *syncOp) {
		sm.SetPeerHead(ctx, "peer1", a)
		assertGetSyncOp(t, stc, a)

		sm.SetPeerHead(ctx, "peer2", b)
		op := <-stc

		sm.SetPeerHead(ctx, "peer2", c1)
		sm.SetPeerHead(ctx, "peer2", c2)
		sm.SetPeerHead(ctx, "peer2", d)

		assertTsEqual(t, op.ts, b)

		// need a better way to 'wait until syncmgr is idle'
		time.Sleep(time.Millisecond * 20)

		op.done()

		assertGetSyncOp(t, stc, d)
	})

	runSyncMgrTest(t, "testSyncIncomingTipset", 1, func(t *testing.T, sm *syncManager, stc chan *syncOp) {
		sm.SetPeerHead(ctx, "peer1", a)
		assertGetSyncOp(t, stc, a)

		sm.SetPeerHead(ctx, "peer2", b)
		op := <-stc
		op.done()

		sm.SetPeerHead(ctx, "peer2", c1)
		op1 := <-stc
		fmt.Println("op1: ", op1.ts.Cids())

		sm.SetPeerHead(ctx, "peer2", c2)
		sm.SetPeerHead(ctx, "peer2", c3)

		op1.done()

		op2 := <-stc
		fmt.Println("op2: ", op2.ts.Cids())
		op2.done()

		op3 := <-stc
		fmt.Println("op3: ", op3.ts.Cids())
		op3.done()
	})
}
