package sealing

import (
	"bytes"
	"fmt"
	"github.com/filecoin-project/lotus/chain/types"
	"time"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/lib/statemachine"
)

const minRetryTime = 1 * time.Minute

func failedCooldown(ctx statemachine.Context, sector SectorInfo) error {
	retryStart := time.Unix(int64(sector.Log[len(sector.Log)-1].Timestamp), 0).Add(minRetryTime)
	if len(sector.Log) > 0 && !time.Now().After(retryStart) {
		log.Infof("%s(%d), waiting %s before retrying", api.SectorStates[sector.State], sector.SectorID, time.Until(retryStart))
		select {
		case <-time.After(time.Until(retryStart)):
		case <-ctx.Context().Done():
			return ctx.Context().Err()
		}
	}

	return nil
}

func (m *Sealing) checkPreCommitted(ctx statemachine.Context, sector SectorInfo) (*actors.PreCommittedSector, bool) {
	act, err := m.api.StateGetActor(ctx.Context(), m.maddr, types.EmptyTSK)
	if err != nil {
		log.Errorf("handleSealFailed(%d): temp error: %+v", sector.SectorID, err)
		return nil, true
	}

	st, err := m.api.ChainReadObj(ctx.Context(), act.Head)
	if err != nil {
		log.Errorf("handleSealFailed(%d): temp error: %+v", sector.SectorID, err)
		return nil, true
	}

	var state actors.StorageMinerActorState
	if err := state.UnmarshalCBOR(bytes.NewReader(st)); err != nil {
		log.Errorf("handleSealFailed(%d): temp error: unmarshaling miner state: %+v", sector.SectorID, err)
		return nil, true
	}

	pci, found := state.PreCommittedSectors[fmt.Sprint(sector.SectorID)]
	if found {
		// TODO: If not expired yet, we can just try reusing sealticket
		log.Warnf("sector %d found in miner preseal array", sector.SectorID)
		return pci, true
	}

	return nil, false
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
	if err := checkSeal(ctx.Context(), m.maddr, sector, m.api); err != nil {
		switch err.(type) {
		case *ErrApi:
			log.Errorf("handlePreCommitFailed: api error, not proceeding: %+v", err)
			return nil
		case *ErrBadCommD: // TODO: Should this just back to packing? (not really needed since handleUnsealed will do that too)
			return ctx.Send(SectorSealFailed{xerrors.Errorf("bad CommD error: %w", err)})
		case *ErrExpiredTicket:
			return ctx.Send(SectorSealFailed{xerrors.Errorf("ticket expired error: %w", err)})
		default:
			return xerrors.Errorf("checkSeal sanity check error: %w", err)
		}
	}

	if pci, is := m.checkPreCommitted(ctx, sector); is && pci != nil {
		if sector.PreCommitMessage != nil {
			log.Warn("sector %d is precommitted on chain, but we don't have precommit message", sector.SectorID)
			return nil // TODO: SeedWait needs this currently
		}

		if string(pci.Info.CommR) != string(sector.CommR) {
			log.Warn("sector %d is precommitted on chain, with different CommR: %x != %x", sector.SectorID, pci.Info.CommR, sector.CommR)
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
