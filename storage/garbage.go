package storage

import (
	"bytes"
	"context"
	"github.com/btcsuite/goleveldb/leveldb/errors"
	"io"
	"math"
	"math/rand"
	"os"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/lib/sectorbuilder"
)

var lastcommP = [32]byte {}
var lastSectorId uint64  = 0

func (m *Miner) pledgeSector(ctx context.Context, sectorID uint64, existingPieceSizes []uint64, sizes ...uint64) ([]Piece, error) {
	if len(sizes) == 0 {
		return nil, nil
	}
	log.Infof("pledgeSector 1: lastSectorId: %d sectorID: %d lastcommP: %s",  lastSectorId, sectorID, lastcommP)
	deals := make([]actors.StorageDealProposal, len(sizes))
	for i, size := range sizes {
		release := m.sb.RateLimit()
		err := errors.ErrNotFound
		if lastSectorId == 0 {
			log.Infof("pledgeSector 2 : lastSectorId: %d sectorID: %d lastcommP: %s",  lastSectorId, sectorID, lastcommP)
			lastcommP, err = sectorbuilder.GeneratePieceCommitment(io.LimitReader(rand.New(rand.NewSource(42)), int64(size)), size)
			if err != nil {
				return nil, err
			}

			lastSectorId = sectorID

		}
		release()


		sdp := actors.StorageDealProposal{
			PieceRef:             lastcommP[:],
			PieceSize:            size,
			Client:               m.worker,
			Provider:             m.maddr,
			ProposalExpiration:   math.MaxUint64,
			Duration:             math.MaxUint64 / 2, // /2 because overflows
			StoragePricePerEpoch: types.NewInt(0),
			StorageCollateral:    types.NewInt(0),
			ProposerSignature:    nil,
		}

		if err := api.SignWith(ctx, m.api.WalletSign, m.worker, &sdp); err != nil {
			return nil, xerrors.Errorf("signing storage deal failed: ", err)
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
		if lastSectorId == 0 {
			log.Infof("pledgeSector 3 : lastSectorId: %d sectorID: %d lastcommP: %s",  lastSectorId, sectorID, lastcommP)
			ppi, err := m.sb.AddPiece(size, sectorID, io.LimitReader(rand.New(rand.NewSource(42)), int64(size)), existingPieceSizes)
			if err != nil {
				return nil, err
			}
			lastSectorId = sectorID
			existingPieceSizes = append(existingPieceSizes, size)

			out[i] = Piece{
				DealID: resp.DealIDs[i],
				Size:   ppi.Size,
				CommP:  ppi.CommP[:],
			}
		} else {
			log.Infof("pledgeSector 4 : lastSectorId: %d sectorID: %d lastcommP: %s",  lastSectorId, sectorID, lastcommP)
			os.Symlink(m.sb.StagedSectorPath(lastSectorId), m.sb.StagedSectorPath(sectorID))

			out[i] = Piece{
				DealID: resp.DealIDs[i],
				Size:   size,
				CommP:  lastcommP[:],
			}
		}
	}

	return out, nil
}

func (m *Miner) PledgeSector() error {
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

		pieces, err := m.pledgeSector(ctx, sid, []uint64{}, size)
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
