package sectorstorage

import (
	"container/list"
	"context"
	"io"
	"net/http"
	"sync"

	"github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log/v2"
	"github.com/mitchellh/go-homedir"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-sectorbuilder"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-storage/storage"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/node/config"
	"github.com/filecoin-project/lotus/storage/sectorstorage/sealtasks"
	"github.com/filecoin-project/lotus/storage/sectorstorage/stores"
)

var log = logging.Logger("advmgr")

type URLs []string

type Worker interface {
	sectorbuilder.Sealer

	TaskTypes(context.Context) (map[sealtasks.TaskType]struct{}, error)

	// Returns paths accessible to the worker
	Paths(context.Context) ([]stores.StoragePath, error)

	Info(context.Context) (api.WorkerInfo, error)
}

type SectorManager interface {
	SectorSize() abi.SectorSize

	ReadPieceFromSealedSector(context.Context, abi.SectorID, sectorbuilder.UnpaddedByteIndex, abi.UnpaddedPieceSize, abi.SealRandomness, cid.Cid) (io.ReadCloser, error)

	sectorbuilder.Sealer
	storage.Prover
}

type WorkerID uint64

type Manager struct {
	scfg *sectorbuilder.Config

	ls         stores.LocalStorage
	storage    *stores.Remote
	localStore *stores.Local
	remoteHnd  *stores.FetchHandler
	index      stores.SectorIndex

	storage.Prover

	workersLk  sync.Mutex
	nextWorker WorkerID
	workers    map[WorkerID]*workerHandle

	newWorkers chan *workerHandle
	schedule   chan *workerRequest
	workerFree chan WorkerID
	closing    chan struct{}

	schedQueue *list.List // List[*workerRequest]
}

func New(ls stores.LocalStorage, si stores.SectorIndex, cfg *sectorbuilder.Config, urls URLs, ca api.Common) (*Manager, error) {
	ctx := context.TODO()

	lstor, err := stores.NewLocal(ctx, ls, si, urls)
	if err != nil {
		return nil, err
	}

	prover, err := sectorbuilder.New(&readonlyProvider{stor: lstor}, cfg)
	if err != nil {
		return nil, xerrors.Errorf("creating prover instance: %w", err)
	}

	token, err := ca.AuthNew(context.TODO(), []api.Permission{"admin"})
	headers := http.Header{}
	headers.Add("Authorization", "Bearer "+string(token))
	stor := stores.NewRemote(lstor, si, headers)

	m := &Manager{
		scfg: cfg,

		ls:         ls,
		storage:    stor,
		localStore: lstor,
		remoteHnd:  &stores.FetchHandler{Local: lstor},
		index:      si,

		nextWorker: 0,
		workers:    map[WorkerID]*workerHandle{},

		newWorkers: make(chan *workerHandle),
		schedule:   make(chan *workerRequest),
		workerFree: make(chan WorkerID),
		closing:    make(chan struct{}),

		schedQueue: list.New(),

		Prover: prover,
	}

	go m.runSched()

	err = m.AddWorker(ctx, NewLocalWorker(WorkerConfig{
		SealProof: cfg.SealProofType,
		TaskTypes: []sealtasks.TaskType{
			sealtasks.TTAddPiece, sealtasks.TTCommit1, sealtasks.TTFinalize,
			sealtasks.TTPreCommit1, sealtasks.TTPreCommit2, sealtasks.TTCommit2, // TODO: Config
		},
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

	if err := m.ls.SetStorage(func(sc *config.StorageConfig) {
		sc.StoragePaths = append(sc.StoragePaths, config.LocalPath{Path: path})
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

	m.newWorkers <- &workerHandle{
		w:    w,
		info: info,
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

func (m *Manager) ReadPieceFromSealedSector(context.Context, abi.SectorID, sectorbuilder.UnpaddedByteIndex, abi.UnpaddedPieceSize, abi.SealRandomness, cid.Cid) (io.ReadCloser, error) {
	panic("implement me")
}

func (m *Manager) getWorkersByPaths(task sealtasks.TaskType, inPaths []stores.StorageInfo) ([]WorkerID, map[WorkerID]stores.StorageInfo) {
	m.workersLk.Lock()
	defer m.workersLk.Unlock()

	var workers []WorkerID
	paths := map[WorkerID]stores.StorageInfo{}

	for i, worker := range m.workers {
		tt, err := worker.w.TaskTypes(context.TODO())
		if err != nil {
			log.Errorf("error getting supported worker task types: %+v", err)
			continue
		}
		if _, ok := tt[task]; !ok {
			log.Debugf("dropping worker %d; task %s not supported (supports %v)", i, task, tt)
			continue
		}

		phs, err := worker.w.Paths(context.TODO())
		if err != nil {
			log.Errorf("error getting worker paths: %+v", err)
			continue
		}

		// check if the worker has access to the path we selected
		var st *stores.StorageInfo
		for _, p := range phs {
			for _, meta := range inPaths {
				if p.ID == meta.ID {
					if st != nil && st.Weight > p.Weight {
						continue
					}

					p := meta // copy
					st = &p
				}
			}
		}
		if st == nil {
			log.Debugf("skipping worker %d; doesn't have any of %v", i, inPaths)
			log.Debugf("skipping worker %d; only has %v", i, phs)
			continue
		}

		paths[i] = *st
		workers = append(workers, i)
	}

	return workers, paths
}

func (m *Manager) getWorker(ctx context.Context, taskType sealtasks.TaskType, accept []WorkerID) (Worker, func(), error) {
	ret := make(chan workerResponse)

	select {
	case m.schedule <- &workerRequest{
		taskType: taskType,
		accept:   accept,

		cancel: ctx.Done(),
		ret:    ret,
	}:
	case <-m.closing:
		return nil, nil, xerrors.New("closing")
	case <-ctx.Done():
		return nil, nil, ctx.Err()
	}

	select {
	case resp := <-ret:
		return resp.worker, resp.done, resp.err
	case <-m.closing:
		return nil, nil, xerrors.New("closing")
	case <-ctx.Done():
		return nil, nil, ctx.Err()
	}
}

func (m *Manager) NewSector(ctx context.Context, sector abi.SectorID) error {
	log.Warnf("stub NewSector")
	return nil
}

func (m *Manager) AddPiece(ctx context.Context, sector abi.SectorID, existingPieces []abi.UnpaddedPieceSize, sz abi.UnpaddedPieceSize, r io.Reader) (abi.PieceInfo, error) {
	// TODO: consider multiple paths vs workers when initially allocating

	var best []stores.StorageInfo
	var err error
	if len(existingPieces) == 0 { // new
		best, err = m.index.StorageBestAlloc(ctx, sectorbuilder.FTUnsealed, true)
	} else { // append to existing
		best, err = m.index.StorageFindSector(ctx, sector, sectorbuilder.FTUnsealed, false)
	}
	if err != nil {
		return abi.PieceInfo{}, xerrors.Errorf("finding sector path: %w", err)
	}

	log.Debugf("find workers for %v", best)
	candidateWorkers, _ := m.getWorkersByPaths(sealtasks.TTAddPiece, best)

	if len(candidateWorkers) == 0 {
		return abi.PieceInfo{}, xerrors.New("no worker found")
	}

	worker, done, err := m.getWorker(ctx, sealtasks.TTAddPiece, candidateWorkers)
	if err != nil {
		return abi.PieceInfo{}, xerrors.Errorf("scheduling worker: %w", err)
	}
	defer done()

	// TODO: select(candidateWorkers, ...)
	// TODO: remove the sectorbuilder abstraction, pass path directly
	return worker.AddPiece(ctx, sector, existingPieces, sz, r)
}

func (m *Manager) SealPreCommit1(ctx context.Context, sector abi.SectorID, ticket abi.SealRandomness, pieces []abi.PieceInfo) (out storage.PreCommit1Out, err error) {
	// TODO: also consider where the unsealed data sits

	best, err := m.index.StorageBestAlloc(ctx, sectorbuilder.FTCache|sectorbuilder.FTSealed, true)
	if err != nil {
		return nil, xerrors.Errorf("finding path for sector sealing: %w", err)
	}

	candidateWorkers, _ := m.getWorkersByPaths(sealtasks.TTPreCommit1, best)
	if len(candidateWorkers) == 0 {
		return nil, xerrors.New("no suitable workers found")
	}

	worker, done, err := m.getWorker(ctx, sealtasks.TTPreCommit1, candidateWorkers)
	if err != nil {
		return nil, xerrors.Errorf("scheduling worker: %w", err)
	}
	defer done()

	// TODO: select(candidateWorkers, ...)
	// TODO: remove the sectorbuilder abstraction, pass path directly
	return worker.SealPreCommit1(ctx, sector, ticket, pieces)
}

func (m *Manager) SealPreCommit2(ctx context.Context, sector abi.SectorID, phase1Out storage.PreCommit1Out) (cids storage.SectorCids, err error) {
	// TODO: allow workers to fetch the sectors

	best, err := m.index.StorageFindSector(ctx, sector, sectorbuilder.FTCache|sectorbuilder.FTSealed, true)
	if err != nil {
		return storage.SectorCids{}, xerrors.Errorf("finding path for sector sealing: %w", err)
	}

	candidateWorkers, _ := m.getWorkersByPaths(sealtasks.TTPreCommit2, best)
	if len(candidateWorkers) == 0 {
		return storage.SectorCids{}, xerrors.New("no suitable workers found")
	}

	worker, done, err := m.getWorker(ctx, sealtasks.TTPreCommit2, candidateWorkers)
	if err != nil {
		return storage.SectorCids{}, xerrors.Errorf("scheduling worker: %w", err)
	}
	defer done()

	// TODO: select(candidateWorkers, ...)
	// TODO: remove the sectorbuilder abstraction, pass path directly
	return worker.SealPreCommit2(ctx, sector, phase1Out)
}

func (m *Manager) SealCommit1(ctx context.Context, sector abi.SectorID, ticket abi.SealRandomness, seed abi.InteractiveSealRandomness, pieces []abi.PieceInfo, cids storage.SectorCids) (output storage.Commit1Out, err error) {
	best, err := m.index.StorageFindSector(ctx, sector, sectorbuilder.FTCache|sectorbuilder.FTSealed, true)
	if err != nil {
		return nil, xerrors.Errorf("finding path for sector sealing: %w", err)
	}

	candidateWorkers, _ := m.getWorkersByPaths(sealtasks.TTCommit1, best)
	if len(candidateWorkers) == 0 {
		return nil, xerrors.New("no suitable workers found") // TODO: wait?
	}

	// TODO: Try very hard to execute on worker with access to the sectors
	worker, done, err := m.getWorker(ctx, sealtasks.TTCommit1, candidateWorkers)
	if err != nil {
		return nil, xerrors.Errorf("scheduling worker: %w", err)
	}
	defer done()

	// TODO: select(candidateWorkers, ...)
	// TODO: remove the sectorbuilder abstraction, pass path directly
	return worker.SealCommit1(ctx, sector, ticket, seed, pieces, cids)
}

func (m *Manager) SealCommit2(ctx context.Context, sector abi.SectorID, phase1Out storage.Commit1Out) (proof storage.Proof, err error) {
	var candidateWorkers []WorkerID

	m.workersLk.Lock()
	for id, worker := range m.workers {
		tt, err := worker.w.TaskTypes(ctx)
		if err != nil {
			log.Errorf("error getting supported worker task types: %+v", err)
			continue
		}
		if _, ok := tt[sealtasks.TTCommit2]; !ok {
			continue
		}
		candidateWorkers = append(candidateWorkers, id)
	}
	m.workersLk.Unlock()

	worker, done, err := m.getWorker(ctx, sealtasks.TTCommit2, candidateWorkers)
	if err != nil {
		return nil, xerrors.Errorf("scheduling worker: %w", err)
	}
	defer done()

	return worker.SealCommit2(ctx, sector, phase1Out)
}

func (m *Manager) FinalizeSector(ctx context.Context, sector abi.SectorID) error {
	best, err := m.index.StorageFindSector(ctx, sector, sectorbuilder.FTCache|sectorbuilder.FTSealed|sectorbuilder.FTUnsealed, true)
	if err != nil {
		return xerrors.Errorf("finding sealed sector: %w", err)
	}

	candidateWorkers, _ := m.getWorkersByPaths(sealtasks.TTFinalize, best)

	// TODO: Remove sector from sealing stores
	// TODO: Move the sector to long-term storage
	return m.workers[candidateWorkers[0]].w.FinalizeSector(ctx, sector)
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

var _ SectorManager = &Manager{}
