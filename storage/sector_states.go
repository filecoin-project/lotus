package storage

import (
	"context"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/lib/sectorbuilder"
)

type providerHandlerFunc func(ctx context.Context, deal SectorInfo) (func(*SectorInfo), error)

func (m *Miner) handle(ctx context.Context, sector SectorInfo, cb providerHandlerFunc, next api.SectorState) {
	go func() {
		mut, err := cb(ctx, sector)

		if err == nil && next == api.SectorNoUpdate {
			return
		}

		select {
		case m.sectorUpdated <- sectorUpdate{
			newState: next,
			id:       sector.SectorID,
			err:      err,
			mut:      mut,
		}:
		case <-m.stop:
		}
	}()
}

func (m *Miner) finishPacking(ctx context.Context, sector SectorInfo) (func(*SectorInfo), error) {
	log.Infow("performing filling up rest of the sector...", "sector", sector.SectorID)

	var allocated uint64
	for _, piece := range sector.Pieces {
		allocated += piece.Size
	}

	ubytes := sectorbuilder.UserBytesForSectorSize(m.sb.SectorSize())

	if allocated > ubytes {
		return nil, xerrors.Errorf("too much data in sector: %d > %d", allocated, ubytes)
	}

	fillerSizes, err := fillersFromRem(ubytes - allocated)
	if err != nil {
		return nil, err
	}

	if len(fillerSizes) > 0 {
		log.Warnf("Creating %d filler pieces for sector %d", len(fillerSizes), sector.SectorID)
	}

	pieces, err := m.storeGarbage(ctx, sector.SectorID, sector.existingPieces(), fillerSizes...)
	if err != nil {
		return nil, xerrors.Errorf("filling up the sector (%v): %w", fillerSizes, err)
	}

	return func(info *SectorInfo) {
		info.Pieces = append(info.Pieces, pieces...)
	}, nil
}

func (m *Miner) sealPreCommit(ctx context.Context, sector SectorInfo) (func(*SectorInfo), error) {
	log.Infow("performing sector replication...", "sector", sector.SectorID)
	ticket, err := m.tktFn(ctx)
	if err != nil {
		return nil, err
	}

	rspco, err := m.sb.SealPreCommit(sector.SectorID, *ticket, sector.pieceInfos())
	if err != nil {
		return nil, xerrors.Errorf("seal pre commit failed: %w", err)
	}

	return func(info *SectorInfo) {
		info.CommC = rspco.CommC[:]
		info.CommD = rspco.CommD[:]
		info.CommR = rspco.CommR[:]
		info.CommRLast = rspco.CommRLast[:]
		info.Ticket = SealTicket{
			BlockHeight: ticket.BlockHeight,
			TicketBytes: ticket.TicketBytes[:],
		}
	}, nil
}

func (m *Miner) preCommit(ctx context.Context, sector SectorInfo) (func(*SectorInfo), error) {
	params := &actors.SectorPreCommitInfo{
		SectorNumber: sector.SectorID,

		CommR:     sector.CommR,
		SealEpoch: sector.Ticket.BlockHeight,
		DealIDs:   sector.deals(),
	}
	enc, aerr := actors.SerializeParams(params)
	if aerr != nil {
		return nil, xerrors.Errorf("could not serialize commit sector parameters: %w", aerr)
	}

	msg := &types.Message{
		To:       m.maddr,
		From:     m.worker,
		Method:   actors.MAMethods.PreCommitSector,
		Params:   enc,
		Value:    types.NewInt(0), // TODO: need to ensure sufficient collateral
		GasLimit: types.NewInt(1000000 /* i dont know help */),
		GasPrice: types.NewInt(1),
	}

	log.Info("submitting precommit for sector: ", sector.SectorID)
	smsg, err := m.api.MpoolPushMessage(ctx, msg)
	if err != nil {
		return nil, xerrors.Errorf("pushing message to mpool: %w", err)
	}

	return func(info *SectorInfo) {
		mcid := smsg.Cid()
		info.PreCommitMessage = &mcid
	}, nil
}

func (m *Miner) preCommitted(ctx context.Context, sector SectorInfo) (func(*SectorInfo), error) {
	// would be ideal to just use the events.Called handler, but it wouldnt be able to handle individual message timeouts
	log.Info("Sector precommitted: ", sector.SectorID)
	mw, err := m.api.StateWaitMsg(ctx, *sector.PreCommitMessage)
	if err != nil {
		return nil, err
	}

	if mw.Receipt.ExitCode != 0 {
		log.Error("sector precommit failed: ", mw.Receipt.ExitCode)
		return nil, err
	}
	log.Info("precommit message landed on chain: ", sector.SectorID)

	randHeight := mw.TipSet.Height() + build.InteractivePoRepDelay - 1 // -1 because of how the messages are applied
	log.Infof("precommit for sector %d made it on chain, will start proof computation at height %d", sector.SectorID, randHeight)

	err = m.events.ChainAt(func(ctx context.Context, ts *types.TipSet, curH uint64) error {
		rand, err := m.api.ChainGetRandomness(ctx, ts.Key(), nil, int(ts.Height()-randHeight))
		if err != nil {
			return xerrors.Errorf("failed to get randomness for computing seal proof: %w", err)
		}

		m.sectorUpdated <- sectorUpdate{
			newState: api.Committing,
			id:       sector.SectorID,
			mut: func(info *SectorInfo) {
				info.Seed = SealSeed{
					BlockHeight: randHeight,
					TicketBytes: rand,
				}
			},
		}

		return nil
	}, func(ctx context.Context, ts *types.TipSet) error {
		log.Warn("revert in interactive commit sector step")
		return nil
	}, 3, mw.TipSet.Height()+build.InteractivePoRepDelay)
	if err != nil {
		log.Warn("waitForPreCommitMessage ChainAt errored: ", err)
	}

	return nil, nil
}

func (m *Miner) committing(ctx context.Context, sector SectorInfo) (func(*SectorInfo), error) {
	log.Info("scheduling seal proof computation...")

	proof, err := m.sb.SealCommit(sector.SectorID, sector.Ticket.SB(), sector.Seed.SB(), sector.pieceInfos(), sector.refs(), sector.rspco())
	if err != nil {
		return nil, xerrors.Errorf("computing seal proof failed: %w", err)
	}

	// TODO: Consider splitting states and persist proof for faster recovery

	params := &actors.SectorProveCommitInfo{
		Proof:    proof,
		SectorID: sector.SectorID,
		DealIDs:  sector.deals(),
	}

	enc, aerr := actors.SerializeParams(params)
	if aerr != nil {
		return nil, xerrors.Errorf("could not serialize commit sector parameters: %w", aerr)
	}

	msg := &types.Message{
		To:       m.maddr,
		From:     m.worker,
		Method:   actors.MAMethods.ProveCommitSector,
		Params:   enc,
		Value:    types.NewInt(0), // TODO: need to ensure sufficient collateral
		GasLimit: types.NewInt(1000000 /* i dont know help */),
		GasPrice: types.NewInt(1),
	}

	smsg, err := m.api.MpoolPushMessage(ctx, msg)
	if err != nil {
		log.Error(xerrors.Errorf("pushing message to mpool: %w", err))
	}

	// TODO: Separate state before this wait, so we persist message cid?

	mw, err := m.api.StateWaitMsg(ctx, smsg.Cid())
	if err != nil {
		return nil, xerrors.Errorf("failed to wait for porep inclusion: %w", err)
	}

	if mw.Receipt.ExitCode != 0 {
		log.Errorf("UNHANDLED: submitting sector proof failed (exit=%d, msg=%s) (t:%x; s:%x(%d); p:%x)", mw.Receipt.ExitCode, smsg.Cid(), sector.Ticket.TicketBytes, sector.Seed.TicketBytes, sector.Seed.BlockHeight, params.Proof)
		return nil, xerrors.New("UNHANDLED: submitting sector proof failed")
	}

	m.beginPosting(ctx)

	return func(info *SectorInfo) {
		mcid := smsg.Cid()
		info.CommitMessage = &mcid
		info.Proof = proof
	}, nil
}
