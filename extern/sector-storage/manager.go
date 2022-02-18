package sectorstorage

import (
	"context"
	"errors"
	"io"
	"net/http"
	"sync"

	"github.com/google/uuid"
	"github.com/hashicorp/go-multierror"
	"github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log/v2"
	"github.com/mitchellh/go-homedir"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-statestore"
	"github.com/filecoin-project/specs-storage/storage"

	"github.com/filecoin-project/lotus/extern/sector-storage/ffiwrapper"
	"github.com/filecoin-project/lotus/extern/sector-storage/fsutil"
	"github.com/filecoin-project/lotus/extern/sector-storage/sealtasks"
	"github.com/filecoin-project/lotus/extern/sector-storage/stores"
	"github.com/filecoin-project/lotus/extern/sector-storage/storiface"
)

var log = logging.Logger("advmgr")

var ErrNoWorkers = errors.New("no suitable workers found")

type Worker interface {
	storiface.WorkerCalls

	TaskTypes(context.Context) (map[sealtasks.TaskType]struct{}, error)

	// Returns paths accessible to the worker
	Paths(context.Context) ([]stores.StoragePath, error)

	Info(context.Context) (storiface.WorkerInfo, error)

	Session(context.Context) (uuid.UUID, error)

	Close() error // TODO: do we need this?
}

type SectorManager interface {
	ffiwrapper.StorageSealer
	storage.Prover
	storiface.WorkerReturn
	FaultTracker
}

var ClosedWorkerID = uuid.UUID{}

type Manager struct {
	ls         stores.LocalStorage
	storage    *stores.Remote
	localStore *stores.Local
	remoteHnd  *stores.FetchHandler
	index      stores.SectorIndex

	sched *scheduler

	storage.Prover

	workLk sync.Mutex
	work   *statestore.StateStore

	callToWork map[storiface.CallID]WorkID
	// used when we get an early return and there's no callToWork mapping
	callRes map[storiface.CallID]chan result

	results map[WorkID]result
	waitRes map[WorkID]chan struct{}
}

type result struct {
	r   interface{}
	err error
}

// ResourceFilteringStrategy is an enum indicating the kinds of resource
// filtering strategies that can be configured for workers.
type ResourceFilteringStrategy string

const (
	// ResourceFilteringHardware specifies that available hardware resources
	// should be evaluated when scheduling a task against the worker.
	ResourceFilteringHardware = ResourceFilteringStrategy("hardware")

	// ResourceFilteringDisabled disables resource filtering against this
	// worker. The scheduler may assign any task to this worker.
	ResourceFilteringDisabled = ResourceFilteringStrategy("disabled")
)

type SealerConfig struct {
	ParallelFetchLimit int

	// Local worker config
	AllowAddPiece            bool
	AllowPreCommit1          bool
	AllowPreCommit2          bool
	AllowCommit              bool
	AllowUnseal              bool
	AllowReplicaUpdate       bool
	AllowProveReplicaUpdate2 bool
	AllowRegenSectorKey      bool

	// ResourceFiltering instructs the system which resource filtering strategy
	// to use when evaluating tasks against this worker. An empty value defaults
	// to "hardware".
	ResourceFiltering ResourceFilteringStrategy
}

type StorageAuth http.Header

type WorkerStateStore *statestore.StateStore
type ManagerStateStore *statestore.StateStore

func New(ctx context.Context, lstor *stores.Local, stor *stores.Remote, ls stores.LocalStorage, si stores.SectorIndex, sc SealerConfig, wss WorkerStateStore, mss ManagerStateStore) (*Manager, error) {
	prover, err := ffiwrapper.New(&readonlyProvider{stor: lstor, index: si})
	if err != nil {
		return nil, xerrors.Errorf("creating prover instance: %w", err)
	}

	m := &Manager{
		ls:         ls,
		storage:    stor,
		localStore: lstor,
		remoteHnd:  &stores.FetchHandler{Local: lstor, PfHandler: &stores.DefaultPartialFileHandler{}},
		index:      si,

		sched: newScheduler(),

		Prover: prover,

		work:       mss,
		callToWork: map[storiface.CallID]WorkID{},
		callRes:    map[storiface.CallID]chan result{},
		results:    map[WorkID]result{},
		waitRes:    map[WorkID]chan struct{}{},
	}

	m.setupWorkTracker()

	go m.sched.runSched()

	localTasks := []sealtasks.TaskType{
		sealtasks.TTCommit1, sealtasks.TTProveReplicaUpdate1, sealtasks.TTFinalize, sealtasks.TTFetch, sealtasks.TTFinalizeReplicaUpdate,
	}
	if sc.AllowAddPiece {
		localTasks = append(localTasks, sealtasks.TTAddPiece)
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
		IgnoreResourceFiltering: sc.ResourceFiltering == ResourceFilteringDisabled,
		TaskTypes:               localTasks,
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

	if err := m.ls.SetStorage(func(sc *stores.StorageConfig) {
		sc.StoragePaths = append(sc.StoragePaths, stores.LocalPath{Path: path})
	}); err != nil {
		return xerrors.Errorf("get storage config: %w", err)
	}
	return nil
}

func (m *Manager) AddWorker(ctx context.Context, w Worker) error {
	return m.sched.runWorker(ctx, w)
}

func (m *Manager) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	m.remoteHnd.ServeHTTP(w, r)
}

func schedNop(context.Context, Worker) error {
	return nil
}

func (m *Manager) schedFetch(sector storage.SectorRef, ft storiface.SectorFileType, ptype storiface.PathType, am storiface.AcquireMode) func(context.Context, Worker) error {
	return func(ctx context.Context, worker Worker) error {
		_, err := m.waitSimpleCall(ctx)(worker.Fetch(ctx, sector, ft, ptype, am))
		return err
	}
}

// SectorsUnsealPiece will Unseal the Sealed sector file for the given sector.
// It will schedule the Unsealing task on a worker that either already has the sealed sector files or has space in
// one of it's sealing scratch spaces to store them after fetching them from another worker.
// If the chosen worker already has the Unsealed sector file, we will NOT Unseal the sealed sector file again.
func (m *Manager) SectorsUnsealPiece(ctx context.Context, sector storage.SectorRef, offset storiface.UnpaddedByteIndex, size abi.UnpaddedPieceSize, ticket abi.SealRandomness, unsealed *cid.Cid) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	log.Debugf("acquire unseal sector lock for sector %d", sector.ID)
	if err := m.index.StorageLock(ctx, sector.ID, storiface.FTSealed|storiface.FTCache, storiface.FTUnsealed); err != nil {
		return xerrors.Errorf("acquiring unseal sector lock: %w", err)
	}

	// if the selected worker does NOT have the sealed files for the sector, instruct it to fetch it from a worker that has them and
	// put it in the sealing scratch space.
	sealFetch := func(ctx context.Context, worker Worker) error {
		log.Debugf("copy sealed/cache sector data for sector %d", sector.ID)
		if _, err := m.waitSimpleCall(ctx)(worker.Fetch(ctx, sector, storiface.FTSealed|storiface.FTCache, storiface.PathSealing, storiface.AcquireCopy)); err != nil {
			return xerrors.Errorf("copy sealed/cache sector data: %w", err)
		}

		return nil
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
	selector := newExistingSelector(m.index, sector.ID, storiface.FTSealed|storiface.FTCache, true)

	log.Debugf("will schedule unseal for sector %d", sector.ID)
	err = m.sched.Schedule(ctx, sector, sealtasks.TTUnseal, selector, sealFetch, func(ctx context.Context, w Worker) error {
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

	return nil
}

func (m *Manager) NewSector(ctx context.Context, sector storage.SectorRef) error {
	log.Warnf("stub NewSector")
	return nil
}

func (m *Manager) AddPiece(ctx context.Context, sector storage.SectorRef, existingPieces []abi.UnpaddedPieceSize, sz abi.UnpaddedPieceSize, r io.Reader) (abi.PieceInfo, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	if err := m.index.StorageLock(ctx, sector.ID, storiface.FTNone, storiface.FTUnsealed); err != nil {
		return abi.PieceInfo{}, xerrors.Errorf("acquiring sector lock: %w", err)
	}

	var selector WorkerSelector
	var err error
	if len(existingPieces) == 0 { // new
		selector = newAllocSelector(m.index, storiface.FTUnsealed, storiface.PathSealing)
	} else { // use existing
		selector = newExistingSelector(m.index, sector.ID, storiface.FTUnsealed, false)
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

func (m *Manager) SealPreCommit1(ctx context.Context, sector storage.SectorRef, ticket abi.SealRandomness, pieces []abi.PieceInfo) (out storage.PreCommit1Out, err error) {
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
			out = p.(storage.PreCommit1Out)
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

	selector := newAllocSelector(m.index, storiface.FTCache|storiface.FTSealed, storiface.PathSealing)

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

func (m *Manager) SealPreCommit2(ctx context.Context, sector storage.SectorRef, phase1Out storage.PreCommit1Out) (out storage.SectorCids, err error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	wk, wait, cancel, err := m.getWork(ctx, sealtasks.TTPreCommit2, sector, phase1Out)
	if err != nil {
		return storage.SectorCids{}, xerrors.Errorf("getWork: %w", err)
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
			out = p.(storage.SectorCids)
		}
	}

	if wait { // already in progress
		waitRes()
		return out, waitErr
	}

	if err := m.index.StorageLock(ctx, sector.ID, storiface.FTSealed, storiface.FTCache); err != nil {
		return storage.SectorCids{}, xerrors.Errorf("acquiring sector lock: %w", err)
	}

	selector := newExistingSelector(m.index, sector.ID, storiface.FTCache|storiface.FTSealed, true)

	err = m.sched.Schedule(ctx, sector, sealtasks.TTPreCommit2, selector, m.schedFetch(sector, storiface.FTCache|storiface.FTSealed, storiface.PathSealing, storiface.AcquireMove), func(ctx context.Context, w Worker) error {
		err := m.startWork(ctx, w, wk)(w.SealPreCommit2(ctx, sector, phase1Out))
		if err != nil {
			return err
		}

		waitRes()
		return nil
	})
	if err != nil {
		return storage.SectorCids{}, err
	}

	return out, waitErr
}

func (m *Manager) SealCommit1(ctx context.Context, sector storage.SectorRef, ticket abi.SealRandomness, seed abi.InteractiveSealRandomness, pieces []abi.PieceInfo, cids storage.SectorCids) (out storage.Commit1Out, err error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	wk, wait, cancel, err := m.getWork(ctx, sealtasks.TTCommit1, sector, ticket, seed, pieces, cids)
	if err != nil {
		return storage.Commit1Out{}, xerrors.Errorf("getWork: %w", err)
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
			out = p.(storage.Commit1Out)
		}
	}

	if wait { // already in progress
		waitRes()
		return out, waitErr
	}

	if err := m.index.StorageLock(ctx, sector.ID, storiface.FTSealed, storiface.FTCache); err != nil {
		return storage.Commit1Out{}, xerrors.Errorf("acquiring sector lock: %w", err)
	}

	// NOTE: We set allowFetch to false in so that we always execute on a worker
	// with direct access to the data. We want to do that because this step is
	// generally very cheap / fast, and transferring data is not worth the effort
	selector := newExistingSelector(m.index, sector.ID, storiface.FTCache|storiface.FTSealed, false)

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

func (m *Manager) SealCommit2(ctx context.Context, sector storage.SectorRef, phase1Out storage.Commit1Out) (out storage.Proof, err error) {
	wk, wait, cancel, err := m.getWork(ctx, sealtasks.TTCommit2, sector, phase1Out)
	if err != nil {
		return storage.Proof{}, xerrors.Errorf("getWork: %w", err)
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
			out = p.(storage.Proof)
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

func (m *Manager) FinalizeSector(ctx context.Context, sector storage.SectorRef, keepUnsealed []storage.Range) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	if err := m.index.StorageLock(ctx, sector.ID, storiface.FTNone, storiface.FTSealed|storiface.FTUnsealed|storiface.FTCache); err != nil {
		return xerrors.Errorf("acquiring sector lock: %w", err)
	}

	unsealed := storiface.FTUnsealed
	{
		unsealedStores, err := m.index.StorageFindSector(ctx, sector.ID, storiface.FTUnsealed, 0, false)
		if err != nil {
			return xerrors.Errorf("finding unsealed sector: %w", err)
		}

		if len(unsealedStores) == 0 { // Is some edge-cases unsealed sector may not exist already, that's fine
			unsealed = storiface.FTNone
		}
	}

	pathType := storiface.PathStorage
	{
		sealedStores, err := m.index.StorageFindSector(ctx, sector.ID, storiface.FTSealed, 0, false)
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

	selector := newExistingSelector(m.index, sector.ID, storiface.FTCache|storiface.FTSealed, false)

	err := m.sched.Schedule(ctx, sector, sealtasks.TTFinalize, selector,
		m.schedFetch(sector, storiface.FTCache|storiface.FTSealed|unsealed, pathType, storiface.AcquireMove),
		func(ctx context.Context, w Worker) error {
			_, err := m.waitSimpleCall(ctx)(w.FinalizeSector(ctx, sector, keepUnsealed))
			return err
		})
	if err != nil {
		return err
	}

	fetchSel := newAllocSelector(m.index, storiface.FTCache|storiface.FTSealed, storiface.PathStorage)
	moveUnsealed := unsealed
	{
		if len(keepUnsealed) == 0 {
			moveUnsealed = storiface.FTNone
		}
	}

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

func (m *Manager) FinalizeReplicaUpdate(ctx context.Context, sector storage.SectorRef, keepUnsealed []storage.Range) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	if err := m.index.StorageLock(ctx, sector.ID, storiface.FTNone, storiface.FTSealed|storiface.FTUnsealed|storiface.FTCache|storiface.FTUpdate|storiface.FTUpdateCache); err != nil {
		return xerrors.Errorf("acquiring sector lock: %w", err)
	}

	fts := storiface.FTUnsealed
	{
		unsealedStores, err := m.index.StorageFindSector(ctx, sector.ID, storiface.FTUnsealed, 0, false)
		if err != nil {
			return xerrors.Errorf("finding unsealed sector: %w", err)
		}

		if len(unsealedStores) == 0 { // Is some edge-cases unsealed sector may not exist already, that's fine
			fts = storiface.FTNone
		}
	}

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

	selector := newExistingSelector(m.index, sector.ID, storiface.FTCache|storiface.FTSealed|storiface.FTUpdate|storiface.FTUpdateCache, false)

	err := m.sched.Schedule(ctx, sector, sealtasks.TTFinalizeReplicaUpdate, selector,
		m.schedFetch(sector, storiface.FTCache|storiface.FTSealed|storiface.FTUpdate|storiface.FTUpdateCache|fts, pathType, storiface.AcquireMove),
		func(ctx context.Context, w Worker) error {
			_, err := m.waitSimpleCall(ctx)(w.FinalizeReplicaUpdate(ctx, sector, keepUnsealed))
			return err
		})
	if err != nil {
		return err
	}

	fetchSel := newAllocSelector(m.index, storiface.FTCache|storiface.FTSealed|storiface.FTUpdate|storiface.FTUpdateCache, storiface.PathStorage)
	moveUnsealed := fts
	{
		if len(keepUnsealed) == 0 {
			moveUnsealed = storiface.FTNone
		}
	}

	err = m.sched.Schedule(ctx, sector, sealtasks.TTFetch, fetchSel,
		m.schedFetch(sector, storiface.FTCache|storiface.FTSealed|storiface.FTUpdate|storiface.FTUpdateCache|moveUnsealed, storiface.PathStorage, storiface.AcquireMove),
		func(ctx context.Context, w Worker) error {
			_, err := m.waitSimpleCall(ctx)(w.MoveStorage(ctx, sector, storiface.FTCache|storiface.FTSealed|storiface.FTUpdate|storiface.FTUpdateCache|moveUnsealed))
			return err
		})
	if err != nil {
		return xerrors.Errorf("moving sector to storage: %w", err)
	}

	return nil
}

func (m *Manager) ReleaseUnsealed(ctx context.Context, sector storage.SectorRef, safeToFree []storage.Range) error {
	return nil
}

func (m *Manager) ReleaseSectorKey(ctx context.Context, sector storage.SectorRef) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	if err := m.index.StorageLock(ctx, sector.ID, storiface.FTNone, storiface.FTSealed); err != nil {
		return xerrors.Errorf("acquiring sector lock: %w", err)
	}

	return m.storage.Remove(ctx, sector.ID, storiface.FTSealed, true, nil)
}

func (m *Manager) ReleaseReplicaUpgrade(ctx context.Context, sector storage.SectorRef) error {
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

func (m *Manager) GenerateSectorKeyFromData(ctx context.Context, sector storage.SectorRef, commD cid.Cid) error {

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
	selector := newExistingSelector(m.index, sector.ID, storiface.FTUnsealed|storiface.FTUpdate|storiface.FTUpdateCache|storiface.FTCache, true)

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

func (m *Manager) Remove(ctx context.Context, sector storage.SectorRef) error {
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

func (m *Manager) ReplicaUpdate(ctx context.Context, sector storage.SectorRef, pieces []abi.PieceInfo) (out storage.ReplicaUpdateOut, err error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	log.Errorf("manager is doing replica update")
	wk, wait, cancel, err := m.getWork(ctx, sealtasks.TTReplicaUpdate, sector, pieces)
	if err != nil {
		return storage.ReplicaUpdateOut{}, xerrors.Errorf("getWork: %w", err)
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
			out = p.(storage.ReplicaUpdateOut)
		}
	}

	if wait { // already in progress
		waitRes()
		return out, waitErr
	}

	if err := m.index.StorageLock(ctx, sector.ID, storiface.FTUnsealed|storiface.FTSealed|storiface.FTCache, storiface.FTUpdate|storiface.FTUpdateCache); err != nil {
		return storage.ReplicaUpdateOut{}, xerrors.Errorf("acquiring sector lock: %w", err)
	}

	selector := newAllocSelector(m.index, storiface.FTUpdate|storiface.FTUpdateCache, storiface.PathSealing)

	err = m.sched.Schedule(ctx, sector, sealtasks.TTReplicaUpdate, selector, m.schedFetch(sector, storiface.FTUnsealed|storiface.FTSealed|storiface.FTCache, storiface.PathSealing, storiface.AcquireCopy), func(ctx context.Context, w Worker) error {
		err := m.startWork(ctx, w, wk)(w.ReplicaUpdate(ctx, sector, pieces))
		if err != nil {
			return xerrors.Errorf("startWork: %w", err)
		}

		waitRes()
		return nil
	})
	if err != nil {
		return storage.ReplicaUpdateOut{}, xerrors.Errorf("Schedule: %w", err)
	}
	return out, waitErr
}

func (m *Manager) ProveReplicaUpdate1(ctx context.Context, sector storage.SectorRef, sectorKey, newSealed, newUnsealed cid.Cid) (out storage.ReplicaVanillaProofs, err error) {
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
			out = p.(storage.ReplicaVanillaProofs)
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
	selector := newExistingSelector(m.index, sector.ID, storiface.FTUpdate|storiface.FTUpdateCache|storiface.FTSealed|storiface.FTCache, false)

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

func (m *Manager) ProveReplicaUpdate2(ctx context.Context, sector storage.SectorRef, sectorKey, newSealed, newUnsealed cid.Cid, vanillaProofs storage.ReplicaVanillaProofs) (out storage.ReplicaUpdateProof, err error) {
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
			out = p.(storage.ReplicaUpdateProof)
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

func (m *Manager) ReturnAddPiece(ctx context.Context, callID storiface.CallID, pi abi.PieceInfo, err *storiface.CallError) error {
	return m.returnResult(ctx, callID, pi, err)
}

func (m *Manager) ReturnSealPreCommit1(ctx context.Context, callID storiface.CallID, p1o storage.PreCommit1Out, err *storiface.CallError) error {
	return m.returnResult(ctx, callID, p1o, err)
}

func (m *Manager) ReturnSealPreCommit2(ctx context.Context, callID storiface.CallID, sealed storage.SectorCids, err *storiface.CallError) error {
	return m.returnResult(ctx, callID, sealed, err)
}

func (m *Manager) ReturnSealCommit1(ctx context.Context, callID storiface.CallID, out storage.Commit1Out, err *storiface.CallError) error {
	return m.returnResult(ctx, callID, out, err)
}

func (m *Manager) ReturnSealCommit2(ctx context.Context, callID storiface.CallID, proof storage.Proof, err *storiface.CallError) error {
	return m.returnResult(ctx, callID, proof, err)
}

func (m *Manager) ReturnFinalizeSector(ctx context.Context, callID storiface.CallID, err *storiface.CallError) error {
	return m.returnResult(ctx, callID, nil, err)
}

func (m *Manager) ReturnReleaseUnsealed(ctx context.Context, callID storiface.CallID, err *storiface.CallError) error {
	return m.returnResult(ctx, callID, nil, err)
}

func (m *Manager) ReturnReplicaUpdate(ctx context.Context, callID storiface.CallID, out storage.ReplicaUpdateOut, err *storiface.CallError) error {
	return m.returnResult(ctx, callID, out, err)
}

func (m *Manager) ReturnProveReplicaUpdate1(ctx context.Context, callID storiface.CallID, out storage.ReplicaVanillaProofs, err *storiface.CallError) error {
	return m.returnResult(ctx, callID, out, err)
}

func (m *Manager) ReturnProveReplicaUpdate2(ctx context.Context, callID storiface.CallID, proof storage.ReplicaUpdateProof, err *storiface.CallError) error {
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

func (m *Manager) ReturnFetch(ctx context.Context, callID storiface.CallID, err *storiface.CallError) error {
	return m.returnResult(ctx, callID, nil, err)
}

func (m *Manager) StorageLocal(ctx context.Context) (map[stores.ID]string, error) {
	l, err := m.localStore.Local(ctx)
	if err != nil {
		return nil, err
	}

	out := map[stores.ID]string{}
	for _, st := range l {
		out[st.ID] = st.LocalPath
	}

	return out, nil
}

func (m *Manager) FsStat(ctx context.Context, id stores.ID) (fsutil.FsStat, error) {
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

func (m *Manager) Close(ctx context.Context) error {
	return m.sched.Close(ctx)
}

var _ Unsealer = &Manager{}
var _ SectorManager = &Manager{}
