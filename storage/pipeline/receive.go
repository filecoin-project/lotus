package sealing

import (
	"context"

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

	switch SectorState(meta.State) {
	case Packing:
		//checkPieces(ctx, m.maddr, meta.Sector.Number, meta.Pieces, m.Api, false)

		fallthrough
	case GetTicket:

		fallthrough
	case PreCommitting:

		fallthrough
	case SubmitCommit:

		fallthrough
	case Proving, Available:

		return nil
	default:
		return xerrors.Errorf("imported sector State in not supported")
	}

}
