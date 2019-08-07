package deals

import (
	"context"

	files "github.com/ipfs/go-ipfs-files"
	"github.com/ipfs/go-merkledag"
	unixfile "github.com/ipfs/go-unixfs/file"
	"golang.org/x/xerrors"
)

type handlerFunc func(ctx context.Context, deal MinerDeal) error

func (h *Handler) handle(ctx context.Context, deal MinerDeal, cb handlerFunc, next DealState) {
	go func() {
		err := cb(ctx, deal)
		select {
		case h.updated <- dealUpdate{
			newState: next,
			id:       deal.ProposalCid,
			err:      err,
		}:
		case <-h.stop:
		}
	}()
}

// ACCEPTED

func (h *Handler) accept(ctx context.Context, deal MinerDeal) error {
	log.Info("acc")
	switch deal.Proposal.SerializationMode {
	//case SerializationRaw:
	//case SerializationIPLD:
	case SerializationUnixFs:
	default:
		return xerrors.Errorf("deal proposal with unsupported serialization: %s", deal.Proposal.SerializationMode)
	}

	log.Info("fetching data for a deal")
	err := h.sendSignedResponse(StorageDealResponse{
		State:    Accepted,
		Message:  "",
		Proposal: deal.ProposalCid,
	})
	if err != nil {
		return err
	}

	return merkledag.FetchGraph(ctx, deal.Ref, h.dag)
}

// STAGED

func (h *Handler) staged(ctx context.Context, deal MinerDeal) error {
	err := h.sendSignedResponse(StorageDealResponse{
		State:    Staged,
		Message:  "",
		Proposal: deal.ProposalCid,
	})
	if err != nil {
		return err
	}

	root, err := h.dag.Get(ctx, deal.Ref)
	if err != nil {
		return xerrors.Errorf("failed to get file root for deal: %s", err)
	}

	// TODO: abstract this away into ReadSizeCloser + implement different modes
	n, err := unixfile.NewUnixfsFile(ctx, h.dag, root)
	if err != nil {
		return xerrors.Errorf("cannot open unixfs file: %s", err)
	}

	uf, ok := n.(files.File)
	if !ok {
		// we probably got directory, unsupported for now
		return xerrors.Errorf("unsupported unixfs type")
	}

	size, err := uf.Size()
	if err != nil {
		return xerrors.Errorf("failed to get file size: %s", err)
	}

	var sectorID uint64
	err = withTemp(uf, func(f string) (err error) {
		sectorID, err = h.sb.AddPiece(deal.Proposal.PieceRef, uint64(size), f)
		return err
	})
	if err != nil {
		return xerrors.Errorf("AddPiece failed: %s", err)
	}

	log.Warnf("New Sector: %d", sectorID)
	return nil
}

// SEALING
