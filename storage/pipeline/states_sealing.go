package sealing

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	cborutil "github.com/filecoin-project/go-cbor-util"
	"github.com/filecoin-project/go-commp-utils/v2/zerocomm"
	"github.com/filecoin-project/go-state-types/abi"
	actorstypes "github.com/filecoin-project/go-state-types/actors"
	"github.com/filecoin-project/go-state-types/big"
	miner2 "github.com/filecoin-project/go-state-types/builtin/v13/miner"
	verifreg13 "github.com/filecoin-project/go-state-types/builtin/v13/verifreg"
	"github.com/filecoin-project/go-state-types/builtin/v9/verifreg"
	"github.com/filecoin-project/go-state-types/crypto"
	"github.com/filecoin-project/go-state-types/exitcode"
	"github.com/filecoin-project/go-state-types/network"
	"github.com/filecoin-project/go-state-types/proof"
	"github.com/filecoin-project/go-statemachine"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/actors/builtin/market"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/actors/policy"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/lib/filler"
	"github.com/filecoin-project/lotus/storage/pipeline/lib/nullreader"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
)

const MinDDONetworkVersion = network.Version22

var DealSectorPriority = 1024
var MaxTicketAge = policy.MaxPreCommitRandomnessLookback

func (m *Sealing) cleanupAssignedDeals(sector SectorInfo) {
	m.inputLk.Lock()
	// make sure we are not accepting deals into this sector
	for _, c := range m.assignedPieces[m.minerSectorID(sector.SectorNumber)] {
		pp := m.pendingPieces[c]
		delete(m.pendingPieces, c)
		if pp == nil {
			log.Errorf("nil assigned pending piece %s", c)
			continue
		}

		// todo: return to the sealing queue (this is extremely unlikely to happen)
		pp.accepted(sector.SectorNumber, 0, xerrors.Errorf("sector %d entered packing state early", sector.SectorNumber))
	}

	delete(m.openSectors, m.minerSectorID(sector.SectorNumber))
	delete(m.assignedPieces, m.minerSectorID(sector.SectorNumber))
	m.inputLk.Unlock()
}

func (m *Sealing) handlePacking(ctx statemachine.Context, sector SectorInfo) error {
	m.cleanupAssignedDeals(sector)

	// if this is a snapdeals sector, but it ended up not having any deals, abort the upgrade
	if sector.State == SnapDealsPacking && !sector.hasData() {
		return ctx.Send(SectorAbortUpgrade{xerrors.New("sector had no deals")})
	}

	log.Infow("performing filling up rest of the sector...", "sector", sector.SectorNumber)

	var allocated abi.UnpaddedPieceSize
	for _, piece := range sector.Pieces {
		allocated += piece.Piece().Size.Unpadded()
	}

	ssize, err := sector.SectorType.SectorSize()
	if err != nil {
		return err
	}

	ubytes := abi.PaddedPieceSize(ssize).Unpadded()

	if allocated > ubytes {
		return xerrors.Errorf("too much data in sector: %d > %d", allocated, ubytes)
	}

	fillerSizes, err := filler.FillersFromRem(ubytes - allocated)
	if err != nil {
		return err
	}

	if len(fillerSizes) > 0 {
		log.Warnf("Creating %d filler pieces for sector %d", len(fillerSizes), sector.SectorNumber)
	}

	fillerPieces, err := m.padSector(sector.sealingCtx(ctx.Context()), m.minerSector(sector.SectorType, sector.SectorNumber), sector.existingPieceSizes(), fillerSizes...)
	if err != nil {
		return xerrors.Errorf("filling up the sector (%v): %w", fillerSizes, err)
	}

	return ctx.Send(SectorPacked{FillerPieces: fillerPieces})
}

func (m *Sealing) padSector(ctx context.Context, sectorID storiface.SectorRef, existingPieceSizes []abi.UnpaddedPieceSize, sizes ...abi.UnpaddedPieceSize) ([]abi.PieceInfo, error) {
	if len(sizes) == 0 {
		return nil, nil
	}

	log.Infof("Pledge %d, contains %+v", sectorID, existingPieceSizes)

	out := make([]abi.PieceInfo, len(sizes))
	for i, size := range sizes {
		expectCid := zerocomm.ZeroPieceCommitment(size)

		ppi, err := m.sealer.AddPiece(ctx, sectorID, existingPieceSizes, size, nullreader.NewNullReader(size))
		if err != nil {
			return nil, xerrors.Errorf("add piece: %w", err)
		}
		if !expectCid.Equals(ppi.PieceCID) {
			return nil, xerrors.Errorf("got unexpected padding piece CID: expected:%s, got:%s", expectCid, ppi.PieceCID)
		}

		existingPieceSizes = append(existingPieceSizes, size)

		out[i] = ppi
	}

	return out, nil
}

func checkTicketExpired(ticket, head abi.ChainEpoch) bool {
	return head-ticket > MaxTicketAge // TODO: allow configuring expected seal durations
}

func checkProveCommitExpired(preCommitEpoch, msd abi.ChainEpoch, currEpoch abi.ChainEpoch) bool {
	return currEpoch > preCommitEpoch+msd
}

func (m *Sealing) getTicket(ctx statemachine.Context, sector SectorInfo) (abi.SealRandomness, abi.ChainEpoch, bool, error) {
	ts, err := m.Api.ChainHead(ctx.Context())
	if err != nil {
		log.Errorf("getTicket: api error, not proceeding: %+v", err)
		return nil, 0, false, nil
	}

	// the reason why the StateMinerSectorAllocated function is placed here, if it is outside,
	//	if the MarshalCBOR function and StateSectorPreCommitInfo function return err, it will be executed
	allocated, aerr := m.Api.StateMinerSectorAllocated(ctx.Context(), m.maddr, sector.SectorNumber, types.EmptyTSK)
	if aerr != nil {
		log.Errorf("getTicket: api error, checking if sector is allocated: %+v", aerr)
		return nil, 0, false, nil
	}

	ticketEpoch := ts.Height() - policy.SealRandomnessLookback
	buf := new(bytes.Buffer)
	if err := m.maddr.MarshalCBOR(buf); err != nil {
		return nil, 0, allocated, err
	}

	pci, err := m.Api.StateSectorPreCommitInfo(ctx.Context(), m.maddr, sector.SectorNumber, ts.Key())
	if err != nil {
		return nil, 0, allocated, xerrors.Errorf("getting precommit info: %w", err)
	}

	if pci != nil {
		ticketEpoch = pci.Info.SealRandEpoch

		nv, err := m.Api.StateNetworkVersion(ctx.Context(), ts.Key())
		if err != nil {
			return nil, 0, allocated, xerrors.Errorf("getTicket: StateNetworkVersion: api error, not proceeding: %+v", err)
		}

		av, err := actorstypes.VersionForNetwork(nv)
		if err != nil {
			return nil, 0, allocated, xerrors.Errorf("getTicket: actor version for network error, not proceeding: %w", err)
		}
		msd, err := policy.GetMaxProveCommitDuration(av, sector.SectorType)
		if err != nil {
			return nil, 0, allocated, xerrors.Errorf("getTicket: max prove commit duration policy error, not proceeding: %w", err)
		}

		if checkProveCommitExpired(pci.PreCommitEpoch, msd, ts.Height()) {
			return nil, 0, allocated, xerrors.Errorf("ticket expired for precommitted sector")
		}
	}

	if pci == nil && allocated { // allocated is true, sector precommitted but expired, will SectorCommitFailed or SectorRemove
		return nil, 0, allocated, xerrors.Errorf("sector %s precommitted but expired", sector.SectorNumber)
	}

	rand, err := m.Api.StateGetRandomnessFromTickets(ctx.Context(), crypto.DomainSeparationTag_SealRandomness, ticketEpoch, buf.Bytes(), ts.Key())
	if err != nil {
		return nil, 0, allocated, err
	}

	return abi.SealRandomness(rand), ticketEpoch, allocated, nil
}

func (m *Sealing) handleGetTicket(ctx statemachine.Context, sector SectorInfo) error {
	ticketValue, ticketEpoch, allocated, err := m.getTicket(ctx, sector)
	if err != nil {
		if allocated {
			if sector.CommitMessage != nil {
				// Some recovery paths with unfortunate timing lead here
				return ctx.Send(SectorCommitFailed{xerrors.Errorf("sector %s is committed but got into the GetTicket state", sector.SectorNumber)})
			}

			log.Errorf("Sector %s precommitted but expired", sector.SectorNumber)
			return ctx.Send(SectorRemove{})
		}

		return ctx.Send(SectorSealPreCommit1Failed{xerrors.Errorf("getting ticket failed: %w", err)})
	}

	return ctx.Send(SectorTicket{
		TicketValue: ticketValue,
		TicketEpoch: ticketEpoch,
	})
}

var SoftErrRetryWait = 5 * time.Second

func retrySoftErr(ctx context.Context, cb func() error) error {
	for {
		err := cb()
		if err == nil {
			return nil
		}

		var cerr storiface.WorkError

		if errors.As(err, &cerr) {
			switch cerr.ErrCode() {
			case storiface.ErrTempWorkerRestart:
				fallthrough
			case storiface.ErrTempAllocateSpace:
				// retry
				log.Errorw("retrying soft error", "err", err, "code", cerr.ErrCode())
			default:
				// non-temp error
				return err
			}

			// check if the context got cancelled early
			if ctx.Err() != nil {
				return ctx.Err()
			}

			// retry
			time.Sleep(SoftErrRetryWait)
		} else {
			return err
		}
	}
}

func (m *Sealing) handlePreCommit1(ctx statemachine.Context, sector SectorInfo) error {
	if err := checkPieces(ctx.Context(), m.maddr, sector.SectorNumber, sector.Pieces, m.Api, false); err != nil { // Sanity check state
		switch err.(type) {
		case *ErrApi:
			log.Errorf("handlePreCommit1: api error, not proceeding: %+v", err)
			return nil
		case *ErrInvalidDeals:
			log.Warnf("invalid deals in sector %d: %v", sector.SectorNumber, err)
			return ctx.Send(SectorInvalidDealIDs{Return: RetPreCommit1})
		case *ErrExpiredDeals: // Probably not much we can do here, maybe re-pack the sector?
			return ctx.Send(SectorDealsExpired{xerrors.Errorf("expired dealIDs in sector: %w", err)})
		default:
			return xerrors.Errorf("checkPieces sanity check error: %w", err)
		}
	}

	ts, err := m.Api.ChainHead(ctx.Context())
	if err != nil {
		log.Errorf("handlePreCommit1: api error, not proceeding: %+v", err)
		return nil
	}

	if checkTicketExpired(sector.TicketEpoch, ts.Height()) {
		pci, err := m.Api.StateSectorPreCommitInfo(ctx.Context(), m.maddr, sector.SectorNumber, ts.Key())
		if err != nil {
			log.Errorf("handlePreCommit1: StateSectorPreCommitInfo: api error, not proceeding: %+v", err)
			return nil
		}

		if pci == nil {
			return ctx.Send(SectorOldTicket{}) // go get new ticket
		}

		nv, err := m.Api.StateNetworkVersion(ctx.Context(), ts.Key())
		if err != nil {
			log.Errorf("handlePreCommit1: StateNetworkVersion: api error, not proceeding: %+v", err)
			return nil
		}

		av, err := actorstypes.VersionForNetwork(nv)
		if err != nil {
			log.Errorf("handlePreCommit1: VersionForNetwork error, not proceeding: %w", err)
			return nil
		}
		msd, err := policy.GetMaxProveCommitDuration(av, sector.SectorType)
		if err != nil {
			log.Errorf("handlePreCommit1: GetMaxProveCommitDuration error, not proceeding: %w", err)
			return nil
		}

		// if height >  PreCommitEpoch + msd, there is no need to recalculate
		if checkProveCommitExpired(pci.PreCommitEpoch, msd, ts.Height()) {
			return ctx.Send(SectorOldTicket{}) // will be removed
		}
	}

	var pc1o storiface.PreCommit1Out
	err = retrySoftErr(ctx.Context(), func() (err error) {
		pc1o, err = m.sealer.SealPreCommit1(sector.sealingCtx(ctx.Context()), m.minerSector(sector.SectorType, sector.SectorNumber), sector.TicketValue, sector.pieceInfos())
		return err
	})
	if err != nil {
		return ctx.Send(SectorSealPreCommit1Failed{xerrors.Errorf("seal pre commit(1) failed: %w", err)})
	}

	return ctx.Send(SectorPreCommit1{
		PreCommit1Out: pc1o,
	})
}

func (m *Sealing) handlePreCommit2(ctx statemachine.Context, sector SectorInfo) error {
	var cids storiface.SectorCids

	err := retrySoftErr(ctx.Context(), func() (err error) {
		cids, err = m.sealer.SealPreCommit2(sector.sealingCtx(ctx.Context()), m.minerSector(sector.SectorType, sector.SectorNumber), sector.PreCommit1Out)
		return err
	})
	if err != nil {
		return ctx.Send(SectorSealPreCommit2Failed{xerrors.Errorf("seal pre commit(2) failed: %w", err)})
	}

	if cids.Unsealed == cid.Undef {
		return ctx.Send(SectorSealPreCommit1Failed{xerrors.Errorf("seal pre commit(2) returned undefined CommD")})
	}

	return ctx.Send(SectorPreCommit2{
		Unsealed: cids.Unsealed,
		Sealed:   cids.Sealed,
	})
}

func (m *Sealing) preCommitInfo(ctx statemachine.Context, sector SectorInfo) (*miner.SectorPreCommitInfo, big.Int, types.TipSetKey, error) {
	ts, err := m.Api.ChainHead(ctx.Context())
	if err != nil {
		log.Errorf("handlePreCommitting: api error, not proceeding: %+v", err)
		return nil, big.Zero(), types.EmptyTSK, nil
	}

	if err := checkPrecommit(ctx.Context(), m.Address(), sector, ts.Key(), ts.Height(), m.Api); err != nil {
		switch err := err.(type) {
		case *ErrApi:
			log.Errorf("handlePreCommitting: api error, not proceeding: %+v", err)
			return nil, big.Zero(), types.EmptyTSK, nil
		case *ErrBadCommD: // TODO: Should this just back to packing? (not really needed since handlePreCommit1 will do that too)
			return nil, big.Zero(), types.EmptyTSK, ctx.Send(SectorSealPreCommit1Failed{xerrors.Errorf("bad CommD error: %w", err)})
		case *ErrExpiredTicket:
			return nil, big.Zero(), types.EmptyTSK, ctx.Send(SectorSealPreCommit1Failed{xerrors.Errorf("ticket expired: %w", err)})
		case *ErrBadTicket:
			return nil, big.Zero(), types.EmptyTSK, ctx.Send(SectorSealPreCommit1Failed{xerrors.Errorf("bad ticket: %w", err)})
		case *ErrInvalidDeals:
			log.Warnf("invalid deals in sector %d: %v", sector.SectorNumber, err)
			return nil, big.Zero(), types.EmptyTSK, ctx.Send(SectorInvalidDealIDs{Return: RetPreCommitting})
		case *ErrExpiredDeals:
			return nil, big.Zero(), types.EmptyTSK, ctx.Send(SectorDealsExpired{xerrors.Errorf("sector deals expired: %w", err)})
		case *ErrPrecommitOnChain:
			return nil, big.Zero(), types.EmptyTSK, ctx.Send(SectorPreCommitLanded{TipSet: ts.Key()}) // we re-did precommit
		case *ErrSectorNumberAllocated:
			log.Errorf("handlePreCommitFailed: sector number already allocated, not proceeding: %+v", err)
			// TODO: check if the sector is committed (not sure how we'd end up here)
			return nil, big.Zero(), types.EmptyTSK, nil
		default:
			return nil, big.Zero(), types.EmptyTSK, xerrors.Errorf("checkPrecommit sanity check error: %w", err)
		}
	}

	expiration, err := m.pcp.Expiration(ctx.Context(), sector.Pieces...)
	if err != nil {
		return nil, big.Zero(), types.EmptyTSK, ctx.Send(SectorSealPreCommit1Failed{xerrors.Errorf("handlePreCommitting: failed to compute pre-commit expiry: %w", err)})
	}

	nv, err := m.Api.StateNetworkVersion(ctx.Context(), ts.Key())
	if err != nil {
		return nil, big.Zero(), types.EmptyTSK, ctx.Send(SectorSealPreCommit1Failed{xerrors.Errorf("failed to get network version: %w", err)})
	}

	av, err := actorstypes.VersionForNetwork(nv)
	if err != nil {
		return nil, big.Zero(), types.EmptyTSK, ctx.Send(SectorSealPreCommit1Failed{xerrors.Errorf("failed to get actors version: %w", err)})
	}
	msd, err := policy.GetMaxProveCommitDuration(av, sector.SectorType)
	if err != nil {
		return nil, big.Zero(), types.EmptyTSK, ctx.Send(SectorSealPreCommit1Failed{xerrors.Errorf("failed to get max prove commit duration: %w", err)})
	}

	if minExpiration := sector.TicketEpoch + policy.MaxPreCommitRandomnessLookback + msd + policy.GetMinSectorExpiration(); expiration < minExpiration {
		expiration = minExpiration
	}

	// Assume: both precommit msg & commit msg land on chain as early as possible
	maxExtension, err := policy.GetMaxSectorExpirationExtension(nv)
	if err != nil {
		return nil, big.Zero(), types.EmptyTSK, ctx.Send(SectorSealPreCommit1Failed{xerrors.Errorf("failed to get max extension: %w", err)})
	}

	maxExpiration := ts.Height() + policy.GetPreCommitChallengeDelay() + maxExtension
	if expiration > maxExpiration {
		expiration = maxExpiration
	}

	params := &miner.SectorPreCommitInfo{
		Expiration:   expiration,
		SectorNumber: sector.SectorNumber,
		SealProof:    sector.SectorType,

		SealedCID:     *sector.CommR,
		SealRandEpoch: sector.TicketEpoch,
	}

	if sector.hasData() {
		// only CC sectors don't have UnsealedCID
		params.UnsealedCid = sector.CommD
	}

	collateral, err := m.Api.StateMinerPreCommitDepositForPower(ctx.Context(), m.maddr, *params, ts.Key())
	if err != nil {
		return nil, big.Zero(), types.EmptyTSK, xerrors.Errorf("getting initial pledge collateral: %w", err)
	}

	return params, collateral, ts.Key(), nil
}

func (m *Sealing) handlePreCommitting(ctx statemachine.Context, sector SectorInfo) error {
	// note: this is a legacy state handler, normally new sectors won't enter this state
	// but we keep this handler in order to not break existing sector state machines.
	// todo: drop after nv21
	return ctx.Send(SectorPreCommitBatch{})
}

func (m *Sealing) handleSubmitPreCommitBatch(ctx statemachine.Context, sector SectorInfo) error {
	if sector.CommD == nil || sector.CommR == nil {
		return ctx.Send(SectorSealPreCommit1Failed{xerrors.Errorf("sector had nil commR or commD")})
	}

	params, deposit, _, err := m.preCommitInfo(ctx, sector)
	if err != nil {
		return ctx.Send(SectorChainPreCommitFailed{xerrors.Errorf("preCommitInfo: %w", err)})
	}
	if params == nil {
		return nil // event was sent in preCommitInfo
	}

	res, err := m.precommiter.AddPreCommit(ctx.Context(), sector, deposit, params)
	if err != nil {
		return ctx.Send(SectorChainPreCommitFailed{xerrors.Errorf("queuing precommit batch failed: %w", err)})
	}

	if res.Error != "" {
		return ctx.Send(SectorChainPreCommitFailed{xerrors.Errorf("precommit batch error: %s", res.Error)})
	}

	if res.Msg == nil {
		return ctx.Send(SectorChainPreCommitFailed{xerrors.Errorf("batch message was nil")})
	}

	return ctx.Send(SectorPreCommitBatchSent{*res.Msg})
}

func (m *Sealing) handlePreCommitWait(ctx statemachine.Context, sector SectorInfo) error {
	if sector.PreCommitMessage == nil {
		return ctx.Send(SectorChainPreCommitFailed{xerrors.Errorf("precommit message was nil")})
	}

	// would be ideal to just use the events.Called handler, but it wouldn't be able to handle individual message timeouts
	log.Info("Sector precommitted: ", sector.SectorNumber)
	mw, err := m.Api.StateWaitMsg(ctx.Context(), *sector.PreCommitMessage, buildconstants.MessageConfidence, api.LookbackNoLimit, true)
	if err != nil {
		return ctx.Send(SectorChainPreCommitFailed{err})
	}

	switch mw.Receipt.ExitCode {
	case exitcode.Ok:
		// this is what we expect
	case exitcode.SysErrInsufficientFunds:
		fallthrough
	case exitcode.SysErrOutOfGas:
		// gas estimator guessed a wrong number / out of funds:
		return ctx.Send(SectorRetryPreCommit{})
	default:
		log.Error("sector precommit failed: ", mw.Receipt.ExitCode)
		err := xerrors.Errorf("sector precommit failed: %d", mw.Receipt.ExitCode)
		return ctx.Send(SectorChainPreCommitFailed{err})
	}

	log.Info("precommit message landed on chain: ", sector.SectorNumber)

	return ctx.Send(SectorPreCommitLanded{TipSet: mw.TipSet})
}

func (m *Sealing) handleWaitSeed(ctx statemachine.Context, sector SectorInfo) error {
	ts, err := m.Api.ChainHead(ctx.Context())
	if err != nil {
		log.Errorf("handleWaitSeed: api error, not proceeding: %+v", err)
		return nil
	}

	pci, err := m.Api.StateSectorPreCommitInfo(ctx.Context(), m.maddr, sector.SectorNumber, ts.Key())
	if err != nil {
		return xerrors.Errorf("getting precommit info: %w", err)
	}
	if pci == nil {
		return ctx.Send(SectorChainPreCommitFailed{error: xerrors.Errorf("precommit info not found on chain")})
	}

	randHeight := pci.PreCommitEpoch + policy.GetPreCommitChallengeDelay()

	err = m.events.ChainAt(context.Background(), func(ectx context.Context, _ *types.TipSet, curH abi.ChainEpoch) error {
		// in case of null blocks the randomness can land after the tipset we
		// get from the events API
		ts, err := m.Api.ChainHead(ctx.Context())
		if err != nil {
			log.Errorf("handleCommitting: api error, not proceeding: %+v", err)
			return nil
		}

		buf := new(bytes.Buffer)
		if err := m.maddr.MarshalCBOR(buf); err != nil {
			return err
		}
		rand, err := m.Api.StateGetRandomnessFromBeacon(ectx, crypto.DomainSeparationTag_InteractiveSealChallengeSeed, randHeight, buf.Bytes(), ts.Key())
		if err != nil {
			err = xerrors.Errorf("failed to get randomness for computing seal proof (ch %d; rh %d; tsk %x): %w", curH, randHeight, ts.Key(), err)

			_ = ctx.Send(SectorChainPreCommitFailed{error: err})
			return err
		}

		_ = ctx.Send(SectorSeedReady{SeedValue: abi.InteractiveSealRandomness(rand), SeedEpoch: randHeight})

		return nil
	}, func(ctx context.Context, ts *types.TipSet) error {
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
	if sector.CommitMessage != nil {
		log.Warnf("sector %d entered committing state with a commit message cid", sector.SectorNumber)

		ml, err := m.Api.StateSearchMsg(ctx.Context(), types.EmptyTSK, *sector.CommitMessage, api.LookbackNoLimit, true)
		if err != nil {
			log.Warnf("sector %d searching existing commit message %s: %+v", sector.SectorNumber, *sector.CommitMessage, err)
		}

		if ml != nil {
			// some weird retry paths can lead here
			return ctx.Send(SectorRetryCommitWait{})
		}
	}

	cfg, err := m.getConfig()
	if err != nil {
		return xerrors.Errorf("getting config: %w", err)
	}

	if sector.CommD == nil || sector.CommR == nil {
		return ctx.Send(SectorCommitFailed{xerrors.Errorf("sector had nil commR or commD")})
	}

	var c2in storiface.Commit1Out
	if sector.RemoteCommit1Endpoint == "" {
		// Local Commit1
		cids := storiface.SectorCids{
			Unsealed: *sector.CommD,
			Sealed:   *sector.CommR,
		}
		c2in, err = m.sealer.SealCommit1(sector.sealingCtx(ctx.Context()), m.minerSector(sector.SectorType, sector.SectorNumber), sector.TicketValue, sector.SeedValue, sector.pieceInfos(), cids)
		if err != nil {
			return ctx.Send(SectorComputeProofFailed{xerrors.Errorf("computing seal proof failed(1): %w", err)})
		}
	} else {
		// Remote Commit1

		reqData := api.RemoteCommit1Params{
			Ticket:    sector.TicketValue,
			Seed:      sector.SeedValue,
			Unsealed:  *sector.CommD,
			Sealed:    *sector.CommR,
			ProofType: sector.SectorType,
		}
		reqBody, err := json.Marshal(&reqData)
		if err != nil {
			return xerrors.Errorf("marshaling remote commit1 request: %w", err)
		}

		req, err := http.NewRequest("POST", sector.RemoteCommit1Endpoint, bytes.NewReader(reqBody))
		if err != nil {
			return ctx.Send(SectorRemoteCommit1Failed{xerrors.Errorf("creating new remote commit1 request: %w", err)})
		}
		req.Header.Set("Content-Type", "application/json")
		req = req.WithContext(ctx.Context())
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return ctx.Send(SectorRemoteCommit1Failed{xerrors.Errorf("requesting remote commit1: %w", err)})
		}

		defer resp.Body.Close() //nolint:errcheck

		if resp.StatusCode != http.StatusOK {
			return ctx.Send(SectorRemoteCommit1Failed{xerrors.Errorf("remote commit1 received non-200 http response %s", resp.Status)})
		}

		c2in, err = io.ReadAll(resp.Body) // todo some len constraint
		if err != nil {
			return ctx.Send(SectorRemoteCommit1Failed{xerrors.Errorf("reading commit1 response: %w", err)})
		}
	}

	var porepProof storiface.Proof

	if sector.RemoteCommit2Endpoint == "" {
		// Local Commit2

		porepProof, err = m.sealer.SealCommit2(sector.sealingCtx(ctx.Context()), m.minerSector(sector.SectorType, sector.SectorNumber), c2in)
		if err != nil {
			log.Errorw("Commit2 error", "error", err)
			return ctx.Send(SectorComputeProofFailed{xerrors.Errorf("computing seal proof failed(2): %w", err)})
		}
	} else {
		// Remote Commit2

		reqData := api.RemoteCommit2Params{
			ProofType: sector.SectorType,
			Sector:    m.minerSectorID(sector.SectorNumber),

			Commit1Out: c2in,
		}
		reqBody, err := json.Marshal(&reqData)
		if err != nil {
			return xerrors.Errorf("marshaling remote commit2 request: %w", err)
		}

		req, err := http.NewRequest("POST", sector.RemoteCommit2Endpoint, bytes.NewReader(reqBody))
		if err != nil {
			return ctx.Send(SectorRemoteCommit2Failed{xerrors.Errorf("creating new remote commit2 request: %w", err)})
		}
		req.Header.Set("Content-Type", "application/json")
		req = req.WithContext(ctx.Context())
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return ctx.Send(SectorRemoteCommit2Failed{xerrors.Errorf("requesting remote commit2: %w", err)})
		}

		defer resp.Body.Close() //nolint:errcheck

		if resp.StatusCode != http.StatusOK {
			return ctx.Send(SectorRemoteCommit2Failed{xerrors.Errorf("remote commit2 received non-200 http response %s", resp.Status)})
		}

		porepProof, err = io.ReadAll(resp.Body) // todo some len constraint
		if err != nil {
			return ctx.Send(SectorRemoteCommit2Failed{xerrors.Errorf("reading commit2 response: %w", err)})
		}
	}

	{
		ts, err := m.Api.ChainHead(ctx.Context())
		if err != nil {
			log.Errorf("handleCommitting: api error, not proceeding: %+v", err)
			return nil
		}

		if err := m.checkCommit(ctx.Context(), sector, porepProof, ts.Key()); err != nil {
			return ctx.Send(SectorCommitFailed{xerrors.Errorf("commit check error: %w", err)})
		}
	}

	if cfg.FinalizeEarly {
		return ctx.Send(SectorProofReady{
			Proof: porepProof,
		})
	}

	return ctx.Send(SectorCommitted{
		Proof: porepProof,
	})
}

func (m *Sealing) handleSubmitCommit(ctx statemachine.Context, sector SectorInfo) error {
	// like precommit this is a deprecated state, but we keep it around for
	// existing state machines
	// todo: drop after nv21
	return ctx.Send(SectorSubmitCommitAggregate{})
}

// processPieces returns either:
// - a list of piece activation manifests
// - a list of deal IDs, if all non-filler pieces are deal-id pieces
func (m *Sealing) processPieces(ctx context.Context, sector SectorInfo) ([]miner.PieceActivationManifest, error) {
	pams := make([]miner.PieceActivationManifest, 0, len(sector.Pieces))

	for _, piece := range sector.Pieces {
		piece := piece
		if piece.HasDealInfo() {
			info := piece.DealInfo()
			// If we have a dealID then convert to PAM
			if info.Impl().DealID > 0 {
				alloc, err := m.Api.StateGetAllocationIdForPendingDeal(ctx, info.Impl().DealID, types.EmptyTSK)
				if err != nil {
					return nil, xerrors.Errorf("getting allocation for deal %d: %w", info.Impl().DealID, err)
				}
				clid, err := m.Api.StateLookupID(ctx, info.Impl().DealProposal.Client, types.EmptyTSK)
				if err != nil {
					return nil, xerrors.Errorf("getting client address for deal %d: %w", info.Impl().DealID, err)
				}

				clientId, err := address.IDFromAddress(clid)
				if err != nil {
					return nil, xerrors.Errorf("getting client address for deal %d: %w", info.Impl().DealID, err)
				}

				var vac *miner2.VerifiedAllocationKey
				if alloc != verifreg.NoAllocationID {
					vac = &miner2.VerifiedAllocationKey{
						Client: abi.ActorID(clientId),
						ID:     verifreg13.AllocationId(alloc),
					}
				}

				payload, err := cborutil.Dump(info.Impl().DealID)
				if err != nil {
					return nil, xerrors.Errorf("serializing deal id: %w", err)
				}

				pams = append(pams, miner.PieceActivationManifest{
					CID:                   piece.Piece().PieceCID,
					Size:                  piece.Piece().Size,
					VerifiedAllocationKey: vac,
					Notify: []miner2.DataActivationNotification{
						{
							Address: market.Address,
							Payload: payload,
						},
					},
				})
			} else {
				// Add PAM directly
				pams = append(pams, *info.Impl().PieceActivationManifest)
			}
		}
	}

	return pams, nil
}

func (m *Sealing) handleSubmitCommitAggregate(ctx statemachine.Context, sector SectorInfo) error {
	if sector.CommD == nil || sector.CommR == nil {
		return ctx.Send(SectorCommitFailed{xerrors.Errorf("sector had nil commR or commD")})
	}

	pams, err := m.processPieces(ctx.Context(), sector)
	if err != nil {
		return err
	}

	res, err := m.commiter.AddCommit(ctx.Context(), sector, AggregateInput{
		Info: proof.AggregateSealVerifyInfo{
			Number:                sector.SectorNumber,
			Randomness:            sector.TicketValue,
			InteractiveRandomness: sector.SeedValue,
			SealedCID:             *sector.CommR,
			UnsealedCID:           *sector.CommD,
		},
		Proof: sector.Proof,
		Spt:   sector.SectorType,

		ActivationManifest: miner2.SectorActivationManifest{
			SectorNumber: sector.SectorNumber,
			Pieces:       pams,
		},
	})

	if err != nil || res.Error != "" {
		ts, err := m.Api.ChainHead(ctx.Context())
		if err != nil {
			log.Errorf("handleSubmitCommit: api error, not proceeding: %+v", err)
			return nil
		}

		if err := m.checkCommit(ctx.Context(), sector, sector.Proof, ts.Key()); err != nil {
			return ctx.Send(SectorCommitFailed{xerrors.Errorf("commit check error: %w", err)})
		}

		return ctx.Send(SectorRetrySubmitCommit{})
	}

	if e, found := res.FailedSectors[sector.SectorNumber]; found {
		return ctx.Send(SectorCommitFailed{xerrors.Errorf("sector failed in aggregate processing: %s", e)})
	}

	if res.Msg == nil {
		return ctx.Send(SectorCommitFailed{xerrors.Errorf("aggregate message was nil")})
	}

	return ctx.Send(SectorCommitAggregateSent{*res.Msg})
}

func (m *Sealing) handleCommitWait(ctx statemachine.Context, sector SectorInfo) error {
	if sector.CommitMessage == nil {
		log.Errorf("sector %d entered commit wait state without a message cid", sector.SectorNumber)
		return ctx.Send(SectorCommitFailed{xerrors.Errorf("entered commit wait with no commit cid")})
	}

	mw, err := m.Api.StateWaitMsg(ctx.Context(), *sector.CommitMessage, buildconstants.MessageConfidence, api.LookbackNoLimit, true)
	if err != nil {
		return ctx.Send(SectorCommitFailed{xerrors.Errorf("failed to wait for porep inclusion: %w", err)})
	}

	switch mw.Receipt.ExitCode {
	case exitcode.Ok:
		// this is what we expect
	case exitcode.SysErrInsufficientFunds:
		fallthrough
	case exitcode.SysErrOutOfGas:
		// gas estimator guessed a wrong number / out of funds
		return ctx.Send(SectorRetrySubmitCommit{})
	default:
		return ctx.Send(SectorCommitFailed{xerrors.Errorf("submitting sector proof failed (exit=%d, msg=%s) (t:%x; s:%x(%d); p:%x)", mw.Receipt.ExitCode, sector.CommitMessage, sector.TicketValue, sector.SeedValue, sector.SeedEpoch, sector.Proof)})
	}

	si, err := m.Api.StateSectorGetInfo(ctx.Context(), m.maddr, sector.SectorNumber, mw.TipSet)
	if err != nil {
		return ctx.Send(SectorCommitFailed{xerrors.Errorf("proof validation failed, calling StateSectorGetInfo: %w", err)})
	}
	if si == nil {
		return ctx.Send(SectorCommitFailed{xerrors.Errorf("proof validation failed, sector not found in sector set after cron")})
	}

	return ctx.Send(SectorProving{})
}

func (m *Sealing) handleFinalizeSector(ctx statemachine.Context, sector SectorInfo) error {
	// TODO: Maybe wait for some finality

	cfg, err := m.getConfig()
	if err != nil {
		return xerrors.Errorf("getting sealing config: %w", err)
	}

	if err := m.sealer.ReleaseUnsealed(ctx.Context(), m.minerSector(sector.SectorType, sector.SectorNumber), sector.keepUnsealedRanges(sector.Pieces, false, cfg.AlwaysKeepUnsealedCopy)); err != nil {
		return ctx.Send(SectorFinalizeFailed{xerrors.Errorf("release unsealed: %w", err)})
	}

	if err := m.sealer.FinalizeSector(sector.sealingCtx(ctx.Context()), m.minerSector(sector.SectorType, sector.SectorNumber)); err != nil {
		return ctx.Send(SectorFinalizeFailed{xerrors.Errorf("finalize sector: %w", err)})
	}

	if cfg.MakeCCSectorsAvailable && !sector.hasData() {
		return ctx.Send(SectorFinalizedAvailable{})
	}
	return ctx.Send(SectorFinalized{})
}
