package sealing

import (
	"time"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/exitcode"
	"github.com/filecoin-project/go-statemachine"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors/policy"
)

func (m *Sealing) handleFaulty(ctx statemachine.Context, sector SectorInfo) error {
	// TODO: noop because this is now handled by the PoSt scheduler. We can reuse
	//  this state for tracking faulty sectors, or remove it when that won't be
	//  a breaking change
	return nil
}

func (m *Sealing) handleFaultReported(ctx statemachine.Context, sector SectorInfo) error {
	if sector.FaultReportMsg == nil {
		return xerrors.Errorf("entered fault reported state without a FaultReportMsg cid")
	}

	mw, err := m.Api.StateWaitMsg(ctx.Context(), *sector.FaultReportMsg)
	if err != nil {
		return xerrors.Errorf("failed to wait for fault declaration: %w", err)
	}

	if mw.Receipt.ExitCode != 0 {
		log.Errorf("UNHANDLED: declaring sector fault failed (exit=%d, msg=%s) (id: %d)", mw.Receipt.ExitCode, *sector.FaultReportMsg, sector.SectorNumber)
		return xerrors.Errorf("UNHANDLED: submitting fault declaration failed (exit %d)", mw.Receipt.ExitCode)
	}

	return ctx.Send(SectorFaultedFinal{})
}

func (m *Sealing) handleTerminating(ctx statemachine.Context, sector SectorInfo) error {
	// First step of sector termination
	// * See if sector is live
	//  * If not, goto removing
	// * Add to termination queue
	// * Wait for message to land on-chain
	// * Check for correct termination
	// * wait for expiration (+winning lookback?)

	si, err := m.Api.StateSectorGetInfo(ctx.Context(), m.maddr, sector.SectorNumber, nil)
	if err != nil {
		return ctx.Send(SectorTerminateFailed{xerrors.Errorf("getting sector info: %w", err)})
	}

	if si == nil {
		// either already terminated or not committed yet

		pci, err := m.Api.StateSectorPreCommitInfo(ctx.Context(), m.maddr, sector.SectorNumber, nil)
		if err != nil {
			return ctx.Send(SectorTerminateFailed{xerrors.Errorf("checking precommit presence: %w", err)})
		}
		if pci != nil {
			return ctx.Send(SectorTerminateFailed{xerrors.Errorf("sector was precommitted but not proven, remove instead of terminating")})
		}

		return ctx.Send(SectorRemove{})
	}

	termCid, terminated, err := m.terminator.AddTermination(ctx.Context(), m.minerSectorID(sector.SectorNumber))
	if err != nil {
		return ctx.Send(SectorTerminateFailed{xerrors.Errorf("queueing termination: %w", err)})
	}

	if terminated {
		return ctx.Send(SectorTerminating{Message: nil})
	}

	return ctx.Send(SectorTerminating{Message: &termCid})
}

func (m *Sealing) handleTerminateWait(ctx statemachine.Context, sector SectorInfo) error {
	if sector.TerminateMessage == nil {
		return xerrors.New("entered TerminateWait with nil TerminateMessage")
	}

	mw, err := m.Api.StateWaitMsg(ctx.Context(), *sector.TerminateMessage)
	if err != nil {
		return ctx.Send(SectorTerminateFailed{xerrors.Errorf("waiting for terminate message to land on chain: %w", err)})
	}

	if mw.Receipt.ExitCode != exitcode.Ok {
		return ctx.Send(SectorTerminateFailed{xerrors.Errorf("terminate message failed to execute: exit %d: %w", mw.Receipt.ExitCode, err)})
	}

	return ctx.Send(SectorTerminated{TerminatedAt: mw.Height})
}

func (m *Sealing) handleTerminateFinality(ctx statemachine.Context, sector SectorInfo) error {
	for {
		tok, epoch, err := m.Api.ChainHead(ctx.Context())
		if err != nil {
			return ctx.Send(SectorTerminateFailed{xerrors.Errorf("getting chain head: %w", err)})
		}

		nv, err := m.Api.StateNetworkVersion(ctx.Context(), tok)
		if err != nil {
			return ctx.Send(SectorTerminateFailed{xerrors.Errorf("getting network version: %w", err)})
		}

		if epoch >= sector.TerminatedAt+policy.GetWinningPoStSectorSetLookback(nv) {
			return ctx.Send(SectorRemove{})
		}

		toWait := time.Duration(epoch-sector.TerminatedAt+policy.GetWinningPoStSectorSetLookback(nv)) * time.Duration(build.BlockDelaySecs) * time.Second
		select {
		case <-time.After(toWait):
			continue
		case <-ctx.Context().Done():
			return ctx.Context().Err()
		}
	}
}

func (m *Sealing) handleRemoving(ctx statemachine.Context, sector SectorInfo) error {
	if err := m.sealer.Remove(ctx.Context(), m.minerSector(sector.SectorType, sector.SectorNumber)); err != nil {
		return ctx.Send(SectorRemoveFailed{err})
	}

	return ctx.Send(SectorRemoved{})
}

func (m *Sealing) handleProvingSector(ctx statemachine.Context, sector SectorInfo) error {
	// TODO: track sector health / expiration

	cfg, err := m.getConfig()
	if err != nil {
		return xerrors.Errorf("getting sealing config: %w", err)
	}

	if err := m.sealer.ReleaseUnsealed(ctx.Context(), m.minerSector(sector.SectorType, sector.SectorNumber), sector.keepUnsealedRanges(true, cfg.AlwaysKeepUnsealedCopy)); err != nil {
		log.Error(err)
	}

	// TODO: Watch termination
	// TODO: Auto-extend if set

	return nil
}
