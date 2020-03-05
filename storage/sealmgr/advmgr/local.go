package advmgr

import (
	"context"
	"io"

	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-sectorbuilder"
	"github.com/filecoin-project/specs-actors/actors/abi"

	"github.com/filecoin-project/lotus/storage/sealmgr"
)

type localWorker struct {
	scfg *sectorbuilder.Config
	storage *storage
}

type localWorkerPathProvider struct {
	w *localWorker
}

func (l *localWorkerPathProvider) AcquireSectorNumber() (abi.SectorNumber, error) {
	return 0, xerrors.Errorf("unsupported")
}

func (l *localWorkerPathProvider) FinalizeSector(abi.SectorNumber) error {
	return xerrors.Errorf("unsupported")
}

func (l *localWorkerPathProvider) AcquireSector(id abi.SectorNumber, existing sectorbuilder.SectorFileType, allocate sectorbuilder.SectorFileType, sealing bool) (sectorbuilder.SectorPaths, func(), error) {
	mid, err := address.IDFromAddress(l.w.scfg.Miner)
	if err != nil {
		return sectorbuilder.SectorPaths{}, nil, xerrors.Errorf("get miner ID: %w", err)
	}

	return l.w.storage.acquireSector(abi.ActorID(mid), id, existing, allocate, sealing)
}

func (l *localWorker) sb() (sectorbuilder.Basic, error) {
	return sectorbuilder.New(&localWorkerPathProvider{w:l}, l.scfg)
}

func (l *localWorker) AddPiece(ctx context.Context, sz abi.UnpaddedPieceSize, sn abi.SectorNumber, r io.Reader, epcs []abi.UnpaddedPieceSize) (abi.PieceInfo, error) {
	sb, err := l.sb()
	if err != nil {
		return abi.PieceInfo{}, err
	}

	return sb.AddPiece(ctx, sz, sn, r, epcs)
}

func (l *localWorker) SealPreCommit1(ctx context.Context, sectorNum abi.SectorNumber, ticket abi.SealRandomness, pieces []abi.PieceInfo) (out []byte, err error) {
	sb, err := l.sb()
	if err != nil {
		return nil, err
	}

	return sb.SealPreCommit1(ctx, sectorNum, ticket, pieces)
}

func (l *localWorker) SealPreCommit2(ctx context.Context, sectorNum abi.SectorNumber, phase1Out []byte) (sealedCID cid.Cid, unsealedCID cid.Cid, err error) {
	sb, err := l.sb()
	if err != nil {
		return cid.Undef, cid.Undef, err
	}

	return sb.SealPreCommit2(ctx, sectorNum, phase1Out)
}

func (l *localWorker) SealCommit1(ctx context.Context, sectorNum abi.SectorNumber, ticket abi.SealRandomness, seed abi.InteractiveSealRandomness, pieces []abi.PieceInfo, sealedCID cid.Cid, unsealedCID cid.Cid) (output []byte, err error) {
	sb, err := l.sb()
	if err != nil {
		return nil, err
	}

	return sb.SealCommit1(ctx, sectorNum, ticket, seed, pieces, sealedCID, unsealedCID)
}

func (l *localWorker) SealCommit2(ctx context.Context, sectorNum abi.SectorNumber, phase1Out []byte) (proof []byte, err error) {
	sb, err := l.sb()
	if err != nil {
		return nil, err
	}

	return sb.SealCommit2(ctx, sectorNum, phase1Out)
}

func (l *localWorker) FinalizeSector(ctx context.Context, sectorNum abi.SectorNumber) error {
	sb, err := l.sb()
	if err != nil {
		return err
	}

	return sb.FinalizeSector(ctx, sectorNum)
}

func (l *localWorker) TaskTypes() map[sealmgr.TaskType]struct{} {
	return map[sealmgr.TaskType]struct{}{
		sealmgr.TTAddPiece: {},
		sealmgr.TTPreCommit1: {},
		sealmgr.TTPreCommit2: {},
		sealmgr.TTCommit2: {},
	}
}

func (l *localWorker) Paths() []Path {
	return l.storage.local()
}

var _ Worker = &localWorker{}