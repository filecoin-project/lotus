package sealmgr

import (
	"context"
	"io"

	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-sectorbuilder"
	"github.com/filecoin-project/specs-actors/actors/abi"
)

type LocalWorker struct {
	sealer sectorbuilder.Basic
	mid    abi.ActorID
}

func (w *LocalWorker) Run(ctx context.Context, task TaskType, si SectorInfo) (SectorInfo, error) {
	if si.ID.Miner != w.mid {
		return si, xerrors.Errorf("received a task with wrong actor id; worker for %d, task for %d", w.mid, si.ID.Miner)
	}

	switch task {
	case TTPreCommit1:
		pco, err := w.sealer.SealPreCommit1(ctx, si.ID.Number, si.Ticket.Value, si.Pieces)
		if err != nil {
			return si, xerrors.Errorf("calling sealer: %w", err)
		}
		si.PreCommit1Out = pco
	case TTPreCommit2:
		sealed, unsealed, err := w.sealer.SealPreCommit2(ctx, si.ID.Number, si.PreCommit1Out)
		if err != nil {
			return si, xerrors.Errorf("calling sealer (precommit2): %w", err)
		}

		si.Sealed = &sealed
		si.Unsealed = &unsealed

		// We also call Commit1 here as it only grabs some inputs for the snark,
		// which is very fast (<1s), and it doesn't really make sense to have a separate
		// task type for it

		c2in, err := w.sealer.SealCommit1(ctx, si.ID.Number, si.Ticket.Value, si.Seed.Value, si.Pieces, *si.Sealed, *si.Unsealed)
		if err != nil {
			return si, xerrors.Errorf("calling sealer (commit1): %w", err)
		}

		si.CommitInput = c2in
	case TTCommit2:
		proof, err := w.sealer.SealCommit2(ctx, si.ID.Number, si.CommitInput)
		if err != nil {
			return SectorInfo{}, xerrors.Errorf("calling sealer: %w", err)
		}

		si.Proof = proof
	default:
		return si, xerrors.Errorf("unknown task type '%s'", task)
	}

	return si, nil
}

func (w *LocalWorker) AddPiece(ctx context.Context, si SectorInfo, sz abi.UnpaddedPieceSize, r io.Reader) (cid.Cid, SectorInfo, error) {
	pi, err := w.sealer.AddPiece(ctx, sz, si.ID.Number, r, si.PieceSizes())
	if err != nil {
		return cid.Cid{}, SectorInfo{}, xerrors.Errorf("addPiece on local worker: %w", err)
	}

	si.Pieces = append(si.Pieces, pi)
	return pi.PieceCID, si, nil
}

var _ Worker = &LocalWorker{}
