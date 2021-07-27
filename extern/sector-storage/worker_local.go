package sectorstorage

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"reflect"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/elastic/go-sysinfo"
	"github.com/google/uuid"
	"github.com/hashicorp/go-multierror"
	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"

	ffi "github.com/filecoin-project/filecoin-ffi"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-statestore"
	"github.com/filecoin-project/specs-actors/actors/runtime/proof"
	"github.com/filecoin-project/specs-storage/storage"

	"github.com/filecoin-project/lotus/extern/sector-storage/ffiwrapper"
	"github.com/filecoin-project/lotus/extern/sector-storage/sealtasks"
	"github.com/filecoin-project/lotus/extern/sector-storage/stores"
	"github.com/filecoin-project/lotus/extern/sector-storage/storiface"
)

var pathTypes = []storiface.SectorFileType{storiface.FTUnsealed, storiface.FTSealed, storiface.FTCache, storiface.FTUpdate, storiface.FTUpdateCache}

type WorkerConfig struct {
	TaskTypes []sealtasks.TaskType
	NoSwap    bool

	// IgnoreResourceFiltering enables task distribution to happen on this
	// worker regardless of its currently available resources. Used in testing
	// with the local worker.
	IgnoreResourceFiltering bool
}

// used do provide custom proofs impl (mostly used in testing)
type ExecutorFunc func() (ffiwrapper.Storage, error)
type EnvFunc func(string) (string, bool)

type LocalWorker struct {
	storage    stores.Store
	localStore *stores.Local
	sindex     stores.SectorIndex
	ret        storiface.WorkerReturn
	executor   ExecutorFunc
	noSwap     bool
	envLookup  EnvFunc

	// see equivalent field on WorkerConfig.
	ignoreResources bool

	ct          *workerCallTracker
	acceptTasks map[sealtasks.TaskType]struct{}
	running     sync.WaitGroup
	taskLk      sync.Mutex

	session     uuid.UUID
	testDisable int64
	closing     chan struct{}
}

func newLocalWorker(executor ExecutorFunc, wcfg WorkerConfig, envLookup EnvFunc, store stores.Store, local *stores.Local, sindex stores.SectorIndex, ret storiface.WorkerReturn, cst *statestore.StateStore) *LocalWorker {
	acceptTasks := map[sealtasks.TaskType]struct{}{}
	for _, taskType := range wcfg.TaskTypes {
		acceptTasks[taskType] = struct{}{}
	}

	w := &LocalWorker{
		storage:    store,
		localStore: local,
		sindex:     sindex,
		ret:        ret,

		ct: &workerCallTracker{
			st: cst,
		},
		acceptTasks:     acceptTasks,
		executor:        executor,
		noSwap:          wcfg.NoSwap,
		envLookup:       envLookup,
		ignoreResources: wcfg.IgnoreResourceFiltering,
		session:         uuid.New(),
		closing:         make(chan struct{}),
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
			err := storiface.Err(storiface.ErrTempWorkerRestart, xerrors.New("worker restarted"))

			// TODO: Handle restarting PC1 once support is merged

			if doReturn(context.TODO(), call.RetType, call.ID, ret, nil, err) {
				if err := w.ct.onReturned(call.ID); err != nil {
					log.Errorf("marking call as returned failed: %s: %+v", call.RetType, err)
				}
			}
		}
	}()

	return w
}

func NewLocalWorker(wcfg WorkerConfig, store stores.Store, local *stores.Local, sindex stores.SectorIndex, ret storiface.WorkerReturn, cst *statestore.StateStore) *LocalWorker {
	return newLocalWorker(nil, wcfg, os.LookupEnv, store, local, sindex, ret, cst)
}

type localWorkerPathProvider struct {
	w  *LocalWorker
	op storiface.AcquireMode
}

func (l *localWorkerPathProvider) AcquireSector(ctx context.Context, sector storage.SectorRef, existing storiface.SectorFileType, allocate storiface.SectorFileType, sealing storiface.PathType) (storiface.SectorPaths, func(), error) {
	paths, storageIDs, err := l.w.storage.AcquireSector(ctx, sector, existing, allocate, sealing, l.op)
	if err != nil {
		return storiface.SectorPaths{}, nil, err
	}

	releaseStorage, err := l.w.localStore.Reserve(ctx, sector, allocate, storageIDs, storiface.FSOverheadSeal)
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
			if err := l.w.sindex.StorageDeclareSector(ctx, stores.ID(sid), sector.ID, fileType, l.op == storiface.AcquireMove); err != nil {
				log.Errorf("declare sector error: %+v", err)
			}
		}
	}, nil
}

func (l *localWorkerPathProvider) AcquireSectorPaths(ctx context.Context, sector storage.SectorRef, existing storiface.SectorFileType, sealing storiface.PathType) (storiface.SectorPaths, func(), error) {
	return storiface.SectorPaths{}, nil, nil
}

func (l *LocalWorker) ffiExec() (ffiwrapper.Storage, error) {
	return ffiwrapper.New(&localWorkerPathProvider{w: l})
}

type ReturnType string

const (
	AddPiece            ReturnType = "AddPiece"
	SealPreCommit1      ReturnType = "SealPreCommit1"
	SealPreCommit2      ReturnType = "SealPreCommit2"
	SealCommit1         ReturnType = "SealCommit1"
	SealCommit2         ReturnType = "SealCommit2"
	FinalizeSector      ReturnType = "FinalizeSector"
	ReplicaUpdate       ReturnType = "ReplicaUpdate"
	ProveReplicaUpdate1 ReturnType = "ProveReplicaUpdate1"
	ProveReplicaUpdate2 ReturnType = "ProveReplicaUpdate2"
	GenerateSectorKey   ReturnType = "GenerateSectorKey"
	ReleaseUnsealed     ReturnType = "ReleaseUnsealed"
	MoveStorage         ReturnType = "MoveStorage"
	UnsealPiece         ReturnType = "UnsealPiece"
	Fetch               ReturnType = "Fetch"
)

// in: func(WorkerReturn, context.Context, CallID, err string)
// in: func(WorkerReturn, context.Context, CallID, ret T, err string)
func rfunc(in interface{}) func(context.Context, storiface.CallID, storiface.WorkerReturn, interface{}, *storiface.CallError) error {
	rf := reflect.ValueOf(in)
	ft := rf.Type()
	withRet := ft.NumIn() == 5

	return func(ctx context.Context, ci storiface.CallID, wr storiface.WorkerReturn, i interface{}, err *storiface.CallError) error {
		rctx := reflect.ValueOf(ctx)
		rwr := reflect.ValueOf(wr)
		rerr := reflect.ValueOf(err)
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

var returnFunc = map[ReturnType]func(context.Context, storiface.CallID, storiface.WorkerReturn, interface{}, *storiface.CallError) error{
	AddPiece:            rfunc(storiface.WorkerReturn.ReturnAddPiece),
	SealPreCommit1:      rfunc(storiface.WorkerReturn.ReturnSealPreCommit1),
	SealPreCommit2:      rfunc(storiface.WorkerReturn.ReturnSealPreCommit2),
	SealCommit1:         rfunc(storiface.WorkerReturn.ReturnSealCommit1),
	SealCommit2:         rfunc(storiface.WorkerReturn.ReturnSealCommit2),
	FinalizeSector:      rfunc(storiface.WorkerReturn.ReturnFinalizeSector),
	ReleaseUnsealed:     rfunc(storiface.WorkerReturn.ReturnReleaseUnsealed),
	ReplicaUpdate:       rfunc(storiface.WorkerReturn.ReturnReplicaUpdate),
	ProveReplicaUpdate1: rfunc(storiface.WorkerReturn.ReturnProveReplicaUpdate1),
	ProveReplicaUpdate2: rfunc(storiface.WorkerReturn.ReturnProveReplicaUpdate2),
	GenerateSectorKey:   rfunc(storiface.WorkerReturn.ReturnGenerateSectorKeyFromData),
	MoveStorage:         rfunc(storiface.WorkerReturn.ReturnMoveStorage),
	UnsealPiece:         rfunc(storiface.WorkerReturn.ReturnUnsealPiece),
	Fetch:               rfunc(storiface.WorkerReturn.ReturnFetch),
}

func (l *LocalWorker) asyncCall(ctx context.Context, sector storage.SectorRef, rt ReturnType, work func(ctx context.Context, ci storiface.CallID) (interface{}, error)) (storiface.CallID, error) {
	ci := storiface.CallID{
		Sector: sector.ID,
		ID:     uuid.New(),
	}

	if err := l.ct.onStart(ci, rt); err != nil {
		log.Errorf("tracking call (start): %+v", err)
	}

	l.running.Add(1)

	go func() {
		defer l.running.Done()

		ctx := &wctx{
			vals:    ctx,
			closing: l.closing,
		}

		res, err := work(ctx, ci)
		if err != nil {
			rb, err := json.Marshal(res)
			if err != nil {
				log.Errorf("tracking call (marshaling results): %+v", err)
			} else {
				if err := l.ct.onDone(ci, rb); err != nil {
					log.Errorf("tracking call (done): %+v", err)
				}
			}
		}

		if doReturn(ctx, rt, ci, l.ret, res, toCallError(err)) {
			if err := l.ct.onReturned(ci); err != nil {
				log.Errorf("tracking call (done): %+v", err)
			}
		}
	}()
	return ci, nil
}

func toCallError(err error) *storiface.CallError {
	var serr *storiface.CallError
	if err != nil && !xerrors.As(err, &serr) {
		serr = storiface.Err(storiface.ErrUnknown, err)
	}

	return serr
}

// doReturn tries to send the result to manager, returns true if successful
func doReturn(ctx context.Context, rt ReturnType, ci storiface.CallID, ret storiface.WorkerReturn, res interface{}, rerr *storiface.CallError) bool {
	for {
		err := returnFunc[rt](ctx, ci, ret, res, rerr)
		if err == nil {
			break
		}

		log.Errorf("return error, will retry in 5s: %s: %+v", rt, err)
		select {
		case <-time.After(5 * time.Second):
		case <-ctx.Done():
			log.Errorf("failed to return results: %s", ctx.Err())

			// fine to just return, worker is most likely shutting down, and
			// we didn't mark the result as returned yet, so we'll try to
			// re-submit it on restart
			return false
		}
	}

	return true
}

func (l *LocalWorker) NewSector(ctx context.Context, sector storage.SectorRef) error {
	sb, err := l.executor()
	if err != nil {
		return err
	}

	return sb.NewSector(ctx, sector)
}

func (l *LocalWorker) AddPiece(ctx context.Context, sector storage.SectorRef, epcs []abi.UnpaddedPieceSize, sz abi.UnpaddedPieceSize, r io.Reader) (storiface.CallID, error) {
	sb, err := l.executor()
	if err != nil {
		return storiface.UndefCall, err
	}

	return l.asyncCall(ctx, sector, AddPiece, func(ctx context.Context, ci storiface.CallID) (interface{}, error) {
		return sb.AddPiece(ctx, sector, epcs, sz, r)
	})
}

func (l *LocalWorker) Fetch(ctx context.Context, sector storage.SectorRef, fileType storiface.SectorFileType, ptype storiface.PathType, am storiface.AcquireMode) (storiface.CallID, error) {
	return l.asyncCall(ctx, sector, Fetch, func(ctx context.Context, ci storiface.CallID) (interface{}, error) {
		_, done, err := (&localWorkerPathProvider{w: l, op: am}).AcquireSector(ctx, sector, fileType, storiface.FTNone, ptype)
		if err == nil {
			done()
		}

		return nil, err
	})
}

func (l *LocalWorker) SealPreCommit1(ctx context.Context, sector storage.SectorRef, ticket abi.SealRandomness, pieces []abi.PieceInfo) (storiface.CallID, error) {
	return l.asyncCall(ctx, sector, SealPreCommit1, func(ctx context.Context, ci storiface.CallID) (interface{}, error) {

		{
			// cleanup previous failed attempts if they exist
			if err := l.storage.Remove(ctx, sector.ID, storiface.FTSealed, true, nil); err != nil {
				return nil, xerrors.Errorf("cleaning up sealed data: %w", err)
			}

			if err := l.storage.Remove(ctx, sector.ID, storiface.FTCache, true, nil); err != nil {
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

func (l *LocalWorker) SealPreCommit2(ctx context.Context, sector storage.SectorRef, phase1Out storage.PreCommit1Out) (storiface.CallID, error) {
	sb, err := l.executor()
	if err != nil {
		return storiface.UndefCall, err
	}

	return l.asyncCall(ctx, sector, SealPreCommit2, func(ctx context.Context, ci storiface.CallID) (interface{}, error) {
		return sb.SealPreCommit2(ctx, sector, phase1Out)
	})
}

func (l *LocalWorker) SealCommit1(ctx context.Context, sector storage.SectorRef, ticket abi.SealRandomness, seed abi.InteractiveSealRandomness, pieces []abi.PieceInfo, cids storage.SectorCids) (storiface.CallID, error) {
	sb, err := l.executor()
	if err != nil {
		return storiface.UndefCall, err
	}

	return l.asyncCall(ctx, sector, SealCommit1, func(ctx context.Context, ci storiface.CallID) (interface{}, error) {
		return sb.SealCommit1(ctx, sector, ticket, seed, pieces, cids)
	})
}

func (l *LocalWorker) SealCommit2(ctx context.Context, sector storage.SectorRef, phase1Out storage.Commit1Out) (storiface.CallID, error) {
	sb, err := l.executor()
	if err != nil {
		return storiface.UndefCall, err
	}

	return l.asyncCall(ctx, sector, SealCommit2, func(ctx context.Context, ci storiface.CallID) (interface{}, error) {
		return sb.SealCommit2(ctx, sector, phase1Out)
	})
}

func (l *LocalWorker) ReplicaUpdate(ctx context.Context, sector storage.SectorRef, pieces []abi.PieceInfo) (storiface.CallID, error) {
	sb, err := l.executor()
	if err != nil {
		return storiface.UndefCall, err
	}

	return l.asyncCall(ctx, sector, ReplicaUpdate, func(ctx context.Context, ci storiface.CallID) (interface{}, error) {
		sealerOut, err := sb.ReplicaUpdate(ctx, sector, pieces)
		return sealerOut, err
	})
}

func (l *LocalWorker) ProveReplicaUpdate1(ctx context.Context, sector storage.SectorRef, sectorKey, newSealed, newUnsealed cid.Cid) (storiface.CallID, error) {
	sb, err := l.executor()
	if err != nil {
		return storiface.UndefCall, err
	}

	return l.asyncCall(ctx, sector, ProveReplicaUpdate1, func(ctx context.Context, ci storiface.CallID) (interface{}, error) {
		return sb.ProveReplicaUpdate1(ctx, sector, sectorKey, newSealed, newUnsealed)
	})
}

func (l *LocalWorker) ProveReplicaUpdate2(ctx context.Context, sector storage.SectorRef, sectorKey, newSealed, newUnsealed cid.Cid, vanillaProofs storage.ReplicaVanillaProofs) (storiface.CallID, error) {
	sb, err := l.executor()
	if err != nil {
		return storiface.UndefCall, err
	}

	return l.asyncCall(ctx, sector, ProveReplicaUpdate2, func(ctx context.Context, ci storiface.CallID) (interface{}, error) {
		return sb.ProveReplicaUpdate2(ctx, sector, sectorKey, newSealed, newUnsealed, vanillaProofs)
	})
}

func (l *LocalWorker) GenerateSectorKeyFromData(ctx context.Context, sector storage.SectorRef, commD cid.Cid) (storiface.CallID, error) {
	sb, err := l.executor()
	if err != nil {
		return storiface.UndefCall, err
	}

	return l.asyncCall(ctx, sector, GenerateSectorKey, func(ctx context.Context, ci storiface.CallID) (interface{}, error) {
		return nil, sb.GenerateSectorKeyFromData(ctx, sector, commD)
	})
}

func (l *LocalWorker) FinalizeSector(ctx context.Context, sector storage.SectorRef, keepUnsealed []storage.Range) (storiface.CallID, error) {
	sb, err := l.executor()
	if err != nil {
		return storiface.UndefCall, err
	}

	return l.asyncCall(ctx, sector, FinalizeSector, func(ctx context.Context, ci storiface.CallID) (interface{}, error) {
		if err := sb.FinalizeSector(ctx, sector, keepUnsealed); err != nil {
			return nil, xerrors.Errorf("finalizing sector: %w", err)
		}

		if len(keepUnsealed) == 0 {
			if err := l.storage.Remove(ctx, sector.ID, storiface.FTUnsealed, true, nil); err != nil {
				return nil, xerrors.Errorf("removing unsealed data: %w", err)
			}
		}

		return nil, err
	})
}

func (l *LocalWorker) ReleaseUnsealed(ctx context.Context, sector storage.SectorRef, safeToFree []storage.Range) (storiface.CallID, error) {
	return storiface.UndefCall, xerrors.Errorf("implement me")
}

func (l *LocalWorker) Remove(ctx context.Context, sector abi.SectorID) error {
	var err error

	if rerr := l.storage.Remove(ctx, sector, storiface.FTSealed, true, nil); rerr != nil {
		err = multierror.Append(err, xerrors.Errorf("removing sector (sealed): %w", rerr))
	}
	if rerr := l.storage.Remove(ctx, sector, storiface.FTCache, true, nil); rerr != nil {
		err = multierror.Append(err, xerrors.Errorf("removing sector (cache): %w", rerr))
	}
	if rerr := l.storage.Remove(ctx, sector, storiface.FTUnsealed, true, nil); rerr != nil {
		err = multierror.Append(err, xerrors.Errorf("removing sector (unsealed): %w", rerr))
	}

	return err
}

func (l *LocalWorker) MoveStorage(ctx context.Context, sector storage.SectorRef, types storiface.SectorFileType) (storiface.CallID, error) {
	return l.asyncCall(ctx, sector, MoveStorage, func(ctx context.Context, ci storiface.CallID) (interface{}, error) {
		return nil, l.storage.MoveStorage(ctx, sector, types)
	})
}

func (l *LocalWorker) UnsealPiece(ctx context.Context, sector storage.SectorRef, index storiface.UnpaddedByteIndex, size abi.UnpaddedPieceSize, randomness abi.SealRandomness, cid cid.Cid) (storiface.CallID, error) {
	sb, err := l.executor()
	if err != nil {
		return storiface.UndefCall, err
	}

	return l.asyncCall(ctx, sector, UnsealPiece, func(ctx context.Context, ci storiface.CallID) (interface{}, error) {
		log.Debugf("worker will unseal piece now, sector=%+v", sector.ID)
		if err = sb.UnsealPiece(ctx, sector, index, size, randomness, cid); err != nil {
			return nil, xerrors.Errorf("unsealing sector: %w", err)
		}

		if err = l.storage.RemoveCopies(ctx, sector.ID, storiface.FTSealed); err != nil {
			return nil, xerrors.Errorf("removing source data: %w", err)
		}

		if err = l.storage.RemoveCopies(ctx, sector.ID, storiface.FTCache); err != nil {
			return nil, xerrors.Errorf("removing source data: %w", err)
		}

		log.Debugf("worker has unsealed piece, sector=%+v", sector.ID)

		return nil, nil
	})
}

func (l *LocalWorker) GenerateWinningPoSt(ctx context.Context, mid abi.ActorID, privsectors ffi.SortedPrivateSectorInfo, randomness abi.PoStRandomness, sectorChallenges *ffi.FallbackChallenges) ([]proof.PoStProof, error) {
	sb, err := l.executor()
	if err != nil {
		return nil, err
	}

	ps := privsectors.Values()
	vanillas := make([][]byte, len(ps))

	var rerr error
	var wg sync.WaitGroup
	for i, v := range ps {
		wg.Add(1)
		go func(index int, sector ffi.PrivateSectorInfo) {
			defer wg.Done()
			vanilla, err := l.storage.GenerateSingleVanillaProof(ctx, mid, &sector, sectorChallenges.Challenges[sector.SectorNumber])
			if err != nil {
				rerr = multierror.Append(rerr, xerrors.Errorf("get winning sector:%d,vanila failed: %w", sector.SectorNumber, err))
				return
			}
			if vanilla == nil {
				rerr = multierror.Append(rerr, xerrors.Errorf("get winning sector:%d,vanila is nil", sector.SectorNumber))
			}
			vanillas[index] = vanilla

		}(i, ffi.PrivateSectorInfo(v))
	}
	wg.Wait()

	if rerr != nil {
		return nil, rerr
	}

	return sb.GenerateWinningPoStWithVanilla(ctx, ps[0].PoStProofType, mid, randomness, vanillas)
}

func (l *LocalWorker) GenerateWindowPoSt(ctx context.Context, mid abi.ActorID, privsectors ffi.SortedPrivateSectorInfo, partitionIdx int, offset int, randomness abi.PoStRandomness, sectorChallenges *ffi.FallbackChallenges) (ffiwrapper.WindowPoStResult, error) {
	sb, err := l.executor()
	if err != nil {
		return ffiwrapper.WindowPoStResult{}, err
	}

	var slk sync.Mutex
	var skipped []abi.SectorID

	ps := privsectors.Values()

	out := make([][]byte, len(ps))
	var wg sync.WaitGroup
	for i := range ps {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()

			sector := ps[index]
			vanilla, err := l.storage.GenerateSingleVanillaProof(ctx, mid, &sector, sectorChallenges.Challenges[sector.SectorNumber])
			if err != nil || vanilla == nil {
				slk.Lock()
				skipped = append(skipped, abi.SectorID{
					Miner:  mid,
					Number: sector.SectorNumber,
				})
				slk.Unlock()
				log.Errorf("get sector: %d, vanilla: %s, offset: %d", sector.SectorNumber, vanilla, offset+index)
				return
			}
			out[index] = vanilla
		}(i)
	}

	wg.Wait()
	if len(skipped) > 0 {
		return ffiwrapper.WindowPoStResult{PoStProofs: ffi.PartitionProof{}, Skipped: skipped}, nil
	}

	PoSts, err := sb.GenerateWindowPoStWithVanilla(ctx, ps[0].PoStProofType, mid, randomness, out, partitionIdx)

	if err != nil {
		return ffiwrapper.WindowPoStResult{PoStProofs: *PoSts, Skipped: skipped}, err
	}

	return ffiwrapper.WindowPoStResult{PoStProofs: *PoSts, Skipped: skipped}, nil
}

func (l *LocalWorker) TaskTypes(context.Context) (map[sealtasks.TaskType]struct{}, error) {
	l.taskLk.Lock()
	defer l.taskLk.Unlock()

	return l.acceptTasks, nil
}

func (l *LocalWorker) TaskDisable(ctx context.Context, tt sealtasks.TaskType) error {
	l.taskLk.Lock()
	defer l.taskLk.Unlock()

	delete(l.acceptTasks, tt)
	return nil
}

func (l *LocalWorker) TaskEnable(ctx context.Context, tt sealtasks.TaskType) error {
	l.taskLk.Lock()
	defer l.taskLk.Unlock()

	l.acceptTasks[tt] = struct{}{}
	return nil
}

func (l *LocalWorker) Paths(ctx context.Context) ([]stores.StoragePath, error) {
	return l.localStore.Local(ctx)
}

func (l *LocalWorker) memInfo() (memPhysical, memUsed, memSwap, memSwapUsed uint64, err error) {
	h, err := sysinfo.Host()
	if err != nil {
		return 0, 0, 0, 0, err
	}

	mem, err := h.Memory()
	if err != nil {
		return 0, 0, 0, 0, err
	}
	memPhysical = mem.Total
	// mem.Available is memory available without swapping, it is more relevant for this calculation
	memUsed = mem.Total - mem.Available
	memSwap = mem.VirtualTotal
	memSwapUsed = mem.VirtualUsed

	if cgMemMax, cgMemUsed, cgSwapMax, cgSwapUsed, err := cgroupV1Mem(); err == nil {
		if cgMemMax > 0 && cgMemMax < memPhysical {
			memPhysical = cgMemMax
			memUsed = cgMemUsed
		}
		if cgSwapMax > 0 && cgSwapMax < memSwap {
			memSwap = cgSwapMax
			memSwapUsed = cgSwapUsed
		}
	}

	if cgMemMax, cgMemUsed, cgSwapMax, cgSwapUsed, err := cgroupV2Mem(); err == nil {
		if cgMemMax > 0 && cgMemMax < memPhysical {
			memPhysical = cgMemMax
			memUsed = cgMemUsed
		}
		if cgSwapMax > 0 && cgSwapMax < memSwap {
			memSwap = cgSwapMax
			memSwapUsed = cgSwapUsed
		}
	}

	if l.noSwap {
		memSwap = 0
		memSwapUsed = 0
	}

	return memPhysical, memUsed, memSwap, memSwapUsed, nil
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

	memPhysical, memUsed, memSwap, memSwapUsed, err := l.memInfo()
	if err != nil {
		return storiface.WorkerInfo{}, xerrors.Errorf("getting memory info: %w", err)
	}

	resEnv, err := storiface.ParseResourceEnv(func(key, def string) (string, bool) {
		return l.envLookup(key)
	})
	if err != nil {
		return storiface.WorkerInfo{}, xerrors.Errorf("interpreting resource env vars: %w", err)
	}

	return storiface.WorkerInfo{
		Hostname:        hostname,
		IgnoreResources: l.ignoreResources,
		Resources: storiface.WorkerResources{
			MemPhysical: memPhysical,
			MemUsed:     memUsed,
			MemSwap:     memSwap,
			MemSwapUsed: memSwapUsed,
			CPUs:        uint64(runtime.NumCPU()),
			GPUs:        gpus,
			Resources:   resEnv,
		},
	}, nil
}

func (l *LocalWorker) Session(ctx context.Context) (uuid.UUID, error) {
	if atomic.LoadInt64(&l.testDisable) == 1 {
		return uuid.UUID{}, xerrors.Errorf("disabled")
	}

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
