package advmgr

import (
	"context"
	"io"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-sectorbuilder"
	"github.com/filecoin-project/specs-actors/actors/abi"
	storage2 "github.com/filecoin-project/specs-storage/storage"

	"github.com/filecoin-project/lotus/storage/sealmgr"
	"github.com/filecoin-project/lotus/storage/sealmgr/sectorutil"
	"github.com/filecoin-project/lotus/storage/sealmgr/stores"
)

var pathTypes = []sectorbuilder.SectorFileType{sectorbuilder.FTUnsealed, sectorbuilder.FTSealed, sectorbuilder.FTCache}

type LocalWorker struct {
	scfg       *sectorbuilder.Config
	storage    stores.Store
	localStore *stores.Local
	sindex     stores.SectorIndex
}

func NewLocalWorker(ma address.Address, spt abi.RegisteredProof, store stores.Store, local *stores.Local, sindex stores.SectorIndex) *LocalWorker {
	ppt, err := spt.RegisteredPoStProof()
	if err != nil {
		panic(err)
	}
	return &LocalWorker{
		scfg: &sectorbuilder.Config{
			SealProofType: spt,
			PoStProofType: ppt,
		},
		storage:    store,
		localStore: local,
		sindex:     sindex,
	}
}

type localWorkerPathProvider struct {
	w *LocalWorker
}

func (l *localWorkerPathProvider) AcquireSector(ctx context.Context, sector abi.SectorID, existing sectorbuilder.SectorFileType, allocate sectorbuilder.SectorFileType, sealing bool) (sectorbuilder.SectorPaths, func(), error) {
	paths, storageIDs, done, err := l.w.storage.AcquireSector(ctx, sector, existing, allocate, sealing)
	if err != nil {
		return sectorbuilder.SectorPaths{}, nil, err
	}

	return paths, func() {
		done()

		for _, fileType := range pathTypes {
			if fileType&allocate == 0 {
				continue
			}

			sid := sectorutil.PathByType(storageIDs, fileType)

			if err := l.w.sindex.StorageDeclareSector(ctx, stores.ID(sid), sector, fileType); err != nil {
				log.Errorf("declare sector error: %+v", err)
			}
		}
	}, nil
}

func (l *LocalWorker) sb() (sectorbuilder.Basic, error) {
	return sectorbuilder.New(&localWorkerPathProvider{w: l}, l.scfg)
}

func (l *LocalWorker) NewSector(ctx context.Context, sector abi.SectorID) error {
	sb, err := l.sb()
	if err != nil {
		return err
	}

	return sb.NewSector(ctx, sector)
}

func (l *LocalWorker) AddPiece(ctx context.Context, sector abi.SectorID, epcs []abi.UnpaddedPieceSize, sz abi.UnpaddedPieceSize, r io.Reader) (abi.PieceInfo, error) {
	sb, err := l.sb()
	if err != nil {
		return abi.PieceInfo{}, err
	}

	return sb.AddPiece(ctx, sector, epcs, sz, r)
}

func (l *LocalWorker) SealPreCommit1(ctx context.Context, sector abi.SectorID, ticket abi.SealRandomness, pieces []abi.PieceInfo) (out storage2.PreCommit1Out, err error) {
	sb, err := l.sb()
	if err != nil {
		return nil, err
	}

	return sb.SealPreCommit1(ctx, sector, ticket, pieces)
}

func (l *LocalWorker) SealPreCommit2(ctx context.Context, sector abi.SectorID, phase1Out storage2.PreCommit1Out) (cids storage2.SectorCids, err error) {
	sb, err := l.sb()
	if err != nil {
		return storage2.SectorCids{}, err
	}

	return sb.SealPreCommit2(ctx, sector, phase1Out)
}

func (l *LocalWorker) SealCommit1(ctx context.Context, sector abi.SectorID, ticket abi.SealRandomness, seed abi.InteractiveSealRandomness, pieces []abi.PieceInfo, cids storage2.SectorCids) (output storage2.Commit1Out, err error) {
	sb, err := l.sb()
	if err != nil {
		return nil, err
	}

	return sb.SealCommit1(ctx, sector, ticket, seed, pieces, cids)
}

func (l *LocalWorker) SealCommit2(ctx context.Context, sector abi.SectorID, phase1Out storage2.Commit1Out) (proof storage2.Proof, err error) {
	sb, err := l.sb()
	if err != nil {
		return nil, err
	}

	return sb.SealCommit2(ctx, sector, phase1Out)
}

func (l *LocalWorker) FinalizeSector(ctx context.Context, sector abi.SectorID) error {
	sb, err := l.sb()
	if err != nil {
		return err
	}

	return sb.FinalizeSector(ctx, sector)
}

func (l *LocalWorker) TaskTypes(context.Context) (map[sealmgr.TaskType]struct{}, error) {
	return map[sealmgr.TaskType]struct{}{
		sealmgr.TTAddPiece:   {},
		sealmgr.TTPreCommit1: {},
		sealmgr.TTPreCommit2: {},
		sealmgr.TTCommit2:    {},
	}, nil
}

func (l *LocalWorker) Paths(ctx context.Context) ([]stores.StoragePath, error) {
	return l.localStore.Local(ctx)
}

var _ Worker = &LocalWorker{}
