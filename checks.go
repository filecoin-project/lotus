package sealing

import (
	"bytes"
	"context"
	"github.com/filecoin-project/lotus/storage/sectorstorage/zerocomm"

	"github.com/ipfs/go-cid"
	cbg "github.com/whyrusleeping/cbor-gen"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/specs-actors/actors/builtin"
	"github.com/filecoin-project/specs-actors/actors/builtin/market"

	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/types"
)

// TODO: For now we handle this by halting state execution, when we get jsonrpc reconnecting
//  We should implement some wait-for-api logic
type ErrApi struct{ error }

type ErrInvalidDeals struct{ error }
type ErrInvalidPiece struct{ error }
type ErrExpiredDeals struct{ error }

type ErrBadCommD struct{ error }
type ErrExpiredTicket struct{ error }

// checkPieces validates that:
//  - Each piece han a corresponding on chain deal
//  - Piece commitments match with on chain deals
//  - Piece sizes match
//  - Deals aren't expired
func checkPieces(ctx context.Context, si SectorInfo, api sealingApi) error {
	head, err := api.ChainHead(ctx)
	if err != nil {
		return &ErrApi{xerrors.Errorf("getting chain head: %w", err)}
	}

	for i, piece := range si.Pieces {
		if piece.DealID == nil {
			exp := zerocomm.ZeroPieceCommitment(piece.Size)
			if piece.CommP != exp {
				return &ErrInvalidPiece{xerrors.Errorf("deal %d piece %d had non-zero CommP %+v", piece.DealID, i, piece.CommP)}
			}
			continue
		}
		deal, err := api.StateMarketStorageDeal(ctx, *piece.DealID, types.EmptyTSK)
		if err != nil {
			return &ErrApi{xerrors.Errorf("getting deal %d for piece %d: %w", piece.DealID, i, err)}
		}

		if deal.Proposal.PieceCID != piece.CommP {
			return &ErrInvalidDeals{xerrors.Errorf("piece %d (or %d) of sector %d refers deal %d with wrong CommP: %x != %x", i, len(si.Pieces), si.SectorID, piece.DealID, piece.CommP, deal.Proposal.PieceCID)}
		}

		if piece.Size != deal.Proposal.PieceSize.Unpadded() {
			return &ErrInvalidDeals{xerrors.Errorf("piece %d (or %d) of sector %d refers deal %d with different size: %d != %d", i, len(si.Pieces), si.SectorID, piece.DealID, piece.Size, deal.Proposal.PieceSize)}
		}

		if head.Height() >= deal.Proposal.StartEpoch {
			return &ErrExpiredDeals{xerrors.Errorf("piece %d (or %d) of sector %d refers expired deal %d - should start at %d, head %d", i, len(si.Pieces), si.SectorID, piece.DealID, deal.Proposal.StartEpoch, head.Height())}
		}
	}

	return nil
}

// checkSeal checks that data commitment generated in the sealing process
//  matches pieces, and that the seal ticket isn't expired
func checkSeal(ctx context.Context, maddr address.Address, si SectorInfo, api sealingApi) (err error) {
	head, err := api.ChainHead(ctx)
	if err != nil {
		return &ErrApi{xerrors.Errorf("getting chain head: %w", err)}
	}

	ccparams, err := actors.SerializeParams(&market.ComputeDataCommitmentParams{
		DealIDs:    si.deals(),
		SectorType: si.SectorType,
	})
	if err != nil {
		return xerrors.Errorf("computing params for ComputeDataCommitment: %w", err)
	}

	ccmt := &types.Message{
		To:       builtin.StorageMarketActorAddr,
		From:     maddr,
		Value:    types.NewInt(0),
		GasPrice: types.NewInt(0),
		GasLimit: 9999999999,
		Method:   builtin.MethodsMarket.ComputeDataCommitment,
		Params:   ccparams,
	}
	r, err := api.StateCall(ctx, ccmt, types.EmptyTSK)
	if err != nil {
		return &ErrApi{xerrors.Errorf("calling ComputeDataCommitment: %w", err)}
	}
	if r.MsgRct.ExitCode != 0 {
		return &ErrBadCommD{xerrors.Errorf("receipt for ComputeDataCommitment had exit code %d", r.MsgRct.ExitCode)}
	}

	var c cbg.CborCid
	if err := c.UnmarshalCBOR(bytes.NewReader(r.MsgRct.Return)); err != nil {
		return err
	}

	if cid.Cid(c) != *si.CommD {
		return &ErrBadCommD{xerrors.Errorf("on chain CommD differs from sector: %s != %s", cid.Cid(c), si.CommD)}
	}

	if int64(head.Height())-int64(si.Ticket.Epoch+build.SealRandomnessLookback) > build.SealRandomnessLookbackLimit {
		return &ErrExpiredTicket{xerrors.Errorf("ticket expired: seal height: %d, head: %d", si.Ticket.Epoch+build.SealRandomnessLookback, head.Height())}
	}

	return nil

}
