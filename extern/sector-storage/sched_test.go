package sectorstorage

import (
	"context"
	"fmt"
	"io"
	"runtime"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log/v2"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/specs-actors/actors/abi"

	"github.com/filecoin-project/lotus/extern/sector-storage/fsutil"
	"github.com/filecoin-project/lotus/extern/sector-storage/sealtasks"
	"github.com/filecoin-project/lotus/extern/sector-storage/stores"
	"github.com/filecoin-project/lotus/extern/sector-storage/storiface"
	"github.com/filecoin-project/specs-storage/storage"
)

func init() {
	InitWait = 10 * time.Millisecond
}

func TestWithPriority(t *testing.T) {
	ctx := context.Background()

	require.Equal(t, DefaultSchedPriority, getPriority(ctx))

	ctx = WithPriority(ctx, 2222)

	require.Equal(t, 2222, getPriority(ctx))
}

type schedTestWorker struct {
	name      string
	taskTypes map[sealtasks.TaskType]struct{}
	paths     []stores.StoragePath

	closed  bool
	closing chan struct{}
}

func (s *schedTestWorker) SealPreCommit1(ctx context.Context, sector abi.SectorID, ticket abi.SealRandomness, pieces []abi.PieceInfo) (storage.PreCommit1Out, error) {
	panic("implement me")
}

func (s *schedTestWorker) SealPreCommit2(ctx context.Context, sector abi.SectorID, pc1o storage.PreCommit1Out) (storage.SectorCids, error) {
	panic("implement me")
}

func (s *schedTestWorker) SealCommit1(ctx context.Context, sector abi.SectorID, ticket abi.SealRandomness, seed abi.InteractiveSealRandomness, pieces []abi.PieceInfo, cids storage.SectorCids) (storage.Commit1Out, error) {
	panic("implement me")
}

func (s *schedTestWorker) SealCommit2(ctx context.Context, sector abi.SectorID, c1o storage.Commit1Out) (storage.Proof, error) {
	panic("implement me")
}

func (s *schedTestWorker) FinalizeSector(ctx context.Context, sector abi.SectorID, keepUnsealed []storage.Range) error {
	panic("implement me")
}

func (s *schedTestWorker) ReleaseUnsealed(ctx context.Context, sector abi.SectorID, safeToFree []storage.Range) error {
	panic("implement me")
}

func (s *schedTestWorker) Remove(ctx context.Context, sector abi.SectorID) error {
	panic("implement me")
}

func (s *schedTestWorker) NewSector(ctx context.Context, sector abi.SectorID) error {
	panic("implement me")
}

func (s *schedTestWorker) AddPiece(ctx context.Context, sector abi.SectorID, pieceSizes []abi.UnpaddedPieceSize, newPieceSize abi.UnpaddedPieceSize, pieceData storage.Data) (abi.PieceInfo, error) {
	panic("implement me")
}

func (s *schedTestWorker) MoveStorage(ctx context.Context, sector abi.SectorID) error {
	panic("implement me")
}

func (s *schedTestWorker) Fetch(ctx context.Context, id abi.SectorID, ft stores.SectorFileType, ptype stores.PathType, am stores.AcquireMode) error {
	panic("implement me")
}

func (s *schedTestWorker) UnsealPiece(ctx context.Context, id abi.SectorID, index storiface.UnpaddedByteIndex, size abi.UnpaddedPieceSize, randomness abi.SealRandomness, cid cid.Cid) error {
	panic("implement me")
}

func (s *schedTestWorker) ReadPiece(ctx context.Context, writer io.Writer, id abi.SectorID, index storiface.UnpaddedByteIndex, size abi.UnpaddedPieceSize) (bool, error) {
	panic("implement me")
}

func (s *schedTestWorker) TaskTypes(ctx context.Context) (map[sealtasks.TaskType]struct{}, error) {
	return s.taskTypes, nil
}

func (s *schedTestWorker) Paths(ctx context.Context) ([]stores.StoragePath, error) {
	return s.paths, nil
}

var decentWorkerResources = storiface.WorkerResources{
	MemPhysical: 128 << 30,
	MemSwap:     200 << 30,
	MemReserved: 2 << 30,
	CPUs:        32,
	GPUs:        []string{"a GPU"},
}

func (s *schedTestWorker) Info(ctx context.Context) (storiface.WorkerInfo, error) {
	return storiface.WorkerInfo{
		Hostname:  s.name,
		Resources: decentWorkerResources,
	}, nil
}

func (s *schedTestWorker) Closing(ctx context.Context) (<-chan struct{}, error) {
	return s.closing, nil
}

func (s *schedTestWorker) Close() error {
	if !s.closed {
		log.Info("close schedTestWorker")
		s.closed = true
		close(s.closing)
	}
	return nil
}

var _ Worker = &schedTestWorker{}

func addTestWorker(t *testing.T, sched *scheduler, index *stores.Index, name string, taskTypes map[sealtasks.TaskType]struct{}) {
	w := &schedTestWorker{
		name:      name,
		taskTypes: taskTypes,
		paths:     []stores.StoragePath{{ID: "bb-8", Weight: 2, LocalPath: "<octopus>food</octopus>", CanSeal: true, CanStore: true}},

		closing: make(chan struct{}),
	}

	for _, path := range w.paths {
		err := index.StorageAttach(context.TODO(), stores.StorageInfo{
			ID:       path.ID,
			URLs:     nil,
			Weight:   path.Weight,
			CanSeal:  path.CanSeal,
			CanStore: path.CanStore,
		}, fsutil.FsStat{
			Capacity:  1 << 40,
			Available: 1 << 40,
			Reserved:  3,
		})
		require.NoError(t, err)
	}

	info, err := w.Info(context.TODO())
	require.NoError(t, err)

	sched.newWorkers <- &workerHandle{
		w: w,
		wt: &workTracker{
			running: map[uint64]storiface.WorkerJob{},
		},
		info:      info,
		preparing: &activeResources{},
		active:    &activeResources{},
	}
}

func TestSchedStartStop(t *testing.T) {
	spt := abi.RegisteredSealProof_StackedDrg32GiBV1
	sched := newScheduler(spt)
	go sched.runSched()

	addTestWorker(t, sched, stores.NewIndex(), "fred", nil)

	require.NoError(t, sched.Close(context.TODO()))
}

func TestSched(t *testing.T) {
	ctx, done := context.WithTimeout(context.Background(), 30*time.Second)
	defer done()

	spt := abi.RegisteredSealProof_StackedDrg32GiBV1

	type workerSpec struct {
		name      string
		taskTypes map[sealtasks.TaskType]struct{}
	}

	noopAction := func(ctx context.Context, w Worker) error {
		return nil
	}

	type runMeta struct {
		done map[string]chan struct{}

		wg sync.WaitGroup
	}

	type task func(*testing.T, *scheduler, *stores.Index, *runMeta)

	sched := func(taskName, expectWorker string, sid abi.SectorNumber, taskType sealtasks.TaskType) task {
		_, _, l, _ := runtime.Caller(1)
		_, _, l2, _ := runtime.Caller(2)

		return func(t *testing.T, sched *scheduler, index *stores.Index, rm *runMeta) {
			done := make(chan struct{})
			rm.done[taskName] = done

			sel := newAllocSelector(index, stores.FTCache, stores.PathSealing)

			rm.wg.Add(1)
			go func() {
				defer rm.wg.Done()

				sectorNum := abi.SectorID{
					Miner:  8,
					Number: sid,
				}

				err := sched.Schedule(ctx, sectorNum, taskType, sel, func(ctx context.Context, w Worker) error {
					wi, err := w.Info(ctx)
					require.NoError(t, err)

					require.Equal(t, expectWorker, wi.Hostname)

					log.Info("IN  ", taskName)

					for {
						_, ok := <-done
						if !ok {
							break
						}
					}

					log.Info("OUT ", taskName)

					return nil
				}, noopAction)
				require.NoError(t, err, fmt.Sprint(l, l2))
			}()

			<-sched.testSync
		}
	}

	taskStarted := func(name string) task {
		_, _, l, _ := runtime.Caller(1)
		_, _, l2, _ := runtime.Caller(2)
		return func(t *testing.T, sched *scheduler, index *stores.Index, rm *runMeta) {
			select {
			case rm.done[name] <- struct{}{}:
			case <-ctx.Done():
				t.Fatal("ctx error", ctx.Err(), l, l2)
			}
		}
	}

	taskDone := func(name string) task {
		_, _, l, _ := runtime.Caller(1)
		_, _, l2, _ := runtime.Caller(2)
		return func(t *testing.T, sched *scheduler, index *stores.Index, rm *runMeta) {
			select {
			case rm.done[name] <- struct{}{}:
			case <-ctx.Done():
				t.Fatal("ctx error", ctx.Err(), l, l2)
			}
			close(rm.done[name])
		}
	}

	taskNotScheduled := func(name string) task {
		_, _, l, _ := runtime.Caller(1)
		_, _, l2, _ := runtime.Caller(2)
		return func(t *testing.T, sched *scheduler, index *stores.Index, rm *runMeta) {
			select {
			case rm.done[name] <- struct{}{}:
				t.Fatal("not expected", l, l2)
			case <-time.After(10 * time.Millisecond): // TODO: better synchronization thingy
			}
		}
	}

	testFunc := func(workers []workerSpec, tasks []task) func(t *testing.T) {
		return func(t *testing.T) {
			index := stores.NewIndex()

			sched := newScheduler(spt)
			sched.testSync = make(chan struct{})

			go sched.runSched()

			for _, worker := range workers {
				addTestWorker(t, sched, index, worker.name, worker.taskTypes)
			}

			rm := runMeta{
				done: map[string]chan struct{}{},
			}

			for i, task := range tasks {
				log.Info("TASK", i)
				task(t, sched, index, &rm)
			}

			log.Info("wait for async stuff")
			rm.wg.Wait()

			require.NoError(t, sched.Close(context.TODO()))
		}
	}

	multTask := func(tasks ...task) task {
		return func(t *testing.T, s *scheduler, index *stores.Index, meta *runMeta) {
			for _, tsk := range tasks {
				tsk(t, s, index, meta)
			}
		}
	}

	t.Run("one-pc1", testFunc([]workerSpec{
		{name: "fred", taskTypes: map[sealtasks.TaskType]struct{}{sealtasks.TTPreCommit1: {}}},
	}, []task{
		sched("pc1-1", "fred", 8, sealtasks.TTPreCommit1),
		taskDone("pc1-1"),
	}))

	t.Run("pc1-2workers-1", testFunc([]workerSpec{
		{name: "fred2", taskTypes: map[sealtasks.TaskType]struct{}{sealtasks.TTPreCommit2: {}}},
		{name: "fred1", taskTypes: map[sealtasks.TaskType]struct{}{sealtasks.TTPreCommit1: {}}},
	}, []task{
		sched("pc1-1", "fred1", 8, sealtasks.TTPreCommit1),
		taskDone("pc1-1"),
	}))

	t.Run("pc1-2workers-2", testFunc([]workerSpec{
		{name: "fred1", taskTypes: map[sealtasks.TaskType]struct{}{sealtasks.TTPreCommit1: {}}},
		{name: "fred2", taskTypes: map[sealtasks.TaskType]struct{}{sealtasks.TTPreCommit2: {}}},
	}, []task{
		sched("pc1-1", "fred1", 8, sealtasks.TTPreCommit1),
		taskDone("pc1-1"),
	}))

	t.Run("pc1-block-pc2", testFunc([]workerSpec{
		{name: "fred", taskTypes: map[sealtasks.TaskType]struct{}{sealtasks.TTPreCommit1: {}, sealtasks.TTPreCommit2: {}}},
	}, []task{
		sched("pc1", "fred", 8, sealtasks.TTPreCommit1),
		taskStarted("pc1"),

		sched("pc2", "fred", 8, sealtasks.TTPreCommit2),
		taskNotScheduled("pc2"),

		taskDone("pc1"),
		taskDone("pc2"),
	}))

	t.Run("pc2-block-pc1", testFunc([]workerSpec{
		{name: "fred", taskTypes: map[sealtasks.TaskType]struct{}{sealtasks.TTPreCommit1: {}, sealtasks.TTPreCommit2: {}}},
	}, []task{
		sched("pc2", "fred", 8, sealtasks.TTPreCommit2),
		taskStarted("pc2"),

		sched("pc1", "fred", 8, sealtasks.TTPreCommit1),
		taskNotScheduled("pc1"),

		taskDone("pc2"),
		taskDone("pc1"),
	}))

	t.Run("pc1-batching", testFunc([]workerSpec{
		{name: "fred", taskTypes: map[sealtasks.TaskType]struct{}{sealtasks.TTPreCommit1: {}}},
	}, []task{
		sched("t1", "fred", 8, sealtasks.TTPreCommit1),
		taskStarted("t1"),

		sched("t2", "fred", 8, sealtasks.TTPreCommit1),
		taskStarted("t2"),

		// with worker settings, we can only run 2 parallel PC1s

		// start 2 more to fill fetch buffer

		sched("t3", "fred", 8, sealtasks.TTPreCommit1),
		taskNotScheduled("t3"),

		sched("t4", "fred", 8, sealtasks.TTPreCommit1),
		taskNotScheduled("t4"),

		taskDone("t1"),
		taskDone("t2"),

		taskStarted("t3"),
		taskStarted("t4"),

		taskDone("t3"),
		taskDone("t4"),
	}))

	twoPC1 := func(prefix string, sid abi.SectorNumber, schedAssert func(name string) task) task {
		return multTask(
			sched(prefix+"-a", "fred", sid, sealtasks.TTPreCommit1),
			schedAssert(prefix+"-a"),

			sched(prefix+"-b", "fred", sid+1, sealtasks.TTPreCommit1),
			schedAssert(prefix+"-b"),
		)
	}

	twoPC1Act := func(prefix string, schedAssert func(name string) task) task {
		return multTask(
			schedAssert(prefix+"-a"),
			schedAssert(prefix+"-b"),
		)
	}

	diag := func() task {
		return func(t *testing.T, s *scheduler, index *stores.Index, meta *runMeta) {
			time.Sleep(20 * time.Millisecond)
			for _, request := range s.diag().Requests {
				log.Infof("!!! sDIAG: sid(%d) task(%s)", request.Sector.Number, request.TaskType)
			}

			wj := (&Manager{sched: s}).WorkerJobs()

			type line struct {
				storiface.WorkerJob
				wid uint64
			}

			lines := make([]line, 0)

			for wid, jobs := range wj {
				for _, job := range jobs {
					lines = append(lines, line{
						WorkerJob: job,
						wid:       wid,
					})
				}
			}

			// oldest first
			sort.Slice(lines, func(i, j int) bool {
				if lines[i].RunWait != lines[j].RunWait {
					return lines[i].RunWait < lines[j].RunWait
				}
				return lines[i].Start.Before(lines[j].Start)
			})

			for _, l := range lines {
				log.Infof("!!! wDIAG: rw(%d) sid(%d) t(%s)", l.RunWait, l.Sector.Number, l.Task)
			}
		}
	}

	// run this one a bunch of times, it had a very annoying tendency to fail randomly
	for i := 0; i < 40; i++ {
		t.Run("pc1-pc2-prio", testFunc([]workerSpec{
			{name: "fred", taskTypes: map[sealtasks.TaskType]struct{}{sealtasks.TTPreCommit1: {}, sealtasks.TTPreCommit2: {}}},
		}, []task{
			// fill queues
			twoPC1("w0", 0, taskStarted),
			twoPC1("w1", 2, taskNotScheduled),
			sched("w2", "fred", 4, sealtasks.TTPreCommit1),
			taskNotScheduled("w2"),

			// windowed

			sched("t1", "fred", 8, sealtasks.TTPreCommit1),
			taskNotScheduled("t1"),

			sched("t2", "fred", 9, sealtasks.TTPreCommit1),
			taskNotScheduled("t2"),

			sched("t3", "fred", 10, sealtasks.TTPreCommit2),
			taskNotScheduled("t3"),

			diag(),

			twoPC1Act("w0", taskDone),
			twoPC1Act("w1", taskStarted),
			taskNotScheduled("w2"),

			twoPC1Act("w1", taskDone),
			taskStarted("w2"),

			taskDone("w2"),

			diag(),

			taskStarted("t3"),
			taskNotScheduled("t1"),
			taskNotScheduled("t2"),

			taskDone("t3"),

			taskStarted("t1"),
			taskStarted("t2"),

			taskDone("t1"),
			taskDone("t2"),
		}))
	}
}

type slowishSelector bool

func (s slowishSelector) Ok(ctx context.Context, task sealtasks.TaskType, spt abi.RegisteredSealProof, a *workerHandle) (bool, error) {
	time.Sleep(200 * time.Microsecond)
	return bool(s), nil
}

func (s slowishSelector) Cmp(ctx context.Context, task sealtasks.TaskType, a, b *workerHandle) (bool, error) {
	time.Sleep(100 * time.Microsecond)
	return true, nil
}

var _ WorkerSelector = slowishSelector(true)

func BenchmarkTrySched(b *testing.B) {
	spt := abi.RegisteredSealProof_StackedDrg32GiBV1
	logging.SetAllLoggers(logging.LevelInfo)
	defer logging.SetAllLoggers(logging.LevelDebug)
	ctx := context.Background()

	test := func(windows, queue int) func(b *testing.B) {
		return func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				b.StopTimer()

				sched := newScheduler(spt)
				sched.workers[0] = &workerHandle{
					w: nil,
					info: storiface.WorkerInfo{
						Hostname:  "t",
						Resources: decentWorkerResources,
					},
					preparing: &activeResources{},
					active:    &activeResources{},
				}

				for i := 0; i < windows; i++ {
					sched.openWindows = append(sched.openWindows, &schedWindowRequest{
						worker: 0,
						done:   make(chan *schedWindow, 1000),
					})
				}

				for i := 0; i < queue; i++ {
					sched.schedQueue.Push(&workerRequest{
						taskType: sealtasks.TTCommit2,
						sel:      slowishSelector(true),
						ctx:      ctx,
					})
				}

				b.StartTimer()

				sched.trySched()
			}
		}
	}

	b.Run("1w-1q", test(1, 1))
	b.Run("500w-1q", test(500, 1))
	b.Run("1w-500q", test(1, 500))
	b.Run("200w-400q", test(200, 400))
}

func TestWindowCompact(t *testing.T) {
	sh := scheduler{
		spt: abi.RegisteredSealProof_StackedDrg32GiBV1,
	}

	test := func(start [][]sealtasks.TaskType, expect [][]sealtasks.TaskType) func(t *testing.T) {
		return func(t *testing.T) {
			wh := &workerHandle{
				info: storiface.WorkerInfo{
					Resources: decentWorkerResources,
				},
			}

			for _, windowTasks := range start {
				window := &schedWindow{}

				for _, task := range windowTasks {
					window.todo = append(window.todo, &workerRequest{taskType: task})
					window.allocated.add(wh.info.Resources, ResourceTable[task][sh.spt])
				}

				wh.activeWindows = append(wh.activeWindows, window)
			}

			n := sh.workerCompactWindows(wh, 0)
			require.Equal(t, len(start)-len(expect), n)

			for wi, tasks := range expect {
				var expectRes activeResources

				for ti, task := range tasks {
					require.Equal(t, task, wh.activeWindows[wi].todo[ti].taskType, "%d, %d", wi, ti)
					expectRes.add(wh.info.Resources, ResourceTable[task][sh.spt])
				}

				require.Equal(t, expectRes.cpuUse, wh.activeWindows[wi].allocated.cpuUse, "%d", wi)
				require.Equal(t, expectRes.gpuUsed, wh.activeWindows[wi].allocated.gpuUsed, "%d", wi)
				require.Equal(t, expectRes.memUsedMin, wh.activeWindows[wi].allocated.memUsedMin, "%d", wi)
				require.Equal(t, expectRes.memUsedMax, wh.activeWindows[wi].allocated.memUsedMax, "%d", wi)
			}

		}
	}

	t.Run("2-pc1-windows", test(
		[][]sealtasks.TaskType{{sealtasks.TTPreCommit1}, {sealtasks.TTPreCommit1}},
		[][]sealtasks.TaskType{{sealtasks.TTPreCommit1, sealtasks.TTPreCommit1}}),
	)

	t.Run("1-window", test(
		[][]sealtasks.TaskType{{sealtasks.TTPreCommit1, sealtasks.TTPreCommit1}},
		[][]sealtasks.TaskType{{sealtasks.TTPreCommit1, sealtasks.TTPreCommit1}}),
	)

	t.Run("2-pc2-windows", test(
		[][]sealtasks.TaskType{{sealtasks.TTPreCommit2}, {sealtasks.TTPreCommit2}},
		[][]sealtasks.TaskType{{sealtasks.TTPreCommit2}, {sealtasks.TTPreCommit2}}),
	)

	t.Run("2pc1-pc1ap", test(
		[][]sealtasks.TaskType{{sealtasks.TTPreCommit1, sealtasks.TTPreCommit1}, {sealtasks.TTPreCommit1, sealtasks.TTAddPiece}},
		[][]sealtasks.TaskType{{sealtasks.TTPreCommit1, sealtasks.TTPreCommit1, sealtasks.TTAddPiece}, {sealtasks.TTPreCommit1}}),
	)

	t.Run("2pc1-pc1appc2", test(
		[][]sealtasks.TaskType{{sealtasks.TTPreCommit1, sealtasks.TTPreCommit1}, {sealtasks.TTPreCommit1, sealtasks.TTAddPiece, sealtasks.TTPreCommit2}},
		[][]sealtasks.TaskType{{sealtasks.TTPreCommit1, sealtasks.TTPreCommit1, sealtasks.TTAddPiece}, {sealtasks.TTPreCommit1, sealtasks.TTPreCommit2}}),
	)
}
