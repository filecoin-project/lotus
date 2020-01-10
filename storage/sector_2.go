package storage

import (
	"context"
	"fmt"

	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/lib/evtsm"
)

type SectorStart struct {
	id     uint64
	pieces []Piece
}
type SectorRestart struct{}

type SectorFatalError struct{ error }

type SectorPacked struct{ pieces []Piece }

type SectorSealed struct {
	commR  []byte
	commD  []byte
	ticket SealTicket
}
type SectorSealFailed struct{ error }

type SectorPreCommitFailed struct{ error }
type SectorPreCommitted struct {
	message cid.Cid
}

type SectorSeedReady struct {
	seed SealSeed
}

type SectorSealCommitFailed struct{ error }
type SectorCommitFailed struct{ error }
type SectorCommitted struct {
	message cid.Cid
	proof   []byte
}

type SectorProving struct{}

type SectorFaultReported struct{ reportMsg cid.Cid }
type SectorFaultedFinal struct{}

type SectorForceState struct {
	state api.SectorState
}

func (m *Miner) Plan(events []evtsm.Event, user interface{}) (interface{}, error) {
	next, err := m.plan(events, user.(*SectorInfo))
	if err != nil || next == nil {
		return nil, err
	}

	return func(ctx evtsm.Context, si SectorInfo) error {
		err := next(ctx, si)
		if err != nil {
			if err := ctx.Send(SectorFatalError{error: err}); err != nil {
				return xerrors.Errorf("error while sending error: reporting %+v: %w", err, err)
			}
		}

		return nil
	}, nil
}

func (m *Miner) plan(events []evtsm.Event, state *SectorInfo) (func(evtsm.Context, SectorInfo) error, error) {
	/////
	// First process all events

	for _, event := range events {
		if err, ok := event.User.(error); ok {
			state.LastErr = fmt.Sprintf("%+v", err)
		}

		switch event := event.User.(type) {
		case SectorStart:
			// TODO: check if state is clean
			state.SectorID = event.id
			state.Pieces = event.pieces
			state.State = api.Packing

		case SectorRestart:
			// noop
		case SectorFatalError:
			log.Errorf("Fatal error on sector %d: %+v", state.SectorID, event.error)
			// TODO: Do we want to mark the state as unrecoverable?
			//  I feel like this should be a softer error, where the user would
			//  be able to send a retry event of some kind
			return nil, nil

		// // TODO: Incoming
		// TODO: for those - look at dealIDs matching chain

		// //
		// Packing

		case SectorPacked:
			// TODO: assert state
			state.Pieces = append(state.Pieces, event.pieces...)
			state.State = api.Unsealed

		// // Unsealed

		case SectorSealFailed:
			// TODO: try to find out the reason, maybe retry
			state.State = api.SealFailed

		case SectorSealed:
			state.CommD = event.commD
			state.CommR = event.commR
			state.Ticket = event.ticket
			state.State = api.PreCommitting

		// // PreCommit

		case SectorPreCommitFailed:
			// TODO: try to find out the reason, maybe retry
			state.State = api.PreCommitFailed
		case SectorPreCommitted:
			state.PreCommitMessage = &event.message
			state.State = api.PreCommitted

		case SectorSeedReady:
			state.Seed = event.seed
			state.State = api.Committing

		// // Commit

		case SectorSealCommitFailed:
			// TODO: try to find out the reason, maybe retry
			state.State = api.SealCommitFailed
		case SectorCommitFailed:
			// TODO: try to find out the reason, maybe retry
			state.State = api.SealFailed
		case SectorCommitted:
			state.Proof = event.proof
			state.State = api.CommitWait
		case SectorProving:
			state.State = api.Proving

		case SectorFaultReported:
			state.FaultReportMsg = &event.reportMsg
			state.State = api.FaultReported
		case SectorFaultedFinal:
			state.State = api.FaultedFinal

		// // Debug triggers
		case SectorForceState:
			state.State = event.state
		}
	}

	/////
	// Now decide what to do next

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

	switch state.State {
	// Happy path
	case api.Packing:
		return m.handlePacking, nil
	case api.Unsealed:
		return m.handleUnsealed, nil
	case api.PreCommitting:
		return m.handlePreCommitting, nil
	case api.PreCommitted:
		return m.handlePreCommitted, nil
	case api.Committing:
		return m.handleCommitting, nil
	case api.CommitWait:
		return m.handleCommitWait, nil
	case api.Proving:
		// TODO: track sector health / expiration
		log.Infof("Proving sector %d", state.SectorID)

	// Handled failure modes
	case api.SealFailed:
		log.Warnf("sector %d entered unimplemented state 'SealFailed'", state.SectorID)
	case api.PreCommitFailed:
		log.Warnf("sector %d entered unimplemented state 'PreCommitFailed'", state.SectorID)
	case api.SealCommitFailed:
		log.Warnf("sector %d entered unimplemented state 'SealCommitFailed'", state.SectorID)
	case api.CommitFailed:
		log.Warnf("sector %d entered unimplemented state 'CommitFailed'", state.SectorID)

		// Faults
	case api.Faulty:
		return m.handleFaulty, nil
	case api.FaultReported:
		return m.handleFaultReported, nil

	// Fatal errors
	case api.UndefinedSectorState:
		log.Error("sector update with undefined state!")
	case api.FailedUnrecoverable:
		log.Errorf("sector %d failed unrecoverably", state.SectorID)
	default:
		log.Errorf("unexpected sector update state: %d", state.State)
	}

	return nil, nil
}

func (m *Miner) restartSectors(ctx context.Context) error {
	trackedSectors, err := m.ListSectors()
	if err != nil {
		log.Errorf("loading sector list: %+v", err)
	}

	for _, sector := range trackedSectors {
		if err := m.sectors.Send(sector.SectorID, SectorRestart{}); err != nil {
			log.Errorf("restarting sector %d: %+v", sector.SectorID, err)
		}
	}

	// TODO: Grab on-chain sector set and diff with trackedSectors

	return nil
}

func (m *Miner) ForceSectorState(ctx context.Context, id uint64, state api.SectorState) error {
	return m.sectors.Send(id, SectorForceState{state})
}
