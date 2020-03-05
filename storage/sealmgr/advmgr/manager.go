package advmgr

import (
	"context"
	"io"

	"github.com/ipfs/go-cid"
	"github.com/mitchellh/go-homedir"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-sectorbuilder"
	"github.com/filecoin-project/specs-actors/actors/abi"

	"github.com/filecoin-project/lotus/node/config"
	"github.com/filecoin-project/lotus/storage/sealmgr"
)

type SectorIDCounter interface {
	Next() (abi.SectorNumber, error)
}

type LocalStorage interface {
	GetStorage() (config.StorageConfig, error)
	SetStorage(config.StorageConfig) error
}

type Path struct {
	ID     string
	Weight uint64

	LocalPath string

	CanSeal  bool
	CanStore bool
}

type Worker interface {
	sectorbuilder.Sealer

	TaskTypes() map[sealmgr.TaskType]struct{}
	Paths() []Path
}

type Manager struct {
	workers []Worker
	scfg    *sectorbuilder.Config
	sc      SectorIDCounter

	storage *storage

	sectorbuilder.Prover
}

func New(ls LocalStorage, cfg *sectorbuilder.Config, sc SectorIDCounter) (*Manager, error) {
	stor := &storage{
		localStorage: ls,
	}
	if err := stor.open(); err != nil {
		return nil, err
	}

	mid, err := address.IDFromAddress(cfg.Miner)
	if err != nil {
		return nil, xerrors.Errorf("getting miner id: %w", err)
	}

	prover, err := sectorbuilder.New(&readonlyProvider{stor: stor, miner: abi.ActorID(mid)}, cfg)
	if err != nil {
		return nil, xerrors.Errorf("creating prover instance: %w", err)
	}

	m := &Manager{
		workers: []Worker{
			&localWorker{scfg: cfg, storage: stor},
		},
		scfg: cfg,
		sc:   sc,

		storage: stor,

		Prover: prover,
	}

	return m, nil
}

func (m *Manager) AddLocalStorage(path string) error {
	path, err := homedir.Expand(path)
	if err != nil {
		return xerrors.Errorf("expanding local path: %w", err)
	}

	if err := m.storage.openPath(path); err != nil {
		return xerrors.Errorf("opening local path: %w", err)
	}

	sc, err := m.storage.localStorage.GetStorage()
	if err != nil {
		return xerrors.Errorf("get storage config: %w", err)
	}

	sc.StoragePaths = append(sc.StoragePaths, config.LocalPath{Path: path})
	return nil
}

func (m *Manager) SectorSize() abi.SectorSize {
	sz, _ := m.scfg.SealProofType.SectorSize()
	return sz
}

func (m *Manager) NewSector() (abi.SectorNumber, error) {
	return m.sc.Next()
}

func (m *Manager) ReadPieceFromSealedSector(context.Context, abi.SectorNumber, sectorbuilder.UnpaddedByteIndex, abi.UnpaddedPieceSize, abi.SealRandomness, cid.Cid) (io.ReadCloser, error) {
	panic("implement me")
}

func (m *Manager) getWorkersByPaths(task sealmgr.TaskType, inPaths []config.StorageMeta) ([]Worker, map[int]config.StorageMeta) {
	var workers []Worker
	paths := map[int]config.StorageMeta{}

	for i, worker := range m.workers {
		if _, ok := worker.TaskTypes()[task]; !ok {
			continue
		}

		// check if the worker has access to the path we selected
		var st *config.StorageMeta
		for _, p := range worker.Paths() {
			for _, m := range inPaths {
				if p.ID == m.ID {
					if st != nil && st.Weight > p.Weight {
						continue
					}

					p := m // copy
					st = &p
				}
			}
		}
		if st == nil {
			continue
		}

		paths[i] = *st
		workers = append(workers, worker)
	}

	return workers, paths
}

func (m *Manager) AddPiece(ctx context.Context, sz abi.UnpaddedPieceSize, sn abi.SectorNumber, r io.Reader, existingPieces []abi.UnpaddedPieceSize) (abi.PieceInfo, error) {
	// TODO: consider multiple paths vs workers when initially allocating

	var best []config.StorageMeta
	var err error
	if len(existingPieces) == 0 { // new
		best, err = m.storage.findBestAllocStorage(sectorbuilder.FTUnsealed, true)
	} else { // append to existing
		best, err = m.storage.findSector(m.minerID(), sn, sectorbuilder.FTUnsealed)
	}
	if err != nil {
		return abi.PieceInfo{}, xerrors.Errorf("finding sector path: %w", err)
	}

	candidateWorkers, _ := m.getWorkersByPaths(sealmgr.TTAddPiece, best)

	if len(candidateWorkers) == 0 {
		return abi.PieceInfo{}, xerrors.New("no worker found")
	}

	// TODO: select(candidateWorkers, ...)
	// TODO: remove the sectorbuilder abstraction, pass path directly
	return candidateWorkers[0].AddPiece(ctx, sz, sn, r, existingPieces)
}

func (m *Manager) SealPreCommit1(ctx context.Context, sectorNum abi.SectorNumber, ticket abi.SealRandomness, pieces []abi.PieceInfo) (out []byte, err error) {
	// TODO: also consider where the unsealed data sits

	best, err := m.storage.findBestAllocStorage(sectorbuilder.FTCache|sectorbuilder.FTSealed, true)
	if err != nil {
		return nil, xerrors.Errorf("finding path for sector sealing: %w", err)
	}

	candidateWorkers, _ := m.getWorkersByPaths(sealmgr.TTPreCommit1, best)

	// TODO: select(candidateWorkers, ...)
	// TODO: remove the sectorbuilder abstraction, pass path directly
	return candidateWorkers[0].SealPreCommit1(ctx, sectorNum, ticket, pieces)
}

func (m *Manager) SealPreCommit2(ctx context.Context, sectorNum abi.SectorNumber, phase1Out []byte) (sealedCID cid.Cid, unsealedCID cid.Cid, err error) {
	// TODO: allow workers to fetch the sectors

	best, err := m.storage.findSector(m.minerID(), sectorNum, sectorbuilder.FTCache|sectorbuilder.FTSealed)
	if err != nil {
		return cid.Undef, cid.Undef, xerrors.Errorf("finding path for sector sealing: %w", err)
	}

	candidateWorkers, _ := m.getWorkersByPaths(sealmgr.TTPreCommit2, best)

	// TODO: select(candidateWorkers, ...)
	// TODO: remove the sectorbuilder abstraction, pass path directly
	return candidateWorkers[0].SealPreCommit2(ctx, sectorNum, phase1Out)
}

func (m *Manager) SealCommit1(ctx context.Context, sectorNum abi.SectorNumber, ticket abi.SealRandomness, seed abi.InteractiveSealRandomness, pieces []abi.PieceInfo, sealedCID cid.Cid, unsealedCID cid.Cid) (output []byte, err error) {
	best, err := m.storage.findSector(m.minerID(), sectorNum, sectorbuilder.FTCache|sectorbuilder.FTSealed)
	if err != nil {
		return nil, xerrors.Errorf("finding path for sector sealing: %w", err)
	}

	candidateWorkers, _ := m.getWorkersByPaths(sealmgr.TTPreCommit2, best)

	// TODO: select(candidateWorkers, ...)
	// TODO: remove the sectorbuilder abstraction, pass path directly
	return candidateWorkers[0].SealCommit1(ctx, sectorNum, ticket, seed, pieces, sealedCID, unsealedCID)
}

func (m *Manager) SealCommit2(ctx context.Context, sectorNum abi.SectorNumber, phase1Out []byte) (proof []byte, err error) {
	for _, worker := range m.workers {
		if _, ok := worker.TaskTypes()[sealmgr.TTCommit2]; !ok {
			continue
		}

		return worker.SealCommit2(ctx, sectorNum, phase1Out)
	}

	return nil, xerrors.New("no worker found")
}

func (m *Manager) FinalizeSector(ctx context.Context, sectorNum abi.SectorNumber) error {
	best, err := m.storage.findSector(m.minerID(), sectorNum, sectorbuilder.FTCache|sectorbuilder.FTSealed|sectorbuilder.FTUnsealed)
	if err != nil {
		return xerrors.Errorf("finding sealed sector: %w", err)
	}

	candidateWorkers, _ := m.getWorkersByPaths(sealmgr.TTPreCommit2, best) // find last worker with the sector

	// TODO: Move the sector to long-term storage
	return candidateWorkers[0].FinalizeSector(ctx, sectorNum)
}

func (m *Manager) minerID() abi.ActorID {
	mid, err := address.IDFromAddress(m.scfg.Miner)
	if err != nil {
		panic(err)
	}
	return abi.ActorID(mid)
}

var _ sealmgr.Manager = &Manager{}
