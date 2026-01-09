package sealer

import (
	"context"
	"fmt"
	"io"
	"runtime"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log/v2"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-state-types/abi"
	prooftypes "github.com/filecoin-project/go-state-types/proof"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/storage/paths"
	"github.com/filecoin-project/lotus/storage/sealer/fsutil"
	"github.com/filecoin-project/lotus/storage/sealer/sealtasks"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
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

var decentWorkerResources = storiface.WorkerResources{
	MemPhysical: 128 << 30,
	MemSwap:     200 << 30,
	MemUsed:     1 << 30,
	MemSwapUsed: 1 << 30,
	CPUs:        32,
	GPUs:        []string{},
}

var constrainedWorkerResources = storiface.WorkerResources{
	MemPhysical: 1 << 30,
	MemUsed:     1 << 30,
	MemSwapUsed: 1 << 30,
	CPUs:        1,
}

type schedTestWorker struct {
	name      string
	taskTypes map[sealtasks.TaskType]struct{}
	paths     []storiface.StoragePath

	closed  bool
	session uuid.UUID

	resources       storiface.WorkerResources
	ignoreResources bool
}

func (s *schedTestWorker) DownloadSectorData(ctx context.Context, sector storiface.SectorRef, finalized bool, src map[storiface.SectorFileType]storiface.SectorLocation) (storiface.CallID, error) {
	panic("implement me")
}

func (s *schedTestWorker) DataCid(ctx context.Context, pieceSize abi.UnpaddedPieceSize, pieceData storiface.Data) (storiface.CallID, error) {
	panic("implement me")
}

func (s *schedTestWorker) SealPreCommit1(ctx context.Context, sector storiface.SectorRef, ticket abi.SealRandomness, pieces []abi.PieceInfo) (storiface.CallID, error) {
	panic("implement me")
}

func (s *schedTestWorker) SealPreCommit2(ctx context.Context, sector storiface.SectorRef, pc1o storiface.PreCommit1Out) (storiface.CallID, error) {
	panic("implement me")
}

func (s *schedTestWorker) SealCommit1(ctx context.Context, sector storiface.SectorRef, ticket abi.SealRandomness, seed abi.InteractiveSealRandomness, pieces []abi.PieceInfo, cids storiface.SectorCids) (storiface.CallID, error) {
	panic("implement me")
}

func (s *schedTestWorker) SealCommit2(ctx context.Context, sector storiface.SectorRef, c1o storiface.Commit1Out) (storiface.CallID, error) {
	panic("implement me")
}

func (s *schedTestWorker) FinalizeSector(ctx context.Context, sector storiface.SectorRef) (storiface.CallID, error) {
	panic("implement me")
}

func (s *schedTestWorker) ReleaseUnsealed(ctx context.Context, sector storiface.SectorRef, keepUnsealed []storiface.Range) (storiface.CallID, error) {
	panic("implement me")
}

func (s *schedTestWorker) Remove(ctx context.Context, sector storiface.SectorRef) (storiface.CallID, error) {
	panic("implement me")
}

func (s *schedTestWorker) NewSector(ctx context.Context, sector storiface.SectorRef) (storiface.CallID, error) {
	panic("implement me")
}

func (s *schedTestWorker) AddPiece(ctx context.Context, sector storiface.SectorRef, pieceSizes []abi.UnpaddedPieceSize, newPieceSize abi.UnpaddedPieceSize, pieceData storiface.Data) (storiface.CallID, error) {
	panic("implement me")
}

func (s *schedTestWorker) ReplicaUpdate(ctx context.Context, sector storiface.SectorRef, pieces []abi.PieceInfo) (storiface.CallID, error) {
	panic("implement me")
}

func (s *schedTestWorker) ProveReplicaUpdate1(ctx context.Context, sector storiface.SectorRef, sectorKey, newSealed, newUnsealed cid.Cid) (storiface.CallID, error) {
	panic("implement me")
}

func (s *schedTestWorker) ProveReplicaUpdate2(ctx context.Context, sector storiface.SectorRef, sectorKey, newSealed, newUnsealed cid.Cid, vanillaProofs storiface.ReplicaVanillaProofs) (storiface.CallID, error) {
	panic("implement me")
}

func (s *schedTestWorker) GenerateSectorKeyFromData(ctx context.Context, sector storiface.SectorRef, commD cid.Cid) (storiface.CallID, error) {
	panic("implement me")
}

func (s *schedTestWorker) FinalizeReplicaUpdate(ctx context.Context, sector storiface.SectorRef) (storiface.CallID, error) {
	panic("implement me")
}

func (s *schedTestWorker) MoveStorage(ctx context.Context, sector storiface.SectorRef, types storiface.SectorFileType) (storiface.CallID, error) {
	panic("implement me")
}

func (s *schedTestWorker) Fetch(ctx context.Context, id storiface.SectorRef, ft storiface.SectorFileType, ptype storiface.PathType, am storiface.AcquireMode) (storiface.CallID, error) {
	panic("implement me")
}

func (s *schedTestWorker) UnsealPiece(ctx context.Context, id storiface.SectorRef, index storiface.UnpaddedByteIndex, size abi.UnpaddedPieceSize, randomness abi.SealRandomness, cid cid.Cid) (storiface.CallID, error) {
	panic("implement me")
}

func (s *schedTestWorker) ReadPiece(ctx context.Context, writer io.Writer, id storiface.SectorRef, index storiface.UnpaddedByteIndex, size abi.UnpaddedPieceSize) (storiface.CallID, error) {
	panic("implement me")
}

func (s *schedTestWorker) GenerateWinningPoSt(ctx context.Context, ppt abi.RegisteredPoStProof, mid abi.ActorID, sectors []storiface.PostSectorChallenge, randomness abi.PoStRandomness) ([]prooftypes.PoStProof, error) {
	panic("implement me")
}

func (s *schedTestWorker) GenerateWindowPoSt(ctx context.Context, ppt abi.RegisteredPoStProof, mid abi.ActorID, sectors []storiface.PostSectorChallenge, partitionIdx int, randomness abi.PoStRandomness) (storiface.WindowPoStResult, error) {
	panic("implement me")
}

func (s *schedTestWorker) TaskTypes(ctx context.Context) (map[sealtasks.TaskType]struct{}, error) {
	return s.taskTypes, nil
}

func (s *schedTestWorker) Paths(ctx context.Context) ([]storiface.StoragePath, error) {
	return s.paths, nil
}

func (s *schedTestWorker) Info(ctx context.Context) (storiface.WorkerInfo, error) {
	return storiface.WorkerInfo{
		Hostname:        s.name,
		IgnoreResources: s.ignoreResources,
		Resources:       s.resources,
	}, nil
}

func (s *schedTestWorker) Session(context.Context) (uuid.UUID, error) {
	return s.session, nil
}

func (s *schedTestWorker) Close() error {
	if !s.closed {
		log.Info("close schedTestWorker")
		s.closed = true
		s.session = uuid.UUID{}
	}
	return nil
}

var _ Worker = &schedTestWorker{}

func addTestWorker(t *testing.T, sched *Scheduler, index *paths.MemIndex, name string, taskTypes map[sealtasks.TaskType]struct{}, resources storiface.WorkerResources, ignoreResources bool) {
	w := &schedTestWorker{
		name:      name,
		taskTypes: taskTypes,
		paths:     []storiface.StoragePath{{ID: "bb-8", Weight: 2, LocalPath: "<octopus>food</octopus>", CanSeal: true, CanStore: true}},

		session: uuid.New(),

		resources:       resources,
		ignoreResources: ignoreResources,
	}

	for _, path := range w.paths {
		err := index.StorageAttach(context.TODO(), storiface.StorageInfo{
			ID:       path.ID,
			URLs:     nil,
			Weight:   path.Weight,
			CanSeal:  path.CanSeal,
			CanStore: path.CanStore,
		}, fsutil.FsStat{
			Capacity:    1 << 40,
			Available:   1 << 40,
			FSAvailable: 1 << 40,
			Reserved:    3,
		})
		require.NoError(t, err)
	}

	sessID, err := w.Session(context.TODO())
	require.NoError(t, err)

	wid := storiface.WorkerID(sessID)

	wh, err := newWorkerHandle(context.TODO(), w)
	require.NoError(t, err)

	require.NoError(t, sched.runWorker(context.TODO(), wid, wh))
}

func TestSchedStartStop(t *testing.T) {
	sched, err := newScheduler(context.Background(), "")
	require.NoError(t, err)
	go sched.runSched()

	addTestWorker(t, sched, paths.NewMemIndex(nil), "fred", nil, decentWorkerResources, false)

	require.NoError(t, sched.Close(context.TODO()))
}

func TestSched(t *testing.T) {
	storiface.ParallelNum = 1
	storiface.ParallelDenom = 1

	ctx, done := context.WithTimeout(context.Background(), 30*time.Second)
	defer done()

	spt := abi.RegisteredSealProof_StackedDrg32GiBV1

	type workerSpec struct {
		name      string
		taskTypes map[sealtasks.TaskType]struct{}

		resources       storiface.WorkerResources
		ignoreResources bool
	}

	noopAction := func(ctx context.Context, w Worker) error {
		return nil
	}

	type runMeta struct {
		done map[string]chan struct{}

		wg sync.WaitGroup
	}

	type task func(*testing.T, *Scheduler, *paths.MemIndex, *runMeta)

	sched := func(taskName, expectWorker string, sid abi.SectorNumber, taskType sealtasks.TaskType) task {
		_, _, l, _ := runtime.Caller(1)
		_, _, l2, _ := runtime.Caller(2)

		return func(t *testing.T, sched *Scheduler, index *paths.MemIndex, rm *runMeta) {
			done := make(chan struct{})
			rm.done[taskName] = done

			sel := newAllocSelector(index, storiface.FTCache, storiface.PathSealing, abi.ActorID(1000))

			rm.wg.Add(1)
			go func() {
				defer rm.wg.Done()

				sectorRef := storiface.SectorRef{
					ID: abi.SectorID{
						Miner:  8,
						Number: sid,
					},
					ProofType: spt,
				}

				prep := PrepareAction{
					Action: func(ctx context.Context, w Worker) error {
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
					},
					PrepType: taskType,
				}

				err := sched.Schedule(ctx, sectorRef, taskType, sel, prep, noopAction)
				if err != context.Canceled {
					require.NoError(t, err, fmt.Sprint(l, l2))
				}
			}()

			<-sched.testSync
		}
	}

	taskStarted := func(name string) task {
		_, _, l, _ := runtime.Caller(1)
		_, _, l2, _ := runtime.Caller(2)
		return func(t *testing.T, sched *Scheduler, index *paths.MemIndex, rm *runMeta) {
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
		return func(t *testing.T, sched *Scheduler, index *paths.MemIndex, rm *runMeta) {
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
		return func(t *testing.T, sched *Scheduler, index *paths.MemIndex, rm *runMeta) {
			select {
			case rm.done[name] <- struct{}{}:
				t.Fatal("not expected", l, l2)
			case <-time.After(10 * time.Millisecond): // TODO: better synchronization thingy
			}
		}
	}

	testFunc := func(workers []workerSpec, tasks []task) func(t *testing.T) {
		return func(t *testing.T) {
			index := paths.NewMemIndex(nil)

			sched, err := newScheduler(ctx, "")
			require.NoError(t, err)
			sched.testSync = make(chan struct{})

			go sched.runSched()

			for _, worker := range workers {
				addTestWorker(t, sched, index, worker.name, worker.taskTypes, worker.resources, worker.ignoreResources)
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
		return func(t *testing.T, s *Scheduler, index *paths.MemIndex, meta *runMeta) {
			for _, tsk := range tasks {
				tsk(t, s, index, meta)
			}
		}
	}

	// checks behaviour with workers with constrained resources
	// the first one is not ignoring resource constraints, so we assign to the second worker, who is
	t.Run("constrained-resources", testFunc([]workerSpec{
		{name: "fred1", resources: constrainedWorkerResources, taskTypes: map[sealtasks.TaskType]struct{}{sealtasks.TTPreCommit1: {}}},
		{name: "fred2", resources: constrainedWorkerResources, ignoreResources: true, taskTypes: map[sealtasks.TaskType]struct{}{sealtasks.TTPreCommit1: {}}},
	}, []task{
		sched("pc1-1", "fred2", 8, sealtasks.TTPreCommit1),
		taskStarted("pc1-1"),
		taskDone("pc1-1"),
	}))

	t.Run("one-pc1", testFunc([]workerSpec{
		{name: "fred", resources: decentWorkerResources, taskTypes: map[sealtasks.TaskType]struct{}{sealtasks.TTPreCommit1: {}}},
	}, []task{
		sched("pc1-1", "fred", 8, sealtasks.TTPreCommit1),
		taskDone("pc1-1"),
	}))

	t.Run("pc1-2workers-1", testFunc([]workerSpec{
		{name: "fred2", resources: decentWorkerResources, taskTypes: map[sealtasks.TaskType]struct{}{sealtasks.TTPreCommit2: {}}},
		{name: "fred1", resources: decentWorkerResources, taskTypes: map[sealtasks.TaskType]struct{}{sealtasks.TTPreCommit1: {}}},
	}, []task{
		sched("pc1-1", "fred1", 8, sealtasks.TTPreCommit1),
		taskDone("pc1-1"),
	}))

	t.Run("pc1-2workers-2", testFunc([]workerSpec{
		{name: "fred1", resources: decentWorkerResources, taskTypes: map[sealtasks.TaskType]struct{}{sealtasks.TTPreCommit1: {}}},
		{name: "fred2", resources: decentWorkerResources, taskTypes: map[sealtasks.TaskType]struct{}{sealtasks.TTPreCommit2: {}}},
	}, []task{
		sched("pc1-1", "fred1", 8, sealtasks.TTPreCommit1),
		taskDone("pc1-1"),
	}))

	t.Run("pc1-block-pc2", testFunc([]workerSpec{
		{name: "fred", resources: decentWorkerResources, taskTypes: map[sealtasks.TaskType]struct{}{sealtasks.TTPreCommit1: {}, sealtasks.TTPreCommit2: {}}},
	}, []task{
		sched("pc1", "fred", 8, sealtasks.TTPreCommit1),
		taskStarted("pc1"),

		sched("pc2", "fred", 8, sealtasks.TTPreCommit2),
		taskNotScheduled("pc2"),

		taskDone("pc1"),
		taskDone("pc2"),
	}))

	t.Run("pc2-block-pc1", testFunc([]workerSpec{
		{name: "fred", resources: decentWorkerResources, taskTypes: map[sealtasks.TaskType]struct{}{sealtasks.TTPreCommit1: {}, sealtasks.TTPreCommit2: {}}},
	}, []task{
		sched("pc2", "fred", 8, sealtasks.TTPreCommit2),
		taskStarted("pc2"),

		sched("pc1", "fred", 8, sealtasks.TTPreCommit1),
		taskNotScheduled("pc1"),

		taskDone("pc2"),
		taskDone("pc1"),
	}))

	t.Run("pc1-batching", testFunc([]workerSpec{
		{name: "fred", resources: decentWorkerResources, taskTypes: map[sealtasks.TaskType]struct{}{sealtasks.TTPreCommit1: {}}},
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
		return func(t *testing.T, s *Scheduler, index *paths.MemIndex, meta *runMeta) {
			time.Sleep(20 * time.Millisecond)
			for _, request := range s.diag().Requests {
				log.Infof("!!! sDIAG: sid(%d) task(%s)", request.Sector.Number, request.TaskType)
			}

			wj := (&Manager{sched: s}).WorkerJobs()

			type line struct {
				storiface.WorkerJob
				wid uuid.UUID
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
			{name: "fred", resources: decentWorkerResources, taskTypes: map[sealtasks.TaskType]struct{}{sealtasks.TTPreCommit1: {}, sealtasks.TTPreCommit2: {}}},
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

func (s slowishSelector) Ok(ctx context.Context, task sealtasks.TaskType, spt abi.RegisteredSealProof, a SchedWorker) (bool, bool, error) {
	// note: we don't care about output here, just the time those calls take
	// (selector Ok/Cmp is called in the scheduler)
	_, _ = a.Paths(ctx)
	_, _ = a.TaskTypes(ctx)
	return bool(s), false, nil
}

func (s slowishSelector) Cmp(ctx context.Context, task sealtasks.TaskType, a, b SchedWorker) (bool, error) {
	// note: we don't care about output here, just the time those calls take
	// (selector Ok/Cmp is called in the scheduler)
	_, _ = a.Paths(ctx)
	return true, nil
}

var _ WorkerSelector = slowishSelector(true)

type tw struct {
	api.Worker
	io.Closer
}

func BenchmarkTrySched(b *testing.B) {
	logging.SetAllLoggers(logging.LevelInfo)
	defer logging.SetAllLoggers(logging.LevelDebug)
	ctx := context.Background()

	test := func(windows, queue int) func(b *testing.B) {
		return func(b *testing.B) {
			for b.Loop() {
				b.StopTimer()

				var whnd api.WorkerStruct
				whnd.Internal.TaskTypes = func(p0 context.Context) (map[sealtasks.TaskType]struct{}, error) {
					time.Sleep(100 * time.Microsecond)
					return nil, nil
				}
				whnd.Internal.Paths = func(p0 context.Context) ([]storiface.StoragePath, error) {
					time.Sleep(100 * time.Microsecond)
					return nil, nil
				}

				sched, err := newScheduler(ctx, "")
				require.NoError(b, err)
				sched.Workers[storiface.WorkerID{}] = &WorkerHandle{
					workerRpc: &tw{Worker: &whnd},
					Info: storiface.WorkerInfo{
						Hostname:  "t",
						Resources: decentWorkerResources,
					},
					Enabled:   true,
					preparing: NewActiveResources(newTaskCounter()),
					active:    NewActiveResources(newTaskCounter()),
				}

				for i := 0; i < windows; i++ {
					sched.OpenWindows = append(sched.OpenWindows, &SchedWindowRequest{
						Worker: storiface.WorkerID{},
						Done:   make(chan *SchedWindow, 1000),
					})
				}

				for i := 0; i < queue; i++ {
					sched.SchedQueue.Push(&WorkerRequest{
						TaskType: sealtasks.TTCommit2,
						Sel:      slowishSelector(true),
						Ctx:      ctx,
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
	sh := Scheduler{}
	spt := abi.RegisteredSealProof_StackedDrg32GiBV1

	test := func(start [][]sealtasks.TaskType, expect [][]sealtasks.TaskType) func(t *testing.T) {
		return func(t *testing.T) {
			wh := &WorkerHandle{
				Info: storiface.WorkerInfo{
					Resources: decentWorkerResources,
				},
			}

			for _, windowTasks := range start {
				window := &SchedWindow{
					Allocated: *NewActiveResources(newTaskCounter()),
				}

				for _, task := range windowTasks {
					window.Todo = append(window.Todo, &WorkerRequest{
						TaskType: task,
						Sector:   storiface.SectorRef{ProofType: spt},
					})
					window.Allocated.Add(uuid.UUID{}, task.SealTask(spt), wh.Info.Resources, storiface.ResourceTable[task][spt])
				}

				wh.activeWindows = append(wh.activeWindows, window)
			}

			sw := schedWorker{
				sched:  &sh,
				worker: wh,
			}

			sw.workerCompactWindows()
			require.Equal(t, len(start)-len(expect), -sw.windowsRequested)

			for wi, tasks := range expect {
				expectRes := NewActiveResources(newTaskCounter())

				for ti, task := range tasks {
					require.Equal(t, task, wh.activeWindows[wi].Todo[ti].TaskType, "%d, %d", wi, ti)
					expectRes.Add(uuid.UUID{}, task.SealTask(spt), wh.Info.Resources, storiface.ResourceTable[task][spt])
				}

				require.Equal(t, expectRes.cpuUse, wh.activeWindows[wi].Allocated.cpuUse, "%d", wi)
				require.Equal(t, expectRes.gpuUsed, wh.activeWindows[wi].Allocated.gpuUsed, "%d", wi)
				require.Equal(t, expectRes.memUsedMin, wh.activeWindows[wi].Allocated.memUsedMin, "%d", wi)
				require.Equal(t, expectRes.memUsedMax, wh.activeWindows[wi].Allocated.memUsedMax, "%d", wi)
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
