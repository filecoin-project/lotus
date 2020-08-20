//go:generate go run ./gen

package sealing

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"time"

	"golang.org/x/xerrors"

	statemachine "github.com/filecoin-project/go-statemachine"
	"github.com/filecoin-project/specs-actors/actors/abi"
)

func (m *Sealing) Plan(events []statemachine.Event, user interface{}) (interface{}, uint64, error) {
	next, err := m.plan(events, user.(*SectorInfo))
	if err != nil || next == nil {
		return nil, uint64(len(events)), err
	}

	return func(ctx statemachine.Context, si SectorInfo) error {
		err := next(ctx, si)
		if err != nil {
			log.Errorf("unhandled sector error (%d): %+v", si.SectorNumber, err)
			return nil
		}

		return nil
	}, uint64(len(events)), nil // TODO: This processed event count is not very correct
}

var fsmPlanners = map[SectorState]func(events []statemachine.Event, state *SectorInfo) error{
	// Sealing

	UndefinedSectorState: planOne(
		on(SectorStart{}, Empty),
		on(SectorStartCC{}, Packing),
	),
	Empty: planOne(on(SectorAddPiece{}, WaitDeals)),
	WaitDeals: planOne(
		on(SectorAddPiece{}, WaitDeals),
		on(SectorStartPacking{}, Packing),
	),
	Packing: planOne(on(SectorPacked{}, PreCommit1)),
	PreCommit1: planOne(
		on(SectorPreCommit1{}, PreCommit2),
		on(SectorSealPreCommit1Failed{}, SealPreCommit1Failed),
		on(SectorPackingFailed{}, PackingFailed),
	),
	PreCommit2: planOne(
		on(SectorPreCommit2{}, PreCommitting),
		on(SectorSealPreCommit2Failed{}, SealPreCommit2Failed),
		on(SectorPackingFailed{}, PackingFailed),
	),
	PreCommitting: planOne(
		on(SectorSealPreCommit1Failed{}, SealPreCommit1Failed),
		on(SectorPreCommitted{}, PreCommitWait),
		on(SectorChainPreCommitFailed{}, PreCommitFailed),
		on(SectorPreCommitLanded{}, WaitSeed),
	),
	PreCommitWait: planOne(
		on(SectorChainPreCommitFailed{}, PreCommitFailed),
		on(SectorPreCommitLanded{}, WaitSeed),
	),
	WaitSeed: planOne(
		on(SectorSeedReady{}, Committing),
		on(SectorChainPreCommitFailed{}, PreCommitFailed),
	),
	Committing: planCommitting,
	CommitWait: planOne(
		on(SectorProving{}, FinalizeSector),
		on(SectorCommitFailed{}, CommitFailed),
	),

	FinalizeSector: planOne(
		on(SectorFinalized{}, Proving),
		on(SectorFinalizeFailed{}, FinalizeFailed),
	),

	// Sealing errors

	SealPreCommit1Failed: planOne(
		on(SectorRetrySealPreCommit1{}, PreCommit1),
	),
	SealPreCommit2Failed: planOne(
		on(SectorRetrySealPreCommit1{}, PreCommit1),
		on(SectorRetrySealPreCommit2{}, PreCommit2),
	),
	PreCommitFailed: planOne(
		on(SectorRetryPreCommit{}, PreCommitting),
		on(SectorRetryWaitSeed{}, WaitSeed),
		on(SectorSealPreCommit1Failed{}, SealPreCommit1Failed),
		on(SectorPreCommitLanded{}, WaitSeed),
	),
	ComputeProofFailed: planOne(
		on(SectorRetryComputeProof{}, Committing),
		on(SectorSealPreCommit1Failed{}, SealPreCommit1Failed),
	),
	CommitFailed: planOne(
		on(SectorSealPreCommit1Failed{}, SealPreCommit1Failed),
		on(SectorRetryWaitSeed{}, WaitSeed),
		on(SectorRetryComputeProof{}, Committing),
		on(SectorRetryInvalidProof{}, Committing),
		on(SectorRetryPreCommitWait{}, PreCommitWait),
		on(SectorChainPreCommitFailed{}, PreCommitFailed),
		on(SectorRetryPreCommit{}, PreCommitting),
		on(SectorRetryCommitWait{}, CommitWait),
	),
	FinalizeFailed: planOne(
		on(SectorRetryFinalize{}, FinalizeSector),
	),

	// Post-seal

	Proving: planOne(
		on(SectorFaultReported{}, FaultReported),
		on(SectorFaulty{}, Faulty),
		on(SectorRemove{}, Removing),
	),
	Removing: planOne(
		on(SectorRemoved{}, Removed),
		on(SectorRemoveFailed{}, RemoveFailed),
	),
	Faulty: planOne(
		on(SectorFaultReported{}, FaultReported),
	),

	FaultedFinal: final,
	Removed:      final,
}

func (m *Sealing) plan(events []statemachine.Event, state *SectorInfo) (func(statemachine.Context, SectorInfo) error, error) {
	/////
	// First process all events

	for _, event := range events {
		e, err := json.Marshal(event)
		if err != nil {
			log.Errorf("marshaling event for logging: %+v", err)
			continue
		}

		l := Log{
			Timestamp: uint64(time.Now().Unix()),
			Message:   string(e),
			Kind:      fmt.Sprintf("event;%T", event.User),
		}

		if err, iserr := event.User.(xerrors.Formatter); iserr {
			l.Trace = fmt.Sprintf("%+v", err)
		}

		if len(state.Log) > 8000 {
			log.Warnw("truncating sector log", "sector", state.SectorNumber)
			state.Log[2000] = Log{
				Timestamp: uint64(time.Now().Unix()),
				Message:   "truncating log (above 8000 entries)",
				Kind:      fmt.Sprintf("truncate"),
			}

			state.Log = append(state.Log[:2000], state.Log[:6000]...)
		}

		state.Log = append(state.Log, l)
	}

	p := fsmPlanners[state.State]
	if p == nil {
		return nil, xerrors.Errorf("planner for state %s not found", state.State)
	}

	if err := p(events, state); err != nil {
		return nil, xerrors.Errorf("running planner for state %s failed: %w", state.State, err)
	}

	/////
	// Now decide what to do next

	/*

			*   Empty <- incoming deals
			|   |
			|   v
		    *<- WaitDeals <- incoming deals
			|   |
			|   v
			*<- Packing <- incoming committed capacity
			|   |
			|   v
			*<- PreCommit1 <--> SealPreCommit1Failed
			|   |       ^          ^^
			|   |       *----------++----\
			|   v       v          ||    |
			*<- PreCommit2 --------++--> SealPreCommit2Failed
			|   |                  ||
			|   v          /-------/|
			*   PreCommitting <-----+---> PreCommitFailed
			|   |                   |     ^
			|   v                   |     |
			*<- WaitSeed -----------+-----/
			|   |||  ^              |
			|   |||  \--------*-----/
			|   |||           |
			|   vvv      v----+----> ComputeProofFailed
			*<- Committing    |
			|   |        ^--> CommitFailed
			|   v             ^
			*<- CommitWait ---/
			|   |
			|   v
			|   FinalizeSector <--> FinalizeFailed
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

	m.stats.updateSector(m.minerSector(state.SectorNumber), state.State)

	switch state.State {
	// Happy path
	case Empty:
		fallthrough
	case WaitDeals:
		log.Infof("Waiting for deals %d", state.SectorNumber)
	case Packing:
		return m.handlePacking, nil
	case PreCommit1:
		return m.handlePreCommit1, nil
	case PreCommit2:
		return m.handlePreCommit2, nil
	case PreCommitting:
		return m.handlePreCommitting, nil
	case PreCommitWait:
		return m.handlePreCommitWait, nil
	case WaitSeed:
		return m.handleWaitSeed, nil
	case Committing:
		return m.handleCommitting, nil
	case CommitWait:
		return m.handleCommitWait, nil
	case FinalizeSector:
		return m.handleFinalizeSector, nil

	// Handled failure modes
	case SealPreCommit1Failed:
		return m.handleSealPrecommit1Failed, nil
	case SealPreCommit2Failed:
		return m.handleSealPrecommit2Failed, nil
	case PreCommitFailed:
		return m.handlePreCommitFailed, nil
	case ComputeProofFailed:
		return m.handleComputeProofFailed, nil
	case CommitFailed:
		return m.handleCommitFailed, nil
	case FinalizeFailed:
		return m.handleFinalizeFailed, nil

	// Post-seal
	case Proving:
		return m.handleProvingSector, nil
	case Removing:
		return m.handleRemoving, nil
	case Removed:
		return nil, nil

		// Faults
	case Faulty:
		return m.handleFaulty, nil
	case FaultReported:
		return m.handleFaultReported, nil

	// Fatal errors
	case UndefinedSectorState:
		log.Error("sector update with undefined state!")
	case FailedUnrecoverable:
		log.Errorf("sector %d failed unrecoverably", state.SectorNumber)
	default:
		log.Errorf("unexpected sector update state: %s", state.State)
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
			state.State = CommitWait
		case SectorSeedReady: // seed changed :/
			if e.SeedEpoch == state.SeedEpoch && bytes.Equal(e.SeedValue, state.SeedValue) {
				log.Warnf("planCommitting: got SectorSeedReady, but the seed didn't change")
				continue // or it didn't!
			}
			log.Warnf("planCommitting: commit Seed changed")
			e.apply(state)
			state.State = Committing
			return nil
		case SectorComputeProofFailed:
			state.State = ComputeProofFailed
		case SectorSealPreCommit1Failed:
			state.State = SealPreCommit1Failed
		case SectorCommitFailed:
			state.State = CommitFailed
		case SectorRetryCommitWait:
			state.State = CommitWait
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

	cfg, err := m.getConfig()
	if err != nil {
		return xerrors.Errorf("getting the sealing delay: %w", err)
	}

	for _, sector := range trackedSectors {
		if err := m.sectors.Send(uint64(sector.SectorNumber), SectorRestart{}); err != nil {
			log.Errorf("restarting sector %d: %+v", sector.SectorNumber, err)
		}

		if sector.State == WaitDeals {
			if cfg.WaitDealsDelay > 0 {
				timer := time.NewTimer(cfg.WaitDealsDelay)
				go func() {
					<-timer.C
					m.StartPacking(sector.SectorNumber)
				}()
			}
		}
	}

	// TODO: Grab on-chain sector set and diff with trackedSectors

	return nil
}

func (m *Sealing) ForceSectorState(ctx context.Context, id abi.SectorNumber, state SectorState) error {
	return m.sectors.Send(id, SectorForceState{state})
}

func final(events []statemachine.Event, state *SectorInfo) error {
	return xerrors.Errorf("didn't expect any events in state %s, got %+v", state.State, events)
}

func on(mut mutator, next SectorState) func() (mutator, SectorState) {
	return func() (mutator, SectorState) {
		return mut, next
	}
}

func planOne(ts ...func() (mut mutator, next SectorState)) func(events []statemachine.Event, state *SectorInfo) error {
	return func(events []statemachine.Event, state *SectorInfo) error {
		if len(events) != 1 {
			for _, event := range events {
				if gm, ok := event.User.(globalMutator); ok {
					gm.applyGlobal(state)
					return nil
				}
			}
			return xerrors.Errorf("planner for state %s only has a plan for a single event only, got %+v", state.State, events)
		}

		if gm, ok := events[0].User.(globalMutator); ok {
			gm.applyGlobal(state)
			return nil
		}

		for _, t := range ts {
			mut, next := t()

			if reflect.TypeOf(events[0].User) != reflect.TypeOf(mut) {
				continue
			}

			if err, iserr := events[0].User.(error); iserr {
				log.Warnf("sector %d got error event %T: %+v", state.SectorNumber, events[0].User, err)
			}

			events[0].User.(mutator).apply(state)
			state.State = next
			return nil
		}

		_, ok := events[0].User.(Ignorable)
		if ok {
			return nil
		}

		return xerrors.Errorf("planner for state %s received unexpected event %T (%+v)", state.State, events[0].User, events[0])
	}
}
