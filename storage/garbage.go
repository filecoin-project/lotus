package storage

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math"
	"math/rand"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/lib/sectorbuilder"
)

// TODO: expected sector ID
func (m *Miner) storeGarbage(ctx context.Context, sizes ...uint64) ([]uint64, error) {
	deals := make([]actors.StorageDeal, len(sizes))
	for i, size := range sizes {
		commP, err := sectorbuilder.GeneratePieceCommitment(io.LimitReader(rand.New(rand.NewSource(42)), int64(size)), size)
		if err != nil {
			return nil, err
		}

		sdp := actors.StorageDealProposal{
			PieceRef:             commP[:],
			PieceSize:            size,
			PieceSerialization:   actors.SerializationUnixFSv0,
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

		storageDeal := actors.StorageDeal{
			Proposal: sdp,
		}
		if err := api.SignWith(ctx, m.api.WalletSign, m.worker, &storageDeal); err != nil {
			return nil, xerrors.Errorf("signing storage deal failed: ", err)
		}

		deals[i] = storageDeal
	}

	params, aerr := actors.SerializeParams(&actors.PublishStorageDealsParams{
		Deals: deals,
	})
	if aerr != nil {
		return nil, xerrors.Errorf("serializing PublishStorageDeals params failed: ", aerr)
	}

	// TODO: We may want this to happen after fetching data
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

	sectorIDs := make([]uint64, len(sizes))

	for i, size := range sizes {
		name := fmt.Sprintf("fake-file-%d", rand.Intn(100000000))
		sectorID, err := m.secst.AddPiece(name, size, io.LimitReader(rand.New(rand.NewSource(42)), int64(size)), resp.DealIDs[i])
		if err != nil {
			return nil, err
		}

		sectorIDs[i] = sectorID
	}

	return sectorIDs, nil
}

func (m *Miner) StoreGarbageData(_ context.Context) error {
	ctx := context.TODO()
	ssize, err := m.SectorSize(ctx)
	if err != nil {
		return xerrors.Errorf("failed to get miner sector size: %w", err)
	}
	go func() {
		size := sectorbuilder.UserBytesForSectorSize(ssize)

		sids, err := m.storeGarbage(ctx, size)
		if err != nil {
			log.Errorf("%+v", err)
			return
		}

		if err := m.SealSector(context.TODO(), sids[0]); err != nil {
			log.Errorf("%+v", err)
			return
		}
	}()

	return err
}
