package sealing

import (
	"context"
	"reflect"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/lib/statemachine"
)

func (m *Sealing) Plan(events []statemachine.Event, user interface{}) (interface{}, error) {
	next, err := m.plan(events, user.(*SectorInfo))
	if err != nil || next == nil {
		return nil, err
	}

	return func(ctx statemachine.Context, si SectorInfo) error {
		err := next(ctx, si)
		if err != nil {
			if err := ctx.Send(SectorFatalError{error: err}); err != nil {
				return xerrors.Errorf("error while sending error: reporting %+v: %w", err, err)
			}
		}

		return nil
	}, nil
}

var fsmPlanners = []func(events []statemachine.Event, state *SectorInfo) error{
	api.UndefinedSectorState: planOne(on(SectorStart{}, api.Packing)),
	api.Packing:              planOne(on(SectorPacked{}, api.Unsealed)),
	api.Unsealed: planOne(
		on(SectorSealed{}, api.PreCommitting),
		on(SectorSealFailed{}, api.SealFailed),
	),
	api.PreCommitting: planOne(
		on(SectorPreCommitted{}, api.PreCommitted),
		on(SectorPreCommitFailed{}, api.PreCommitFailed),
	),
	api.PreCommitted: planOne(
		on(SectorSeedReady{}, api.Committing),
		on(SectorPreCommitFailed{}, api.PreCommitFailed),
	),
	api.Committing: planCommitting,
	api.CommitWait: planOne(
		on(SectorProving{}, api.Proving),
		on(SectorCommitFailed{}, api.CommitFailed),
	),

	api.Proving: planOne(
		on(SectorFaultReported{}, api.FaultReported),
		on(SectorFaulty{}, api.Faulty),
	),

	api.Faulty: planOne(
		on(SectorFaultReported{}, api.FaultReported),
	),
	api.FaultedFinal: final,
}

func (m *Sealing) plan(events []statemachine.Event, state *SectorInfo) (func(statemachine.Context, SectorInfo) error, error) {
	/////
	// First process all events

	p := fsmPlanners[state.State]
	if p == nil {
		return nil, xerrors.Errorf("planner for state %d not found", state.State)
	}

	if err := p(events, state); err != nil {
		return nil, xerrors.Errorf("running planner for state %s failed: %w", api.SectorStates[state.State], err)
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

func planCommitting(events []statemachine.Event, state *SectorInfo) error {
	for _, event := range events {
		switch e := event.User.(type) {
		case globalMutator:
			if e.applyGlobal(state) {
				return nil
			}
		case SectorCommitted: // the normal case
			e.apply(state)
			state.State = api.CommitWait
		case SectorSeedReady: // seed changed :/
			if e.seed.Equals(&state.Seed) {
				log.Warnf("planCommitting: got SectorSeedReady, but the seed didn't change")
				continue // or it didn't!
			}
			log.Warnf("planCommitting: commit Seed changed")
			e.apply(state)
			state.State = api.Committing
			return nil
		case SectorSealCommitFailed:
			state.State = api.SealCommitFailed
		case SectorSealFailed:
			state.State = api.CommitFailed
		default:
			return xerrors.Errorf("planCommitting got event of unknown type %T, events: %+v", event.User, events)
		}
	}
	return nil
}

func (m *Sealing) restartSectors(ctx context.Context) error {
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

func (m *Sealing) ForceSectorState(ctx context.Context, id uint64, state api.SectorState) error {
	return m.sectors.Send(id, SectorForceState{state})
}

func final(events []statemachine.Event, state *SectorInfo) error {
	return xerrors.Errorf("didn't expect any events in state %s, got %+v", api.SectorStates[state.State], events)
}

func on(mut mutator, next api.SectorState) func() (mutator, api.SectorState) {
	return func() (mutator, api.SectorState) {
		return mut, next
	}
}

func planOne(ts ...func() (mut mutator, next api.SectorState)) func(events []statemachine.Event, state *SectorInfo) error {
	return func(events []statemachine.Event, state *SectorInfo) error {
		if len(events) != 1 {
			for _, event := range events {
				if gm, ok := event.User.(globalMutator); !ok {
					gm.applyGlobal(state)
					return nil
				}
			}
			return xerrors.Errorf("planner for state %s only has a plan for a single event only, got %+v", api.SectorStates[state.State], events)
		}

		for _, t := range ts {
			mut, next := t()

			if reflect.TypeOf(events[0].User) != reflect.TypeOf(mut) {
				continue
			}

			if err, iserr := events[0].User.(error); iserr {
				log.Warnf("sector %d got error event %T: %+v", state.SectorID, events[0].User, err)
			}

			events[0].User.(mutator).apply(state)
			state.State = next
			return nil
		}

		return xerrors.Errorf("planner for state %s received unexpected event %+v", api.SectorStates[state.State], events[0])
	}
}
