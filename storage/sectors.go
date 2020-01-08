package storage

import (
	"context"
	"fmt"
	"io"
	"math"

	sectorbuilder "github.com/filecoin-project/go-sectorbuilder"
	xerrors "golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/lib/padreader"
)

const NonceIncrement = math.MaxUint64

type sectorUpdate struct {
	newState api.SectorState
	id       uint64
	err      error
	nonce    uint64
	mut      func(*SectorInfo)
}

func (u *sectorUpdate) fatal(err error) *sectorUpdate {
	u.newState = api.FailedUnrecoverable
	u.err = err
	return u
}

func (u *sectorUpdate) error(err error) *sectorUpdate {
	u.err = err
	return u
}

func (u *sectorUpdate) state(m func(*SectorInfo)) *sectorUpdate {
	u.mut = m
	return u
}

func (u *sectorUpdate) to(newState api.SectorState) *sectorUpdate {
	u.newState = newState
	return u
}

func (u *sectorUpdate) setNonce(nc uint64) *sectorUpdate {
	u.nonce = nc
	return u
}

func (m *Miner) UpdateSectorState(ctx context.Context, sector uint64, snonce uint64, state api.SectorState) error {
	select {
	case m.sectorUpdated <- sectorUpdate{
		newState: state,
		nonce:    snonce,
		id:       sector,
	}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (m *Miner) sectorStateLoop(ctx context.Context) error {
	trackedSectors, err := m.ListSectors()
	if err != nil {
		log.Errorf("loading sector list: %+v", err)
	}

	go func() {
		for _, si := range trackedSectors {
			select {
			case m.sectorUpdated <- sectorUpdate{
				newState: si.State,
				nonce:    si.Nonce,
				id:       si.SectorID,
				err:      nil,
				mut:      nil,
			}:
			case <-ctx.Done():
				log.Warn("didn't restart processing for all sectors: ", ctx.Err())
				return
			}
		}
	}()

	{
		// verify on-chain state
		trackedByID := map[uint64]*SectorInfo{}
		for _, si := range trackedSectors {
			i := si
			trackedByID[si.SectorID] = &i
		}

		curTs, err := m.api.ChainHead(ctx)
		if err != nil {
			return xerrors.Errorf("getting chain head: %w", err)
		}

		ps, err := m.api.StateMinerProvingSet(ctx, m.maddr, curTs)
		if err != nil {
			return xerrors.Errorf("getting miner proving set: %w", err)
		}
		for _, ocs := range ps {
			if _, ok := trackedByID[ocs.SectorID]; ok {
				continue // TODO: check state
			}

			// TODO: attempt recovery
			log.Warnf("untracked sector %d found on chain", ocs.SectorID)
		}
	}

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

	return nil
}

func (m *Miner) onSectorIncoming(sector *SectorInfo) {
	has, err := m.sectors.Has(sector.SectorID)
	if err != nil {
		return
	}
	if has {
		log.Warnf("SealPiece called more than once for sector %d", sector.SectorID)
		return
	}

	if err := m.sectors.Begin(sector.SectorID, sector); err != nil {
		log.Errorf("sector tracking failed: %s", err)
		return
	}

	go func() {
		select {
		case m.sectorUpdated <- sectorUpdate{
			newState: api.Packing,
			id:       sector.SectorID,
		}:
		case <-m.stop:
			log.Warn("failed to send incoming sector update, miner shutting down")
		}
	}()
}

func (m *Miner) onSectorUpdated(ctx context.Context, update sectorUpdate) {
	log.Infof("Sector %d updated state to %s", update.id, api.SectorStates[update.newState])
	var sector SectorInfo
	err := m.sectors.Mutate(update.id, func(s *SectorInfo) error {
		if update.nonce < s.Nonce {
			return xerrors.Errorf("update nonce too low, ignoring (%d < %d)", update.nonce, s.Nonce)
		}

		if update.nonce != NonceIncrement {
			s.Nonce = update.nonce
		} else {
			s.Nonce++ // forced update
		}
		s.State = update.newState
		if update.err != nil {
			if s.LastErr != "" {
				s.LastErr += "---------\n\n"
			}
			s.LastErr += fmt.Sprintf("entering state %s: %+v", api.SectorStates[update.newState], update.err)
		}

		if update.mut != nil {
			update.mut(s)
		}
		sector = *s
		return nil
	})
	if update.err != nil {
		log.Errorf("sector %d failed: %+v", update.id, update.err)
	}
	if err != nil {
		log.Errorf("sector %d update error: %+v", update.id, err)
		return
	}

	/*

		*   Empty
		|   |
		|   v
		*<- Packing <- incoming
		|   |
		|   v
		*<- Unsealed <--> SealFailed
		|   |
		|   v
		*   PreCommitting <--> PreCommitFailed
		|   |                  ^
		|   v                  |
		*<- PreCommitted ------/
		|   |||
		|   vvv      v--> SealCommitFailed
		*<- Committing
		|   |        ^--> CommitFailed
		|   v             ^
		*<- CommitWait ---/
		|   |
		|   v
		*<- Proving
		|
		v
		FailedUnrecoverable

		UndefinedSectorState <- ¯\_(ツ)_/¯
		    |                     ^
		    *---------------------/

	*/

	switch update.newState {
	// Happy path
	case api.Packing:
		m.handleSectorUpdate(ctx, sector, m.handlePacking)
	case api.Unsealed:
		m.handleSectorUpdate(ctx, sector, m.handleUnsealed)
	case api.PreCommitting:
		m.handleSectorUpdate(ctx, sector, m.handlePreCommitting)
	case api.PreCommitted:
		m.handleSectorUpdate(ctx, sector, m.handlePreCommitted)
	case api.Committing:
		m.handleSectorUpdate(ctx, sector, m.handleCommitting)
	case api.CommitWait:
		m.handleSectorUpdate(ctx, sector, m.handleCommitWait)
	case api.Proving:
		// TODO: track sector health / expiration
		log.Infof("Proving sector %d", update.id)

	// Handled failure modes
	case api.SealFailed:
		log.Warnf("sector %d entered unimplemented state 'SealFailed'", update.id)
	case api.PreCommitFailed:
		log.Warnf("sector %d entered unimplemented state 'PreCommitFailed'", update.id)
	case api.SealCommitFailed:
		log.Warnf("sector %d entered unimplemented state 'SealCommitFailed'", update.id)
	case api.CommitFailed:
		log.Warnf("sector %d entered unimplemented state 'CommitFailed'", update.id)

		// Faults
	case api.Faulty:
		m.handleSectorUpdate(ctx, sector, m.handleFaulty)
	case api.FaultReported:
		m.handleSectorUpdate(ctx, sector, m.handleFaultReported)

	// Fatal errors
	case api.UndefinedSectorState:
		log.Error("sector update with undefined state!")
	case api.FailedUnrecoverable:
		log.Errorf("sector %d failed unrecoverably", update.id)
	default:
		log.Errorf("unexpected sector update state: %d", update.newState)
	}
}

func (m *Miner) AllocatePiece(size uint64) (sectorID uint64, offset uint64, err error) {
	if padreader.PaddedSize(size) != size {
		return 0, 0, xerrors.Errorf("cannot allocate unpadded piece")
	}

	sid, err := m.sb.AcquireSectorId() // TODO: Put more than one thing in a sector
	if err != nil {
		return 0, 0, xerrors.Errorf("acquiring sector ID: %w", err)
	}

	// offset hard-coded to 0 since we only put one thing in a sector for now
	return sid, 0, nil
}

func (m *Miner) SealPiece(ctx context.Context, size uint64, r io.Reader, sectorID uint64, dealID uint64) error {
	log.Infof("Seal piece for deal %d", dealID)

	ppi, err := m.sb.AddPiece(size, sectorID, r, []uint64{})
	if err != nil {
		return xerrors.Errorf("adding piece to sector: %w", err)
	}

	return m.newSector(ctx, sectorID, dealID, ppi)
}

func (m *Miner) newSector(ctx context.Context, sid uint64, dealID uint64, ppi sectorbuilder.PublicPieceInfo) error {
	si := &SectorInfo{
		SectorID: sid,

		Pieces: []Piece{
			{
				DealID: dealID,

				Size:  ppi.Size,
				CommP: ppi.CommP[:],
			},
		},
	}
	select {
	case m.sectorIncoming <- si:
		return nil
	case <-ctx.Done():
		return xerrors.Errorf("failed to submit sector for sealing, queue full: %w", ctx.Err())
	}
}
