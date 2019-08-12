package deals

import (
	"context"
	"time"

	"github.com/filecoin-project/go-lotus/lib/sectorbuilder"

	files "github.com/ipfs/go-ipfs-files"
	"github.com/ipfs/go-merkledag"
	unixfile "github.com/ipfs/go-unixfs/file"
	"golang.org/x/xerrors"
)

type handlerFunc func(ctx context.Context, deal MinerDeal) (func(*MinerDeal), error)

func (h *Handler) handle(ctx context.Context, deal MinerDeal, cb handlerFunc, next DealState) {
	go func() {
		mut, err := cb(ctx, deal)
		select {
		case h.updated <- dealUpdate{
			newState: next,
			id:       deal.ProposalCid,
			err:      err,
			mut:      mut,
		}:
		case <-h.stop:
		}
	}()
}

// ACCEPTED

func (h *Handler) accept(ctx context.Context, deal MinerDeal) (func(*MinerDeal), error) {
	switch deal.Proposal.SerializationMode {
	//case SerializationRaw:
	//case SerializationIPLD:
	case SerializationUnixFs:
	default:
		return nil, xerrors.Errorf("deal proposal with unsupported serialization: %s", deal.Proposal.SerializationMode)
	}

	// TODO: check payment

	log.Info("fetching data for a deal")
	err := h.sendSignedResponse(StorageDealResponse{
		State:    Accepted,
		Message:  "",
		Proposal: deal.ProposalCid,
	})
	if err != nil {
		return nil, err
	}

	return nil, merkledag.FetchGraph(ctx, deal.Ref, h.dag)
}

// STAGED

func (h *Handler) staged(ctx context.Context, deal MinerDeal) (func(*MinerDeal), error) {
	err := h.sendSignedResponse(StorageDealResponse{
		State:    Staged,
		Proposal: deal.ProposalCid,
	})
	if err != nil {
		log.Warnf("Sending deal response failed: %s", err)
	}

	root, err := h.dag.Get(ctx, deal.Ref)
	if err != nil {
		return nil, xerrors.Errorf("failed to get file root for deal: %s", err)
	}

	// TODO: abstract this away into ReadSizeCloser + implement different modes
	n, err := unixfile.NewUnixfsFile(ctx, h.dag, root)
	if err != nil {
		return nil, xerrors.Errorf("cannot open unixfs file: %s", err)
	}

	uf, ok := n.(files.File)
	if !ok {
		// we probably got directory, unsupported for now
		return nil, xerrors.Errorf("unsupported unixfs type")
	}

	size, err := uf.Size()
	if err != nil {
		return nil, xerrors.Errorf("failed to get file size: %s", err)
	}

	var sectorID uint64
	err = withTemp(uf, func(f string) (err error) {
		sectorID, err = h.sb.AddPiece(deal.Proposal.PieceRef, uint64(size), f)
		return err
	})
	if err != nil {
		return nil, xerrors.Errorf("AddPiece failed: %s", err)
	}

	log.Warnf("New Sector: %d", sectorID)
	return func(deal *MinerDeal) {
		deal.SectorID = sectorID
	}, nil
}

// SEALING

func getInclusionProof(ref string, status sectorbuilder.SectorSealingStatus) (PieceInclusionProof, error) {
	for i, p := range status.Pieces {
		if p.Key == ref {
			return PieceInclusionProof{
				Position:      uint64(i),
				ProofElements: p.InclusionProof,
			}, nil
		}
	}
	return PieceInclusionProof{}, xerrors.Errorf("pieceInclusionProof for %s in sector %d not found", ref, status.SectorID)
}

func (h *Handler) pollSectorSealed(deal MinerDeal) (status sectorbuilder.SectorSealingStatus, err error){
loop:
	for {
		status, err = h.sb.SealStatus(deal.SectorID)
		if err != nil {
			return sectorbuilder.SectorSealingStatus{}, err
		}

		switch status.SealStatusCode {
		case 0: // sealed
			break loop
		case 2: // failed
			return sectorbuilder.SectorSealingStatus{}, xerrors.Errorf("sealing sector %d for deal %s (ref=%s) failed: %s", deal.SectorID, deal.ProposalCid, deal.Ref, status.SealErrorMsg)
		case 1: // pending
			if err := h.sb.SealAllStagedSectors(); err != nil {
				return sectorbuilder.SectorSealingStatus{}, err
			}
			// start seal
			fallthrough
		case 3: // sealing
			// wait
		default:
			return sectorbuilder.SectorSealingStatus{}, xerrors.Errorf("unknown SealStatusCode: %d", status.SectorID)
		}
		time.Sleep(3 * time.Second)
	}
	return status, nil
}

func (h *Handler) sealing(ctx context.Context, deal MinerDeal) (func(*MinerDeal), error) {
	status, err := h.pollSectorSealed(deal)
	if err != nil {
		return nil, err
	}

	ip, err := getInclusionProof(deal.Ref.String(), status)
	if err != nil {
		return nil, err
	}

	err = h.sendSignedResponse(StorageDealResponse{
		State:               Sealing,
		Proposal:            deal.ProposalCid,
		PieceInclusionProof: ip,
	})
	if err != nil {
		log.Warnf("Sending deal response failed: %s", err)
	}

	return nil, nil
}
