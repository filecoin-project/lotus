package sealer

import (
	"context"
	"errors"
	"io"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/hashicorp/go-multierror"
	"github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log/v2"
	"github.com/mitchellh/go-homedir"
	"go.uber.org/multierr"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-statestore"

	"github.com/filecoin-project/lotus/node/config"
	"github.com/filecoin-project/lotus/storage/paths"
	"github.com/filecoin-project/lotus/storage/sealer/ffiwrapper"
	"github.com/filecoin-project/lotus/storage/sealer/fsutil"
	"github.com/filecoin-project/lotus/storage/sealer/sealtasks"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
)

var log = logging.Logger("advmgr")

var ErrNoWorkers = errors.New("no suitable workers found")

type Worker interface {
	storiface.WorkerCalls

	TaskTypes(context.Context) (map[sealtasks.TaskType]struct{}, error)

	// Returns paths accessible to the worker
	Paths(context.Context) ([]storiface.StoragePath, error)

	Info(context.Context) (storiface.WorkerInfo, error)

	Session(context.Context) (uuid.UUID, error)

	Close() error // TODO: do we need this?
}

type SectorManager interface {
	storiface.Sealer
	storiface.ProverPoSt
	storiface.WorkerReturn
	FaultTracker
}

var ClosedWorkerID = uuid.UUID{}

type Manager struct {
	ls         paths.LocalStorage
	storage    paths.Store
	localStore *paths.Local
	remoteHnd  *paths.FetchHandler
	index      paths.SectorIndex

	sched            *Scheduler
	windowPoStSched  *poStScheduler
	winningPoStSched *poStScheduler

	localProver storiface.ProverPoSt

	workLk sync.Mutex
	work   *statestore.StateStore

	parallelCheckLimit        int
	singleCheckTimeout        time.Duration
	partitionCheckTimeout     time.Duration
	disableBuiltinWindowPoSt  bool
	disableBuiltinWinningPoSt bool
	disallowRemoteFinalize    bool

	callToWork map[storiface.CallID]WorkID
	// used when we get an early return and there's no callToWork mapping
	callRes map[storiface.CallID]chan result

	results map[WorkID]result
	waitRes map[WorkID]chan struct{}
}

var _ storiface.ProverPoSt = &Manager{}

type result struct {
	r   interface{}
	err error
}

type StorageAuth http.Header

type WorkerStateStore *statestore.StateStore
type ManagerStateStore *statestore.StateStore

func New(ctx context.Context, lstor *paths.Local, stor paths.Store, ls paths.LocalStorage, si paths.SectorIndex, sc config.SealerConfig, pc config.ProvingConfig, wss WorkerStateStore, mss ManagerStateStore) (*Manager, error) {
	prover, err := ffiwrapper.New(&readonlyProvider{stor: lstor, index: si})
	if err != nil {
		return nil, xerrors.Errorf("creating prover instance: %w", err)
	}

	sh, err := newScheduler(ctx, sc.Assigner)
	if err != nil {
		return nil, err
	}

	m := &Manager{
		ls:         ls,
		storage:    stor,
		localStore: lstor,
		remoteHnd:  &paths.FetchHandler{Local: lstor, PfHandler: &paths.DefaultPartialFileHandler{}},
		index:      si,

		sched:            sh,
		windowPoStSched:  newPoStScheduler(sealtasks.TTGenerateWindowPoSt),
		winningPoStSched: newPoStScheduler(sealtasks.TTGenerateWinningPoSt),

		localProver: prover,

		parallelCheckLimit:        pc.ParallelCheckLimit,
		singleCheckTimeout:        time.Duration(pc.SingleCheckTimeout),
		partitionCheckTimeout:     time.Duration(pc.PartitionCheckTimeout),
		disableBuiltinWindowPoSt:  pc.DisableBuiltinWindowPoSt,
		disableBuiltinWinningPoSt: pc.DisableBuiltinWinningPoSt,
		disallowRemoteFinalize:    sc.DisallowRemoteFinalize,

		work:       mss,
		callToWork: map[storiface.CallID]WorkID{},
		callRes:    map[storiface.CallID]chan result{},
		results:    map[WorkID]result{},
		waitRes:    map[WorkID]chan struct{}{},
	}

	m.setupWorkTracker()

	go m.sched.runSched()

	localTasks := []sealtasks.TaskType{
		sealtasks.TTCommit1, sealtasks.TTProveReplicaUpdate1, sealtasks.TTFinalize, sealtasks.TTFetch, sealtasks.TTFinalizeUnsealed, sealtasks.TTFinalizeReplicaUpdate,
	}
	if sc.AllowSectorDownload {
		localTasks = append(localTasks, sealtasks.TTDownloadSector)
	}
	if sc.AllowAddPiece {
		localTasks = append(localTasks, sealtasks.TTAddPiece, sealtasks.TTDataCid)
	}
	if sc.AllowPreCommit1 {
		localTasks = append(localTasks, sealtasks.TTPreCommit1)
	}
	if sc.AllowPreCommit2 {
		localTasks = append(localTasks, sealtasks.TTPreCommit2)
	}
	if sc.AllowCommit {
		localTasks = append(localTasks, sealtasks.TTCommit2)
	}
	if sc.AllowUnseal {
		localTasks = append(localTasks, sealtasks.TTUnseal)
	}
	if sc.AllowReplicaUpdate {
		localTasks = append(localTasks, sealtasks.TTReplicaUpdate)
	}
	if sc.AllowProveReplicaUpdate2 {
		localTasks = append(localTasks, sealtasks.TTProveReplicaUpdate2)
	}
	if sc.AllowRegenSectorKey {
		localTasks = append(localTasks, sealtasks.TTRegenSectorKey)
	}

	wcfg := WorkerConfig{
		IgnoreResourceFiltering: sc.ResourceFiltering == config.ResourceFilteringDisabled,
		TaskTypes:               localTasks,
		Name:                    sc.LocalWorkerName,
	}
	worker := NewLocalWorker(wcfg, stor, lstor, si, m, wss)
	err = m.AddWorker(ctx, worker)
	if err != nil {
		return nil, xerrors.Errorf("adding local worker: %w", err)
	}

	return m, nil
}

func (m *Manager) AddLocalStorage(ctx context.Context, path string) error {
	path, err := homedir.Expand(path)
	if err != nil {
		return xerrors.Errorf("expanding local path: %w", err)
	}

	if err := m.localStore.OpenPath(ctx, path); err != nil {
		return xerrors.Errorf("opening local path: %w", err)
	}

	if err := m.ls.SetStorage(func(sc *storiface.StorageConfig) {
		sc.StoragePaths = append(sc.StoragePaths, storiface.LocalPath{Path: path})
	}); err != nil {
		return xerrors.Errorf("get storage config: %w", err)
	}
	return nil
}

func (m *Manager) DetachLocalStorage(ctx context.Context, path string) error {
	path, err := homedir.Expand(path)
	if err != nil {
		return xerrors.Errorf("expanding local path: %w", err)
	}

	// check that we have the path opened
	lps, err := m.localStore.Local(ctx)
	if err != nil {
		return xerrors.Errorf("getting local path list: %w", err)
	}

	var localPath *storiface.StoragePath
	for _, lp := range lps {
		if lp.LocalPath == path {
			lp := lp // copy to make the linter happy
			localPath = &lp
			break
		}
	}
	if localPath == nil {
		return xerrors.Errorf("no local paths match '%s'", path)
	}

	// drop from the persisted storage.json
	var found bool
	if err := m.ls.SetStorage(func(sc *storiface.StorageConfig) {
		out := make([]storiface.LocalPath, 0, len(sc.StoragePaths))
		for _, storagePath := range sc.StoragePaths {
			if storagePath.Path != path {
				out = append(out, storagePath)
				continue
			}
			found = true
		}
		sc.StoragePaths = out
	}); err != nil {
		return xerrors.Errorf("set storage config: %w", err)
	}
	if !found {
		// maybe this is fine?
		return xerrors.Errorf("path not found in storage.json")
	}

	// unregister locally, drop from sector index
	return m.localStore.ClosePath(ctx, localPath.ID)
}

func (m *Manager) RedeclareLocalStorage(ctx context.Context, id *storiface.ID, dropMissing bool) error {
	return m.localStore.Redeclare(ctx, id, dropMissing)
}

func (m *Manager) AddWorker(ctx context.Context, w Worker) error {
	sessID, err := w.Session(ctx)
	if err != nil {
		return xerrors.Errorf("getting worker session: %w", err)
	}
	if sessID == ClosedWorkerID {
		return xerrors.Errorf("worker already closed")
	}

	wid := storiface.WorkerID(sessID)

	whnd, err := newWorkerHandle(ctx, w)
	if err != nil {
		return err
	}

	tasks, err := w.TaskTypes(ctx)
	if err != nil {
		return xerrors.Errorf("getting worker tasks: %w", err)
	}

	if m.windowPoStSched.MaybeAddWorker(wid, tasks, whnd) ||
		m.winningPoStSched.MaybeAddWorker(wid, tasks, whnd) {
		return nil
	}

	return m.sched.runWorker(ctx, wid, whnd)
}

func (m *Manager) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	m.remoteHnd.ServeHTTP(w, r)
}

var schedNop = PrepareAction{
	Action: func(ctx context.Context, w Worker) error {
		return nil
	},
	PrepType: sealtasks.TTNoop,
}

func (m *Manager) schedFetch(sector storiface.SectorRef, ft storiface.SectorFileType, ptype storiface.PathType, am storiface.AcquireMode) PrepareAction {
	return PrepareAction{
		Action: func(ctx context.Context, worker Worker) error {
			_, err := m.waitSimpleCall(ctx)(worker.Fetch(ctx, sector, ft, ptype, am))
			return err
		},
		PrepType: sealtasks.TTFetch,
	}
}

// SectorsUnsealPiece will Unseal the Sealed sector file for the given sector.
// It will schedule the Unsealing task on a worker that either already has the sealed sector files or has space in
// one of it's sealing scratch spaces to store them after fetching them from another worker.
// If the chosen worker already has the Unsealed sector file, we will NOT Unseal the sealed sector file again.
func (m *Manager) SectorsUnsealPiece(ctx context.Context, sector storiface.SectorRef, offset storiface.UnpaddedByteIndex, size abi.UnpaddedPieceSize, ticket abi.SealRandomness, unsealed *cid.Cid) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	log.Debugf("acquire unseal sector lock for sector %d", sector.ID)
	if err := m.index.StorageLock(ctx, sector.ID, storiface.FTSealed|storiface.FTCache|storiface.FTUpdate|storiface.FTUpdateCache, storiface.FTUnsealed); err != nil {
		return xerrors.Errorf("acquiring unseal sector lock: %w", err)
	}

	// Check if sealed or update sector file exists
	s, err := m.index.StorageFindSector(ctx, sector.ID, storiface.FTSealed|storiface.FTUpdate, 0, false)
	if err != nil {
		return xerrors.Errorf("finding sealed or updated sector: %w", err)
	}
	if len(s) == 0 {
		return xerrors.Errorf("sealed or updated sector file not found for sector %d", sector.ID)
	}

	// if the selected worker does NOT have the sealed files for the sector, instruct it to fetch it from a worker that has them and
	// put it in the sealing scratch space.
	unsealFetch := PrepareAction{
		Action: func(ctx context.Context, worker Worker) error {
			log.Debugf("copy sealed/cache sector data for sector %d", sector.ID)
			_, err := m.waitSimpleCall(ctx)(worker.Fetch(ctx, sector, storiface.FTSealed|storiface.FTCache, storiface.PathSealing, storiface.AcquireCopy))
			_, err2 := m.waitSimpleCall(ctx)(worker.Fetch(ctx, sector, storiface.FTUpdate|storiface.FTUpdateCache, storiface.PathSealing, storiface.AcquireCopy))

			if err != nil && err2 != nil {
				return xerrors.Errorf("cannot unseal piece. error fetching sealed data: %w. error fetching replica data: %w", err, err2)
			}

			return nil
		},
		PrepType: sealtasks.TTFetch,
	}

	if unsealed == nil {
		return xerrors.Errorf("cannot unseal piece (sector: %d, offset: %d size: %d) - unsealed cid is undefined", sector, offset, size)
	}

	ssize, err := sector.ProofType.SectorSize()
	if err != nil {
		return xerrors.Errorf("getting sector size: %w", err)
	}

	// selector will schedule the Unseal task on a worker that either already has the sealed sector files or has space in
	// one of it's sealing scratch spaces to store them after fetching them from another worker.
	selector := newExistingSelector(m.index, sector.ID, storiface.FTSealed|storiface.FTCache, sector.ID.Miner, true)

	log.Debugf("will schedule unseal for sector %d", sector.ID)
	err = m.sched.Schedule(ctx, sector, sealtasks.TTUnseal, selector, unsealFetch, func(ctx context.Context, w Worker) error {
		// TODO: make restartable

		// NOTE: we're unsealing the whole sector here as with SDR we can't really
		//  unseal the sector partially. Requesting the whole sector here can
		//  save us some work in case another piece is requested from here
		log.Debugf("calling unseal sector on worker, sectoID=%d", sector.ID)

		// Note: This unseal piece call will essentially become a no-op if the worker already has an Unsealed sector file for the given sector.
		_, err := m.waitSimpleCall(ctx)(w.UnsealPiece(ctx, sector, 0, abi.PaddedPieceSize(ssize).Unpadded(), ticket, *unsealed))
		log.Debugf("completed unseal sector %d", sector.ID)
		return err
	})
	if err != nil {
		return xerrors.Errorf("worker UnsealPiece call: %s", err)
	}

	// get a selector for moving unsealed sector into long-term storage
	fetchSel := newMoveSelector(m.index, sector.ID, storiface.FTUnsealed, storiface.PathStorage, sector.ID.Miner, !m.disallowRemoteFinalize)

	// move unsealed sector to long-term storage
	// Possible TODO: Add an option to not keep the unsealed sector in long term storage?
	err = m.sched.Schedule(ctx, sector, sealtasks.TTFetch, fetchSel,
		m.schedFetch(sector, storiface.FTUnsealed, storiface.PathStorage, storiface.AcquireMove),
		func(ctx context.Context, w Worker) error {
			_, err := m.waitSimpleCall(ctx)(w.MoveStorage(ctx, sector, storiface.FTUnsealed))
			return err
		})
	if err != nil {
		return xerrors.Errorf("moving unsealed sector to long term storage: %w", err)
	}

	return nil
}

func (m *Manager) NewSector(ctx context.Context, sector storiface.SectorRef) error {
	log.Warnf("stub NewSector")
	return nil
}

func (m *Manager) DataCid(ctx context.Context, pieceSize abi.UnpaddedPieceSize, pieceData storiface.Data) (abi.PieceInfo, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	selector := newTaskSelector()

	var out abi.PieceInfo
	err := m.sched.Schedule(ctx, storiface.NoSectorRef, sealtasks.TTDataCid, selector, schedNop, func(ctx context.Context, w Worker) error {
		p, err := m.waitSimpleCall(ctx)(w.DataCid(ctx, pieceSize, pieceData))
		if err != nil {
			return err
		}
		if p != nil {
			out = p.(abi.PieceInfo)
		}
		return nil
	})

	return out, err
}

func (m *Manager) AddPiece(ctx context.Context, sector storiface.SectorRef, existingPieces []abi.UnpaddedPieceSize, sz abi.UnpaddedPieceSize, r io.Reader) (abi.PieceInfo, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	if err := m.index.StorageLock(ctx, sector.ID, storiface.FTNone, storiface.FTUnsealed); err != nil {
		return abi.PieceInfo{}, xerrors.Errorf("acquiring sector lock: %w", err)
	}

	var selector WorkerSelector
	var err error
	if len(existingPieces) == 0 { // new
		selector = newAllocSelector(m.index, storiface.FTUnsealed, storiface.PathSealing, sector.ID.Miner)
	} else { // use existing
		selector = newExistingSelector(m.index, sector.ID, storiface.FTUnsealed, sector.ID.Miner, false)
	}

	var out abi.PieceInfo
	err = m.sched.Schedule(ctx, sector, sealtasks.TTAddPiece, selector, schedNop, func(ctx context.Context, w Worker) error {
		p, err := m.waitSimpleCall(ctx)(w.AddPiece(ctx, sector, existingPieces, sz, r))
		if err != nil {
			return err
		}
		if p != nil {
			out = p.(abi.PieceInfo)
		}
		return nil
	})

	return out, err
}

func (m *Manager) SealPreCommit1(ctx context.Context, sector storiface.SectorRef, ticket abi.SealRandomness, pieces []abi.PieceInfo) (out storiface.PreCommit1Out, err error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	wk, wait, cancel, err := m.getWork(ctx, sealtasks.TTPreCommit1, sector, ticket, pieces)
	if err != nil {
		return nil, xerrors.Errorf("getWork: %w", err)
	}
	defer cancel()

	var waitErr error
	waitRes := func() {
		p, werr := m.waitWork(ctx, wk)
		if werr != nil {
			waitErr = werr
			return
		}
		if p != nil {
			out = p.(storiface.PreCommit1Out)
		}
	}

	if wait { // already in progress
		waitRes()
		return out, waitErr
	}

	if err := m.index.StorageLock(ctx, sector.ID, storiface.FTUnsealed, storiface.FTSealed|storiface.FTCache); err != nil {
		return nil, xerrors.Errorf("acquiring sector lock: %w", err)
	}

	// TODO: also consider where the unsealed data sits

	selector := newAllocSelector(m.index, storiface.FTCache|storiface.FTSealed, storiface.PathSealing, sector.ID.Miner)

	err = m.sched.Schedule(ctx, sector, sealtasks.TTPreCommit1, selector, m.schedFetch(sector, storiface.FTUnsealed, storiface.PathSealing, storiface.AcquireMove), func(ctx context.Context, w Worker) error {
		err := m.startWork(ctx, w, wk)(w.SealPreCommit1(ctx, sector, ticket, pieces))
		if err != nil {
			return err
		}

		waitRes()
		return nil
	})
	if err != nil {
		return nil, err
	}

	return out, waitErr
}

func (m *Manager) SealPreCommit2(ctx context.Context, sector storiface.SectorRef, phase1Out storiface.PreCommit1Out) (out storiface.SectorCids, err error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	wk, wait, cancel, err := m.getWork(ctx, sealtasks.TTPreCommit2, sector, phase1Out)
	if err != nil {
		return storiface.SectorCids{}, xerrors.Errorf("getWork: %w", err)
	}
	defer cancel()

	var waitErr error
	waitRes := func() {
		p, werr := m.waitWork(ctx, wk)
		if werr != nil {
			waitErr = werr
			return
		}
		if p != nil {
			out = p.(storiface.SectorCids)
		}
	}

	if wait { // already in progress
		waitRes()
		return out, waitErr
	}

	if err := m.index.StorageLock(ctx, sector.ID, storiface.FTSealed, storiface.FTCache); err != nil {
		return storiface.SectorCids{}, xerrors.Errorf("acquiring sector lock: %w", err)
	}

	selector := newExistingSelector(m.index, sector.ID, storiface.FTCache|storiface.FTSealed, sector.ID.Miner, true)

	err = m.sched.Schedule(ctx, sector, sealtasks.TTPreCommit2, selector, m.schedFetch(sector, storiface.FTCache|storiface.FTSealed, storiface.PathSealing, storiface.AcquireMove), func(ctx context.Context, w Worker) error {
		err := m.startWork(ctx, w, wk)(w.SealPreCommit2(ctx, sector, phase1Out))
		if err != nil {
			return err
		}

		waitRes()
		return nil
	})
	if err != nil {
		return storiface.SectorCids{}, err
	}

	return out, waitErr
}

func (m *Manager) SealCommit1(ctx context.Context, sector storiface.SectorRef, ticket abi.SealRandomness, seed abi.InteractiveSealRandomness, pieces []abi.PieceInfo, cids storiface.SectorCids) (out storiface.Commit1Out, err error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	wk, wait, cancel, err := m.getWork(ctx, sealtasks.TTCommit1, sector, ticket, seed, pieces, cids)
	if err != nil {
		return storiface.Commit1Out{}, xerrors.Errorf("getWork: %w", err)
	}
	defer cancel()

	var waitErr error
	waitRes := func() {
		p, werr := m.waitWork(ctx, wk)
		if werr != nil {
			waitErr = werr
			return
		}
		if p != nil {
			out = p.(storiface.Commit1Out)
		}
	}

	if wait { // already in progress
		waitRes()
		return out, waitErr
	}

	if err := m.index.StorageLock(ctx, sector.ID, storiface.FTSealed, storiface.FTCache); err != nil {
		return storiface.Commit1Out{}, xerrors.Errorf("acquiring sector lock: %w", err)
	}

	// NOTE: We set allowFetch to false in so that we always execute on a worker
	// with direct access to the data. We want to do that because this step is
	// generally very cheap / fast, and transferring data is not worth the effort
	selector := newExistingSelector(m.index, sector.ID, storiface.FTCache|storiface.FTSealed, sector.ID.Miner, false)

	err = m.sched.Schedule(ctx, sector, sealtasks.TTCommit1, selector, m.schedFetch(sector, storiface.FTCache|storiface.FTSealed, storiface.PathSealing, storiface.AcquireMove), func(ctx context.Context, w Worker) error {
		err := m.startWork(ctx, w, wk)(w.SealCommit1(ctx, sector, ticket, seed, pieces, cids))
		if err != nil {
			return err
		}

		waitRes()
		return nil
	})
	if err != nil {
		return nil, err
	}

	return out, waitErr
}

func (m *Manager) SealCommit2(ctx context.Context, sector storiface.SectorRef, phase1Out storiface.Commit1Out) (out storiface.Proof, err error) {
	wk, wait, cancel, err := m.getWork(ctx, sealtasks.TTCommit2, sector, phase1Out)
	if err != nil {
		return storiface.Proof{}, xerrors.Errorf("getWork: %w", err)
	}
	defer cancel()

	var waitErr error
	waitRes := func() {
		p, werr := m.waitWork(ctx, wk)
		if werr != nil {
			waitErr = werr
			return
		}
		if p != nil {
			out = p.(storiface.Proof)
		}
	}

	if wait { // already in progress
		waitRes()
		return out, waitErr
	}

	selector := newTaskSelector()

	err = m.sched.Schedule(ctx, sector, sealtasks.TTCommit2, selector, schedNop, func(ctx context.Context, w Worker) error {
		err := m.startWork(ctx, w, wk)(w.SealCommit2(ctx, sector, phase1Out))
		if err != nil {
			return err
		}

		waitRes()
		return nil
	})

	if err != nil {
		return nil, err
	}

	return out, waitErr
}

// sectorStorageType tries to figure out storage type for a given sector; expects only a single copy of the file in the
// storage system
func (m *Manager) sectorStorageType(ctx context.Context, sector storiface.SectorRef, ft storiface.SectorFileType) (sectorFound bool, ptype storiface.PathType, err error) {
	stores, err := m.index.StorageFindSector(ctx, sector.ID, ft, 0, false)
	if err != nil {
		return false, "", xerrors.Errorf("finding sector: %w", err)
	}
	if len(stores) == 0 {
		return false, "", nil
	}

	for _, store := range stores {
		if store.CanSeal {
			return true, storiface.PathSealing, nil
		}
	}

	return true, storiface.PathStorage, nil
}

func (m *Manager) FinalizeSector(ctx context.Context, sector storiface.SectorRef) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	if err := m.index.StorageLock(ctx, sector.ID, storiface.FTNone, storiface.FTSealed|storiface.FTUnsealed|storiface.FTCache); err != nil {
		return xerrors.Errorf("acquiring sector lock: %w", err)
	}

	/*
		We want to:
		- Trim cache
		- Move stuff to long-term storage
	*/

	// remove redundant copies if there are any
	if err := m.storage.RemoveCopies(ctx, sector.ID, storiface.FTUnsealed); err != nil {
		return xerrors.Errorf("remove copies (sealed): %w", err)
	}
	if err := m.storage.RemoveCopies(ctx, sector.ID, storiface.FTSealed); err != nil {
		return xerrors.Errorf("remove copies (sealed): %w", err)
	}
	if err := m.storage.RemoveCopies(ctx, sector.ID, storiface.FTCache); err != nil {
		return xerrors.Errorf("remove copies (cache): %w", err)
	}

	// Make sure that the cache files are still in sealing storage; In case not,
	// we want to do finalize in long-term storage
	_, cachePathType, err := m.sectorStorageType(ctx, sector, storiface.FTCache)
	if err != nil {
		return xerrors.Errorf("checking cache storage type: %w", err)
	}

	// do the cache trimming wherever the likely still very large cache lives.
	// we really don't want to move it.
	selector := newExistingSelector(m.index, sector.ID, storiface.FTCache, sector.ID.Miner, false)

	err = m.sched.Schedule(ctx, sector, sealtasks.TTFinalize, selector,
		m.schedFetch(sector, storiface.FTCache, cachePathType, storiface.AcquireMove),
		func(ctx context.Context, w Worker) error {
			_, err := m.waitSimpleCall(ctx)(w.FinalizeSector(ctx, sector))
			return err
		})
	if err != nil {
		return err
	}

	// get a selector for moving stuff into long-term storage
	fetchSel := newMoveSelector(m.index, sector.ID, storiface.FTCache|storiface.FTSealed, storiface.PathStorage, sector.ID.Miner, !m.disallowRemoteFinalize)

	// only move the unsealed file if it still exists and needs moving
	moveUnsealed := storiface.FTUnsealed
	{
		found, unsealedPathType, err := m.sectorStorageType(ctx, sector, storiface.FTUnsealed)
		if err != nil {
			return xerrors.Errorf("checking cache storage type: %w", err)
		}

		if !found || unsealedPathType == storiface.PathStorage {
			moveUnsealed = storiface.FTNone
		}
	}

	// move stuff to long-term storage
	err = m.sched.Schedule(ctx, sector, sealtasks.TTFetch, fetchSel,
		m.schedFetch(sector, storiface.FTCache|storiface.FTSealed|moveUnsealed, storiface.PathStorage, storiface.AcquireMove),
		func(ctx context.Context, w Worker) error {
			_, err := m.waitSimpleCall(ctx)(w.MoveStorage(ctx, sector, storiface.FTCache|storiface.FTSealed|moveUnsealed))
			return err
		})
	if err != nil {
		return xerrors.Errorf("moving sector to storage: %w", err)
	}

	return nil
}

func (m *Manager) FinalizeReplicaUpdate(ctx context.Context, sector storiface.SectorRef) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	if err := m.index.StorageLock(ctx, sector.ID, storiface.FTNone, storiface.FTSealed|storiface.FTUnsealed|storiface.FTCache|storiface.FTUpdate|storiface.FTUpdateCache); err != nil {
		return xerrors.Errorf("acquiring sector lock: %w", err)
	}

	// Make sure that the update file is still in sealing storage; In case it already
	// isn't, we want to do finalize in long-term storage
	pathType := storiface.PathStorage
	{
		sealedStores, err := m.index.StorageFindSector(ctx, sector.ID, storiface.FTUpdate, 0, false)
		if err != nil {
			return xerrors.Errorf("finding sealed sector: %w", err)
		}

		for _, store := range sealedStores {
			if store.CanSeal {
				pathType = storiface.PathSealing
				break
			}
		}
	}

	// do the cache trimming wherever the likely still large cache lives.
	// we really don't want to move it.
	selector := newExistingSelector(m.index, sector.ID, storiface.FTUpdateCache, sector.ID.Miner, false)

	err := m.sched.Schedule(ctx, sector, sealtasks.TTFinalizeReplicaUpdate, selector,
		m.schedFetch(sector, storiface.FTCache|storiface.FTUpdateCache, pathType, storiface.AcquireMove),
		func(ctx context.Context, w Worker) error {
			_, err := m.waitSimpleCall(ctx)(w.FinalizeReplicaUpdate(ctx, sector))
			return err
		})
	if err != nil {
		return err
	}

	move := func(types storiface.SectorFileType) error {
		// get a selector for moving stuff into long-term storage
		fetchSel := newMoveSelector(m.index, sector.ID, types, storiface.PathStorage, sector.ID.Miner, !m.disallowRemoteFinalize)

		err = m.sched.Schedule(ctx, sector, sealtasks.TTFetch, fetchSel,
			m.schedFetch(sector, types, storiface.PathStorage, storiface.AcquireMove),
			func(ctx context.Context, w Worker) error {
				_, err := m.waitSimpleCall(ctx)(w.MoveStorage(ctx, sector, types))
				return err
			})
		if err != nil {
			return xerrors.Errorf("moving sector to storage: %w", err)
		}
		return nil
	}

	err = multierr.Append(move(storiface.FTUpdate|storiface.FTUpdateCache), move(storiface.FTCache))
	err = multierr.Append(err, move(storiface.FTSealed)) // Sealed separate from cache just in case ReleaseSectorKey was already called

	{
		unsealedStores, ferr := m.index.StorageFindSector(ctx, sector.ID, storiface.FTUnsealed, 0, false)
		if ferr != nil {
			err = multierr.Append(err, xerrors.Errorf("find unsealed sector before move: %w", ferr))
		} else if len(unsealedStores) > 0 {
			// if we found unsealed files, AND have been asked to keep at least one piece, move unsealed
			err = multierr.Append(err, move(storiface.FTUnsealed))
		}
	}

	if err != nil {
		return xerrors.Errorf("moving sector to storage: %w", err)
	}

	return nil
}

func (m *Manager) ReleaseUnsealed(ctx context.Context, sector storiface.SectorRef, keepUnsealed []storiface.Range) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	if err := m.index.StorageLock(ctx, sector.ID, storiface.FTNone, storiface.FTUnsealed); err != nil {
		return xerrors.Errorf("acquiring sector lock: %w", err)
	}

	found, pathType, err := m.sectorStorageType(ctx, sector, storiface.FTUnsealed)
	if err != nil {
		return xerrors.Errorf("checking cache storage type: %w", err)
	}
	if !found {
		// already removed
		return nil
	}

	selector := newExistingSelector(m.index, sector.ID, storiface.FTUnsealed, sector.ID.Miner, false)

	return m.sched.Schedule(ctx, sector, sealtasks.TTFinalizeUnsealed, selector, m.schedFetch(sector, storiface.FTUnsealed, pathType, storiface.AcquireMove), func(ctx context.Context, w Worker) error {
		_, err := m.waitSimpleCall(ctx)(w.ReleaseUnsealed(ctx, sector, keepUnsealed))
		if err != nil {
			return err
		}

		return nil
	})
}

func (m *Manager) ReleaseSectorKey(ctx context.Context, sector storiface.SectorRef) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	if err := m.index.StorageLock(ctx, sector.ID, storiface.FTNone, storiface.FTSealed); err != nil {
		return xerrors.Errorf("acquiring sector lock: %w", err)
	}

	return m.storage.Remove(ctx, sector.ID, storiface.FTSealed, true, nil)
}

func (m *Manager) ReleaseReplicaUpgrade(ctx context.Context, sector storiface.SectorRef) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	if err := m.index.StorageLock(ctx, sector.ID, storiface.FTNone, storiface.FTUpdateCache|storiface.FTUpdate); err != nil {
		return xerrors.Errorf("acquiring sector lock: %w", err)
	}

	if err := m.storage.Remove(ctx, sector.ID, storiface.FTUpdateCache, true, nil); err != nil {
		return xerrors.Errorf("removing update cache: %w", err)
	}
	if err := m.storage.Remove(ctx, sector.ID, storiface.FTUpdate, true, nil); err != nil {
		return xerrors.Errorf("removing update: %w", err)
	}
	return nil
}

func (m *Manager) GenerateSectorKeyFromData(ctx context.Context, sector storiface.SectorRef, commD cid.Cid) error {

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	wk, wait, cancel, err := m.getWork(ctx, sealtasks.TTRegenSectorKey, sector, commD)
	if err != nil {
		return xerrors.Errorf("getWork: %w", err)
	}
	defer cancel()

	var waitErr error
	waitRes := func() {
		_, werr := m.waitWork(ctx, wk)
		if werr != nil {
			waitErr = werr
			return
		}
	}

	if wait { // already in progress
		waitRes()
		return waitErr
	}

	if err := m.index.StorageLock(ctx, sector.ID, storiface.FTUnsealed|storiface.FTUpdate|storiface.FTUpdateCache, storiface.FTSealed|storiface.FTCache); err != nil {
		return xerrors.Errorf("acquiring sector lock: %w", err)
	}

	// NOTE: We set allowFetch to false in so that we always execute on a worker
	// with direct access to the data. We want to do that because this step is
	// generally very cheap / fast, and transferring data is not worth the effort
	selector := newExistingSelector(m.index, sector.ID, storiface.FTUnsealed|storiface.FTUpdate|storiface.FTUpdateCache|storiface.FTCache, sector.ID.Miner, true)

	err = m.sched.Schedule(ctx, sector, sealtasks.TTRegenSectorKey, selector, m.schedFetch(sector, storiface.FTUpdate|storiface.FTUnsealed, storiface.PathSealing, storiface.AcquireMove), func(ctx context.Context, w Worker) error {
		err := m.startWork(ctx, w, wk)(w.GenerateSectorKeyFromData(ctx, sector, commD))
		if err != nil {
			return err
		}

		waitRes()
		return nil
	})
	if err != nil {
		return err
	}

	return waitErr
}

func (m *Manager) Remove(ctx context.Context, sector storiface.SectorRef) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	if err := m.index.StorageLock(ctx, sector.ID, storiface.FTNone, storiface.FTSealed|storiface.FTUnsealed|storiface.FTCache|storiface.FTUpdate|storiface.FTUpdateCache); err != nil {
		return xerrors.Errorf("acquiring sector lock: %w", err)
	}

	var err error

	if rerr := m.storage.Remove(ctx, sector.ID, storiface.FTSealed, true, nil); rerr != nil {
		err = multierror.Append(err, xerrors.Errorf("removing sector (sealed): %w", rerr))
	}
	if rerr := m.storage.Remove(ctx, sector.ID, storiface.FTCache, true, nil); rerr != nil {
		err = multierror.Append(err, xerrors.Errorf("removing sector (cache): %w", rerr))
	}
	if rerr := m.storage.Remove(ctx, sector.ID, storiface.FTUnsealed, true, nil); rerr != nil {
		err = multierror.Append(err, xerrors.Errorf("removing sector (unsealed): %w", rerr))
	}
	if rerr := m.storage.Remove(ctx, sector.ID, storiface.FTUpdate, true, nil); rerr != nil {
		err = multierror.Append(err, xerrors.Errorf("removing sector (unsealed): %w", rerr))
	}
	if rerr := m.storage.Remove(ctx, sector.ID, storiface.FTUpdateCache, true, nil); rerr != nil {
		err = multierror.Append(err, xerrors.Errorf("removing sector (unsealed): %w", rerr))
	}

	return err
}

func (m *Manager) ReplicaUpdate(ctx context.Context, sector storiface.SectorRef, pieces []abi.PieceInfo) (out storiface.ReplicaUpdateOut, err error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	log.Debugf("manager is doing replica update")
	wk, wait, cancel, err := m.getWork(ctx, sealtasks.TTReplicaUpdate, sector, pieces)
	if err != nil {
		return storiface.ReplicaUpdateOut{}, xerrors.Errorf("getWork: %w", err)
	}
	defer cancel()

	var waitErr error
	waitRes := func() {
		p, werr := m.waitWork(ctx, wk)
		if werr != nil {
			waitErr = xerrors.Errorf("waitWork: %w", werr)
			return
		}
		if p != nil {
			out = p.(storiface.ReplicaUpdateOut)
		}
	}

	if wait { // already in progress
		waitRes()
		return out, waitErr
	}

	if err := m.index.StorageLock(ctx, sector.ID, storiface.FTUnsealed|storiface.FTSealed|storiface.FTCache, storiface.FTUpdate|storiface.FTUpdateCache); err != nil {
		return storiface.ReplicaUpdateOut{}, xerrors.Errorf("acquiring sector lock: %w", err)
	}

	selector := newAllocSelector(m.index, storiface.FTUpdate|storiface.FTUpdateCache, storiface.PathSealing, sector.ID.Miner)

	err = m.sched.Schedule(ctx, sector, sealtasks.TTReplicaUpdate, selector, m.schedFetch(sector, storiface.FTUnsealed|storiface.FTSealed|storiface.FTCache, storiface.PathSealing, storiface.AcquireCopy), func(ctx context.Context, w Worker) error {
		err := m.startWork(ctx, w, wk)(w.ReplicaUpdate(ctx, sector, pieces))
		if err != nil {
			return xerrors.Errorf("startWork: %w", err)
		}

		waitRes()
		return nil
	})
	if err != nil {
		return storiface.ReplicaUpdateOut{}, xerrors.Errorf("Schedule: %w", err)
	}
	return out, waitErr
}

func (m *Manager) ProveReplicaUpdate1(ctx context.Context, sector storiface.SectorRef, sectorKey, newSealed, newUnsealed cid.Cid) (out storiface.ReplicaVanillaProofs, err error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	wk, wait, cancel, err := m.getWork(ctx, sealtasks.TTProveReplicaUpdate1, sector, sectorKey, newSealed, newUnsealed)
	if err != nil {
		return nil, xerrors.Errorf("getWork: %w", err)
	}
	defer cancel()

	var waitErr error
	waitRes := func() {
		p, werr := m.waitWork(ctx, wk)
		if werr != nil {
			waitErr = werr
			return
		}
		if p != nil {
			out = p.(storiface.ReplicaVanillaProofs)
		}
	}

	if wait { // already in progress
		waitRes()
		return out, waitErr
	}

	if err := m.index.StorageLock(ctx, sector.ID, storiface.FTSealed|storiface.FTUpdate|storiface.FTCache|storiface.FTUpdateCache, storiface.FTNone); err != nil {
		return nil, xerrors.Errorf("acquiring sector lock: %w", err)
	}

	// NOTE: We set allowFetch to false in so that we always execute on a worker
	// with direct access to the data. We want to do that because this step is
	// generally very cheap / fast, and transferring data is not worth the effort
	selector := newExistingSelector(m.index, sector.ID, storiface.FTUpdate|storiface.FTUpdateCache, sector.ID.Miner, false)

	err = m.sched.Schedule(ctx, sector, sealtasks.TTProveReplicaUpdate1, selector, m.schedFetch(sector, storiface.FTSealed|storiface.FTCache|storiface.FTUpdate|storiface.FTUpdateCache, storiface.PathSealing, storiface.AcquireCopy), func(ctx context.Context, w Worker) error {

		err := m.startWork(ctx, w, wk)(w.ProveReplicaUpdate1(ctx, sector, sectorKey, newSealed, newUnsealed))
		if err != nil {
			return err
		}

		waitRes()
		return nil
	})
	if err != nil {
		return nil, err
	}

	return out, waitErr
}

func (m *Manager) ProveReplicaUpdate2(ctx context.Context, sector storiface.SectorRef, sectorKey, newSealed, newUnsealed cid.Cid, vanillaProofs storiface.ReplicaVanillaProofs) (out storiface.ReplicaUpdateProof, err error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	wk, wait, cancel, err := m.getWork(ctx, sealtasks.TTProveReplicaUpdate2, sector, sectorKey, newSealed, newUnsealed, vanillaProofs)
	if err != nil {
		return nil, xerrors.Errorf("getWork: %w", err)
	}
	defer cancel()

	var waitErr error
	waitRes := func() {
		p, werr := m.waitWork(ctx, wk)
		if werr != nil {
			waitErr = werr
			return
		}
		if p != nil {
			out = p.(storiface.ReplicaUpdateProof)
		}
	}

	if wait { // already in progress
		waitRes()
		return out, waitErr
	}

	selector := newTaskSelector()

	err = m.sched.Schedule(ctx, sector, sealtasks.TTProveReplicaUpdate2, selector, schedNop, func(ctx context.Context, w Worker) error {
		err := m.startWork(ctx, w, wk)(w.ProveReplicaUpdate2(ctx, sector, sectorKey, newSealed, newUnsealed, vanillaProofs))
		if err != nil {
			return err
		}

		waitRes()
		return nil
	})

	if err != nil {
		return nil, err
	}

	return out, waitErr
}

func (m *Manager) DownloadSectorData(ctx context.Context, sector storiface.SectorRef, finalized bool, src map[storiface.SectorFileType]storiface.SectorLocation) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var toFetch storiface.SectorFileType

	// get a sorted list of sectors files to make a consistent work key from
	ents := make([]struct {
		T storiface.SectorFileType
		S storiface.SectorLocation
	}, 0, len(src))
	for fileType, data := range src {
		if len(fileType.AllSet()) != 1 {
			return xerrors.Errorf("sector data entry must be for a single file type")
		}

		toFetch |= fileType

		ents = append(ents, struct {
			T storiface.SectorFileType
			S storiface.SectorLocation
		}{T: fileType, S: data})
	}
	sort.Slice(ents, func(i, j int) bool {
		return ents[i].T < ents[j].T
	})

	// get a work key
	wk, wait, cancel, err := m.getWork(ctx, sealtasks.TTDownloadSector, sector, ents)
	if err != nil {
		return xerrors.Errorf("getWork: %w", err)
	}
	defer cancel()

	var waitErr error
	waitRes := func() {
		_, werr := m.waitWork(ctx, wk)
		if werr != nil {
			waitErr = werr
			return
		}
	}

	if wait { // already in progress
		waitRes()
		return waitErr
	}

	ptype := storiface.PathSealing
	if finalized {
		ptype = storiface.PathStorage
	}

	selector := newAllocSelector(m.index, toFetch, ptype, sector.ID.Miner)

	err = m.sched.Schedule(ctx, sector, sealtasks.TTDownloadSector, selector, schedNop, func(ctx context.Context, w Worker) error {
		err := m.startWork(ctx, w, wk)(w.DownloadSectorData(ctx, sector, finalized, src))
		if err != nil {
			return err
		}

		waitRes()
		return nil
	})

	if err != nil {
		return err
	}

	return waitErr
}

func (m *Manager) ReturnDataCid(ctx context.Context, callID storiface.CallID, pi abi.PieceInfo, err *storiface.CallError) error {
	return m.returnResult(ctx, callID, pi, err)
}

func (m *Manager) ReturnAddPiece(ctx context.Context, callID storiface.CallID, pi abi.PieceInfo, err *storiface.CallError) error {
	return m.returnResult(ctx, callID, pi, err)
}

func (m *Manager) ReturnSealPreCommit1(ctx context.Context, callID storiface.CallID, p1o storiface.PreCommit1Out, err *storiface.CallError) error {
	return m.returnResult(ctx, callID, p1o, err)
}

func (m *Manager) ReturnSealPreCommit2(ctx context.Context, callID storiface.CallID, sealed storiface.SectorCids, err *storiface.CallError) error {
	return m.returnResult(ctx, callID, sealed, err)
}

func (m *Manager) ReturnSealCommit1(ctx context.Context, callID storiface.CallID, out storiface.Commit1Out, err *storiface.CallError) error {
	return m.returnResult(ctx, callID, out, err)
}

func (m *Manager) ReturnSealCommit2(ctx context.Context, callID storiface.CallID, proof storiface.Proof, err *storiface.CallError) error {
	return m.returnResult(ctx, callID, proof, err)
}

func (m *Manager) ReturnFinalizeSector(ctx context.Context, callID storiface.CallID, err *storiface.CallError) error {
	return m.returnResult(ctx, callID, nil, err)
}

func (m *Manager) ReturnReleaseUnsealed(ctx context.Context, callID storiface.CallID, err *storiface.CallError) error {
	return m.returnResult(ctx, callID, nil, err)
}

func (m *Manager) ReturnReplicaUpdate(ctx context.Context, callID storiface.CallID, out storiface.ReplicaUpdateOut, err *storiface.CallError) error {
	return m.returnResult(ctx, callID, out, err)
}

func (m *Manager) ReturnProveReplicaUpdate1(ctx context.Context, callID storiface.CallID, out storiface.ReplicaVanillaProofs, err *storiface.CallError) error {
	return m.returnResult(ctx, callID, out, err)
}

func (m *Manager) ReturnProveReplicaUpdate2(ctx context.Context, callID storiface.CallID, proof storiface.ReplicaUpdateProof, err *storiface.CallError) error {
	return m.returnResult(ctx, callID, proof, err)
}

func (m *Manager) ReturnFinalizeReplicaUpdate(ctx context.Context, callID storiface.CallID, err *storiface.CallError) error {
	return m.returnResult(ctx, callID, nil, err)
}

func (m *Manager) ReturnGenerateSectorKeyFromData(ctx context.Context, callID storiface.CallID, err *storiface.CallError) error {
	return m.returnResult(ctx, callID, nil, err)
}

func (m *Manager) ReturnMoveStorage(ctx context.Context, callID storiface.CallID, err *storiface.CallError) error {
	return m.returnResult(ctx, callID, nil, err)
}

func (m *Manager) ReturnUnsealPiece(ctx context.Context, callID storiface.CallID, err *storiface.CallError) error {
	return m.returnResult(ctx, callID, nil, err)
}

func (m *Manager) ReturnReadPiece(ctx context.Context, callID storiface.CallID, ok bool, err *storiface.CallError) error {
	return m.returnResult(ctx, callID, ok, err)
}

func (m *Manager) ReturnDownloadSector(ctx context.Context, callID storiface.CallID, err *storiface.CallError) error {
	return m.returnResult(ctx, callID, nil, err)
}

func (m *Manager) ReturnFetch(ctx context.Context, callID storiface.CallID, err *storiface.CallError) error {
	return m.returnResult(ctx, callID, nil, err)
}

func (m *Manager) StorageLocal(ctx context.Context) (map[storiface.ID]string, error) {
	l, err := m.localStore.Local(ctx)
	if err != nil {
		return nil, err
	}

	out := map[storiface.ID]string{}
	for _, st := range l {
		out[st.ID] = st.LocalPath
	}

	return out, nil
}

func (m *Manager) FsStat(ctx context.Context, id storiface.ID) (fsutil.FsStat, error) {
	return m.storage.FsStat(ctx, id)
}

func (m *Manager) SchedDiag(ctx context.Context, doSched bool) (interface{}, error) {
	if doSched {
		select {
		case m.sched.workerChange <- struct{}{}:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	si, err := m.sched.Info(ctx)
	if err != nil {
		return nil, err
	}

	type SchedInfo interface{}
	i := struct {
		SchedInfo

		ReturnedWork []string
		Waiting      []string

		CallToWork map[string]string

		EarlyRet []string
	}{
		SchedInfo: si,

		CallToWork: map[string]string{},
	}

	m.workLk.Lock()

	for w := range m.results {
		i.ReturnedWork = append(i.ReturnedWork, w.String())
	}

	for id := range m.callRes {
		i.EarlyRet = append(i.EarlyRet, id.String())
	}

	for w := range m.waitRes {
		i.Waiting = append(i.Waiting, w.String())
	}

	for c, w := range m.callToWork {
		i.CallToWork[c.String()] = w.String()
	}

	m.workLk.Unlock()

	return i, nil
}

func (m *Manager) RemoveSchedRequest(ctx context.Context, schedId uuid.UUID) error {
	return m.sched.RemoveRequest(ctx, schedId)
}

func (m *Manager) Close(ctx context.Context) error {
	m.windowPoStSched.schedClose()
	m.winningPoStSched.schedClose()
	return m.sched.Close(ctx)
}

var _ Unsealer = &Manager{}
var _ SectorManager = &Manager{}
