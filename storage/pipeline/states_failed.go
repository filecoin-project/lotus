package sealing

import (
	"context"
	"time"

	"github.com/hashicorp/go-multierror"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-commp-utils/v2/zerocomm"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/exitcode"
	"github.com/filecoin-project/go-statemachine"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/actors/builtin/market"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
)

var MinRetryTime = 1 * time.Minute

func failedCooldown(ctx statemachine.Context, sector SectorInfo) error {
	// TODO: Exponential backoff when we see consecutive failures

	retryStart := time.Unix(int64(sector.Log[len(sector.Log)-1].Timestamp), 0).Add(MinRetryTime)
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
	ts, err := m.Api.ChainHead(ctx.Context())
	if err != nil {
		log.Errorf("handleSealPrecommit1Failed(%d): temp error: %+v", sector.SectorNumber, err)
		return nil, false
	}

	info, err := m.Api.StateSectorPreCommitInfo(ctx.Context(), m.maddr, sector.SectorNumber, ts.Key())
	if err != nil {
		log.Errorf("handleSealPrecommit1Failed(%d): temp error: %+v", sector.SectorNumber, err)
		return nil, false
	}

	return info, true
}

var MaxPreCommit1Retries = uint64(3)

func (m *Sealing) handleSealPrecommit1Failed(ctx statemachine.Context, sector SectorInfo) error {
	if sector.PreCommit1Fails > MaxPreCommit1Retries {
		return ctx.Send(SectorRemove{})
	}

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
	ts, err := m.Api.ChainHead(ctx.Context())
	if err != nil {
		log.Errorf("handlePreCommitFailed: api error, not proceeding: %+v", err)
		return nil
	}

	if sector.PreCommitMessage != nil {
		mw, err := m.Api.StateSearchMsg(ctx.Context(), ts.Key(), *sector.PreCommitMessage, api.LookbackNoLimit, true)
		if err != nil {
			// API error
			if err := failedCooldown(ctx, sector); err != nil {
				return err
			}

			return ctx.Send(SectorRetryPreCommitWait{})
		}

		if mw == nil {
			// API error in precommit
			return ctx.Send(SectorRetryPreCommitWait{})
		}

		switch mw.Receipt.ExitCode {
		case exitcode.Ok:
			// API error in PreCommitWait
			return ctx.Send(SectorRetryPreCommitWait{})
		case exitcode.SysErrOutOfGas:
			// API error in PreCommitWait AND gas estimator guessed a wrong number in PreCommit
			return ctx.Send(SectorRetryPreCommit{})
		default:
			// something else went wrong
		}
	}

	if err := checkPrecommit(ctx.Context(), m.Address(), sector, ts.Key(), ts.Height(), m.Api); err != nil {
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
			log.Warnf("sector %d is precommitted on chain, but we don't have precommit message", sector.SectorNumber)
			return ctx.Send(SectorPreCommitLanded{TipSet: ts.Key()})
		}

		if pci.Info.SealedCID != *sector.CommR {
			log.Warnf("sector %d is precommitted on chain, with different CommR: %s != %s", sector.SectorNumber, pci.Info.SealedCID, sector.CommR)
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

func (m *Sealing) handleRemoteCommitFailed(ctx statemachine.Context, sector SectorInfo) error {
	if err := failedCooldown(ctx, sector); err != nil {
		return err
	}

	if sector.InvalidProofs > 1 {
		log.Errorw("consecutive remote commit fails", "sector", sector.SectorNumber, "c1url", sector.RemoteCommit1Endpoint, "c2url", sector.RemoteCommit2Endpoint)
	}

	return ctx.Send(SectorRetryComputeProof{})
}

func (m *Sealing) handleSubmitReplicaUpdateFailed(ctx statemachine.Context, sector SectorInfo) error {
	if err := failedCooldown(ctx, sector); err != nil {
		return err
	}

	if sector.ReplicaUpdateMessage != nil {
		mw, err := m.Api.StateSearchMsg(ctx.Context(), types.EmptyTSK, *sector.ReplicaUpdateMessage, api.LookbackNoLimit, true)
		if err != nil {
			// API error
			return ctx.Send(SectorRetrySubmitReplicaUpdateWait{})
		}

		if mw == nil {
			return ctx.Send(SectorRetrySubmitReplicaUpdateWait{})
		}

		switch mw.Receipt.ExitCode {
		case exitcode.Ok:
			return ctx.Send(SectorRetrySubmitReplicaUpdateWait{})
		case exitcode.SysErrOutOfGas:
			return ctx.Send(SectorRetrySubmitReplicaUpdate{})
		default:
			// something else went wrong
		}
	}

	ts, err := m.Api.ChainHead(ctx.Context())
	if err != nil {
		log.Errorf("handleSubmitReplicaUpdateFailed: api error, not proceeding: %+v", err)
		return nil
	}

	if err := checkReplicaUpdate(ctx.Context(), m.maddr, sector, m.Api); err != nil {
		switch err.(type) {
		case *ErrApi:
			log.Errorf("handleSubmitReplicaUpdateFailed: api error, not proceeding: %+v", err)
			return nil
		case *ErrBadRU:
			log.Errorf("bad replica update: %+v", err)
			return ctx.Send(SectorRetryReplicaUpdate{})
		case *ErrBadPR:
			log.Errorf("bad PR1: +%v", err)
			return ctx.Send(SectorRetryProveReplicaUpdate{})

		case *ErrInvalidDeals:
			return ctx.Send(SectorInvalidDealIDs{})
		case *ErrExpiredDeals:
			return ctx.Send(SectorDealsExpired{xerrors.Errorf("expired dealIDs in sector: %w", err)})
		default:
			log.Errorf("sanity check error, not proceeding: +%v", err)
			return xerrors.Errorf("checkReplica sanity check error: %w", err)
		}
	}

	// Abort upgrade for sectors that went faulty since being marked for upgrade
	active, err := m.sectorActive(ctx.Context(), ts.Key(), sector.SectorNumber)
	if err != nil {
		log.Errorf("sector active check: api error, not proceeding: %+v", err)
		return nil
	}
	if !active {
		err := xerrors.Errorf("sector marked for upgrade %d no longer active, aborting upgrade", sector.SectorNumber)
		log.Errorf("%s", err)
		return ctx.Send(SectorAbortUpgrade{err})
	}

	return ctx.Send(SectorRetrySubmitReplicaUpdate{})
}

func (m *Sealing) handleReleaseSectorKeyFailed(ctx statemachine.Context, sector SectorInfo) error {
	// not much we can do, wait for a bit and try again

	if err := failedCooldown(ctx, sector); err != nil {
		return err
	}

	return ctx.Send(SectorUpdateActive{})
}

func (m *Sealing) handleCommitFailed(ctx statemachine.Context, sector SectorInfo) error {
	ts, err := m.Api.ChainHead(ctx.Context())
	if err != nil {
		log.Errorf("handleCommitting: api error, not proceeding: %+v", err)
		return nil
	}

	if sector.CommitMessage != nil {
		mw, err := m.Api.StateSearchMsg(ctx.Context(), ts.Key(), *sector.CommitMessage, api.LookbackNoLimit, true)
		if err != nil {
			// API error
			if err := failedCooldown(ctx, sector); err != nil {
				return err
			}

			return ctx.Send(SectorRetryCommitWait{})
		}

		if mw == nil {
			// API error in commit
			return ctx.Send(SectorRetryCommitWait{})
		}

		switch mw.Receipt.ExitCode {
		case exitcode.Ok:
			si, err := m.Api.StateSectorGetInfo(ctx.Context(), m.maddr, sector.SectorNumber, mw.TipSet)
			if err != nil {
				// API error
				if err := failedCooldown(ctx, sector); err != nil {
					return err
				}

				return ctx.Send(SectorRetryCommitWait{})
			}
			if si != nil {
				// API error in CommitWait?
				return ctx.Send(SectorRetryCommitWait{})
			}
			// if si == nil, something else went wrong; Likely expired deals, we'll
			// find out in checkCommit
		case exitcode.SysErrOutOfGas:
			// API error in CommitWait AND gas estimator guessed a wrong number in SubmitCommit
			return ctx.Send(SectorRetrySubmitCommit{})
		default:
			// something else went wrong
		}
	}

	if err := m.checkCommit(ctx.Context(), sector, sector.Proof, ts.Key()); err != nil {
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

func (m *Sealing) handleTerminateFailed(ctx statemachine.Context, sector SectorInfo) error {
	// ignoring error as it's most likely an API error - `pci` will be nil, and we'll go back to
	// the Terminating state after cooldown. If the API is still failing, well get back to here
	// with the error in SectorInfo log.
	pci, _ := m.Api.StateSectorPreCommitInfo(ctx.Context(), m.maddr, sector.SectorNumber, types.EmptyTSK)
	if pci != nil {
		return nil // pause the fsm, needs manual user action
	}

	if err := failedCooldown(ctx, sector); err != nil {
		return err
	}

	return ctx.Send(SectorTerminate{})
}

func (m *Sealing) handleDealsExpired(ctx statemachine.Context, sector SectorInfo) error {
	// First make vary sure the sector isn't committed
	si, err := m.Api.StateSectorGetInfo(ctx.Context(), m.maddr, sector.SectorNumber, types.EmptyTSK)
	if err != nil {
		return xerrors.Errorf("getting sector info: %w", err)
	}
	if si != nil {
		// TODO: this should never happen, but in case it does, try to go back to
		//  the proving state after running some checks
		return xerrors.Errorf("sector is committed on-chain, but we're in DealsExpired")
	}

	// Not much to do here, we can't go back in time to commit this sector
	return ctx.Send(SectorRemove{})
}

func (m *Sealing) handleDealsExpiredSnapDeals(ctx statemachine.Context, sector SectorInfo) error {
	if !sector.CCUpdate {
		// Should be impossible
		return xerrors.Errorf("should never reach SnapDealsDealsExpired as a non-CCUpdate sector")
	}

	return ctx.Send(SectorAbortUpgrade{xerrors.Errorf("one of upgrade deals expired")})
}

func (m *Sealing) handleAbortUpgrade(ctx statemachine.Context, sector SectorInfo) error {
	if !sector.CCUpdate {
		return xerrors.Errorf("should never reach AbortUpgrade as a non-CCUpdate sector")
	}

	m.cleanupAssignedDeals(sector)

	// Remove snap deals replica if any
	// This removes update / update-cache from all storage
	if err := m.sealer.ReleaseReplicaUpgrade(ctx.Context(), m.minerSector(sector.SectorType, sector.SectorNumber)); err != nil {
		return xerrors.Errorf("removing CC update files from sector storage")
	}

	// This removes the unsealed file from all storage
	// note: we're not keeping anything unsealed because we're reverting to CC
	if err := m.sealer.ReleaseUnsealed(ctx.Context(), m.minerSector(sector.SectorType, sector.SectorNumber), []storiface.Range{}); err != nil {
		log.Error(err)
	}

	// and makes sure sealed/cache files only exist in long-term-storage
	if err := m.sealer.FinalizeSector(ctx.Context(), m.minerSector(sector.SectorType, sector.SectorNumber)); err != nil {
		log.Error(err)
	}

	return ctx.Send(SectorRevertUpgradeToProving{})
}

// failWith is a mutator or global mutator
func (m *Sealing) handleRecoverDealIDsOrFailWith(ctx statemachine.Context, sector SectorInfo, failWith interface{}) error {
	toFix, nonBuiltinMarketPieces, err := recoveryPiecesToFix(ctx.Context(), m.Api, sector, m.maddr)
	if err != nil {
		return err
	}
	ts, err := m.Api.ChainHead(ctx.Context())
	if err != nil {
		return err
	}
	failed := map[int]error{}
	updates := map[int]abi.DealID{}

	for _, i := range toFix {
		// note: all toFix pieces are builtin-market pieces

		p := sector.Pieces[i]

		if p.Impl().PublishCid == nil {
			// TODO: check if we are in an early enough state try to remove this piece
			log.Errorf("can't fix sector deals: piece %d (of %d) of sector %d has nil DealInfo.PublishCid (refers to deal %d)", i, len(sector.Pieces), sector.SectorNumber, p.Impl().DealID)
			// Not much to do here (and this can only happen for old spacerace sectors)
			return ctx.Send(failWith)
		}

		var dp *market.DealProposal
		if p.Impl().DealProposal != nil {
			mdp := *p.Impl().DealProposal
			dp = &mdp
		}
		res, err := m.DealInfo.GetCurrentDealInfo(ctx.Context(), ts.Key(), dp, *p.Impl().PublishCid)
		if err != nil {
			failed[i] = xerrors.Errorf("getting current deal info for piece %d: %w", i, err)
			continue
		}

		if res.MarketDeal == nil {
			failed[i] = xerrors.Errorf("nil market deal (%d,%d,%d,%s)", i, sector.SectorNumber, p.Impl().DealID, p.Impl().DealProposal.PieceCID)
			continue
		}

		if res.MarketDeal.Proposal.PieceCID != p.PieceCID() {
			failed[i] = xerrors.Errorf("recovered piece (%d) deal in sector %d (dealid %d) has different PieceCID %s != %s", i, sector.SectorNumber, p.Impl().DealID, p.Impl().DealProposal.PieceCID, res.MarketDeal.Proposal.PieceCID)
			continue
		}

		updates[i] = res.DealID
	}

	if len(failed) > 0 {
		var merr error
		for _, e := range failed {
			merr = multierror.Append(merr, e)
		}

		if len(failed)+nonBuiltinMarketPieces == len(sector.Pieces) {
			log.Errorf("removing sector %d: all deals expired or unrecoverable: %+v", sector.SectorNumber, merr)
			return ctx.Send(failWith)
		}

		// todo: try to remove bad pieces (hard; see the todo above)

		// for now removing sectors is probably better than having them stuck in RecoverDealIDs
		// and expire anyways
		log.Errorf("removing sector %d: deals expired or unrecoverable: %+v", sector.SectorNumber, merr)
		return ctx.Send(failWith)
	}

	// Not much to do here, we can't go back in time to commit this sector
	return ctx.Send(SectorUpdateDealIDs{Updates: updates})
}

func (m *Sealing) HandleRecoverDealIDs(ctx statemachine.Context, sector SectorInfo) error {
	return m.handleRecoverDealIDsOrFailWith(ctx, sector, SectorRemove{})
}

func (m *Sealing) handleSnapDealsRecoverDealIDs(ctx statemachine.Context, sector SectorInfo) error {
	return m.handleRecoverDealIDsOrFailWith(ctx, sector, SectorAbortUpgrade{xerrors.New("failed recovering deal ids")})
}

// recoveryPiecesToFix returns the list of sector piece indexes to fix, and the number of non-builtin-market pieces
func recoveryPiecesToFix(ctx context.Context, api SealingAPI, sector SectorInfo, maddr address.Address) ([]int, int, error) {
	ts, err := api.ChainHead(ctx)
	if err != nil {
		return nil, 0, xerrors.Errorf("getting chain head: %w", err)
	}

	var toFix []int
	nonBuiltinMarketPieces := 0

	for i, p := range sector.Pieces {
		i, p := i, p

		err := p.handleDealInfo(handleDealInfoParams{
			FillerHandler: func(info UniversalPieceInfo) error {
				// if no deal is associated with the piece, ensure that we added it as
				// filler (i.e. ensure that it has a zero PieceCID)
				exp := zerocomm.ZeroPieceCommitment(p.Piece().Size.Unpadded())
				if !info.PieceCID().Equals(exp) {
					return xerrors.Errorf("sector %d piece %d had non-zero PieceCID %+v", sector.SectorNumber, i, p.Piece().PieceCID)
				}
				nonBuiltinMarketPieces++
				return nil
			},
			BuiltinMarketHandler: func(info UniversalPieceInfo) error {
				deal, err := api.StateMarketStorageDeal(ctx, p.DealInfo().Impl().DealID, ts.Key())
				if err != nil {
					log.Warnf("getting deal %d for piece %d: %+v", p.DealInfo().Impl().DealID, i, err)
					toFix = append(toFix, i)
					return nil
				}

				if deal.Proposal.Provider != maddr {
					log.Warnf("piece %d (of %d) of sector %d refers deal %d with wrong provider: %s != %s", i, len(sector.Pieces), sector.SectorNumber, p.Impl().DealID, deal.Proposal.Provider, maddr)
					toFix = append(toFix, i)
					return nil
				}

				if deal.Proposal.PieceCID != p.Piece().PieceCID {
					log.Warnf("piece %d (of %d) of sector %d refers deal %d with wrong PieceCID: %s != %s", i, len(sector.Pieces), sector.SectorNumber, p.Impl().DealID, p.Piece().PieceCID, deal.Proposal.PieceCID)
					toFix = append(toFix, i)
					return nil
				}

				if p.Piece().Size != deal.Proposal.PieceSize {
					log.Warnf("piece %d (of %d) of sector %d refers deal %d with different size: %d != %d", i, len(sector.Pieces), sector.SectorNumber, p.Impl().DealID, p.Piece().Size, deal.Proposal.PieceSize)
					toFix = append(toFix, i)
					return nil
				}

				if ts.Height() >= deal.Proposal.StartEpoch {
					// TODO: check if we are in an early enough state (before precommit), try to remove the offending pieces
					//  (tricky as we have to 'defragment' the sector while doing that, and update piece references for retrieval)
					return xerrors.Errorf("can't fix sector deals: piece %d (of %d) of sector %d refers expired deal %d - should start at %d, head %d", i, len(sector.Pieces), sector.SectorNumber, p.Impl().DealID, deal.Proposal.StartEpoch, ts.Height())
				}

				return nil
			},
			DDOHandler: func(info UniversalPieceInfo) error {
				// DDO pieces have no repair strategy

				nonBuiltinMarketPieces++
				return nil
			},
		})

		if err != nil {
			return nil, 0, xerrors.Errorf("checking piece %d: %w", i, err)
		}
	}

	return toFix, nonBuiltinMarketPieces, nil
}
