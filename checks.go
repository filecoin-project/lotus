package sealing

import (
	"context"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/types"
)

func checkPieces(ctx context.Context, si *SectorInfo, api sealingApi) error {
	for i, piece := range si.Pieces {
		deal, err := api.StateMarketStorageDeal(ctx, piece.DealID, nil)
		if err != nil {
			return xerrors.Errorf("getting deal %d for piece %d: %w", piece.DealID, i, err)
		}

		if string(deal.PieceRef) != string(piece.CommP) {
			return xerrors.Errorf("piece %d of sector %d refers deal %d with wrong CommP: %x != %x", i, si.SectorID, piece.DealID, piece.CommP, deal.PieceRef)
		}

		if piece.Size != deal.PieceSize {
			return xerrors.Errorf("piece %d of sector %d refers deal %d with different size: %d != %d", i, si.SectorID, piece.DealID, piece.Size, deal.PieceSize)
		}
	}

	return nil
}

func checkSeal(ctx context.Context, maddr address.Address, si *SectorInfo, api sealingApi) (err error) {
	ssize, err := api.StateMinerSectorSize(ctx, maddr, nil)
	if err != nil {
		return err
	}

	ccparams, err := actors.SerializeParams(&actors.ComputeDataCommitmentParams{
		DealIDs:    si.deals(),
		SectorSize: ssize,
	})
	if err != nil {
		return xerrors.Errorf("computing params for ComputeDataCommitment: %w", err)
	}

	ccmt := &types.Message{
		To:       actors.StorageMarketAddress,
		From:     actors.StorageMarketAddress,
		Value:    types.NewInt(0),
		GasPrice: types.NewInt(0),
		GasLimit: types.NewInt(9999999999),
		Method:   actors.SMAMethods.ComputeDataCommitment,
		Params:   ccparams,
	}
	r, err := api.StateCall(ctx, ccmt, nil)
	if err != nil {
		return xerrors.Errorf("calling ComputeDataCommitment: %w", err)
	}
	if r.ExitCode != 0 {
		return xerrors.Errorf("receipt for ComputeDataCommitment han exit code %d", r.ExitCode)
	}
	if string(r.Return) != string(si.CommD) {
		return xerrors.Errorf("on chain CommD differs from sector: %x != %x", r.Return, si.CommD)
	}

	// TODO: Validate ticket
	// TODO: Verify commp / commr / proof
	// TODO: (StateCall PreCommit)
	return nil

}
