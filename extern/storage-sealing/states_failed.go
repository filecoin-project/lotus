package sealing

import (
	"bytes"
	"time"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/chain/actors/builtin/market"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/exitcode"
	"github.com/filecoin-project/go-statemachine"

	"github.com/filecoin-project/lotus/extern/sector-storage/zerocomm"
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
		log.Errorf("handleSealPrecommit1Failed(%d): temp error: %+v", sector.SectorNumber, err)
		return nil, false
	}

	info, err := m.api.StateSectorPreCommitInfo(ctx.Context(), m.maddr, sector.SectorNumber, tok)
	if err != nil {
		log.Errorf("handleSealPrecommit1Failed(%d): temp error: %+v", sector.SectorNumber, err)
		return nil, false
	}

	return info, true
}

func (m *Sealing) handleSealPrecommit1Failed(ctx statemachine.Context, sector SectorInfo) error {
	if err := failedCooldown(ctx, sector); err != nil {
		return err
	}

	return ctx.Send(SectorRetrySealPreCommit1{})
}

func (m *Sealing) handleSealPrecommit2Failed(ctx statemachine.Context, sector SectorInfo) error {
	if err := failedCooldown(ctx, sector); err != nil {
		return err
	}

	if sector.PreCommit2Fails > 3 {
		return ctx.Send(SectorRetrySealPreCommit1{})
	}

	return ctx.Send(SectorRetrySealPreCommit2{})
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
			return ctx.Send(SectorSealPreCommit1Failed{xerrors.Errorf("bad CommD error: %w", err)})
		case *ErrExpiredTicket:
			return ctx.Send(SectorSealPreCommit1Failed{xerrors.Errorf("ticket expired error: %w", err)})
		case *ErrBadTicket:
			return ctx.Send(SectorSealPreCommit1Failed{xerrors.Errorf("bad expired: %w", err)})
		case *ErrInvalidDeals:
			log.Warnf("invalid deals in sector %d: %v", sector.SectorNumber, err)
			return ctx.Send(SectorInvalidDealIDs{Return: RetPreCommitFailed})
		case *ErrExpiredDeals:
			return ctx.Send(SectorDealsExpired{xerrors.Errorf("sector deals expired: %w", err)})
		case *ErrNoPrecommit:
			return ctx.Send(SectorRetryPreCommit{})
		case *ErrPrecommitOnChain:
			// noop
		case *ErrSectorNumberAllocated:
			log.Errorf("handlePreCommitFailed: sector number already allocated, not proceeding: %+v", err)
			// TODO: check if the sector is committed (not sure how we'd end up here)
			// TODO: check on-chain state, adjust local sector number counter to not give out allocated numbers
			return nil
		default:
			return xerrors.Errorf("checkPrecommit sanity check error: %w", err)
		}
	}

	if pci, is := m.checkPreCommitted(ctx, sector); is && pci != nil {
		if sector.PreCommitMessage == nil {
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
		return ctx.Send(SectorSealPreCommit1Failed{xerrors.Errorf("consecutive compute fails")})
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
			return ctx.Send(SectorSealPreCommit1Failed{xerrors.Errorf("bad CommD error: %w", err)})
		case *ErrExpiredTicket:
			return ctx.Send(SectorSealPreCommit1Failed{xerrors.Errorf("ticket expired error: %w", err)})
		case *ErrBadTicket:
			return ctx.Send(SectorSealPreCommit1Failed{xerrors.Errorf("bad ticket: %w", err)})
		case *ErrInvalidDeals:
			log.Warnf("invalid deals in sector %d: %v", sector.SectorNumber, err)
			return ctx.Send(SectorInvalidDealIDs{Return: RetCommitFailed})
		case *ErrExpiredDeals:
			return ctx.Send(SectorDealsExpired{xerrors.Errorf("sector deals expired: %w", err)})
		case nil:
			return ctx.Send(SectorChainPreCommitFailed{xerrors.Errorf("no precommit: %w", err)})
		case *ErrPrecommitOnChain:
			// noop, this is expected
		case *ErrSectorNumberAllocated:
			// noop, already committed?
		default:
			return xerrors.Errorf("checkPrecommit sanity check error (%T): %w", err, err)
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
				return ctx.Send(SectorSealPreCommit1Failed{xerrors.Errorf("consecutive invalid proofs")})
			}

			return ctx.Send(SectorRetryInvalidProof{})
		case *ErrPrecommitOnChain:
			log.Errorf("no precommit on chain, will retry: %+v", err)
			return ctx.Send(SectorRetryPreCommitWait{})
		case *ErrNoPrecommit:
			return ctx.Send(SectorRetryPreCommit{})
		case *ErrInvalidDeals:
			log.Warnf("invalid deals in sector %d: %v", sector.SectorNumber, err)
			return ctx.Send(SectorInvalidDealIDs{Return: RetCommitFailed})
		case *ErrExpiredDeals:
			return ctx.Send(SectorDealsExpired{xerrors.Errorf("sector deals expired: %w", err)})
		case *ErrCommitWaitFailed:
			if err := failedCooldown(ctx, sector); err != nil {
				return err
			}

			return ctx.Send(SectorRetryCommitWait{})
		default:
			return xerrors.Errorf("checkCommit sanity check error (%T): %w", err, err)
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

func (m *Sealing) handleRemoveFailed(ctx statemachine.Context, sector SectorInfo) error {
	if err := failedCooldown(ctx, sector); err != nil {
		return err
	}

	return ctx.Send(SectorRemove{})
}

func (m *Sealing) handleDealsExpired(ctx statemachine.Context, sector SectorInfo) error {
	// First make vary sure the sector isn't committed
	si, err := m.api.StateSectorGetInfo(ctx.Context(), m.maddr, sector.SectorNumber, nil)
	if err != nil {
		return xerrors.Errorf("getting sector info: %w", err)
	}
	if si != nil {
		// TODO: this should never happen, but in case it does, try to go back to
		//  the proving state after running some checks
		return xerrors.Errorf("sector is committed on-chain, but we're in DealsExpired")
	}

	if sector.PreCommitInfo == nil {
		// TODO: Create a separate state which will remove those pieces, and go back to PC1
		log.Errorf("non-precommitted sector with expired deals, can't recover from this yet")
	}

	// Not much to do here, we can't go back in time to commit this sector
	return ctx.Send(SectorRemove{})
}

func (m *Sealing) handleRecoverDealIDs(ctx statemachine.Context, sector SectorInfo) error {
	tok, height, err := m.api.ChainHead(ctx.Context())
	if err != nil {
		return xerrors.Errorf("getting chain head: %w", err)
	}

	var toFix []int

	for i, p := range sector.Pieces {
		// if no deal is associated with the piece, ensure that we added it as
		// filler (i.e. ensure that it has a zero PieceCID)
		if p.DealInfo == nil {
			exp := zerocomm.ZeroPieceCommitment(p.Piece.Size.Unpadded())
			if !p.Piece.PieceCID.Equals(exp) {
				return xerrors.Errorf("sector %d piece %d had non-zero PieceCID %+v", sector.SectorNumber, i, p.Piece.PieceCID)
			}
			continue
		}

		proposal, err := m.api.StateMarketStorageDeal(ctx.Context(), p.DealInfo.DealID, tok)
		if err != nil {
			log.Warnf("getting deal %d for piece %d: %+v", p.DealInfo.DealID, i, err)
			toFix = append(toFix, i)
			continue
		}

		if proposal.Provider != m.maddr {
			log.Warnf("piece %d (of %d) of sector %d refers deal %d with wrong provider: %s != %s", i, len(sector.Pieces), sector.SectorNumber, p.DealInfo.DealID, proposal.Provider, m.maddr)
			toFix = append(toFix, i)
			continue
		}

		if proposal.PieceCID != p.Piece.PieceCID {
			log.Warnf("piece %d (of %d) of sector %d refers deal %d with wrong PieceCID: %x != %x", i, len(sector.Pieces), sector.SectorNumber, p.DealInfo.DealID, p.Piece.PieceCID, proposal.PieceCID)
			toFix = append(toFix, i)
			continue
		}

		if p.Piece.Size != proposal.PieceSize {
			log.Warnf("piece %d (of %d) of sector %d refers deal %d with different size: %d != %d", i, len(sector.Pieces), sector.SectorNumber, p.DealInfo.DealID, p.Piece.Size, proposal.PieceSize)
			toFix = append(toFix, i)
			continue
		}

		if height >= proposal.StartEpoch {
			// TODO: check if we are in an early enough state (before precommit), try to remove the offending pieces
			//  (tricky as we have to 'defragment' the sector while doing that, and update piece references for retrieval)
			return xerrors.Errorf("can't fix sector deals: piece %d (of %d) of sector %d refers expired deal %d - should start at %d, head %d", i, len(sector.Pieces), sector.SectorNumber, p.DealInfo.DealID, proposal.StartEpoch, height)
		}
	}

	updates := map[int]abi.DealID{}
	for _, i := range toFix {
		p := sector.Pieces[i]

		if p.DealInfo.PublishCid == nil {
			// TODO: check if we are in an early enough state try to remove this piece
			log.Error("can't fix sector deals: piece %d (of %d) of sector %d has nil DealInfo.PublishCid (refers to deal %d)", i, len(sector.Pieces), sector.SectorNumber, p.DealInfo.DealID)
			// Not much to do here (and this can only happen for old spacerace sectors)
			return ctx.Send(SectorRemove{})
		}

		ml, err := m.api.StateSearchMsg(ctx.Context(), *p.DealInfo.PublishCid)
		if err != nil {
			return xerrors.Errorf("looking for publish deal message %s (sector %d, piece %d): %w", *p.DealInfo.PublishCid, sector.SectorNumber, i, err)
		}

		if ml.Receipt.ExitCode != exitcode.Ok {
			return xerrors.Errorf("looking for publish deal message %s (sector %d, piece %d): non-ok exit code: %s", *p.DealInfo.PublishCid, sector.SectorNumber, i, ml.Receipt.ExitCode)
		}

		var retval market.PublishStorageDealsReturn
		if err := retval.UnmarshalCBOR(bytes.NewReader(ml.Receipt.Return)); err != nil {
			return xerrors.Errorf("looking for publish deal message: unmarshaling message return: %w", err)
		}

		if len(retval.IDs) != 1 {
			// market currently only ever sends messages with 1 deal
			return xerrors.Errorf("can't recover dealIDs from publish deal message with more than 1 deal")
		}

		updates[i] = retval.IDs[0]
	}

	// Not much to do here, we can't go back in time to commit this sector
	return ctx.Send(SectorUpdateDealIDs{Updates: updates})
}
