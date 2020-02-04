package storage

import (
	"bytes"
	"context"
	"fmt"
	"math"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-sectorbuilder"
	s2 "github.com/filecoin-project/go-storage-miner"
	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/events"
	"github.com/filecoin-project/lotus/chain/types"
)

type TicketFn func(context.Context) (*sectorbuilder.SealTicket, error)

type StorageMinerNodeAdapter struct {
	api    storageMinerApi
	events *events.Events

	maddr address.Address
	waddr address.Address

	tktFn TicketFn
}

func NewStorageMinerNodeAdapter(n storageMinerApi, evts *events.Events, maddr, waddr address.Address, tktFn TicketFn) *StorageMinerNodeAdapter {
	return &StorageMinerNodeAdapter{api: n, events: evts, tktFn: tktFn, maddr: maddr, waddr: waddr}
}

func (m *StorageMinerNodeAdapter) SendSelfDeals(ctx context.Context, pieces ...s2.PieceInfo) (cid.Cid, error) {
	deals := make([]actors.StorageDealProposal, len(pieces))
	for i, p := range pieces {
		sdp := actors.StorageDealProposal{
			PieceRef:             p.CommP[:],
			PieceSize:            p.Size,
			Client:               m.waddr,
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
		return cid.Undef, xerrors.Errorf("serializing PublishStorageDeals params failed: ", aerr)
	}

	smsg, err := m.api.MpoolPushMessage(ctx, &types.Message{
		To:       actors.StorageMarketAddress,
		From:     m.waddr,
		Value:    types.NewInt(0),
		GasPrice: types.NewInt(0),
		GasLimit: types.NewInt(1000000),
		Method:   actors.SMAMethods.PublishStorageDeals,
		Params:   params,
	})
	if err != nil {
		return cid.Undef, err
	}

	return smsg.Cid(), nil
}

func (m *StorageMinerNodeAdapter) WaitForSelfDeals(ctx context.Context, publicStorageDealsMsgCid cid.Cid) ([]uint64, uint8, error) {
	r, err := m.api.StateWaitMsg(ctx, publicStorageDealsMsgCid)
	if err != nil {
		return nil, 0, err
	}

	if r.Receipt.ExitCode != 0 {
		return nil, 0, nil
	}

	var resp actors.PublishStorageDealResponse
	if err := resp.UnmarshalCBOR(bytes.NewReader(r.Receipt.Return)); err != nil {
		return nil, 0, err
	}

	return resp.DealIDs, r.Receipt.ExitCode, nil
}

func (m *StorageMinerNodeAdapter) SendPreCommitSector(ctx context.Context, sectorID uint64, commR []byte, ticket s2.SealTicket, pieces ...s2.Piece) (cid.Cid, error) {
	dealIDs := make([]uint64, len(pieces))
	for idx, p := range pieces {
		dealIDs[idx] = p.DealID
	}

	params := &actors.SectorPreCommitInfo{
		SectorNumber: sectorID,
		CommR:        commR,
		SealEpoch:    ticket.BlockHeight,
		DealIDs:      dealIDs,
	}
	enc, aerr := actors.SerializeParams(params)
	if aerr != nil {
		return cid.Undef, xerrors.Errorf("could not serialize commit sector parameters: %w", aerr)
	}

	msg := &types.Message{
		To:       m.maddr,
		From:     m.waddr,
		Method:   actors.MAMethods.PreCommitSector,
		Params:   enc,
		Value:    types.NewInt(0), // TODO: need to ensure sufficient collateral
		GasLimit: types.NewInt(1000000 /* i dont know help */),
		GasPrice: types.NewInt(1),
	}

	log.Info("submitting precommit for sector: ", sectorID)
	smsg, err := m.api.MpoolPushMessage(ctx, msg)
	if err != nil {
		return cid.Undef, xerrors.Errorf("pushing message to mpool: %w", err)
	}

	return smsg.Cid(), nil
}

func (m *StorageMinerNodeAdapter) SendProveCommitSector(ctx context.Context, sectorID uint64, proof []byte, dealIDs ...uint64) (cid.Cid, error) {
	// TODO: Consider splitting states and persist proof for faster recovery
	params := &actors.SectorProveCommitInfo{
		Proof:    proof,
		SectorID: sectorID,
		DealIDs:  dealIDs,
	}

	enc, aerr := actors.SerializeParams(params)
	if aerr != nil {
		return cid.Undef, xerrors.Errorf("could not serialize commit sector parameters: %w", aerr)
	}

	msg := &types.Message{
		To:       m.maddr,
		From:     m.waddr,
		Method:   actors.MAMethods.ProveCommitSector,
		Params:   enc,
		Value:    types.NewInt(0), // TODO: need to ensure sufficient collateral
		GasLimit: types.NewInt(1000000 /* i dont know help */),
		GasPrice: types.NewInt(1),
	}

	// TODO: check seed / ticket are up to date

	smsg, err := m.api.MpoolPushMessage(ctx, msg)
	if err != nil {
		return cid.Undef, xerrors.Errorf("pushing message to mpool: %w", err)
	}

	return smsg.Cid(), nil
}

func (m *StorageMinerNodeAdapter) WaitForProveCommitSector(ctx context.Context, proveCommitSectorMsgCid cid.Cid) (uint8, error) {
	mw, err := m.api.StateWaitMsg(ctx, proveCommitSectorMsgCid)
	if err != nil {
		return 0, err
	}

	return mw.Receipt.ExitCode, nil
}

func (m *StorageMinerNodeAdapter) SendReportFaults(ctx context.Context, sectorIDs ...uint64) (cid.Cid, error) {
	// TODO: check if the fault has already been reported, and that this sector is even valid

	bf := types.NewBitField()
	for _, id := range sectorIDs {
		bf.Set(id)
	}

	enc, aerr := actors.SerializeParams(&actors.DeclareFaultsParams{bf})
	if aerr != nil {
		return cid.Undef, xerrors.Errorf("failed to serialize declare fault params: %w", aerr)
	}

	msg := &types.Message{
		To:       m.maddr,
		From:     m.waddr,
		Method:   actors.MAMethods.DeclareFaults,
		Params:   enc,
		Value:    types.NewInt(0), // TODO: need to ensure sufficient collateral
		GasLimit: types.NewInt(1000000 /* i dont know help */),
		GasPrice: types.NewInt(1),
	}

	smsg, err := m.api.MpoolPushMessage(ctx, msg)
	if err != nil {
		return cid.Undef, xerrors.Errorf("failed to push declare faults message to network: %w", err)
	}

	return smsg.Cid(), nil
}

func (m *StorageMinerNodeAdapter) WaitForReportFaults(ctx context.Context, declareFaultsMsgCid cid.Cid) (uint8, error) {
	mw, err := m.api.StateWaitMsg(ctx, declareFaultsMsgCid)
	if err != nil {
		return 0, err
	}

	return mw.Receipt.ExitCode, nil
}

func (m *StorageMinerNodeAdapter) GetSealTicket(ctx context.Context) (s2.SealTicket, error) {
	ticket, err := m.tktFn(ctx)
	if err != nil {
		return s2.SealTicket{}, xerrors.Errorf("getting ticket failed: %w", err)
	}

	return s2.SealTicket{
		BlockHeight: ticket.BlockHeight,
		TicketBytes: ticket.TicketBytes[:],
	}, nil
}

func (m *StorageMinerNodeAdapter) GetReplicaCommitmentByID(ctx context.Context, sectorID uint64) (commR []byte, wasFound bool, err error) {
	act, err := m.api.StateGetActor(ctx, m.maddr, nil)
	if err != nil {
		return nil, true, err
	}

	st, err := m.api.ChainReadObj(ctx, act.Head)
	if err != nil {
		return nil, true, err
	}

	var state actors.StorageMinerActorState
	if err := state.UnmarshalCBOR(bytes.NewReader(st)); err != nil {
		return nil, true, xerrors.Errorf("unmarshaling miner state: %+v", err)
	}

	pci, found := state.PreCommittedSectors[fmt.Sprint(sectorID)]
	if found {
		// TODO: If not expired yet, we can just try reusing sealticket
		return pci.Info.CommR, true, nil
	}

	return nil, false, nil
}

func (m *StorageMinerNodeAdapter) GetSealSeed(ctx context.Context, preCommitMsgCid cid.Cid, interval uint64) (<-chan s2.SealSeed, <-chan s2.SeedInvalidated, <-chan s2.FinalityReached, <-chan *s2.GetSealSeedError) {
	ready := make(chan s2.SealSeed)
	invalidated := make(chan s2.SeedInvalidated)
	finalized := make(chan s2.FinalityReached)
	errs := make(chan *s2.GetSealSeedError)

	// would be ideal to just use the events.Called handler, but it wouldnt be able to handle individual message timeouts

	go func() {
		mw, err := m.api.StateWaitMsg(ctx, preCommitMsgCid)
		if err != nil {
			err = xerrors.Errorf("failed to wait for pre-commit message: %+v", err)
			errs <- s2.NewGetSealSeedError(err, s2.GetSealSeedFailedError)
			return
		}

		if mw.Receipt.ExitCode != 0 {
			log.Error("sector precommit failed: ", mw.Receipt.ExitCode)
			err := xerrors.Errorf("sector precommit failed: %d", mw.Receipt.ExitCode)
			errs <- s2.NewGetSealSeedError(err, s2.GetSealSeedFailedError)
			return
		}

		n := mw.TipSet.Height() + build.InteractivePoRepDelay

		// waiting for a seed to become available, e.g. mined into a block at
		// the provided confidence level-block height
		err = m.events.ChainAt(func(ectx context.Context, ts *types.TipSet, curH uint64) error {
			randHeight := n - 1 // -1 because of how the messages are applied
			rand, err := m.api.ChainGetRandomness(ectx, ts.Key(), int64(randHeight))
			if err != nil {
				err = xerrors.Errorf("failed to get randomness for computing seal proof: %w", err)
				errs <- s2.NewGetSealSeedError(err, s2.GetSealSeedFatalError)
				return err
			}

			ready <- s2.SealSeed{
				BlockHeight: randHeight,
				TicketBytes: rand,
			}

			return nil
		}, func(ctx context.Context, ts *types.TipSet) error {
			log.Warn("revert in interactive commit sector step")
			// TODO: need to cancel running process and restart...
			invalidated <- s2.SeedInvalidated{}

			return nil
		}, build.InteractivePoRepConfidence, n)
		if err != nil {
			log.Warn("waitForPreCommitMessage ChainAt errored: ", err)
		}

		// waiting for the seed to become final, i.e. mined into a block at
		// finality
		err = m.events.ChainAt(func(ectx context.Context, ts *types.TipSet, curH uint64) error {
			finalized <- s2.FinalityReached{}
			return nil
		}, func(ctx context.Context, ts *types.TipSet) error {
			// noop
			return nil
		}, build.Finality, n)

		if err != nil {
			log.Warn("waitForPreCommitMessage ChainAt errored: ", err)
		}
	}()

	return ready, invalidated, finalized, errs
}

func (m *StorageMinerNodeAdapter) CheckPieces(ctx context.Context, sectorID uint64, pieces []s2.Piece) *s2.CheckPiecesError {
	head, err := m.api.ChainHead(ctx)
	if err != nil {
		err = xerrors.Errorf("getting chain head: %w", err)
		return s2.NewCheckPiecesError(err, s2.CheckPiecesAPI)
	}
	for i, piece := range pieces {
		deal, err := m.api.StateMarketStorageDeal(ctx, piece.DealID, nil)
		if err != nil {
			err = xerrors.Errorf("getting deal %d for piece %d: %w", piece.DealID, i, err)
			return s2.NewCheckPiecesError(err, s2.CheckPiecesAPI)
		}
		if string(deal.PieceRef) != string(piece.CommP) {
			err := xerrors.Errorf("piece %d (or %d) of sector %d refers deal %d with wrong CommP: %x != %x", i, len(pieces), sectorID, piece.DealID, piece.CommP, deal.PieceRef)
			return s2.NewCheckPiecesError(err, s2.CheckPiecesInvalidDeals)
		}
		if piece.Size != deal.PieceSize {
			err := xerrors.Errorf("piece %d (or %d) of sector %d refers deal %d with different size: %d != %d", i, len(pieces), sectorID, piece.DealID, piece.Size, deal.PieceSize)
			return s2.NewCheckPiecesError(err, s2.CheckPiecesInvalidDeals)
		}
		if head.Height() >= deal.ProposalExpiration {
			err := xerrors.Errorf("piece %d (or %d) of sector %d refers expired deal %d - expires %d, head %d", i, len(pieces), sectorID, piece.DealID, deal.ProposalExpiration, head.Height())
			return s2.NewCheckPiecesError(err, s2.CheckPiecesExpiredDeals)
		}
	}
	return nil
}

func (m *StorageMinerNodeAdapter) CheckSealing(ctx context.Context, commD []byte, dealIDs []uint64, ticket s2.SealTicket) *s2.CheckSealingError {
	head, err := m.api.ChainHead(ctx)
	if err != nil {
		err = xerrors.Errorf("getting chain head: %w", err)
		return s2.NewCheckSealingError(err, s2.CheckSealingAPI)
	}

	ssize, err := m.api.StateMinerSectorSize(ctx, m.maddr, head)
	if err != nil {
		err = xerrors.Errorf("getting miner sector size: %w", err)
		return s2.NewCheckSealingError(err, s2.CheckSealingAPI)
	}

	ccparams, err := actors.SerializeParams(&actors.ComputeDataCommitmentParams{
		DealIDs:    dealIDs,
		SectorSize: ssize,
	})
	if err != nil {
		err = xerrors.Errorf("computing params for ComputeDataCommitment: %w", err)
		return s2.NewCheckSealingError(err, s2.CheckSealingAPI)
	}

	ccmt := &types.Message{
		To:       actors.StorageMarketAddress,
		From:     m.maddr,
		Value:    types.NewInt(0),
		GasPrice: types.NewInt(0),
		GasLimit: types.NewInt(9999999999),
		Method:   actors.SMAMethods.ComputeDataCommitment,
		Params:   ccparams,
	}
	r, err := m.api.StateCall(ctx, ccmt, nil)
	if err != nil {
		err = xerrors.Errorf("calling ComputeDataCommitment: %w", err)
		return s2.NewCheckSealingError(err, s2.CheckSealingAPI)
	}

	if r.ExitCode != 0 {
		err := xerrors.Errorf("receipt for ComputeDataCommitment had exit code %d", r.ExitCode)
		return s2.NewCheckSealingError(err, s2.CheckSealingBadCommD)
	}

	if string(r.Return) != string(commD) {
		err := xerrors.Errorf("on chain CommD differs from sector: %x != %x", r.Return, commD)
		return s2.NewCheckSealingError(err, s2.CheckSealingBadCommD)
	}

	if int64(head.Height())-int64(ticket.BlockHeight+build.SealRandomnessLookback) > build.SealRandomnessLookbackLimit {
		err := xerrors.Errorf("ticket expired: seal height: %d, head: %d", ticket.BlockHeight+build.SealRandomnessLookback, head.Height())
		return s2.NewCheckSealingError(err, s2.CheckSealingExpiredTicket)
	}

	return nil
}

func (m *StorageMinerNodeAdapter) WalletHas(ctx context.Context, addr address.Address) (bool, error) {
	return m.api.WalletHas(ctx, addr)
}
