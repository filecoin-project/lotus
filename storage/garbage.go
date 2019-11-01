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

func (m *Miner) StoreGarbageData(_ context.Context) error {
	ctx := context.TODO()
	ssize, err := m.SectorSize(ctx)
	if err != nil {
		return xerrors.Errorf("failed to get miner sector size: %w", err)
	}
	go func() {
		size := sectorbuilder.UserBytesForSectorSize(ssize)

		/*// Add market funds
		smsg, err := m.api.MpoolPushMessage(ctx, &types.Message{
			To:       actors.StorageMarketAddress,
			From:     m.worker,
			Value:    types.NewInt(size),
			GasPrice: types.NewInt(0),
			GasLimit: types.NewInt(1000000),
			Method:   actors.SMAMethods.AddBalance,
		})
		if err != nil {
			log.Error(err)
			return
		}

		r, err := m.api.StateWaitMsg(ctx, smsg.Cid())
		if err != nil {
			log.Error(err)
			return
		}

		if r.Receipt.ExitCode != 0 {
			log.Error(xerrors.Errorf("adding funds to storage miner market actor failed: exit %d", r.Receipt.ExitCode))
			return
		}*/
		// Publish a deal

		// TODO: Maybe cache
		commP, err := sectorbuilder.GeneratePieceCommitment(io.LimitReader(rand.New(rand.NewSource(42)), int64(size)), size)
		if err != nil {
			log.Error(err)
			return
		}

		sdp := actors.StorageDealProposal{
			PieceRef:             commP[:],
			PieceSize:            size,
			PieceSerialization:   actors.SerializationUnixFSv0,
			Client:               m.worker,
			Provider:             m.worker,
			ProposalExpiration:   math.MaxUint64,
			Duration:             math.MaxUint64 / 2, // /2 because overflows
			StoragePricePerEpoch: types.NewInt(0),
			StorageCollateral:    types.NewInt(0),
			ProposerSignature:    nil,
		}

		if err := api.SignWith(ctx, m.api.WalletSign, m.worker, &sdp); err != nil {
			log.Error(xerrors.Errorf("signing storage deal failed: ", err))
			return
		}

		storageDeal := actors.StorageDeal{
			Proposal: sdp,
		}
		if err := api.SignWith(ctx, m.api.WalletSign, m.worker, &storageDeal); err != nil {
			log.Error(xerrors.Errorf("signing storage deal failed: ", err))
			return
		}

		params, err := actors.SerializeParams(&actors.PublishStorageDealsParams{
			Deals: []actors.StorageDeal{storageDeal},
		})
		if err != nil {
			log.Error(xerrors.Errorf("serializing PublishStorageDeals params failed: ", err))
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
			log.Error(err)
			return
		}
		r, err := m.api.StateWaitMsg(ctx, smsg.Cid())
		if err != nil {
			log.Error(err)
			return
		}
		if r.Receipt.ExitCode != 0 {
			log.Error(xerrors.Errorf("publishing deal failed: exit %d", r.Receipt.ExitCode))
		}
		var resp actors.PublishStorageDealResponse
		if err := resp.UnmarshalCBOR(bytes.NewReader(r.Receipt.Return)); err != nil {
			log.Error(err)
			return
		}
		if len(resp.DealIDs) != 1 {
			log.Error("got unexpected number of DealIDs from")
			return
		}

		name := fmt.Sprintf("fake-file-%d", rand.Intn(100000000))
		sectorId, err := m.secst.AddPiece(name, size, io.LimitReader(rand.New(rand.NewSource(42)), int64(size)), resp.DealIDs[0])
		if err != nil {
			log.Error(err)
			return
		}

		if err := m.SealSector(context.TODO(), sectorId); err != nil {
			log.Error(err)
			return
		}
	}()

	return err
}
