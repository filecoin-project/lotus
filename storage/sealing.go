package storage

import (
	"context"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/types"
	cid "github.com/ipfs/go-cid"
	xerrors "golang.org/x/xerrors"
)

type SealTicket struct {
	BlockHeight uint64
	TicketBytes []byte
}

type SectorInfo struct {
	State    api.SectorState
	SectorID uint64

	// PreCommit
	CommD  []byte
	CommR  []byte
	Ticket SealTicket

	PreCommitMessage *cid.Cid

	// PreCommitted
	RandHeight uint64
	RandTs     *types.TipSet

	// Committing
	CommitMessage *cid.Cid
}

type sectorUpdate struct {
	newState api.SectorState
	id       uint64
	err      error
	mut      func(*SectorInfo)
}

func (m *Miner) sectorStateLoop(ctx context.Context) {
	// TODO: restore state

	go func() {
		defer log.Warn("quitting deal provider loop")
		defer close(m.stopped)

		for {
			select {
			case sector := <-m.sectorIncoming:
				m.onSectorIncoming(sector)
			case update := <-m.sectorUpdated:
				m.onSectorUpdated(ctx, update)
			case <-m.stop:
				return
			}
		}
	}()
}

func (m *Miner) onSectorIncoming(sector *SectorInfo) {
	if err := m.sectors.Begin(sector.SectorID, sector); err != nil {
		// We may have re-sent the proposal
		log.Errorf("deal tracking failed: %s", err)
		m.failSector(sector.SectorID, err)
		return
	}

	go func() {
		select {
		case m.sectorUpdated <- sectorUpdate{
			newState: api.Unsealed,
			id:       sector.SectorID,
		}:
		case <-m.stop:
			log.Warn("failed to send incoming sector update, miner shutting down")
		}
	}()
}

func (m *Miner) onSectorUpdated(ctx context.Context, update sectorUpdate) {
	log.Infof("Sector %d updated state to %s", update.id, api.SectorStateStr(update.newState))
	var sector SectorInfo
	err := m.sectors.Mutate(update.id, func(s *SectorInfo) error {
		s.State = update.newState
		if update.mut != nil {
			update.mut(s)
		}
		sector = *s
		return nil
	})
	if update.err != nil {
		log.Errorf("sector %d failed: %s", update.id, update.err)
		m.failSector(update.id, update.err)
		return
	}
	if err != nil {
		m.failSector(update.id, err)
		return
	}

	switch update.newState {
	case api.Unsealed:
		m.handle(ctx, sector, m.sealPreCommit, api.PreCommitting)
	case api.PreCommitting:
		m.handle(ctx, sector, m.preCommit, api.PreCommitted)
	case api.PreCommitted:
		m.handle(ctx, sector, m.preCommitted, api.SectorNoUpdate)
	case api.Committing:
		m.handle(ctx, sector, m.committing, api.Proving)
	}
}

func (m *Miner) failSector(id uint64, err error) {
	log.Errorf("sector %d error: %+v", id, err)
	panic(err) // todo: better error handling strategy
}

func (m *Miner) SealSector(ctx context.Context, sid uint64) error {
	log.Infof("Begin sealing sector %d", sid)

	si := &SectorInfo{
		State:    api.UndefinedSectorState,
		SectorID: sid,
	}
	select {
	case m.sectorIncoming <- si:
		return nil
	case <-ctx.Done():
		return xerrors.Errorf("failed to submit sector for sealing, queue full: %w", ctx.Err())
	}
}
