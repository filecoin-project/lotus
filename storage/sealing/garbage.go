package sealing

import (
	"bytes"
	"context"
	sectorbuilder "github.com/filecoin-project/go-sectorbuilder"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/types"
	"golang.org/x/xerrors"
	"io"
	"math"
	"math/rand"
)

func (m *Sealing) pledgeReader(size uint64, parts uint64) io.Reader {
	piece := sectorbuilder.UserBytesForSectorSize((size/127 + size) / parts)

	readers := make([]io.Reader, parts)
	for i := range readers {
		readers[i] = io.LimitReader(rand.New(rand.NewSource(42+int64(i))), int64(piece))
	}

	return io.MultiReader(readers...)
}

func (m *Sealing) pledgeSector(ctx context.Context, sectorID uint64, existingPieceSizes []uint64, sizes ...uint64) ([]Piece, error) {
	if len(sizes) == 0 {
		return nil, nil
	}

	out, err := m.repledgeSector(ctx,sectorID,existingPieceSizes, sizes...)

	if err == nil{
		return out, nil
	}

	return m.firstPledgeSector(ctx,sectorID, existingPieceSizes, sizes...)
}


func (m *Sealing) firstPledgeSector(ctx context.Context, sectorID uint64, existingPieceSizes []uint64, sizes ...uint64) ([]Piece, error) {
	deals := make([]actors.StorageDealProposal, len(sizes))
	for i, size := range sizes {

		//commP, err := m.fastPledgeCommitment(size, uint64(runtime.NumCPU()))


		release := m.sb.RateLimit()
		commP, err := sectorbuilder.GeneratePieceCommitment(io.LimitReader(rand.New(rand.NewSource(42)), int64(size)), size)
		release()



		if err != nil {
			return nil, err
		}

		sdp := actors.StorageDealProposal{
			PieceRef:             commP[:],
			PieceSize:            size,
			Client:               m.worker,
			Provider:             m.maddr,
			ProposalExpiration:   math.MaxUint64,
			Duration:             math.MaxUint64 / 2, // /2 because overflows
			StoragePricePerEpoch: types.NewInt(0),
			StorageCollateral:    types.NewInt(0),
			ProposerSignature:    nil, // nil because self dealing
		}

		deals[i] = sdp
	}

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
	r, err := m.api.StateWaitMsg(ctx, smsg.Cid())
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
	if len(resp.DealIDs) != len(sizes) {
		return nil, xerrors.New("got unexpected number of DealIDs from PublishStorageDeals")
	}

	out := make([]Piece, len(sizes))

	for i, size := range sizes {

		//ppi, err := m.sb.AddPiece(size, sectorID, m.pledgeReader(size, uint64(runtime.NumCPU())), existingPieceSizes)
		ppi, err := m.sb.AddPiece(size, sectorID, io.LimitReader(rand.New(rand.NewSource(42)), int64(size)), existingPieceSizes)
		if err != nil {
			return nil, err
		}

		existingPieceSizes = append(existingPieceSizes, size)

		out[i] = Piece{
			DealID: resp.DealIDs[i],
			Size:   ppi.Size,
			CommP:  ppi.CommP[:],
		}
	}

	m.saveCacheInfo(sectorID, out,deals)

	return out, nil
}

func (m *Sealing) PledgeSector() error {
	go func() {
		ctx := context.TODO() // we can't use the context from command which invokes
		// this, as we run everything here async, and it's cancelled when the
		// command exits

		size := sectorbuilder.UserBytesForSectorSize(m.sb.SectorSize())

		sid, err := m.sb.AcquireSectorId()
		if err != nil {
			log.Errorf("%+v", err)
			return
		}

		var pieces []Piece
		for{

			lock.Lock()
			defer lock.Unlock()

			pieces, err = m.pledgeSector(ctx, sid, []uint64{}, size)
			if err != nil {
				log.Errorf("%+v", err)
				return
			}
			break;
		}

		if err := m.newSector(context.TODO(), sid, pieces[0].DealID, pieces[0].ppi()); err != nil {
			log.Errorf("%+v", err)
			return
		}
	}()
	return nil
}