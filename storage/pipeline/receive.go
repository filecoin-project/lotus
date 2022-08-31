package sealing

import (
	"context"
	"github.com/filecoin-project/go-statemachine"
	"github.com/ipfs/go-datastore"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/api"
)

func (m *Sealing) Receive(ctx context.Context, meta api.RemoteSectorMeta) error {
	si, err := m.checkSectorMeta(ctx, meta)
	if err != nil {
		return err
	}

	exists, err := m.sectors.Has(uint64(meta.Sector.Number))
	if err != nil {
		return xerrors.Errorf("checking if sector exists: %w", err)
	}
	if exists {
		return xerrors.Errorf("sector %d state already exists", meta.Sector.Number)
	}

	err = m.sectors.Send(uint64(meta.Sector.Number), SectorReceive{
		State: si,
	})
	if err != nil {
		return xerrors.Errorf("receiving sector: %w", err)
	}

	return nil
}

func (m *Sealing) checkSectorMeta(ctx context.Context, meta api.RemoteSectorMeta) (SectorInfo, error) {
	{
		mid, err := address.IDFromAddress(m.maddr)
		if err != nil {
			panic(err)
		}

		if meta.Sector.Miner != abi.ActorID(mid) {
			return SectorInfo{}, xerrors.Errorf("sector for wrong actor - expected actor id %d, sector was for actor %d", mid, meta.Sector.Miner)
		}
	}

	{
		// initial sanity check, doesn't prevent races
		_, err := m.GetSectorInfo(meta.Sector.Number)
		if err != nil && !xerrors.Is(err, datastore.ErrNotFound) {
			return SectorInfo{}, err
		}
		if err == nil {
			return SectorInfo{}, xerrors.Errorf("sector with ID %d already exists in the sealing pipeline", meta.Sector.Number)
		}
	}

	{
		spt, err := m.currentSealProof(ctx)
		if err != nil {
			return SectorInfo{}, err
		}

		if meta.Type != spt {
			return SectorInfo{}, xerrors.Errorf("sector seal proof type doesn't match current seal proof type (%d!=%d)", meta.Type, spt)
		}
	}

	var info SectorInfo

	switch SectorState(meta.State) {
	case Proving, Available:
		// todo possibly check
		info.CommitMessage = meta.CommitMessage

		fallthrough
	case SubmitCommit:
		info.PreCommitInfo = meta.PreCommitInfo
		info.PreCommitDeposit = meta.PreCommitDeposit
		info.PreCommitMessage = meta.PreCommitMessage
		info.PreCommitTipSet = meta.PreCommitTipSet

		// todo check
		info.SeedValue = meta.SeedValue
		info.SeedEpoch = meta.SeedEpoch

		// todo validate
		info.Proof = meta.CommitProof

		fallthrough
	case PreCommitting:
		info.TicketValue = meta.TicketValue
		info.TicketEpoch = meta.TicketEpoch

		info.PreCommit1Out = meta.PreCommit1Out

		info.CommD = meta.CommD // todo check cid prefixes
		info.CommR = meta.CommR

		fallthrough
	case GetTicket:
		fallthrough
	case Packing:
		// todo check num free
		info.Return = ReturnState(meta.State) // todo dedupe states
		info.State = ReceiveSector

		info.SectorNumber = meta.Sector.Number
		info.Pieces = meta.Pieces
		info.SectorType = meta.Type

		if err := checkPieces(ctx, m.maddr, meta.Sector.Number, meta.Pieces, m.Api, false); err != nil {
			return SectorInfo{}, xerrors.Errorf("checking pieces: %w", err)
		}

		return info, nil
	default:
		return SectorInfo{}, xerrors.Errorf("imported sector State in not supported")
	}
}

func (m *Sealing) handleReceiveSector(ctx statemachine.Context, sector SectorInfo) error {
	// todo fetch stuff

	return ctx.Send(SectorReceived{})
}
