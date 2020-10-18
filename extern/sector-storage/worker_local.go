package sectorstorage

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"reflect"
	"runtime"
	"sync"
	"time"

	"github.com/elastic/go-sysinfo"
	"github.com/google/uuid"
	"github.com/hashicorp/go-multierror"
	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"

	ffi "github.com/filecoin-project/filecoin-ffi"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-statestore"
	storage2 "github.com/filecoin-project/specs-storage/storage"

	"github.com/filecoin-project/lotus/extern/sector-storage/ffiwrapper"
	"github.com/filecoin-project/lotus/extern/sector-storage/sealtasks"
	"github.com/filecoin-project/lotus/extern/sector-storage/stores"
	"github.com/filecoin-project/lotus/extern/sector-storage/storiface"
)

var pathTypes = []storiface.SectorFileType{storiface.FTUnsealed, storiface.FTSealed, storiface.FTCache}

type WorkerConfig struct {
	SealProof abi.RegisteredSealProof
	TaskTypes []sealtasks.TaskType
	NoSwap    bool
}

type LocalWorker struct {
	scfg       *ffiwrapper.Config
	storage    stores.Store
	localStore *stores.Local
	sindex     stores.SectorIndex
	ret        storiface.WorkerReturn
	executor   func() (ffiwrapper.Storage, error)
	noSwap     bool

	ct          *workerCallTracker
	acceptTasks map[sealtasks.TaskType]struct{}
	running     sync.WaitGroup

	session uuid.UUID
	closing chan struct{}
}

func newLocalWorker(executor func() (ffiwrapper.Storage, error), wcfg WorkerConfig, store stores.Store, local *stores.Local, sindex stores.SectorIndex, ret storiface.WorkerReturn, cst *statestore.StateStore) *LocalWorker {
	acceptTasks := map[sealtasks.TaskType]struct{}{}
	for _, taskType := range wcfg.TaskTypes {
		acceptTasks[taskType] = struct{}{}
	}

	w := &LocalWorker{
		scfg: &ffiwrapper.Config{
			SealProofType: wcfg.SealProof,
		},
		storage:    store,
		localStore: local,
		sindex:     sindex,
		ret:        ret,

		ct: &workerCallTracker{
			st: cst,
		},
		acceptTasks: acceptTasks,
		executor:    executor,
		noSwap:      wcfg.NoSwap,

		session: uuid.New(),
		closing: make(chan struct{}),
	}

	if w.executor == nil {
		w.executor = w.ffiExec
	}

	unfinished, err := w.ct.unfinished()
	if err != nil {
		log.Errorf("reading unfinished tasks: %+v", err)
		return w
	}

	go func() {
		for _, call := range unfinished {
			err := xerrors.Errorf("worker restarted")

			if err := returnFunc[call.RetType](context.TODO(), call.ID, ret, nil, err); err != nil {
				log.Errorf("return error: %s: %+v", call.RetType, err)
				continue
			}

			if err := w.ct.onReturned(call.ID); err != nil {
				log.Errorf("marking call as returned failed: %s: %+v", call.RetType, err)
			}
		}
	}()

	return w
}

func NewLocalWorker(wcfg WorkerConfig, store stores.Store, local *stores.Local, sindex stores.SectorIndex, ret storiface.WorkerReturn, cst *statestore.StateStore) *LocalWorker {
	return newLocalWorker(nil, wcfg, store, local, sindex, ret, cst)
}

type localWorkerPathProvider struct {
	w  *LocalWorker
	op storiface.AcquireMode
}

func (l *localWorkerPathProvider) AcquireSector(ctx context.Context, sector abi.SectorID, existing storiface.SectorFileType, allocate storiface.SectorFileType, sealing storiface.PathType) (storiface.SectorPaths, func(), error) {

	paths, storageIDs, err := l.w.storage.AcquireSector(ctx, sector, l.w.scfg.SealProofType, existing, allocate, sealing, l.op)
	if err != nil {
		return storiface.SectorPaths{}, nil, err
	}

	releaseStorage, err := l.w.localStore.Reserve(ctx, sector, l.w.scfg.SealProofType, allocate, storageIDs, storiface.FSOverheadSeal)
	if err != nil {
		return storiface.SectorPaths{}, nil, xerrors.Errorf("reserving storage space: %w", err)
	}

	log.Debugf("acquired sector %d (e:%d; a:%d): %v", sector, existing, allocate, paths)

	return paths, func() {
		releaseStorage()

		for _, fileType := range pathTypes {
			if fileType&allocate == 0 {
				continue
			}

			sid := storiface.PathByType(storageIDs, fileType)

			if err := l.w.sindex.StorageDeclareSector(ctx, stores.ID(sid), sector, fileType, l.op == storiface.AcquireMove); err != nil {
				log.Errorf("declare sector error: %+v", err)
			}
		}
	}, nil
}

func (l *LocalWorker) ffiExec() (ffiwrapper.Storage, error) {
	return ffiwrapper.New(&localWorkerPathProvider{w: l}, l.scfg)
}

type ReturnType string

// in: func(WorkerReturn, context.Context, CallID, err string)
// in: func(WorkerReturn, context.Context, CallID, ret T, err string)
func rfunc(in interface{}) func(context.Context, storiface.CallID, storiface.WorkerReturn, interface{}, error) error {
	rf := reflect.ValueOf(in)
	ft := rf.Type()
	withRet := ft.NumIn() == 5

	return func(ctx context.Context, ci storiface.CallID, wr storiface.WorkerReturn, i interface{}, err error) error {
		rctx := reflect.ValueOf(ctx)
		rwr := reflect.ValueOf(wr)
		rerr := reflect.ValueOf(errstr(err))
		rci := reflect.ValueOf(ci)

		var ro []reflect.Value

		if withRet {
			ret := reflect.ValueOf(i)
			if i == nil {
				ret = reflect.Zero(rf.Type().In(3))
			}

			ro = rf.Call([]reflect.Value{rwr, rctx, rci, ret, rerr})
		} else {
			ro = rf.Call([]reflect.Value{rwr, rctx, rci, rerr})
		}

		if !ro[0].IsNil() {
			return ro[0].Interface().(error)
		}

		return nil
	}
}

var returnFunc = map[ReturnType]func(context.Context, storiface.CallID, storiface.WorkerReturn, interface{}, error) error{
	"AddPiece":        rfunc(storiface.WorkerReturn.ReturnAddPiece),
	"SealPreCommit1":  rfunc(storiface.WorkerReturn.ReturnSealPreCommit1),
	"SealPreCommit2":  rfunc(storiface.WorkerReturn.ReturnSealPreCommit2),
	"SealCommit1":     rfunc(storiface.WorkerReturn.ReturnSealCommit1),
	"SealCommit2":     rfunc(storiface.WorkerReturn.ReturnSealCommit2),
	"FinalizeSector":  rfunc(storiface.WorkerReturn.ReturnFinalizeSector),
	"ReleaseUnsealed": rfunc(storiface.WorkerReturn.ReturnReleaseUnsealed),
	"MoveStorage":     rfunc(storiface.WorkerReturn.ReturnMoveStorage),
	"UnsealPiece":     rfunc(storiface.WorkerReturn.ReturnUnsealPiece),
	"ReadPiece":       rfunc(storiface.WorkerReturn.ReturnReadPiece),
	"Fetch":           rfunc(storiface.WorkerReturn.ReturnFetch),
}

func (l *LocalWorker) asyncCall(ctx context.Context, sector abi.SectorID, rt ReturnType, work func(ctx context.Context, ci storiface.CallID) (interface{}, error)) (storiface.CallID, error) {
	ci := storiface.CallID{
		Sector: sector,
		ID:     uuid.New(),
	}

	if err := l.ct.onStart(ci, rt); err != nil {
		log.Errorf("tracking call (start): %+v", err)
	}

	l.running.Add(1)

	go func() {
		defer l.running.Done()

		res, err := work(&wctx{
			vals:    ctx,
			closing: l.closing,
		}, ci)

		{
			rb, err := json.Marshal(res)
			if err != nil {
				log.Errorf("tracking call (marshaling results): %+v", err)
			} else {
				if err := l.ct.onDone(ci, rb); err != nil {
					log.Errorf("tracking call (done): %+v", err)
				}
			}

		}

		if err := returnFunc[rt](ctx, ci, l.ret, res, err); err != nil {
			log.Errorf("return error: %s: %+v", rt, err)
			return
		}

		if err := l.ct.onReturned(ci); err != nil {
			log.Errorf("tracking call (done): %+v", err)
		}
	}()

	return ci, nil
}

func errstr(err error) string {
	if err != nil {
		return err.Error()
	}

	return ""
}

func (l *LocalWorker) NewSector(ctx context.Context, sector abi.SectorID) error {
	sb, err := l.executor()
	if err != nil {
		return err
	}

	return sb.NewSector(ctx, sector)
}

func (l *LocalWorker) AddPiece(ctx context.Context, sector abi.SectorID, epcs []abi.UnpaddedPieceSize, sz abi.UnpaddedPieceSize, r io.Reader) (storiface.CallID, error) {
	sb, err := l.executor()
	if err != nil {
		return storiface.UndefCall, err
	}

	return l.asyncCall(ctx, sector, "AddPiece", func(ctx context.Context, ci storiface.CallID) (interface{}, error) {
		return sb.AddPiece(ctx, sector, epcs, sz, r)
	})
}

func (l *LocalWorker) Fetch(ctx context.Context, sector abi.SectorID, fileType storiface.SectorFileType, ptype storiface.PathType, am storiface.AcquireMode) (storiface.CallID, error) {
	return l.asyncCall(ctx, sector, "Fetch", func(ctx context.Context, ci storiface.CallID) (interface{}, error) {
		_, done, err := (&localWorkerPathProvider{w: l, op: am}).AcquireSector(ctx, sector, fileType, storiface.FTNone, ptype)
		if err == nil {
			done()
		}

		return nil, err
	})
}

func (l *LocalWorker) SealPreCommit1(ctx context.Context, sector abi.SectorID, ticket abi.SealRandomness, pieces []abi.PieceInfo) (storiface.CallID, error) {
	return l.asyncCall(ctx, sector, "SealPreCommit1", func(ctx context.Context, ci storiface.CallID) (interface{}, error) {

		{
			// cleanup previous failed attempts if they exist
			if err := l.storage.Remove(ctx, sector, storiface.FTSealed, true); err != nil {
				return nil, xerrors.Errorf("cleaning up sealed data: %w", err)
			}

			if err := l.storage.Remove(ctx, sector, storiface.FTCache, true); err != nil {
				return nil, xerrors.Errorf("cleaning up cache data: %w", err)
			}
		}

		sb, err := l.executor()
		if err != nil {
			return nil, err
		}

		return sb.SealPreCommit1(ctx, sector, ticket, pieces)
	})
}

func (l *LocalWorker) SealPreCommit2(ctx context.Context, sector abi.SectorID, phase1Out storage2.PreCommit1Out) (storiface.CallID, error) {
	sb, err := l.executor()
	if err != nil {
		return storiface.UndefCall, err
	}

	return l.asyncCall(ctx, sector, "SealPreCommit2", func(ctx context.Context, ci storiface.CallID) (interface{}, error) {
		return sb.SealPreCommit2(ctx, sector, phase1Out)
	})
}

func (l *LocalWorker) SealCommit1(ctx context.Context, sector abi.SectorID, ticket abi.SealRandomness, seed abi.InteractiveSealRandomness, pieces []abi.PieceInfo, cids storage2.SectorCids) (storiface.CallID, error) {
	sb, err := l.executor()
	if err != nil {
		return storiface.UndefCall, err
	}

	return l.asyncCall(ctx, sector, "SealCommit1", func(ctx context.Context, ci storiface.CallID) (interface{}, error) {
		return sb.SealCommit1(ctx, sector, ticket, seed, pieces, cids)
	})
}

func (l *LocalWorker) SealCommit2(ctx context.Context, sector abi.SectorID, phase1Out storage2.Commit1Out) (storiface.CallID, error) {
	sb, err := l.executor()
	if err != nil {
		return storiface.UndefCall, err
	}

	return l.asyncCall(ctx, sector, "SealCommit2", func(ctx context.Context, ci storiface.CallID) (interface{}, error) {
		return sb.SealCommit2(ctx, sector, phase1Out)
	})
}

func (l *LocalWorker) FinalizeSector(ctx context.Context, sector abi.SectorID, keepUnsealed []storage2.Range) (storiface.CallID, error) {
	sb, err := l.executor()
	if err != nil {
		return storiface.UndefCall, err
	}

	return l.asyncCall(ctx, sector, "FinalizeSector", func(ctx context.Context, ci storiface.CallID) (interface{}, error) {
		if err := sb.FinalizeSector(ctx, sector, keepUnsealed); err != nil {
			return nil, xerrors.Errorf("finalizing sector: %w", err)
		}

		if len(keepUnsealed) == 0 {
			if err := l.storage.Remove(ctx, sector, storiface.FTUnsealed, true); err != nil {
				return nil, xerrors.Errorf("removing unsealed data: %w", err)
			}
		}

		return nil, err
	})
}

func (l *LocalWorker) ReleaseUnsealed(ctx context.Context, sector abi.SectorID, safeToFree []storage2.Range) (storiface.CallID, error) {
	return storiface.UndefCall, xerrors.Errorf("implement me")
}

func (l *LocalWorker) Remove(ctx context.Context, sector abi.SectorID) error {
	var err error

	if rerr := l.storage.Remove(ctx, sector, storiface.FTSealed, true); rerr != nil {
		err = multierror.Append(err, xerrors.Errorf("removing sector (sealed): %w", rerr))
	}
	if rerr := l.storage.Remove(ctx, sector, storiface.FTCache, true); rerr != nil {
		err = multierror.Append(err, xerrors.Errorf("removing sector (cache): %w", rerr))
	}
	if rerr := l.storage.Remove(ctx, sector, storiface.FTUnsealed, true); rerr != nil {
		err = multierror.Append(err, xerrors.Errorf("removing sector (unsealed): %w", rerr))
	}

	return err
}

func (l *LocalWorker) MoveStorage(ctx context.Context, sector abi.SectorID, types storiface.SectorFileType) (storiface.CallID, error) {
	return l.asyncCall(ctx, sector, "MoveStorage", func(ctx context.Context, ci storiface.CallID) (interface{}, error) {
		return nil, l.storage.MoveStorage(ctx, sector, l.scfg.SealProofType, types)
	})
}

func (l *LocalWorker) UnsealPiece(ctx context.Context, sector abi.SectorID, index storiface.UnpaddedByteIndex, size abi.UnpaddedPieceSize, randomness abi.SealRandomness, cid cid.Cid) (storiface.CallID, error) {
	sb, err := l.executor()
	if err != nil {
		return storiface.UndefCall, err
	}

	return l.asyncCall(ctx, sector, "UnsealPiece", func(ctx context.Context, ci storiface.CallID) (interface{}, error) {
		if err = sb.UnsealPiece(ctx, sector, index, size, randomness, cid); err != nil {
			return nil, xerrors.Errorf("unsealing sector: %w", err)
		}

		if err = l.storage.RemoveCopies(ctx, sector, storiface.FTSealed); err != nil {
			return nil, xerrors.Errorf("removing source data: %w", err)
		}

		if err = l.storage.RemoveCopies(ctx, sector, storiface.FTCache); err != nil {
			return nil, xerrors.Errorf("removing source data: %w", err)
		}

		return nil, nil
	})
}

func (l *LocalWorker) ReadPiece(ctx context.Context, writer io.Writer, sector abi.SectorID, index storiface.UnpaddedByteIndex, size abi.UnpaddedPieceSize) (storiface.CallID, error) {
	sb, err := l.executor()
	if err != nil {
		return storiface.UndefCall, err
	}

	return l.asyncCall(ctx, sector, "ReadPiece", func(ctx context.Context, ci storiface.CallID) (interface{}, error) {
		return sb.ReadPiece(ctx, writer, sector, index, size)
	})
}

func (l *LocalWorker) TaskTypes(context.Context) (map[sealtasks.TaskType]struct{}, error) {
	return l.acceptTasks, nil
}

func (l *LocalWorker) Paths(ctx context.Context) ([]stores.StoragePath, error) {
	return l.localStore.Local(ctx)
}

func (l *LocalWorker) Info(context.Context) (storiface.WorkerInfo, error) {
	hostname, err := os.Hostname() // TODO: allow overriding from config
	if err != nil {
		panic(err)
	}

	gpus, err := ffi.GetGPUDevices()
	if err != nil {
		log.Errorf("getting gpu devices failed: %+v", err)
	}

	h, err := sysinfo.Host()
	if err != nil {
		return storiface.WorkerInfo{}, xerrors.Errorf("getting host info: %w", err)
	}

	mem, err := h.Memory()
	if err != nil {
		return storiface.WorkerInfo{}, xerrors.Errorf("getting memory info: %w", err)
	}

	memSwap := mem.VirtualTotal
	if l.noSwap {
		memSwap = 0
	}

	return storiface.WorkerInfo{
		Hostname: hostname,
		Resources: storiface.WorkerResources{
			MemPhysical: mem.Total,
			MemSwap:     memSwap,
			MemReserved: mem.VirtualUsed + mem.Total - mem.Available, // TODO: sub this process
			CPUs:        uint64(runtime.NumCPU()),
			GPUs:        gpus,
		},
	}, nil
}

func (l *LocalWorker) Session(ctx context.Context) (uuid.UUID, error) {
	select {
	case <-l.closing:
		return ClosedWorkerID, nil
	default:
		return l.session, nil
	}
}

func (l *LocalWorker) Close() error {
	close(l.closing)
	return nil
}

// WaitQuiet blocks as long as there are tasks running
func (l *LocalWorker) WaitQuiet() {
	l.running.Wait()
}

type wctx struct {
	vals    context.Context
	closing chan struct{}
}

func (w *wctx) Deadline() (time.Time, bool) {
	return time.Time{}, false
}

func (w *wctx) Done() <-chan struct{} {
	return w.closing
}

func (w *wctx) Err() error {
	select {
	case <-w.closing:
		return context.Canceled
	default:
		return nil
	}
}

func (w *wctx) Value(key interface{}) interface{} {
	return w.vals.Value(key)
}

var _ context.Context = &wctx{}

var _ Worker = &LocalWorker{}
