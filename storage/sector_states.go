package storage

import (
	"bytes"
	"context"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/lib/sectorbuilder"
)

type providerHandlerFunc func(ctx context.Context, deal SectorInfo) *sectorUpdate

func (m *Miner) handleSectorUpdate(ctx context.Context, sector SectorInfo, cb providerHandlerFunc) {
	go func() {
		update := cb(ctx, sector)

		if update == nil {
			return // async
		}

		select {
		case m.sectorUpdated <- *update:
		case <-m.stop:
		}
	}()
}

func (m *Miner) handlePacking(ctx context.Context, sector SectorInfo) *sectorUpdate {
	log.Infow("performing filling up rest of the sector...", "sector", sector.SectorID)

	var allocated uint64
	for _, piece := range sector.Pieces {
		allocated += piece.Size
	}

	ubytes := sectorbuilder.UserBytesForSectorSize(m.sb.SectorSize())

	if allocated > ubytes {
		return sector.upd().fatal(xerrors.Errorf("too much data in sector: %d > %d", allocated, ubytes))
	}

	fillerSizes, err := fillersFromRem(ubytes - allocated)
	if err != nil {
		return sector.upd().fatal(err)
	}

	if len(fillerSizes) > 0 {
		log.Warnf("Creating %d filler pieces for sector %d", len(fillerSizes), sector.SectorID)
	}

	pieces, err := m.pledgeSector(ctx, sector.SectorID, sector.existingPieces(), fillerSizes...)
	if err != nil {
		return sector.upd().fatal(xerrors.Errorf("filling up the sector (%v): %w", fillerSizes, err))
	}

	return sector.upd().to(api.Unsealed).state(func(info *SectorInfo) {
		info.Pieces = append(info.Pieces, pieces...)
	})
}

func (m *Miner) handleUnsealed(ctx context.Context, sector SectorInfo) *sectorUpdate {
	log.Infow("performing sector replication...", "sector", sector.SectorID)
	ticket, err := m.tktFn(ctx)
	if err != nil {
		return sector.upd().fatal(err)
	}

	rspco, err := m.sb.SealPreCommit(sector.SectorID, *ticket, sector.pieceInfos())
	if err != nil {
		return sector.upd().to(api.SealFailed).error(xerrors.Errorf("seal pre commit failed: %w", err))
	}

	return sector.upd().to(api.PreCommitting).state(func(info *SectorInfo) {
		info.CommD = rspco.CommD[:]
		info.CommR = rspco.CommR[:]
		info.Ticket = SealTicket{
			BlockHeight: ticket.BlockHeight,
			TicketBytes: ticket.TicketBytes[:],
		}
	})
}

func (m *Miner) handlePreCommitting(ctx context.Context, sector SectorInfo) *sectorUpdate {
	params := &actors.SectorPreCommitInfo{
		SectorNumber: sector.SectorID,

		CommR:     sector.CommR,
		SealEpoch: sector.Ticket.BlockHeight,
		DealIDs:   sector.deals(),
	}
	enc, aerr := actors.SerializeParams(params)
	if aerr != nil {
		return sector.upd().to(api.PreCommitFailed).error(xerrors.Errorf("could not serialize commit sector parameters: %w", aerr))
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
		return sector.upd().to(api.PreCommitFailed).error(xerrors.Errorf("pushing message to mpool: %w", err))
	}

	return sector.upd().to(api.PreCommitted).state(func(info *SectorInfo) {
		mcid := smsg.Cid()
		info.PreCommitMessage = &mcid
	})
}

func (m *Miner) handlePreCommitted(ctx context.Context, sector SectorInfo) *sectorUpdate {
	// would be ideal to just use the events.Called handler, but it wouldnt be able to handle individual message timeouts
	log.Info("Sector precommitted: ", sector.SectorID)
	mw, err := m.api.StateWaitMsg(ctx, *sector.PreCommitMessage)
	if err != nil {
		return sector.upd().to(api.PreCommitFailed).error(err)
	}

	if mw.Receipt.ExitCode != 0 {
		log.Error("sector precommit failed: ", mw.Receipt.ExitCode)
		err := xerrors.Errorf("sector precommit failed: %d", mw.Receipt.ExitCode)
		return sector.upd().to(api.PreCommitFailed).error(err)
	}
	log.Info("precommit message landed on chain: ", sector.SectorID)

	randHeight := mw.TipSet.Height() + build.InteractivePoRepDelay - 1 // -1 because of how the messages are applied
	log.Infof("precommit for sector %d made it on chain, will start proof computation at height %d", sector.SectorID, randHeight)

	updateNonce := sector.Nonce

	err = m.events.ChainAt(func(ctx context.Context, ts *types.TipSet, curH uint64) error {
		rand, err := m.api.ChainGetRandomness(ctx, ts.Key(), int64(randHeight))
		if err != nil {
			err = xerrors.Errorf("failed to get randomness for computing seal proof: %w", err)

			m.sectorUpdated <- *sector.upd().fatal(err)
			return err
		}

		m.sectorUpdated <- *sector.upd().to(api.Committing).setNonce(updateNonce).state(func(info *SectorInfo) {
			info.Seed = SealSeed{
				BlockHeight: randHeight,
				TicketBytes: rand,
			}
		})

		updateNonce++

		return nil
	}, func(ctx context.Context, ts *types.TipSet) error {
		log.Warn("revert in interactive commit sector step")
		// TODO: need to cancel running process and restart...
		return nil
	}, build.InteractivePoRepConfidence, mw.TipSet.Height()+build.InteractivePoRepDelay)
	if err != nil {
		log.Warn("waitForPreCommitMessage ChainAt errored: ", err)
	}

	return nil
}

func (m *Miner) handleCommitting(ctx context.Context, sector SectorInfo) *sectorUpdate {
	ok, err := m.sb.CanCommit(sector.SectorID)
	if err != nil {
		return sector.upd().fatal(xerrors.Errorf("canCommit check failed: %w", err))
	}
	if !ok {
		log.Infof("sector %d in invalid state for commit, attempting reseal", sector.SectorID)
		return sector.upd().to(api.Reseal)
	}

	log.Info("scheduling seal proof computation...")

	proof, err := m.sb.SealCommit(sector.SectorID, sector.Ticket.SB(), sector.Seed.SB(), sector.pieceInfos(), sector.rspco())
	if err != nil {
		return sector.upd().to(api.SealCommitFailed).error(xerrors.Errorf("computing seal proof failed: %w", err))
	}

	// TODO: Consider splitting states and persist proof for faster recovery

	params := &actors.SectorProveCommitInfo{
		Proof:    proof,
		SectorID: sector.SectorID,
		DealIDs:  sector.deals(),
	}

	enc, aerr := actors.SerializeParams(params)
	if aerr != nil {
		return sector.upd().to(api.CommitFailed).error(xerrors.Errorf("could not serialize commit sector parameters: %w", aerr))
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
		return sector.upd().to(api.CommitFailed).error(xerrors.Errorf("pushing message to mpool: %w", err))
	}

	// TODO: Separate state before this wait, so we persist message cid?
	return sector.upd().to(api.CommitWait).state(func(info *SectorInfo) {
		mcid := smsg.Cid()
		info.CommitMessage = &mcid
		info.Proof = proof
	})
}

func (m *Miner) handleCommitWait(ctx context.Context, sector SectorInfo) *sectorUpdate {
	if sector.CommitMessage == nil {
		log.Errorf("sector %d entered commit wait state without a message cid", sector.SectorID)
		return sector.upd().to(api.CommitFailed).error(xerrors.Errorf("entered commit wait with no commit cid"))
	}

	mw, err := m.api.StateWaitMsg(ctx, *sector.CommitMessage)
	if err != nil {
		return sector.upd().to(api.CommitFailed).error(xerrors.Errorf("failed to wait for porep inclusion: %w", err))
	}

	if mw.Receipt.ExitCode != 0 {
		log.Errorf("UNHANDLED: submitting sector proof failed (exit=%d, msg=%s) (t:%x; s:%x(%d); p:%x)", mw.Receipt.ExitCode, sector.CommitMessage, sector.Ticket.TicketBytes, sector.Seed.TicketBytes, sector.Seed.BlockHeight, sector.Proof)
		return sector.upd().fatal(xerrors.Errorf("UNHANDLED: submitting sector proof failed (exit: %d)", mw.Receipt.ExitCode))
	}

	return sector.upd().to(api.Proving)
}

func (m *Miner) handleFaulty(ctx context.Context, sector SectorInfo) *sectorUpdate {
	// TODO: check if the fault has already been reported, and that this sector is even valid

	// TODO: coalesce faulty sector reporting
	bf := types.NewBitField()
	bf.Set(sector.SectorID)

	fp := &actors.DeclareFaultsParams{bf}
	_ = fp
	enc, aerr := actors.SerializeParams(nil)
	if aerr != nil {
		return sector.upd().fatal(xerrors.Errorf("failed to serialize declare fault params: %w", aerr))
	}

	msg := &types.Message{
		To:       m.maddr,
		From:     m.worker,
		Method:   actors.MAMethods.DeclareFaults,
		Params:   enc,
		Value:    types.NewInt(0), // TODO: need to ensure sufficient collateral
		GasLimit: types.NewInt(1000000 /* i dont know help */),
		GasPrice: types.NewInt(1),
	}

	smsg, err := m.api.MpoolPushMessage(ctx, msg)
	if err != nil {
		return sector.upd().to(api.FailedUnrecoverable).error(xerrors.Errorf("failed to push declare faults message to network: %w", err))
	}

	return sector.upd().to(api.FaultReported).state(func(info *SectorInfo) {
		c := smsg.Cid()
		info.FaultReportMsg = &c
	})
}

func (m *Miner) handleFaultReported(ctx context.Context, sector SectorInfo) *sectorUpdate {
	if sector.FaultReportMsg == nil {
		return sector.upd().to(api.FailedUnrecoverable).error(xerrors.Errorf("entered fault reported state without a FaultReportMsg cid"))
	}

	mw, err := m.api.StateWaitMsg(ctx, *sector.FaultReportMsg)
	if err != nil {
		return sector.upd().to(api.CommitFailed).error(xerrors.Errorf("failed to wait for fault declaration: %w", err))
	}

	if mw.Receipt.ExitCode != 0 {
		log.Errorf("UNHANDLED: declaring sector fault failed (exit=%d, msg=%s) (id: %d)", mw.Receipt.ExitCode, *sector.FaultReportMsg, sector.SectorID)
		return sector.upd().fatal(xerrors.Errorf("UNHANDLED: submitting fault declaration failed (exit %d)", mw.Receipt.ExitCode))
	}

	return sector.upd().to(api.FaultedFinal).state(func(info *SectorInfo) {})

}

func (m *Miner) handleReseal(ctx context.Context, sector SectorInfo) *sectorUpdate {
	log.Infow("performing sector replication again...", "sector", sector.SectorID)

	if err := m.sb.CleanupFailedData(sector.SectorID); err != nil {
		return sector.upd().to(api.SealFailed).error(xerrors.Errorf("reseal cleanup failed: %w", err))
	}

	rspco, err := m.sb.SealPreCommit(sector.SectorID, sector.Ticket.SB(), sector.pieceInfos())
	if err != nil {
		return sector.upd().to(api.SealFailed).error(xerrors.Errorf("reseal failed: %w", err))
	}

	if !bytes.Equal(rspco.CommD[:], sector.CommD) {
		return sector.upd().fatal(xerrors.Errorf("recomputed CommD didn't match: %v --- %v", rspco.CommD[:], sector.CommD))
	}

	if !bytes.Equal(rspco.CommR[:], sector.CommR) {
		return sector.upd().fatal(xerrors.Errorf("recomputed CommR didn't match: %v --- %v", rspco.CommR[:], sector.CommR))
	}

	return sector.upd().to(api.Committing)
}
