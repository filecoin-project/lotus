package sealing

import (
	"bytes"
	"context"
	"io"
	"math"
	"math/bits"
	"math/rand"

	commcid "github.com/filecoin-project/go-fil-commcid"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/builtin/market"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/types"
)

func (m *Sealing) pledgeReader(size abi.UnpaddedPieceSize, parts uint64) io.Reader {
	parts = 1 << bits.Len64(parts) // round down to nearest power of 2
	if uint64(size)/parts < 127 {
		parts = uint64(size) / 127
	}

	piece := abi.PaddedPieceSize(uint64(size.Padded()) / parts).Unpadded()

	readers := make([]io.Reader, parts)
	for i := range readers {
		readers[i] = io.LimitReader(rand.New(rand.NewSource(42+int64(i))), int64(piece))
	}

	return io.MultiReader(readers...)
}

func (m *Sealing) pledgeSector(ctx context.Context, sectorID abi.SectorNumber, existingPieceSizes []abi.UnpaddedPieceSize, sizes ...abi.UnpaddedPieceSize) ([]Piece, error) {
	if len(sizes) == 0 {
		return nil, nil
	}

	log.Infof("Pledge %d, contains %+v", sectorID, existingPieceSizes)

	deals := make([]market.ClientDealProposal, len(sizes))
	for i, size := range sizes {
		commP, err := m.fastPledgeCommitment(size, uint64(1))
		if err != nil {
			return nil, err
		}

		sdp := market.DealProposal{
			PieceCID:             commcid.PieceCommitmentV1ToCID(commP[:]),
			PieceSize:            size.Padded(),
			Client:               m.worker,
			Provider:             m.maddr,
			StartEpoch:           math.MaxInt64,
			EndEpoch:             math.MaxInt64,
			StoragePricePerEpoch: types.NewInt(0),
			ProviderCollateral:   types.NewInt(0),
		}

		deals[i] = market.ClientDealProposal{
			Proposal: sdp,
		}
	}

	log.Infof("Publishing deals for %d", sectorID)

	params, aerr := actors.SerializeParams(&actors.PublishStorageDealsParams{
		Deals: deals,
	})
	if aerr != nil {
		return nil, xerrors.Errorf("serializing PublishStorageDeals params failed: ", aerr)
	}

	smsg, err := m.api.MpoolPushMessage(ctx, &types.Message{
		To:       actors.StorageMarketAddress,
		From:     m.worker,
		Value:    types.NewInt(0),
		GasPrice: types.NewInt(0),
		GasLimit: types.NewInt(1000000),
		Method:   actors.SMAMethods.PublishStorageDeals,
		Params:   params,
	})
	if err != nil {
		return nil, err
	}
	r, err := m.api.StateWaitMsg(ctx, smsg.Cid()) // TODO: more finality
	if err != nil {
		return nil, err
	}
	if r.Receipt.ExitCode != 0 {
		log.Error(xerrors.Errorf("publishing deal failed: exit %d", r.Receipt.ExitCode))
	}
	var resp actors.PublishStorageDealResponse
	if err := resp.UnmarshalCBOR(bytes.NewReader(r.Receipt.Return)); err != nil {
		return nil, err
	}
	if len(resp.IDs) != len(sizes) {
		return nil, xerrors.New("got unexpected number of DealIDs from PublishStorageDeals")
	}

	log.Infof("Deals for sector %d: %+v", sectorID, resp.IDs)

	out := make([]Piece, len(sizes))
	for i, size := range sizes {
		ppi, err := m.sb.AddPiece(ctx, size, sectorID, m.pledgeReader(size, uint64(1)), existingPieceSizes)
		if err != nil {
			return nil, xerrors.Errorf("add piece: %w", err)
		}

		existingPieceSizes = append(existingPieceSizes, size)

		out[i] = Piece{
			DealID: resp.IDs[i],
			Size:   abi.UnpaddedPieceSize(ppi.Size),
			CommP:  ppi.CommP[:],
		}
	}

	return out, nil
}

func (m *Sealing) PledgeSector() error {
	go func() {
		ctx := context.TODO() // we can't use the context from command which invokes
		// this, as we run everything here async, and it's cancelled when the
		// command exits

		size := abi.PaddedPieceSize(m.sb.SectorSize()).Unpadded()

		sid, err := m.sb.AcquireSectorNumber()
		if err != nil {
			log.Errorf("%+v", err)
			return
		}

		pieces, err := m.pledgeSector(ctx, sid, []abi.UnpaddedPieceSize{}, abi.UnpaddedPieceSize(size))
		if err != nil {
			log.Errorf("%+v", err)
			return
		}

		if err := m.newSector(context.TODO(), sid, pieces[0].DealID, pieces[0].ppi()); err != nil {
			log.Errorf("%+v", err)
			return
		}
	}()
	return nil
}
