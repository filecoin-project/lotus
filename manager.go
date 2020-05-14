package sectorstorage

import (
	"context"
	"errors"
	"io"
	"net/http"

	"github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log/v2"
	"github.com/mitchellh/go-homedir"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-storage/storage"

	"github.com/filecoin-project/sector-storage/ffiwrapper"
	"github.com/filecoin-project/sector-storage/sealtasks"
	"github.com/filecoin-project/sector-storage/stores"
	"github.com/filecoin-project/sector-storage/storiface"
)

var log = logging.Logger("advmgr")

var ErrNoWorkers = errors.New("no suitable workers found")

type URLs []string

type Worker interface {
	ffiwrapper.StorageSealer

	Fetch(context.Context, abi.SectorID, stores.SectorFileType, bool) error
	UnsealPiece(context.Context, abi.SectorID, ffiwrapper.UnpaddedByteIndex, abi.UnpaddedPieceSize, abi.SealRandomness, cid.Cid) error
	ReadPiece(context.Context, io.Writer, abi.SectorID, ffiwrapper.UnpaddedByteIndex, abi.UnpaddedPieceSize) error

	TaskTypes(context.Context) (map[sealtasks.TaskType]struct{}, error)

	// Returns paths accessible to the worker
	Paths(context.Context) ([]stores.StoragePath, error)

	Info(context.Context) (storiface.WorkerInfo, error)

	// returns channel signalling worker shutdown
	Closing(context.Context) (<-chan struct{}, error)

	Close() error
}

type SectorManager interface {
	SectorSize() abi.SectorSize

	ReadPiece(context.Context, io.Writer, abi.SectorID, ffiwrapper.UnpaddedByteIndex, abi.UnpaddedPieceSize, abi.SealRandomness, cid.Cid) error

	ffiwrapper.StorageSealer
	storage.Prover
}

type WorkerID uint64

type Manager struct {
	scfg *ffiwrapper.Config

	ls         stores.LocalStorage
	storage    *stores.Remote
	localStore *stores.Local
	remoteHnd  *stores.FetchHandler
	index      stores.SectorIndex

	sched *scheduler

	storage.Prover
}

type SealerConfig struct {
	// Local worker config
	AllowPreCommit1 bool
	AllowPreCommit2 bool
	AllowCommit     bool
	AllowUnseal     bool
}

type StorageAuth http.Header

func New(ctx context.Context, ls stores.LocalStorage, si stores.SectorIndex, cfg *ffiwrapper.Config, sc SealerConfig, urls URLs, sa StorageAuth) (*Manager, error) {
	lstor, err := stores.NewLocal(ctx, ls, si, urls)
	if err != nil {
		return nil, err
	}

	prover, err := ffiwrapper.New(&readonlyProvider{stor: lstor}, cfg)
	if err != nil {
		return nil, xerrors.Errorf("creating prover instance: %w", err)
	}

	stor := stores.NewRemote(lstor, si, http.Header(sa))

	m := &Manager{
		scfg: cfg,

		ls:         ls,
		storage:    stor,
		localStore: lstor,
		remoteHnd:  &stores.FetchHandler{Local: lstor},
		index:      si,

		sched: newScheduler(cfg.SealProofType),

		Prover: prover,
	}

	go m.sched.runSched()

	localTasks := []sealtasks.TaskType{
		sealtasks.TTAddPiece, sealtasks.TTCommit1, sealtasks.TTFinalize, sealtasks.TTFetch, sealtasks.TTReadUnsealed,
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

	err = m.AddWorker(ctx, NewLocalWorker(WorkerConfig{
		SealProof: cfg.SealProofType,
		TaskTypes: localTasks,
	}, stor, lstor, si))
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
	info, err := w.Info(ctx)
	if err != nil {
		return xerrors.Errorf("getting worker info: %w", err)
	}

	m.sched.newWorkers <- &workerHandle{
		w:         w,
		info:      info,
		preparing: &activeResources{},
		active:    &activeResources{},
	}
	return nil
}

func (m *Manager) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	m.remoteHnd.ServeHTTP(w, r)
}

func (m *Manager) SectorSize() abi.SectorSize {
	sz, _ := m.scfg.SealProofType.SectorSize()
	return sz
}

func schedNop(context.Context, Worker) error {
	return nil
}

func schedFetch(sector abi.SectorID, ft stores.SectorFileType, sealing bool) func(context.Context, Worker) error {
	return func(ctx context.Context, worker Worker) error {
		return worker.Fetch(ctx, sector, ft, sealing)
	}
}

func (m *Manager) ReadPiece(ctx context.Context, sink io.Writer, sector abi.SectorID, offset ffiwrapper.UnpaddedByteIndex, size abi.UnpaddedPieceSize, ticket abi.SealRandomness, unsealed cid.Cid) error {
	best, err := m.index.StorageFindSector(ctx, sector, stores.FTUnsealed, false)
	if err != nil {
		return xerrors.Errorf("read piece: checking for already existing unsealed sector: %w", err)
	}

	var selector WorkerSelector
	if len(best) == 0 { // new
		selector, err = newAllocSelector(ctx, m.index, stores.FTUnsealed)
	} else { // append to existing
		selector, err = newExistingSelector(ctx, m.index, sector, stores.FTUnsealed, false)
	}
	if err != nil {
		return xerrors.Errorf("creating unsealPiece selector: %w", err)
	}

	// TODO: Optimization: don't send unseal to a worker if the requested range is already unsealed

	// TODO!!!! make schedFetch COPY stores.FTSealed and stores.FTCache
	//  Moving those to a temp sealing storage may make PoSts fail

	err = m.sched.Schedule(ctx, sealtasks.TTUnseal, selector, schedFetch(sector, stores.FTUnsealed|stores.FTSealed|stores.FTCache, true), func(ctx context.Context, w Worker) error {
		return w.UnsealPiece(ctx, sector, offset, size, ticket, unsealed)
	})
	if err != nil {
		return err
	}

	selector, err = newExistingSelector(ctx, m.index, sector, stores.FTUnsealed, false)
	if err != nil {
		return xerrors.Errorf("creating readPiece selector: %w", err)
	}

	err = m.sched.Schedule(ctx, sealtasks.TTReadUnsealed, selector, schedFetch(sector, stores.FTUnsealed, true), func(ctx context.Context, w Worker) error {
		return w.ReadPiece(ctx, sink, sector, offset, size)
	})
	if err != nil {
		return xerrors.Errorf("reading piece from sealed sector: %w", err)
	}

	return nil
}

func (m *Manager) NewSector(ctx context.Context, sector abi.SectorID) error {
	log.Warnf("stub NewSector")
	return nil
}

func (m *Manager) AddPiece(ctx context.Context, sector abi.SectorID, existingPieces []abi.UnpaddedPieceSize, sz abi.UnpaddedPieceSize, r io.Reader) (abi.PieceInfo, error) {
	var selector WorkerSelector
	var err error
	if len(existingPieces) == 0 { // new
		selector, err = newAllocSelector(ctx, m.index, stores.FTUnsealed)
	} else { // use existing
		selector, err = newExistingSelector(ctx, m.index, sector, stores.FTUnsealed, false)
	}
	if err != nil {
		return abi.PieceInfo{}, xerrors.Errorf("creating path selector: %w", err)
	}

	var out abi.PieceInfo
	err = m.sched.Schedule(ctx, sealtasks.TTAddPiece, selector, schedNop, func(ctx context.Context, w Worker) error {
		p, err := w.AddPiece(ctx, sector, existingPieces, sz, r)
		if err != nil {
			return err
		}
		out = p
		return nil
	})

	return out, err
}

func (m *Manager) SealPreCommit1(ctx context.Context, sector abi.SectorID, ticket abi.SealRandomness, pieces []abi.PieceInfo) (out storage.PreCommit1Out, err error) {
	// TODO: also consider where the unsealed data sits

	selector, err := newAllocSelector(ctx, m.index, stores.FTCache|stores.FTSealed)
	if err != nil {
		return nil, xerrors.Errorf("creating path selector: %w", err)
	}

	err = m.sched.Schedule(ctx, sealtasks.TTPreCommit1, selector, schedFetch(sector, stores.FTUnsealed, true), func(ctx context.Context, w Worker) error {
		p, err := w.SealPreCommit1(ctx, sector, ticket, pieces)
		if err != nil {
			return err
		}
		out = p
		return nil
	})

	return out, err
}

func (m *Manager) SealPreCommit2(ctx context.Context, sector abi.SectorID, phase1Out storage.PreCommit1Out) (out storage.SectorCids, err error) {
	selector, err := newExistingSelector(ctx, m.index, sector, stores.FTCache|stores.FTSealed, true)
	if err != nil {
		return storage.SectorCids{}, xerrors.Errorf("creating path selector: %w", err)
	}

	err = m.sched.Schedule(ctx, sealtasks.TTPreCommit2, selector, schedFetch(sector, stores.FTCache|stores.FTSealed, true), func(ctx context.Context, w Worker) error {
		p, err := w.SealPreCommit2(ctx, sector, phase1Out)
		if err != nil {
			return err
		}
		out = p
		return nil
	})
	return out, err
}

func (m *Manager) SealCommit1(ctx context.Context, sector abi.SectorID, ticket abi.SealRandomness, seed abi.InteractiveSealRandomness, pieces []abi.PieceInfo, cids storage.SectorCids) (out storage.Commit1Out, err error) {
	selector, err := newExistingSelector(ctx, m.index, sector, stores.FTCache|stores.FTSealed, false)
	if err != nil {
		return storage.Commit1Out{}, xerrors.Errorf("creating path selector: %w", err)
	}

	// TODO: Try very hard to execute on worker with access to the sectors
	//  (except, don't.. for now at least - we are using this step to bring data
	//  into 'provable' storage. Optimally we'd do that in commit2, in parallel
	//  with snark compute)
	err = m.sched.Schedule(ctx, sealtasks.TTCommit1, selector, schedFetch(sector, stores.FTCache|stores.FTSealed, true), func(ctx context.Context, w Worker) error {
		p, err := w.SealCommit1(ctx, sector, ticket, seed, pieces, cids)
		if err != nil {
			return err
		}
		out = p
		return nil
	})
	return out, err
}

func (m *Manager) SealCommit2(ctx context.Context, sector abi.SectorID, phase1Out storage.Commit1Out) (out storage.Proof, err error) {
	selector := newTaskSelector()

	err = m.sched.Schedule(ctx, sealtasks.TTCommit2, selector, schedNop, func(ctx context.Context, w Worker) error {
		p, err := w.SealCommit2(ctx, sector, phase1Out)
		if err != nil {
			return err
		}
		out = p
		return nil
	})

	return out, err
}

func (m *Manager) FinalizeSector(ctx context.Context, sector abi.SectorID) error {
	selector, err := newExistingSelector(ctx, m.index, sector, stores.FTCache|stores.FTSealed|stores.FTUnsealed, false)
	if err != nil {
		return xerrors.Errorf("creating path selector: %w", err)
	}

	return m.sched.Schedule(ctx, sealtasks.TTFinalize, selector,
		schedFetch(sector, stores.FTCache|stores.FTSealed|stores.FTUnsealed, false),
		func(ctx context.Context, w Worker) error {
			return w.FinalizeSector(ctx, sector)
		})
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

func (m *Manager) FsStat(ctx context.Context, id stores.ID) (stores.FsStat, error) {
	return m.storage.FsStat(ctx, id)
}

func (m *Manager) Close() error {
	return m.sched.Close()
}

var _ SectorManager = &Manager{}
