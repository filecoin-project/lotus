package advmgr

import (
	"context"
	"io"

	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/specs-actors/actors/abi"
	storage2 "github.com/filecoin-project/specs-storage/storage"

	"github.com/filecoin-project/go-sectorbuilder"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/storage/sealmgr/stores"

	"github.com/filecoin-project/lotus/storage/sealmgr"
)

type LocalWorker struct {
	scfg    *sectorbuilder.Config
	storage stores.Store
	localStore *stores.Local
}

func NewLocalWorker(ma address.Address, spt abi.RegisteredProof, store stores.Store, local *stores.Local) *LocalWorker {
	ppt, err := spt.RegisteredPoStProof()
	if err != nil {
		panic(err)
	}
	return &LocalWorker{
		scfg:       &sectorbuilder.Config{
			SealProofType: spt,
			PoStProofType: ppt,
			Miner:         ma,
		},
		storage:    store,
		localStore: local,
	}
}

type localWorkerPathProvider struct {
	w *LocalWorker
}

func (l *localWorkerPathProvider) AcquireSector(ctx context.Context, id abi.SectorNumber, existing sectorbuilder.SectorFileType, allocate sectorbuilder.SectorFileType, sealing bool) (sectorbuilder.SectorPaths, func(), error) {
	mid, err := address.IDFromAddress(l.w.scfg.Miner)
	if err != nil {
		return sectorbuilder.SectorPaths{}, nil, xerrors.Errorf("get miner ID: %w", err)
	}

	return l.w.storage.AcquireSector(ctx, abi.SectorID{
		Miner: abi.ActorID(mid),
		Number: id,
	}, existing, allocate, sealing)
}

func (l *LocalWorker) sb() (sectorbuilder.Basic, error) {
	return sectorbuilder.New(&localWorkerPathProvider{w: l}, l.scfg)
}

func (l *LocalWorker) AddPiece(ctx context.Context, sn abi.SectorNumber, epcs []abi.UnpaddedPieceSize, sz abi.UnpaddedPieceSize, r io.Reader) (abi.PieceInfo, error) {
	sb, err := l.sb()
	if err != nil {
		return abi.PieceInfo{}, err
	}

	return sb.AddPiece(ctx, sn, epcs, sz, r)
}

func (l *LocalWorker) SealPreCommit1(ctx context.Context, sectorNum abi.SectorNumber, ticket abi.SealRandomness, pieces []abi.PieceInfo) (out storage2.PreCommit1Out, err error) {
	sb, err := l.sb()
	if err != nil {
		return nil, err
	}

	return sb.SealPreCommit1(ctx, sectorNum, ticket, pieces)
}

func (l *LocalWorker) SealPreCommit2(ctx context.Context, sectorNum abi.SectorNumber, phase1Out storage2.PreCommit1Out) (sealedCID cid.Cid, unsealedCID cid.Cid, err error) {
	sb, err := l.sb()
	if err != nil {
		return cid.Undef, cid.Undef, err
	}

	return sb.SealPreCommit2(ctx, sectorNum, phase1Out)
}

func (l *LocalWorker) SealCommit1(ctx context.Context, sectorNum abi.SectorNumber, ticket abi.SealRandomness, seed abi.InteractiveSealRandomness, pieces []abi.PieceInfo, sealedCID cid.Cid, unsealedCID cid.Cid) (output storage2.Commit1Out, err error) {
	sb, err := l.sb()
	if err != nil {
		return nil, err
	}

	return sb.SealCommit1(ctx, sectorNum, ticket, seed, pieces, sealedCID, unsealedCID)
}

func (l *LocalWorker) SealCommit2(ctx context.Context, sectorNum abi.SectorNumber, phase1Out storage2.Commit1Out) (proof storage2.Proof, err error) {
	sb, err := l.sb()
	if err != nil {
		return nil, err
	}

	return sb.SealCommit2(ctx, sectorNum, phase1Out)
}

func (l *LocalWorker) FinalizeSector(ctx context.Context, sectorNum abi.SectorNumber) error {
	sb, err := l.sb()
	if err != nil {
		return err
	}

	return sb.FinalizeSector(ctx, sectorNum)
}

func (l *LocalWorker) TaskTypes(context.Context) (map[sealmgr.TaskType]struct{}, error) {
	return map[sealmgr.TaskType]struct{}{
		sealmgr.TTAddPiece:   {},
		sealmgr.TTPreCommit1: {},
		sealmgr.TTPreCommit2: {},
		sealmgr.TTCommit2:    {},
	}, nil
}

func (l *LocalWorker) Paths(context.Context) ([]api.StoragePath, error) {
	return l.localStore.Local(), nil
}

var _ Worker = &LocalWorker{}
