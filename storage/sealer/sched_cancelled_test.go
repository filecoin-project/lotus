package sealer

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/storage/sealer/sealtasks"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
)

// ctxTaskTypesWorker fails TaskTypes on a done context, like a worker behind
// an RPC client does.
type ctxTaskTypesWorker struct {
	*schedTestWorker
}

func (w *ctxTaskTypesWorker) TaskTypes(ctx context.Context) (map[sealtasks.TaskType]struct{}, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return w.schedTestWorker.TaskTypes(ctx)
}

func testRequest(ctx context.Context, sector abi.SectorNumber) *WorkerRequest {
	return &WorkerRequest{
		Sector: storiface.SectorRef{
			ID:        abi.SectorID{Miner: 1000, Number: sector},
			ProofType: abi.RegisteredSealProof_StackedDrg2KiBV1_1,
		},
		TaskType: sealtasks.TTPreCommit1,
		Sel:      newTaskSelector(),
		Ctx:      ctx,
	}
}

func TestSchedCancelledRequestDoesNotBlockOthers(t *testing.T) {
	sched, err := newScheduler(context.Background(), "")
	require.NoError(t, err)

	w := &ctxTaskTypesWorker{&schedTestWorker{
		name:      "fred",
		taskTypes: map[sealtasks.TaskType]struct{}{sealtasks.TTPreCommit1: {}},
		session:   uuid.New(),
		resources: decentWorkerResources,
	}}
	wh, err := newWorkerHandle(context.Background(), w)
	require.NoError(t, err)
	wid := storiface.WorkerID(w.session)
	sched.Workers[wid] = wh

	// With one open window the assigner checks requests one at a time, in
	// queue order, so the cancelled request is checked first.
	done := make(chan *SchedWindow, 1)
	sched.OpenWindows = []*SchedWindowRequest{{Worker: wid, Done: done}}

	cctx, cancel := context.WithCancel(context.Background())
	cancel()
	sched.SchedQueue.Push(testRequest(cctx, 1))
	live := testRequest(context.Background(), 2)
	sched.SchedQueue.Push(live)

	sched.trySched()

	require.Zero(t, sched.SchedQueue.Len())
	select {
	case wnd := <-done:
		require.Equal(t, []*WorkerRequest{live}, wnd.Todo)
	default:
		t.Fatal("expected the live request to be scheduled")
	}
}

func TestRequestQueueRemoveCancelled(t *testing.T) {
	cctx, cancel := context.WithCancel(context.Background())
	cancel()

	rq := &RequestQueue{}
	for i, ctx := range []context.Context{context.Background(), cctx, context.Background(), cctx} {
		rq.Push(testRequest(ctx, abi.SectorNumber(i)))
	}

	require.Equal(t, 2, rq.RemoveCancelled())
	require.Equal(t, 2, rq.Len())
	for i, req := range *rq {
		require.NoError(t, req.Ctx.Err())
		require.Equal(t, i, req.index)
	}
	require.Equal(t, abi.SectorNumber(0), (*rq)[0].Sector.ID.Number)
	require.Equal(t, abi.SectorNumber(2), (*rq)[1].Sector.ID.Number)
}
