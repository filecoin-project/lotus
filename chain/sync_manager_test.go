package chain

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/types/mock"
)

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
	sm.bspThresh = thresh

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
		assertGetSyncOp(t, stc, a)

		sm.SetPeerHead(ctx, "peer1", b1)
		sm.SetPeerHead(ctx, "peer1", b2)
		// b1 and b2 are being processed

		b1op := <-stc
		b2op := <-stc
		if !b1op.ts.Equals(b1) {
			b1op, b2op = b2op, b1op
		}

		sm.SetPeerHead(ctx, "peer2", c2) // c2 is put into activeSyncTips at index 0
		sm.SetPeerHead(ctx, "peer2", c1) // c1 is put into activeSyncTips at index 1
		sm.SetPeerHead(ctx, "peer3", b2) // b2 is related to c2 and even though it is actively synced it is put into activeSyncTips index 0
		sm.SetPeerHead(ctx, "peer1", a)  // a is related to b2 and is put into activeSyncTips index 0

		b1op.done() // b1 completes first, is related to a, so it pops activeSyncTips index 0
		// even though correct one is index 1

		b2op.done()
		// b2 completes and is not related to c1, so it leaves activeSyncTips as it is

		waitUntilAllWorkersAreDone(stc)

		if len(sm.activeSyncTips.buckets) != 0 {
			t.Errorf("activeSyncTips expected empty but got: %s", sm.activeSyncTips.String())
		}
	})
}

func waitUntilAllWorkersAreDone(stc chan *syncOp) {
	for i := 0; i < 10; {
		select {
		case so := <-stc:
			so.done()
		default:
			i++
			time.Sleep(10 * time.Millisecond)
		}
	}
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
