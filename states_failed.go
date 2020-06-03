package sealing

import (
	"time"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-statemachine"
	"github.com/filecoin-project/specs-actors/actors/builtin/miner"
)

const minRetryTime = 1 * time.Minute

func failedCooldown(ctx statemachine.Context, sector SectorInfo) error {
	// TODO: Exponential backoff when we see consecutive failures

	retryStart := time.Unix(int64(sector.Log[len(sector.Log)-1].Timestamp), 0).Add(minRetryTime)
	if len(sector.Log) > 0 && !time.Now().After(retryStart) {
		log.Infof("%s(%d), waiting %s before retrying", sector.State, sector.SectorNumber, time.Until(retryStart))
		select {
		case <-time.After(time.Until(retryStart)):
		case <-ctx.Context().Done():
			return ctx.Context().Err()
		}
	}

	return nil
}

func (m *Sealing) checkPreCommitted(ctx statemachine.Context, sector SectorInfo) (*miner.SectorPreCommitOnChainInfo, bool) {
	tok, _, err := m.api.ChainHead(ctx.Context())
	if err != nil {
		log.Errorf("handleSealFailed(%d): temp error: %+v", sector.SectorNumber, err)
		return nil, true
	}

	info, err := m.api.StateSectorPreCommitInfo(ctx.Context(), m.maddr, sector.SectorNumber, tok)
	if err != nil {
		log.Errorf("handleSealFailed(%d): temp error: %+v", sector.SectorNumber, err)
		return nil, true
	}

	return info, false
}

func (m *Sealing) handleSealFailed(ctx statemachine.Context, sector SectorInfo) error {
	if _, is := m.checkPreCommitted(ctx, sector); is {
		// TODO: Remove this after we can re-precommit
		return nil // noop, for now
	}

	if err := failedCooldown(ctx, sector); err != nil {
		return err
	}

	return ctx.Send(SectorRetrySeal{})
}

func (m *Sealing) handlePreCommitFailed(ctx statemachine.Context, sector SectorInfo) error {
	tok, height, err := m.api.ChainHead(ctx.Context())
	if err != nil {
		log.Errorf("handlePreCommitFailed: api error, not proceeding: %+v", err)
		return nil
	}

	if err := checkPrecommit(ctx.Context(), m.Address(), sector, tok, height, m.api); err != nil {
		switch err.(type) {
		case *ErrApi:
			log.Errorf("handlePreCommitFailed: api error, not proceeding: %+v", err)
			return nil
		case *ErrBadCommD: // TODO: Should this just back to packing? (not really needed since handlePreCommit1 will do that too)
			return ctx.Send(SectorSealPreCommitFailed{xerrors.Errorf("bad CommD error: %w", err)})
		case *ErrExpiredTicket:
			return ctx.Send(SectorSealPreCommitFailed{xerrors.Errorf("ticket expired error: %w", err)})
		case *ErrBadTicket:
			return ctx.Send(SectorSealPreCommitFailed{xerrors.Errorf("bad expired: %w", err)})
		case *ErrPrecommitOnChain:
			// noop
		default:
			return xerrors.Errorf("checkPrecommit sanity check error: %w", err)
		}
	}

	if pci, is := m.checkPreCommitted(ctx, sector); is && pci != nil {
		if sector.PreCommitMessage != nil {
			log.Warn("sector %d is precommitted on chain, but we don't have precommit message", sector.SectorNumber)
			return ctx.Send(SectorPreCommitLanded{TipSet: tok})
		}

		if pci.Info.SealedCID != *sector.CommR {
			log.Warn("sector %d is precommitted on chain, with different CommR: %x != %x", sector.SectorNumber, pci.Info.SealedCID, sector.CommR)
			return nil // TODO: remove when the actor allows re-precommit
		}

		// TODO: we could compare more things, but I don't think we really need to
		//  CommR tells us that CommD (and CommPs), and the ticket are all matching

		if err := failedCooldown(ctx, sector); err != nil {
			return err
		}

		return ctx.Send(SectorRetryWaitSeed{})
	}

	if sector.PreCommitMessage != nil {
		log.Warn("retrying precommit even though the message failed to apply")
	}

	if err := failedCooldown(ctx, sector); err != nil {
		return err
	}

	return ctx.Send(SectorRetryPreCommit{})
}

func (m *Sealing) handleComputeProofFailed(ctx statemachine.Context, sector SectorInfo) error {
	// TODO: Check sector files

	if err := failedCooldown(ctx, sector); err != nil {
		return err
	}

	if sector.InvalidProofs > 1 {
		return ctx.Send(SectorSealPreCommitFailed{xerrors.Errorf("consecutive compute fails")})
	}

	return ctx.Send(SectorRetryComputeProof{})
}

func (m *Sealing) handleCommitFailed(ctx statemachine.Context, sector SectorInfo) error {
	tok, height, err := m.api.ChainHead(ctx.Context())
	if err != nil {
		log.Errorf("handleCommitting: api error, not proceeding: %+v", err)
		return nil
	}

	if err := checkPrecommit(ctx.Context(), m.maddr, sector, tok, height, m.api); err != nil {
		switch err.(type) {
		case *ErrApi:
			log.Errorf("handleCommitFailed: api error, not proceeding: %+v", err)
			return nil
		case *ErrBadCommD:
			return ctx.Send(SectorSealPreCommitFailed{xerrors.Errorf("bad CommD error: %w", err)})
		case *ErrExpiredTicket:
			return ctx.Send(SectorSealPreCommitFailed{xerrors.Errorf("ticket expired error: %w", err)})
		case *ErrBadTicket:
			return ctx.Send(SectorSealPreCommitFailed{xerrors.Errorf("bad expired: %w", err)})
		case *ErrPrecommitOnChain:
			// noop, this is expected
		default:
			return xerrors.Errorf("checkPrecommit sanity check error: %w", err)
		}
	}

	if err := m.checkCommit(ctx.Context(), sector, sector.Proof, tok); err != nil {
		switch err.(type) {
		case *ErrApi:
			log.Errorf("handleCommitFailed: api error, not proceeding: %+v", err)
			return nil
		case *ErrBadSeed:
			log.Errorf("seed changed, will retry: %+v", err)
			return ctx.Send(SectorRetryWaitSeed{})
		case *ErrInvalidProof:
			if err := failedCooldown(ctx, sector); err != nil {
				return err
			}

			if sector.InvalidProofs > 0 {
				return ctx.Send(SectorSealPreCommitFailed{xerrors.Errorf("consecutive invalid proofs")})
			}

			return ctx.Send(SectorRetryInvalidProof{})
		default:
			return xerrors.Errorf("checkCommit sanity check error: %w", err)
		}
	}

	// TODO: Check sector files

	if err := failedCooldown(ctx, sector); err != nil {
		return err
	}

	return ctx.Send(SectorRetryComputeProof{})
}

func (m *Sealing) handleFinalizeFailed(ctx statemachine.Context, sector SectorInfo) error {
	// TODO: Check sector files

	if err := failedCooldown(ctx, sector); err != nil {
		return err
	}

	return ctx.Send(SectorRetryFinalize{})
}
