package sealing

import (
	"context"
	"errors"
	cbg "github.com/whyrusleeping/cbor-gen"

	"github.com/ipfs/go-datastore"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/api"
)

func (m *Sealing) Receive(ctx context.Context, meta api.RemoteSectorMeta) error {
	if err := m.checkSectorMeta(ctx, meta); err != nil {
		return err
	}

	err := m.sectors.Get(uint64(meta.Sector.Number)).Get(&cbg.Deferred{})
	if errors.Is(err, datastore.ErrNotFound) {

	} else if err != nil {
		return xerrors.Errorf("checking if sector exists: %w", err)
	} else if err == nil {
		return xerrors.Errorf("sector %d state already exists", meta.Sector.Number)
	}

	panic("impl me")
}

func (m *Sealing) checkSectorMeta(ctx context.Context, meta api.RemoteSectorMeta) error {
	{
		mid, err := address.IDFromAddress(m.maddr)
		if err != nil {
			panic(err)
		}

		if meta.Sector.Miner != abi.ActorID(mid) {
			return xerrors.Errorf("sector for wrong actor - expected actor id %d, sector was for actor %d", mid, meta.Sector.Miner)
		}
	}

	{
		// initial sanity check, doesn't prevent races
		_, err := m.GetSectorInfo(meta.Sector.Number)
		if err != nil && !xerrors.Is(err, datastore.ErrNotFound) {
			return err
		}
		if err == nil {
			return xerrors.Errorf("sector with ID %d already exists in the sealing pipeline", meta.Sector.Number)
		}
	}

	{
		spt, err := m.currentSealProof(ctx)
		if err != nil {
			return err
		}

		if meta.Type != spt {
			return xerrors.Errorf("sector seal proof type doesn't match current seal proof type (%d!=%d)", meta.Type, spt)
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
		info.State = SectorState(meta.State) // todo dedupe states
		info.SectorNumber = meta.Sector.Number
		info.Pieces = meta.Pieces
		info.SectorType = meta.Type

		if err := checkPieces(ctx, m.maddr, meta.Sector.Number, meta.Pieces, m.Api, false); err != nil {
			return xerrors.Errorf("checking pieces: %w", err)
		}

		return nil
	default:
		return xerrors.Errorf("imported sector State in not supported")
	}
}
