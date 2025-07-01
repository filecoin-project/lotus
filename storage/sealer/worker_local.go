package sealer

import (
	"context"
	"encoding/json"
	"errors"
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
	"github.com/filecoin-project/go-state-types/proof"
	"github.com/filecoin-project/go-statestore"

	"github.com/filecoin-project/lotus/storage/paths"
	"github.com/filecoin-project/lotus/storage/sealer/ffiwrapper"
	"github.com/filecoin-project/lotus/storage/sealer/sealtasks"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
)

type WorkerConfig struct {
	TaskTypes []sealtasks.TaskType
	NoSwap    bool

	// os.Hostname if not set
	Name string

	// IgnoreResourceFiltering enables task distribution to happen on this
	// worker regardless of its currently available resources. Used in testing
	// with the local worker.
	IgnoreResourceFiltering bool

	MaxParallelChallengeReads int           // 0 = no limit
	ChallengeReadTimeout      time.Duration // 0 = no timeout
}

// used do provide custom proofs impl (mostly used in testing)

type ExecutorFunc func(w *LocalWorker) (storiface.Storage, error)
type EnvFunc func(string) (string, bool)

type LocalWorker struct {
	storage    paths.Store
	localStore *paths.Local
	sindex     paths.SectorIndex
	ret        storiface.WorkerReturn
	executor   ExecutorFunc
	noSwap     bool
	envLookup  EnvFunc

	name string

	// see equivalent field on WorkerConfig.
	ignoreResources bool

	ct          *workerCallTracker
	acceptTasks map[sealtasks.TaskType]struct{}
	running     sync.WaitGroup
	taskLk      sync.Mutex

	challengeThrottle    chan struct{}
	challengeReadTimeout time.Duration

	session     uuid.UUID
	testDisable int64
	closing     chan struct{}
}

func NewLocalWorkerWithExecutor(executor ExecutorFunc, wcfg WorkerConfig, envLookup EnvFunc, store paths.Store, local *paths.Local, sindex paths.SectorIndex, ret storiface.WorkerReturn, cst *statestore.StateStore) *LocalWorker {
	acceptTasks := map[sealtasks.TaskType]struct{}{}
	for _, taskType := range wcfg.TaskTypes {
		acceptTasks[taskType] = struct{}{}
	}

	w := &LocalWorker{
		storage:    store,
		localStore: local,
		sindex:     sindex,
		ret:        ret,
		name:       wcfg.Name,

		ct: &workerCallTracker{
			st: cst,
		},
		acceptTasks:          acceptTasks,
		executor:             executor,
		noSwap:               wcfg.NoSwap,
		envLookup:            envLookup,
		ignoreResources:      wcfg.IgnoreResourceFiltering,
		challengeReadTimeout: wcfg.ChallengeReadTimeout,
		session:              uuid.New(),
		closing:              make(chan struct{}),
	}

	if w.name == "" {
		var err error
		w.name, err = os.Hostname()
		if err != nil {
			panic(err)
		}
	}

	if wcfg.MaxParallelChallengeReads > 0 {
		w.challengeThrottle = make(chan struct{}, wcfg.MaxParallelChallengeReads)
	}

	if w.executor == nil {
		w.executor = FFIExec()
	}

	unfinished, err := w.ct.unfinished()
	if err != nil {
		log.Errorf("reading unfinished tasks: %+v", err)
		return w
	}

	go func() {
		for _, call := range unfinished {
			err := storiface.Err(storiface.ErrTempWorkerRestart, xerrors.Errorf("worker [name: %s] restarted", w.name))

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

func NewLocalWorker(wcfg WorkerConfig, store paths.Store, local *paths.Local, sindex paths.SectorIndex, ret storiface.WorkerReturn, cst *statestore.StateStore) *LocalWorker {
	return NewLocalWorkerWithExecutor(nil, wcfg, os.LookupEnv, store, local, sindex, ret, cst)
}

type localWorkerPathProvider struct {
	w  *LocalWorker
	op storiface.AcquireMode
}

func (l *localWorkerPathProvider) AcquireSector(ctx context.Context, sector storiface.SectorRef, existing storiface.SectorFileType, allocate storiface.SectorFileType, sealing storiface.PathType) (storiface.SectorPaths, func(), error) {
	spaths, storageIDs, err := l.w.storage.AcquireSector(ctx, sector, existing, allocate, sealing, l.op)
	if err != nil {
		return storiface.SectorPaths{}, nil, err
	}

	releaseStorage, err := l.w.localStore.Reserve(ctx, sector, allocate, storageIDs, storiface.FSOverheadSeal, paths.MinFreeStoragePercentage)
	if err != nil {
		return storiface.SectorPaths{}, nil, xerrors.Errorf("reserving storage space: %w", err)
	}

	log.Debugf("acquired sector %d (e:%d; a:%d): %v", sector, existing, allocate, spaths)

	return spaths, func() {
		releaseStorage()

		for _, fileType := range storiface.PathTypes {
			if fileType&allocate == 0 {
				continue
			}

			sid := storiface.PathByType(storageIDs, fileType)
			if err := l.w.sindex.StorageDeclareSector(ctx, storiface.ID(sid), sector.ID, fileType, l.op == storiface.AcquireMove); err != nil {
				log.Errorf("declare sector error: %+v", err)
			}
		}
	}, nil
}

func (l *localWorkerPathProvider) AcquireSectorCopy(ctx context.Context, id storiface.SectorRef, existing storiface.SectorFileType, allocate storiface.SectorFileType, ptype storiface.PathType) (storiface.SectorPaths, func(), error) {
	return (&localWorkerPathProvider{w: l.w, op: storiface.AcquireCopy}).AcquireSector(ctx, id, existing, allocate, ptype)
}

func FFIExec(opts ...ffiwrapper.FFIWrapperOpt) func(l *LocalWorker) (storiface.Storage, error) {
	return func(l *LocalWorker) (storiface.Storage, error) {
		return ffiwrapper.New(&localWorkerPathProvider{w: l}, opts...)
	}
}

type ReturnType string

const (
	DataCid               ReturnType = "DataCid"
	AddPiece              ReturnType = "AddPiece"
	SealPreCommit1        ReturnType = "SealPreCommit1"
	SealPreCommit2        ReturnType = "SealPreCommit2"
	SealCommit1           ReturnType = "SealCommit1"
	SealCommit2           ReturnType = "SealCommit2"
	FinalizeSector        ReturnType = "FinalizeSector"
	FinalizeReplicaUpdate ReturnType = "FinalizeReplicaUpdate"
	ReplicaUpdate         ReturnType = "ReplicaUpdate"
	ProveReplicaUpdate1   ReturnType = "ProveReplicaUpdate1"
	ProveReplicaUpdate2   ReturnType = "ProveReplicaUpdate2"
	GenerateSectorKey     ReturnType = "GenerateSectorKey"
	ReleaseUnsealed       ReturnType = "ReleaseUnsealed"
	MoveStorage           ReturnType = "MoveStorage"
	UnsealPiece           ReturnType = "UnsealPiece"
	DownloadSector        ReturnType = "DownloadSector"
	Fetch                 ReturnType = "Fetch"
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
	DataCid:               rfunc(storiface.WorkerReturn.ReturnDataCid),
	AddPiece:              rfunc(storiface.WorkerReturn.ReturnAddPiece),
	SealPreCommit1:        rfunc(storiface.WorkerReturn.ReturnSealPreCommit1),
	SealPreCommit2:        rfunc(storiface.WorkerReturn.ReturnSealPreCommit2),
	SealCommit1:           rfunc(storiface.WorkerReturn.ReturnSealCommit1),
	SealCommit2:           rfunc(storiface.WorkerReturn.ReturnSealCommit2),
	FinalizeSector:        rfunc(storiface.WorkerReturn.ReturnFinalizeSector),
	ReleaseUnsealed:       rfunc(storiface.WorkerReturn.ReturnReleaseUnsealed),
	ReplicaUpdate:         rfunc(storiface.WorkerReturn.ReturnReplicaUpdate),
	ProveReplicaUpdate1:   rfunc(storiface.WorkerReturn.ReturnProveReplicaUpdate1),
	ProveReplicaUpdate2:   rfunc(storiface.WorkerReturn.ReturnProveReplicaUpdate2),
	GenerateSectorKey:     rfunc(storiface.WorkerReturn.ReturnGenerateSectorKeyFromData),
	FinalizeReplicaUpdate: rfunc(storiface.WorkerReturn.ReturnFinalizeReplicaUpdate),
	MoveStorage:           rfunc(storiface.WorkerReturn.ReturnMoveStorage),
	UnsealPiece:           rfunc(storiface.WorkerReturn.ReturnUnsealPiece),
	DownloadSector:        rfunc(storiface.WorkerReturn.ReturnDownloadSector),
	Fetch:                 rfunc(storiface.WorkerReturn.ReturnFetch),
}

func (l *LocalWorker) asyncCall(ctx context.Context, sector storiface.SectorRef, rt ReturnType, work func(ctx context.Context, ci storiface.CallID) (interface{}, error)) (storiface.CallID, error) {
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

		if err != nil {
			err = xerrors.Errorf("%w [name: %s]", err, l.name)
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
	if err != nil && !errors.As(err, &serr) {
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

func (l *LocalWorker) NewSector(ctx context.Context, sector storiface.SectorRef) error {
	sb, err := l.executor(l)
	if err != nil {
		return err
	}

	return sb.NewSector(ctx, sector)
}

func (l *LocalWorker) DataCid(ctx context.Context, pieceSize abi.UnpaddedPieceSize, pieceData storiface.Data) (storiface.CallID, error) {
	sb, err := l.executor(l)
	if err != nil {
		return storiface.UndefCall, err
	}

	return l.asyncCall(ctx, storiface.NoSectorRef, DataCid, func(ctx context.Context, ci storiface.CallID) (interface{}, error) {
		return sb.DataCid(ctx, pieceSize, pieceData)
	})
}

func (l *LocalWorker) AddPiece(ctx context.Context, sector storiface.SectorRef, epcs []abi.UnpaddedPieceSize, sz abi.UnpaddedPieceSize, r io.Reader) (storiface.CallID, error) {
	sb, err := l.executor(l)
	if err != nil {
		return storiface.UndefCall, err
	}

	return l.asyncCall(ctx, sector, AddPiece, func(ctx context.Context, ci storiface.CallID) (interface{}, error) {
		return sb.AddPiece(ctx, sector, epcs, sz, r)
	})
}

func (l *LocalWorker) Fetch(ctx context.Context, sector storiface.SectorRef, fileType storiface.SectorFileType, ptype storiface.PathType, am storiface.AcquireMode) (storiface.CallID, error) {
	return l.asyncCall(ctx, sector, Fetch, func(ctx context.Context, ci storiface.CallID) (interface{}, error) {
		_, done, err := (&localWorkerPathProvider{w: l, op: am}).AcquireSector(ctx, sector, fileType, storiface.FTNone, ptype)
		if err == nil {
			done()
		}

		return nil, err
	})
}

func (l *LocalWorker) SealPreCommit1(ctx context.Context, sector storiface.SectorRef, ticket abi.SealRandomness, pieces []abi.PieceInfo) (storiface.CallID, error) {
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

		sb, err := l.executor(l)
		if err != nil {
			return nil, err
		}

		return sb.SealPreCommit1(ctx, sector, ticket, pieces)
	})
}

func (l *LocalWorker) SealPreCommit2(ctx context.Context, sector storiface.SectorRef, phase1Out storiface.PreCommit1Out) (storiface.CallID, error) {
	sb, err := l.executor(l)
	if err != nil {
		return storiface.UndefCall, err
	}

	return l.asyncCall(ctx, sector, SealPreCommit2, func(ctx context.Context, ci storiface.CallID) (interface{}, error) {
		return sb.SealPreCommit2(ctx, sector, phase1Out)
	})
}

func (l *LocalWorker) SealCommit1(ctx context.Context, sector storiface.SectorRef, ticket abi.SealRandomness, seed abi.InteractiveSealRandomness, pieces []abi.PieceInfo, cids storiface.SectorCids) (storiface.CallID, error) {
	sb, err := l.executor(l)
	if err != nil {
		return storiface.UndefCall, err
	}

	return l.asyncCall(ctx, sector, SealCommit1, func(ctx context.Context, ci storiface.CallID) (interface{}, error) {
		return sb.SealCommit1(ctx, sector, ticket, seed, pieces, cids)
	})
}

func (l *LocalWorker) SealCommit2(ctx context.Context, sector storiface.SectorRef, phase1Out storiface.Commit1Out) (storiface.CallID, error) {
	sb, err := l.executor(l)
	if err != nil {
		return storiface.UndefCall, err
	}

	return l.asyncCall(ctx, sector, SealCommit2, func(ctx context.Context, ci storiface.CallID) (interface{}, error) {
		return sb.SealCommit2(ctx, sector, phase1Out)
	})
}

func (l *LocalWorker) ReplicaUpdate(ctx context.Context, sector storiface.SectorRef, pieces []abi.PieceInfo) (storiface.CallID, error) {
	sb, err := l.executor(l)
	if err != nil {
		return storiface.UndefCall, err
	}

	return l.asyncCall(ctx, sector, ReplicaUpdate, func(ctx context.Context, ci storiface.CallID) (interface{}, error) {
		sealerOut, err := sb.ReplicaUpdate(ctx, sector, pieces)
		return sealerOut, err
	})
}

func (l *LocalWorker) ProveReplicaUpdate1(ctx context.Context, sector storiface.SectorRef, sectorKey, newSealed, newUnsealed cid.Cid) (storiface.CallID, error) {
	sb, err := l.executor(l)
	if err != nil {
		return storiface.UndefCall, err
	}

	return l.asyncCall(ctx, sector, ProveReplicaUpdate1, func(ctx context.Context, ci storiface.CallID) (interface{}, error) {
		return sb.ProveReplicaUpdate1(ctx, sector, sectorKey, newSealed, newUnsealed)
	})
}

func (l *LocalWorker) ProveReplicaUpdate2(ctx context.Context, sector storiface.SectorRef, sectorKey, newSealed, newUnsealed cid.Cid, vanillaProofs storiface.ReplicaVanillaProofs) (storiface.CallID, error) {
	sb, err := l.executor(l)
	if err != nil {
		return storiface.UndefCall, err
	}

	return l.asyncCall(ctx, sector, ProveReplicaUpdate2, func(ctx context.Context, ci storiface.CallID) (interface{}, error) {
		return sb.ProveReplicaUpdate2(ctx, sector, sectorKey, newSealed, newUnsealed, vanillaProofs)
	})
}

func (l *LocalWorker) GenerateSectorKeyFromData(ctx context.Context, sector storiface.SectorRef, commD cid.Cid) (storiface.CallID, error) {
	sb, err := l.executor(l)
	if err != nil {
		return storiface.UndefCall, err
	}

	return l.asyncCall(ctx, sector, GenerateSectorKey, func(ctx context.Context, ci storiface.CallID) (interface{}, error) {
		return nil, sb.GenerateSectorKeyFromData(ctx, sector, commD)
	})
}

func (l *LocalWorker) FinalizeSector(ctx context.Context, sector storiface.SectorRef) (storiface.CallID, error) {
	sb, err := l.executor(l)
	if err != nil {
		return storiface.UndefCall, err
	}

	return l.asyncCall(ctx, sector, FinalizeSector, func(ctx context.Context, ci storiface.CallID) (interface{}, error) {
		return nil, sb.FinalizeSector(ctx, sector)
	})
}

func (l *LocalWorker) FinalizeReplicaUpdate(ctx context.Context, sector storiface.SectorRef) (storiface.CallID, error) {
	sb, err := l.executor(l)
	if err != nil {
		return storiface.UndefCall, err
	}

	return l.asyncCall(ctx, sector, FinalizeReplicaUpdate, func(ctx context.Context, ci storiface.CallID) (interface{}, error) {
		return nil, sb.FinalizeReplicaUpdate(ctx, sector)
	})
}

func (l *LocalWorker) ReleaseUnsealed(ctx context.Context, sector storiface.SectorRef, keepUnsealed []storiface.Range) (storiface.CallID, error) {
	sb, err := l.executor(l)
	if err != nil {
		return storiface.UndefCall, err
	}

	return l.asyncCall(ctx, sector, ReleaseUnsealed, func(ctx context.Context, ci storiface.CallID) (interface{}, error) {
		if err := sb.ReleaseUnsealed(ctx, sector, keepUnsealed); err != nil {
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

func (l *LocalWorker) MoveStorage(ctx context.Context, sector storiface.SectorRef, types storiface.SectorFileType) (storiface.CallID, error) {
	return l.asyncCall(ctx, sector, MoveStorage, func(ctx context.Context, ci storiface.CallID) (interface{}, error) {
		if err := l.storage.MoveStorage(ctx, sector, types); err != nil {
			return nil, xerrors.Errorf("move to storage: %w", err)
		}

		for _, fileType := range storiface.PathTypes {
			if fileType&types == 0 {
				continue
			}

			if err := l.storage.RemoveCopies(ctx, sector.ID, fileType); err != nil {
				return nil, xerrors.Errorf("rm copies (t:%s, s:%v): %w", fileType, sector, err)
			}
		}
		return nil, nil
	})
}

func (l *LocalWorker) UnsealPiece(ctx context.Context, sector storiface.SectorRef, index storiface.UnpaddedByteIndex, size abi.UnpaddedPieceSize, randomness abi.SealRandomness, cid cid.Cid) (storiface.CallID, error) {
	sb, err := l.executor(l)
	if err != nil {
		return storiface.UndefCall, err
	}

	return l.asyncCall(ctx, sector, UnsealPiece, func(ctx context.Context, ci storiface.CallID) (interface{}, error) {
		log.Debugf("worker will unseal piece now, sector=%+v", sector.ID)
		if err = sb.UnsealPiece(ctx, sector, index, size, randomness, cid); err != nil {
			return nil, xerrors.Errorf("unsealing sector: %w", err)
		}

		// note: the unsealed file is moved to long-term storage in Manager.SectorsUnsealPiece

		storageTypes := []storiface.SectorFileType{storiface.FTSealed, storiface.FTCache, storiface.FTUpdate, storiface.FTUpdateCache}
		for _, fileType := range storageTypes {
			if err = l.storage.RemoveCopies(ctx, sector.ID, fileType); err != nil {
				return nil, xerrors.Errorf("removing source data: %w", err)
			}
		}

		log.Debugf("unsealed piece, sector=%+v", sector.ID)

		return nil, nil
	})
}

func (l *LocalWorker) DownloadSectorData(ctx context.Context, sector storiface.SectorRef, finalized bool, src map[storiface.SectorFileType]storiface.SectorLocation) (storiface.CallID, error) {
	sb, err := l.executor(l)
	if err != nil {
		return storiface.UndefCall, err
	}

	return l.asyncCall(ctx, sector, DownloadSector, func(ctx context.Context, ci storiface.CallID) (interface{}, error) {
		return nil, sb.DownloadSectorData(ctx, sector, finalized, src)
	})
}

func (l *LocalWorker) GenerateWinningPoSt(ctx context.Context, ppt abi.RegisteredPoStProof, mid abi.ActorID, sectors []storiface.PostSectorChallenge, randomness abi.PoStRandomness) ([]proof.PoStProof, error) {
	sb, err := l.executor(l)
	if err != nil {
		return nil, err
	}

	// don't throttle winningPoSt
	// * Always want it done asap
	// * It's usually just one sector
	var wg sync.WaitGroup
	wg.Add(len(sectors))

	vproofs := make([][]byte, len(sectors))
	var rerr error

	for i, s := range sectors {
		go func(i int, s storiface.PostSectorChallenge) {
			defer wg.Done()

			if l.challengeReadTimeout > 0 {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, l.challengeReadTimeout)
				defer cancel()
			}

			vanilla, err := l.storage.GenerateSingleVanillaProof(ctx, mid, s, ppt)
			if err != nil {
				rerr = multierror.Append(rerr, xerrors.Errorf("get winning sector:%d,vanilla failed: %w", s.SectorNumber, err))
				return
			}
			if vanilla == nil {
				rerr = multierror.Append(rerr, xerrors.Errorf("get winning sector:%d,vanilla is nil", s.SectorNumber))
			}
			vproofs[i] = vanilla
		}(i, s)
	}
	wg.Wait()

	if rerr != nil {
		return nil, rerr
	}

	return sb.GenerateWinningPoStWithVanilla(ctx, ppt, mid, randomness, vproofs)
}

func (l *LocalWorker) GenerateWindowPoSt(ctx context.Context, ppt abi.RegisteredPoStProof, mid abi.ActorID, sectors []storiface.PostSectorChallenge, partitionIdx int, randomness abi.PoStRandomness) (storiface.WindowPoStResult, error) {
	return l.GenerateWindowPoStAdv(ctx, ppt, mid, sectors, partitionIdx, randomness, false)
}

func (l *LocalWorker) GenerateWindowPoStAdv(ctx context.Context, ppt abi.RegisteredPoStProof, mid abi.ActorID, sectors []storiface.PostSectorChallenge, partitionIdx int, randomness abi.PoStRandomness, allowSkip bool) (storiface.WindowPoStResult, error) {
	sb, err := l.executor(l)
	if err != nil {
		return storiface.WindowPoStResult{}, err
	}

	var slk sync.Mutex
	var skipped []abi.SectorID

	var wg sync.WaitGroup
	wg.Add(len(sectors))

	vproofs := make([][]byte, len(sectors))

	for i, s := range sectors {
		if l.challengeThrottle != nil {
			select {
			case l.challengeThrottle <- struct{}{}:
			case <-ctx.Done():
				return storiface.WindowPoStResult{}, xerrors.Errorf("context error waiting on challengeThrottle %w", err)
			}
		}

		go func(i int, s storiface.PostSectorChallenge) {
			defer wg.Done()
			ctx := ctx

			defer func() {
				if l.challengeThrottle != nil {
					<-l.challengeThrottle
				}
			}()

			if l.challengeReadTimeout > 0 {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, l.challengeReadTimeout)
				defer cancel()
			}

			vanilla, err := l.storage.GenerateSingleVanillaProof(ctx, mid, s, ppt)
			slk.Lock()
			defer slk.Unlock()

			if err != nil || vanilla == nil {
				skipped = append(skipped, abi.SectorID{
					Miner:  mid,
					Number: s.SectorNumber,
				})
				log.Errorf("reading PoSt challenge for sector %d, vlen:%d, err: %s", s.SectorNumber, len(vanilla), err)
				return
			}

			vproofs[i] = vanilla
		}(i, s)
	}
	wg.Wait()

	if len(skipped) > 0 && !allowSkip {
		// This should happen rarely because before entering GenerateWindowPoSt we check all sectors by reading challenges.
		// When it does happen, window post runner logic will just re-check sectors, and retry with newly-discovered-bad sectors skipped
		log.Errorf("couldn't read some challenges (skipped %d)", len(skipped))

		// note: can't return an error as this in an jsonrpc call
		return storiface.WindowPoStResult{Skipped: skipped}, nil
	}

	// compact skipped sectors
	var skippedSoFar int
	for i := range vproofs {
		if len(vproofs[i]) == 0 {
			skippedSoFar++
			continue
		}

		if skippedSoFar > 0 {
			vproofs[i-skippedSoFar] = vproofs[i]
		}
	}

	vproofs = vproofs[:len(vproofs)-skippedSoFar]

	// compute the PoSt!
	res, err := sb.GenerateWindowPoStWithVanilla(ctx, ppt, mid, randomness, vproofs, partitionIdx)
	r := storiface.WindowPoStResult{
		PoStProofs: res,
		Skipped:    skipped,
	}
	if err != nil {
		log.Errorw("generating window PoSt failed", "error", err)
		return r, xerrors.Errorf("generate window PoSt with vanilla proofs: %w", err)
	}
	return r, nil
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

func (l *LocalWorker) Paths(ctx context.Context) ([]storiface.StoragePath, error) {
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
	gpus, err := ffi.GetGPUDevices()
	if err != nil {
		log.Errorf("getting gpu devices failed: %+v", err)
	}
	log.Infow("Detected GPU devices.", "count", len(gpus))

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
		Hostname:        l.name,
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

func (l *LocalWorker) Done() <-chan struct{} {
	return l.closing
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
