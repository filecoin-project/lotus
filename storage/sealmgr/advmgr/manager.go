package advmgr

import (
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
	storage2 "github.com/filecoin-project/specs-storage/storage"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/node/config"
	"github.com/filecoin-project/lotus/storage/sealmgr"
	"github.com/filecoin-project/lotus/storage/sealmgr/stores"
)

var log = logging.Logger("advmgr")

type URLs []string

type Worker interface {
	sectorbuilder.Sealer

	TaskTypes(context.Context) (map[sealmgr.TaskType]struct{}, error)

	// Returns paths accessible to the worker
	Paths(context.Context) ([]stores.StoragePath, error)
}

type Manager struct {
	workers []Worker
	scfg    *sectorbuilder.Config

	ls         stores.LocalStorage
	storage    *stores.Remote
	localStore *stores.Local
	remoteHnd  *stores.FetchHandler
	index      stores.SectorIndex

	storage2.Prover

	lk sync.Mutex
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
		workers: []Worker{
			NewLocalWorker(WorkerConfig{
				SealProof: cfg.SealProofType,
				TaskTypes: []sealmgr.TaskType{sealmgr.TTAddPiece, sealmgr.TTCommit1, sealmgr.TTFinalize},
			}, stor, lstor, si),
		},
		scfg: cfg,

		ls:         ls,
		storage:    stor,
		localStore: lstor,
		remoteHnd:  &stores.FetchHandler{Store: lstor},
		index:      si,

		Prover: prover,
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

func (m *Manager) AddWorker(w Worker) error {
	m.lk.Lock()
	defer m.lk.Unlock()

	m.workers = append(m.workers, w)
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

func (m *Manager) getWorkersByPaths(task sealmgr.TaskType, inPaths []stores.StorageInfo) ([]Worker, map[int]stores.StorageInfo) {
	var workers []Worker
	paths := map[int]stores.StorageInfo{}

	for i, worker := range m.workers {
		tt, err := worker.TaskTypes(context.TODO())
		if err != nil {
			log.Errorf("error getting supported worker task types: %+v", err)
			continue
		}
		if _, ok := tt[task]; !ok {
			log.Debugf("dropping worker %d; task %s not supported (supports %v)", i, task, tt)
			continue
		}

		phs, err := worker.Paths(context.TODO())
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
		workers = append(workers, worker)
	}

	return workers, paths
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
	candidateWorkers, _ := m.getWorkersByPaths(sealmgr.TTAddPiece, best)

	if len(candidateWorkers) == 0 {
		return abi.PieceInfo{}, xerrors.New("no worker found")
	}

	// TODO: select(candidateWorkers, ...)
	// TODO: remove the sectorbuilder abstraction, pass path directly
	return candidateWorkers[0].AddPiece(ctx, sector, existingPieces, sz, r)
}

func (m *Manager) SealPreCommit1(ctx context.Context, sector abi.SectorID, ticket abi.SealRandomness, pieces []abi.PieceInfo) (out storage2.PreCommit1Out, err error) {
	// TODO: also consider where the unsealed data sits

	best, err := m.index.StorageBestAlloc(ctx, sectorbuilder.FTCache|sectorbuilder.FTSealed, true)
	if err != nil {
		return nil, xerrors.Errorf("finding path for sector sealing: %w", err)
	}

	candidateWorkers, _ := m.getWorkersByPaths(sealmgr.TTPreCommit1, best)
	if len(candidateWorkers) == 0 {
		return nil, xerrors.New("no suitable workers found") // TODO: wait?
	}

	// TODO: select(candidateWorkers, ...)
	// TODO: remove the sectorbuilder abstraction, pass path directly
	return candidateWorkers[0].SealPreCommit1(ctx, sector, ticket, pieces)
}

func (m *Manager) SealPreCommit2(ctx context.Context, sector abi.SectorID, phase1Out storage2.PreCommit1Out) (cids storage2.SectorCids, err error) {
	// TODO: allow workers to fetch the sectors

	best, err := m.index.StorageFindSector(ctx, sector, sectorbuilder.FTCache|sectorbuilder.FTSealed, true)
	if err != nil {
		return storage2.SectorCids{}, xerrors.Errorf("finding path for sector sealing: %w", err)
	}

	candidateWorkers, _ := m.getWorkersByPaths(sealmgr.TTPreCommit2, best)
	if len(candidateWorkers) == 0 {
		return storage2.SectorCids{}, xerrors.New("no suitable workers found") // TODO: wait?
	}

	// TODO: select(candidateWorkers, ...)
	// TODO: remove the sectorbuilder abstraction, pass path directly
	return candidateWorkers[0].SealPreCommit2(ctx, sector, phase1Out)
}

func (m *Manager) SealCommit1(ctx context.Context, sector abi.SectorID, ticket abi.SealRandomness, seed abi.InteractiveSealRandomness, pieces []abi.PieceInfo, cids storage2.SectorCids) (output storage2.Commit1Out, err error) {
	best, err := m.index.StorageFindSector(ctx, sector, sectorbuilder.FTCache|sectorbuilder.FTSealed, true)
	if err != nil {
		return nil, xerrors.Errorf("finding path for sector sealing: %w", err)
	}

	candidateWorkers, _ := m.getWorkersByPaths(sealmgr.TTCommit1, best)
	if len(candidateWorkers) == 0 {
		return nil, xerrors.New("no suitable workers found") // TODO: wait?
	}

	// TODO: select(candidateWorkers, ...)
	// TODO: remove the sectorbuilder abstraction, pass path directly
	return candidateWorkers[0].SealCommit1(ctx, sector, ticket, seed, pieces, cids)
}

func (m *Manager) SealCommit2(ctx context.Context, sector abi.SectorID, phase1Out storage2.Commit1Out) (proof storage2.Proof, err error) {
	for _, worker := range m.workers {
		tt, err := worker.TaskTypes(context.TODO())
		if err != nil {
			log.Errorf("error getting supported worker task types: %+v", err)
			continue
		}
		if _, ok := tt[sealmgr.TTCommit2]; !ok {
			continue
		}

		return worker.SealCommit2(ctx, sector, phase1Out)
	}

	return nil, xerrors.New("no worker found")
}

func (m *Manager) FinalizeSector(ctx context.Context, sector abi.SectorID) error {
	best, err := m.index.StorageFindSector(ctx, sector, sectorbuilder.FTCache|sectorbuilder.FTSealed|sectorbuilder.FTUnsealed, true)
	if err != nil {
		return xerrors.Errorf("finding sealed sector: %w", err)
	}

	candidateWorkers, _ := m.getWorkersByPaths(sealmgr.TTCommit1, best)

	// TODO: Remove sector from sealing stores
	// TODO: Move the sector to long-term storage
	return candidateWorkers[0].FinalizeSector(ctx, sector)
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

var _ sealmgr.Manager = &Manager{}
