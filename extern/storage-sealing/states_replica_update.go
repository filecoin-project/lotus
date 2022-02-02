package sealing

import (
	"bytes"

	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/exitcode"
	statemachine "github.com/filecoin-project/go-statemachine"
	api "github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"golang.org/x/xerrors"
)

func (m *Sealing) handleReplicaUpdate(ctx statemachine.Context, sector SectorInfo) error {
	if err := checkPieces(ctx.Context(), m.maddr, sector, m.Api); err != nil { // Sanity check state
		return handleErrors(ctx, err, sector)
	}
	out, err := m.sealer.ReplicaUpdate(sector.sealingCtx(ctx.Context()), m.minerSector(sector.SectorType, sector.SectorNumber), sector.pieceInfos())
	if err != nil {
		return ctx.Send(SectorUpdateReplicaFailed{xerrors.Errorf("replica update failed: %w", err)})
	}
	return ctx.Send(SectorReplicaUpdate{
		Out: out,
	})
}

func (m *Sealing) handleProveReplicaUpdate(ctx statemachine.Context, sector SectorInfo) error {
	if sector.UpdateSealed == nil || sector.UpdateUnsealed == nil {
		return xerrors.Errorf("invalid sector %d with nil UpdateSealed or UpdateUnsealed output", sector.SectorNumber)
	}
	if sector.CommR == nil {
		return xerrors.Errorf("invalid sector %d with nil CommR", sector.SectorNumber)
	}

	vanillaProofs, err := m.sealer.ProveReplicaUpdate1(sector.sealingCtx(ctx.Context()), m.minerSector(sector.SectorType, sector.SectorNumber), *sector.CommR, *sector.UpdateSealed, *sector.UpdateUnsealed)
	if err != nil {
		return ctx.Send(SectorProveReplicaUpdateFailed{xerrors.Errorf("prove replica update (1) failed: %w", err)})
	}

	if err := checkPieces(ctx.Context(), m.maddr, sector, m.Api); err != nil { // Sanity check state
		return handleErrors(ctx, err, sector)
	}

	proof, err := m.sealer.ProveReplicaUpdate2(sector.sealingCtx(ctx.Context()), m.minerSector(sector.SectorType, sector.SectorNumber), *sector.CommR, *sector.UpdateSealed, *sector.UpdateUnsealed, vanillaProofs)
	if err != nil {
		return ctx.Send(SectorProveReplicaUpdateFailed{xerrors.Errorf("prove replica update (2) failed: %w", err)})

	}
	return ctx.Send(SectorProveReplicaUpdate{
		Proof: proof,
	})
}

func (m *Sealing) handleSubmitReplicaUpdate(ctx statemachine.Context, sector SectorInfo) error {

	tok, _, err := m.Api.ChainHead(ctx.Context())
	if err != nil {
		log.Errorf("handleSubmitReplicaUpdate: api error, not proceeding: %+v", err)
		return nil
	}

	if err := checkPieces(ctx.Context(), m.maddr, sector, m.Api); err != nil { // Sanity check state
		return handleErrors(ctx, err, sector)
	}

	if err := checkReplicaUpdate(ctx.Context(), m.maddr, sector, tok, m.Api); err != nil {
		return ctx.Send(SectorSubmitReplicaUpdateFailed{})
	}

	sl, err := m.Api.StateSectorPartition(ctx.Context(), m.maddr, sector.SectorNumber, tok)
	if err != nil {
		log.Errorf("handleSubmitReplicaUpdate: api error, not proceeding: %+v", err)
		return nil
	}
	updateProof, err := sector.SectorType.RegisteredUpdateProof()
	if err != nil {
		log.Errorf("failed to get update proof type from seal proof: %+v", err)
		return ctx.Send(SectorSubmitReplicaUpdateFailed{})
	}
	enc := new(bytes.Buffer)
	params := &miner.ProveReplicaUpdatesParams{
		Updates: []miner.ReplicaUpdate{
			{
				SectorID:           sector.SectorNumber,
				Deadline:           sl.Deadline,
				Partition:          sl.Partition,
				NewSealedSectorCID: *sector.UpdateSealed,
				Deals:              sector.dealIDs(),
				UpdateProofType:    updateProof,
				ReplicaProof:       sector.ReplicaUpdateProof,
			},
		},
	}
	if err := params.MarshalCBOR(enc); err != nil {
		log.Errorf("failed to serialize update replica params: %w", err)
		return ctx.Send(SectorSubmitReplicaUpdateFailed{})
	}

	cfg, err := m.getConfig()
	if err != nil {
		return xerrors.Errorf("getting config: %w", err)
	}

	onChainInfo, err := m.Api.StateSectorGetInfo(ctx.Context(), m.maddr, sector.SectorNumber, tok)
	if err != nil {
		log.Errorf("handleSubmitReplicaUpdate: api error, not proceeding: %+v", err)
		return nil
	}
	sp, err := m.currentSealProof(ctx.Context())
	if err != nil {
		log.Errorf("sealer failed to return current seal proof not proceeding: %+v", err)
		return nil
	}
	virtualPCI := miner.SectorPreCommitInfo{
		SealProof:    sp,
		SectorNumber: sector.SectorNumber,
		SealedCID:    *sector.UpdateSealed,
		//SealRandEpoch: 0,
		DealIDs:    sector.dealIDs(),
		Expiration: onChainInfo.Expiration,
		//ReplaceCapacity: false,
		//ReplaceSectorDeadline: 0,
		//ReplaceSectorPartition: 0,
		//ReplaceSectorNumber: 0,
	}

	collateral, err := m.Api.StateMinerInitialPledgeCollateral(ctx.Context(), m.maddr, virtualPCI, tok)
	if err != nil {
		return xerrors.Errorf("getting initial pledge collateral: %w", err)
	}

	collateral = big.Sub(collateral, onChainInfo.InitialPledge)
	if collateral.LessThan(big.Zero()) {
		collateral = big.Zero()
	}

	collateral, err = collateralSendAmount(ctx.Context(), m.Api, m.maddr, cfg, collateral)
	if err != nil {
		log.Errorf("collateral send amount failed not proceeding: %+v", err)
		return nil
	}

	goodFunds := big.Add(collateral, big.Int(m.feeCfg.MaxCommitGasFee))

	mi, err := m.Api.StateMinerInfo(ctx.Context(), m.maddr, tok)
	if err != nil {
		log.Errorf("handleSubmitReplicaUpdate: api error, not proceeding: %+v", err)
		return nil
	}

	from, _, err := m.addrSel(ctx.Context(), mi, api.CommitAddr, goodFunds, collateral)
	if err != nil {
		log.Errorf("no good address to send replica update message from: %+v", err)
		return ctx.Send(SectorSubmitReplicaUpdateFailed{})
	}
	mcid, err := m.Api.SendMsg(ctx.Context(), from, m.maddr, miner.Methods.ProveReplicaUpdates, big.Zero(), big.Int(m.feeCfg.MaxCommitGasFee), enc.Bytes())
	if err != nil {
		log.Errorf("handleSubmitReplicaUpdate: error sending message: %+v", err)
		return ctx.Send(SectorSubmitReplicaUpdateFailed{})
	}

	return ctx.Send(SectorReplicaUpdateSubmitted{Message: mcid})
}

func (m *Sealing) handleReplicaUpdateWait(ctx statemachine.Context, sector SectorInfo) error {
	if sector.ReplicaUpdateMessage == nil {
		log.Errorf("handleReplicaUpdateWait: no replica update message cid recorded")
		return ctx.Send(SectorSubmitReplicaUpdateFailed{})
	}

	mw, err := m.Api.StateWaitMsg(ctx.Context(), *sector.ReplicaUpdateMessage)
	if err != nil {
		log.Errorf("handleReplicaUpdateWait: failed to wait for message: %+v", err)
		return ctx.Send(SectorSubmitReplicaUpdateFailed{})
	}

	switch mw.Receipt.ExitCode {
	case exitcode.Ok:
		//expected
	case exitcode.SysErrInsufficientFunds:
		fallthrough
	case exitcode.SysErrOutOfGas:
		log.Errorf("gas estimator was wrong or out of funds")
		return ctx.Send(SectorSubmitReplicaUpdateFailed{})
	default:
		return ctx.Send(SectorSubmitReplicaUpdateFailed{})
	}
	si, err := m.Api.StateSectorGetInfo(ctx.Context(), m.maddr, sector.SectorNumber, mw.TipSetTok)
	if err != nil {
		log.Errorf("api err failed to get sector info: %+v", err)
		return ctx.Send(SectorSubmitReplicaUpdateFailed{})
	}
	if si == nil {
		log.Errorf("api err sector not found")
		return ctx.Send(SectorSubmitReplicaUpdateFailed{})
	}

	if !si.SealedCID.Equals(*sector.UpdateSealed) {
		log.Errorf("mismatch of expected onchain sealed cid after replica update, expected %s got %s", sector.UpdateSealed, si.SealedCID)
		return ctx.Send(SectorAbortUpgrade{})
	}
	return ctx.Send(SectorReplicaUpdateLanded{})
}

func (m *Sealing) handleFinalizeReplicaUpdate(ctx statemachine.Context, sector SectorInfo) error {
	cfg, err := m.getConfig()
	if err != nil {
		return xerrors.Errorf("getting sealing config: %w", err)
	}

	if err := m.sealer.FinalizeReplicaUpdate(sector.sealingCtx(ctx.Context()), m.minerSector(sector.SectorType, sector.SectorNumber), sector.keepUnsealedRanges(false, cfg.AlwaysKeepUnsealedCopy)); err != nil {
		return ctx.Send(SectorFinalizeFailed{xerrors.Errorf("finalize sector: %w", err)})
	}

	return ctx.Send(SectorFinalized{})
}

func handleErrors(ctx statemachine.Context, err error, sector SectorInfo) error {
	switch err.(type) {
	case *ErrApi:
		log.Errorf("handleReplicaUpdate: api error, not proceeding: %+v", err)
		return nil
	case *ErrInvalidDeals:
		log.Warnf("invalid deals in sector %d: %v", sector.SectorNumber, err)
		return ctx.Send(SectorInvalidDealIDs{})
	case *ErrExpiredDeals: // Probably not much we can do here, maybe re-pack the sector?
		return ctx.Send(SectorDealsExpired{xerrors.Errorf("expired dealIDs in sector: %w", err)})
	default:
		return xerrors.Errorf("checkPieces sanity check error: %w", err)
	}
}
