package sealing

import (
	"bytes"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-statemachine"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/abi/big"
	"github.com/filecoin-project/specs-actors/actors/builtin"
	"github.com/filecoin-project/specs-actors/actors/builtin/miner"
)

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

func (m *Sealing) handleRemoving(ctx statemachine.Context, sector SectorInfo) error {
	if err := m.sealer.Remove(ctx.Context(), m.minerSector(sector.SectorNumber)); err != nil {
		return ctx.Send(SectorRemoveFailed{err})
	}

	return ctx.Send(SectorRemoved{})
}
