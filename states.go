package sealing

import (
	"bytes"
	"context"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-statemachine"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/abi/big"
	"github.com/filecoin-project/specs-actors/actors/builtin"
	"github.com/filecoin-project/specs-actors/actors/builtin/miner"
	"github.com/filecoin-project/specs-actors/actors/crypto"
	"github.com/filecoin-project/specs-storage/storage"
)

func (m *Sealing) handlePacking(ctx statemachine.Context, sector SectorInfo) error {
	log.Infow("performing filling up rest of the sector...", "sector", sector.SectorNumber)

	var allocated abi.UnpaddedPieceSize
	for _, piece := range sector.Pieces {
		allocated += piece.Piece.Size.Unpadded()
	}

	ubytes := abi.PaddedPieceSize(m.sealer.SectorSize()).Unpadded()

	if allocated > ubytes {
		return xerrors.Errorf("too much data in sector: %d > %d", allocated, ubytes)
	}

	fillerSizes, err := fillersFromRem(ubytes - allocated)
	if err != nil {
		return err
	}

	if len(fillerSizes) > 0 {
		log.Warnf("Creating %d filler pieces for sector %d", len(fillerSizes), sector.SectorNumber)
	}

	fillerPieces, err := m.pledgeSector(ctx.Context(), m.minerSector(sector.SectorNumber), sector.existingPieceSizes(), fillerSizes...)
	if err != nil {
		return xerrors.Errorf("filling up the sector (%v): %w", fillerSizes, err)
	}

	return ctx.Send(SectorPacked{FillerPieces: fillerPieces})
}

func (m *Sealing) handlePreCommit1(ctx statemachine.Context, sector SectorInfo) error {
	tok, epoch, err := m.api.ChainHead(ctx.Context())
	if err != nil {
		log.Errorf("handlePreCommit1: api error, not proceeding: %+v", err)
		return nil
	}

	if err := checkPieces(ctx.Context(), sector, m.api); err != nil { // Sanity check state
		switch err.(type) {
		case *ErrApi:
			log.Errorf("handlePreCommit1: api error, not proceeding: %+v", err)
			return nil
		case *ErrInvalidDeals:
			return ctx.Send(SectorPackingFailed{xerrors.Errorf("invalid dealIDs in sector: %w", err)})
		case *ErrExpiredDeals: // Probably not much we can do here, maybe re-pack the sector?
			return ctx.Send(SectorPackingFailed{xerrors.Errorf("expired dealIDs in sector: %w", err)})
		default:
			return xerrors.Errorf("checkPieces sanity check error: %w", err)
		}
	}

	log.Infow("performing sector replication...", "sector", sector.SectorNumber)
	ticketEpoch := epoch - miner.ChainFinalityish
	buf := new(bytes.Buffer)
	if err := m.maddr.MarshalCBOR(buf); err != nil {
		return err
	}
	rand, err := m.api.ChainGetRandomness(ctx.Context(), tok, crypto.DomainSeparationTag_SealRandomness, ticketEpoch, buf.Bytes())
	if err != nil {
		return ctx.Send(SectorSealPreCommitFailed{xerrors.Errorf("getting ticket failed: %w", err)})
	}
	ticketValue := abi.SealRandomness(rand)

	pc1o, err := m.sealer.SealPreCommit1(ctx.Context(), m.minerSector(sector.SectorNumber), ticketValue, sector.pieceInfos())
	if err != nil {
		return ctx.Send(SectorSealPreCommitFailed{xerrors.Errorf("seal pre commit(1) failed: %w", err)})
	}

	return ctx.Send(SectorPreCommit1{
		PreCommit1Out: pc1o,
		TicketValue:   ticketValue,
		TicketEpoch:   ticketEpoch,
	})
}

func (m *Sealing) handlePreCommit2(ctx statemachine.Context, sector SectorInfo) error {
	cids, err := m.sealer.SealPreCommit2(ctx.Context(), m.minerSector(sector.SectorNumber), sector.PreCommit1Out)
	if err != nil {
		return ctx.Send(SectorSealPreCommitFailed{xerrors.Errorf("seal pre commit(2) failed: %w", err)})
	}

	return ctx.Send(SectorPreCommit2{
		Unsealed: cids.Unsealed,
		Sealed:   cids.Sealed,
	})
}

func (m *Sealing) handlePreCommitting(ctx statemachine.Context, sector SectorInfo) error {
	tok, height, err := m.api.ChainHead(ctx.Context())
	if err != nil {
		log.Errorf("handlePreCommitting: api error, not proceeding: %+v", err)
		return nil
	}

	waddr, err := m.api.StateMinerWorkerAddress(ctx.Context(), m.maddr, tok)
	if err != nil {
		log.Errorf("handlePreCommitting: api error, not proceeding: %+v", err)
		return nil
	}

	if err := checkPrecommit(ctx.Context(), m.Address(), sector, tok, height, m.api); err != nil {
		switch err.(type) {
		case *ErrApi:
			log.Errorf("handlePreCommitting: api error, not proceeding: %+v", err)
			return nil
		case *ErrBadCommD: // TODO: Should this just back to packing? (not really needed since handlePreCommit1 will do that too)
			return ctx.Send(SectorSealPreCommitFailed{xerrors.Errorf("bad CommD error: %w", err)})
		case *ErrExpiredTicket:
			return ctx.Send(SectorSealPreCommitFailed{xerrors.Errorf("ticket expired: %w", err)})
		default:
			return xerrors.Errorf("checkPrecommit sanity check error: %w", err)
		}
	}

	expiration, err := m.pcp.Expiration(ctx.Context(), sector.Pieces...)
	if err != nil {
		return ctx.Send(SectorSealPreCommitFailed{xerrors.Errorf("handlePreCommitting: failed to compute pre-commit expiry: %w", err)})
	}

	params := &miner.SectorPreCommitInfo{
		Expiration:      expiration,
		SectorNumber:    sector.SectorNumber,
		RegisteredProof: sector.SectorType,

		SealedCID:     *sector.CommR,
		SealRandEpoch: sector.TicketEpoch,
		DealIDs:       sector.dealIDs(),
	}

	enc := new(bytes.Buffer)
	if err := params.MarshalCBOR(enc); err != nil {
		return ctx.Send(SectorChainPreCommitFailed{xerrors.Errorf("could not serialize pre-commit sector parameters: %w", err)})
	}

	log.Info("submitting precommit for sector: ", sector.SectorNumber)
	mcid, err := m.api.SendMsg(ctx.Context(), waddr, m.maddr, builtin.MethodsMiner.PreCommitSector, big.NewInt(0), big.NewInt(1), 1000000, enc.Bytes())
	if err != nil {
		return ctx.Send(SectorChainPreCommitFailed{xerrors.Errorf("pushing message to mpool: %w", err)})
	}

	return ctx.Send(SectorPreCommitted{Message: mcid})
}

func (m *Sealing) handleWaitSeed(ctx statemachine.Context, sector SectorInfo) error {
	// would be ideal to just use the events.Called handler, but it wouldnt be able to handle individual message timeouts
	log.Info("Sector precommitted: ", sector.SectorNumber)
	mw, err := m.api.StateWaitMsg(ctx.Context(), *sector.PreCommitMessage)
	if err != nil {
		return ctx.Send(SectorChainPreCommitFailed{err})
	}

	if mw.Receipt.ExitCode != 0 {
		log.Error("sector precommit failed: ", mw.Receipt.ExitCode)
		err := xerrors.Errorf("sector precommit failed: %d", mw.Receipt.ExitCode)
		return ctx.Send(SectorChainPreCommitFailed{err})
	}
	log.Info("precommit message landed on chain: ", sector.SectorNumber)

	pci, err := m.api.StateSectorPreCommitInfo(ctx.Context(), m.maddr, sector.SectorNumber, mw.TipSetTok)
	if err != nil {
		return xerrors.Errorf("getting precommit info: %w", err)
	}

	randHeight := pci.PreCommitEpoch + miner.PreCommitChallengeDelay

	err = m.events.ChainAt(func(ectx context.Context, tok TipSetToken, curH abi.ChainEpoch) error {
		buf := new(bytes.Buffer)
		if err := m.maddr.MarshalCBOR(buf); err != nil {
			return err
		}
		rand, err := m.api.ChainGetRandomness(ectx, tok, crypto.DomainSeparationTag_InteractiveSealChallengeSeed, randHeight, buf.Bytes())
		if err != nil {
			err = xerrors.Errorf("failed to get randomness for computing seal proof: %w", err)

			_ = ctx.Send(SectorFatalError{error: err})
			return err
		}

		_ = ctx.Send(SectorSeedReady{SeedValue: abi.InteractiveSealRandomness(rand), SeedEpoch: randHeight})

		return nil
	}, func(ctx context.Context, ts TipSetToken) error {
		log.Warn("revert in interactive commit sector step")
		// TODO: need to cancel running process and restart...
		return nil
	}, InteractivePoRepConfidence, randHeight)
	if err != nil {
		log.Warn("waitForPreCommitMessage ChainAt errored: ", err)
	}

	return nil
}

func (m *Sealing) handleCommitting(ctx statemachine.Context, sector SectorInfo) error {
	log.Info("scheduling seal proof computation...")

	log.Infof("KOMIT %d %x(%d); %x(%d); %v; r:%x; d:%x", sector.SectorNumber, sector.TicketValue, sector.TicketEpoch, sector.SeedValue, sector.SeedEpoch, sector.pieceInfos(), sector.CommR, sector.CommD)

	cids := storage.SectorCids{
		Unsealed: *sector.CommD,
		Sealed:   *sector.CommR,
	}
	c2in, err := m.sealer.SealCommit1(ctx.Context(), m.minerSector(sector.SectorNumber), sector.TicketValue, sector.SeedValue, sector.pieceInfos(), cids)
	if err != nil {
		return ctx.Send(SectorComputeProofFailed{xerrors.Errorf("computing seal proof failed: %w", err)})
	}

	proof, err := m.sealer.SealCommit2(ctx.Context(), m.minerSector(sector.SectorNumber), c2in)
	if err != nil {
		return ctx.Send(SectorComputeProofFailed{xerrors.Errorf("computing seal proof failed: %w", err)})
	}

	tok, _, err := m.api.ChainHead(ctx.Context())
	if err != nil {
		log.Errorf("handleCommitting: api error, not proceeding: %+v", err)
		return nil
	}

	if err := m.checkCommit(ctx.Context(), sector, proof, tok); err != nil {
		return ctx.Send(SectorCommitFailed{xerrors.Errorf("commit check error: %w", err)})
	}

	// TODO: Consider splitting states and persist proof for faster recovery

	params := &miner.ProveCommitSectorParams{
		SectorNumber: sector.SectorNumber,
		Proof:        proof,
	}

	enc := new(bytes.Buffer)
	if err := params.MarshalCBOR(enc); err != nil {
		return ctx.Send(SectorCommitFailed{xerrors.Errorf("could not serialize commit sector parameters: %w", err)})
	}

	waddr, err := m.api.StateMinerWorkerAddress(ctx.Context(), m.maddr, tok)
	if err != nil {
		log.Errorf("handleCommitting: api error, not proceeding: %+v", err)
		return nil
	}

	// TODO: check seed / ticket are up to date
	mcid, err := m.api.SendMsg(ctx.Context(), waddr, m.maddr, builtin.MethodsMiner.ProveCommitSector, big.NewInt(0), big.NewInt(1), 1000000, enc.Bytes())
	if err != nil {
		return ctx.Send(SectorCommitFailed{xerrors.Errorf("pushing message to mpool: %w", err)})
	}

	return ctx.Send(SectorCommitted{
		Proof:   proof,
		Message: mcid,
	})
}

func (m *Sealing) handleCommitWait(ctx statemachine.Context, sector SectorInfo) error {
	if sector.CommitMessage == nil {
		log.Errorf("sector %d entered commit wait state without a message cid", sector.SectorNumber)
		return ctx.Send(SectorCommitFailed{xerrors.Errorf("entered commit wait with no commit cid")})
	}

	mw, err := m.api.StateWaitMsg(ctx.Context(), *sector.CommitMessage)
	if err != nil {
		return ctx.Send(SectorCommitFailed{xerrors.Errorf("failed to wait for porep inclusion: %w", err)})
	}

	if mw.Receipt.ExitCode != 0 {
		return ctx.Send(SectorCommitFailed{xerrors.Errorf("submitting sector proof failed (exit=%d, msg=%s) (t:%x; s:%x(%d); p:%x)", mw.Receipt.ExitCode, sector.CommitMessage, sector.TicketValue, sector.SeedValue, sector.SeedEpoch, sector.Proof)})
	}

	return ctx.Send(SectorProving{})
}

func (m *Sealing) handleFinalizeSector(ctx statemachine.Context, sector SectorInfo) error {
	// TODO: Maybe wait for some finality

	if err := m.sealer.FinalizeSector(ctx.Context(), m.minerSector(sector.SectorNumber)); err != nil {
		return ctx.Send(SectorFinalizeFailed{xerrors.Errorf("finalize sector: %w", err)})
	}

	return ctx.Send(SectorFinalized{})
}

func (m *Sealing) handleFaulty(ctx statemachine.Context, sector SectorInfo) error {
	// TODO: check if the fault has already been reported, and that this sector is even valid

	// TODO: coalesce faulty sector reporting

	// TODO: ReportFaultFailed
	bf := abi.NewBitField()
	bf.Set(uint64(sector.SectorNumber))

	deadlines, err := m.api.StateMinerDeadlines(ctx.Context(), m.maddr, nil)
	if err != nil {
		log.Errorf("handleFaulty: api error, not proceeding: %+v", err)
		return nil
	}

	deadline := -1
	for d, field := range deadlines.Due {
		set, err := field.IsSet(uint64(sector.SectorNumber))
		if err != nil {
			return err
		}
		if set {
			deadline = d
			break
		}
	}
	if deadline == -1 {
		log.Errorf("handleFaulty: deadline not found")
		return nil
	}

	params := &miner.DeclareFaultsParams{
		Faults: []miner.FaultDeclaration{
			{
				Deadline: uint64(deadline),
				Sectors:  bf,
			},
		},
	}

	enc := new(bytes.Buffer)
	if err := params.MarshalCBOR(enc); err != nil {
		return ctx.Send(SectorCommitFailed{xerrors.Errorf("failed to serialize declare fault params: %w", err)})
	}

	tok, _, err := m.api.ChainHead(ctx.Context())
	if err != nil {
		log.Errorf("handleFaulty: api error, not proceeding: %+v", err)
		return nil
	}

	waddr, err := m.api.StateMinerWorkerAddress(ctx.Context(), m.maddr, tok)
	if err != nil {
		log.Errorf("handleFaulty: api error, not proceeding: %+v", err)
		return nil
	}

	mcid, err := m.api.SendMsg(ctx.Context(), waddr, m.maddr, builtin.MethodsMiner.DeclareFaults, big.NewInt(0), big.NewInt(1), 1000000, enc.Bytes())
	if err != nil {
		return xerrors.Errorf("failed to push declare faults message to network: %w", err)
	}

	return ctx.Send(SectorFaultReported{reportMsg: mcid})
}

func (m *Sealing) handleFaultReported(ctx statemachine.Context, sector SectorInfo) error {
	if sector.FaultReportMsg == nil {
		return xerrors.Errorf("entered fault reported state without a FaultReportMsg cid")
	}

	mw, err := m.api.StateWaitMsg(ctx.Context(), *sector.FaultReportMsg)
	if err != nil {
		return xerrors.Errorf("failed to wait for fault declaration: %w", err)
	}

	if mw.Receipt.ExitCode != 0 {
		log.Errorf("UNHANDLED: declaring sector fault failed (exit=%d, msg=%s) (id: %d)", mw.Receipt.ExitCode, *sector.FaultReportMsg, sector.SectorNumber)
		return xerrors.Errorf("UNHANDLED: submitting fault declaration failed (exit %d)", mw.Receipt.ExitCode)
	}

	return ctx.Send(SectorFaultedFinal{})
}
