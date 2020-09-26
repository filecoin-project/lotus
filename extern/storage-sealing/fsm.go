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

	"github.com/filecoin-project/go-state-types/abi"
	statemachine "github.com/filecoin-project/go-statemachine"
)

func (m *Sealing) Plan(events []statemachine.Event, user interface{}) (interface{}, uint64, error) {
	next, processed, err := m.plan(events, user.(*SectorInfo))
	if err != nil || next == nil {
		return nil, processed, err
	}

	return func(ctx statemachine.Context, si SectorInfo) error {
		err := next(ctx, si)
		if err != nil {
			log.Errorf("unhandled sector error (%d): %+v", si.SectorNumber, err)
			return nil
		}

		return nil
	}, processed, nil // TODO: This processed event count is not very correct
}

var fsmPlanners = map[SectorState]func(events []statemachine.Event, state *SectorInfo) (uint64, error){
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
		on(SectorDealsExpired{}, DealsExpired),
		on(SectorInvalidDealIDs{}, RecoverDealIDs),
	),
	PreCommit2: planOne(
		on(SectorPreCommit2{}, PreCommitting),
		on(SectorSealPreCommit2Failed{}, SealPreCommit2Failed),
	),
	PreCommitting: planOne(
		on(SectorSealPreCommit1Failed{}, SealPreCommit1Failed),
		on(SectorPreCommitted{}, PreCommitWait),
		on(SectorChainPreCommitFailed{}, PreCommitFailed),
		on(SectorPreCommitLanded{}, WaitSeed),
		on(SectorDealsExpired{}, DealsExpired),
		on(SectorInvalidDealIDs{}, RecoverDealIDs),
	),
	PreCommitWait: planOne(
		on(SectorChainPreCommitFailed{}, PreCommitFailed),
		on(SectorPreCommitLanded{}, WaitSeed),
		on(SectorRetryPreCommit{}, PreCommitting),
	),
	WaitSeed: planOne(
		on(SectorSeedReady{}, Committing),
		on(SectorChainPreCommitFailed{}, PreCommitFailed),
	),
	Committing: planCommitting,
	SubmitCommit: planOne(
		on(SectorCommitSubmitted{}, CommitWait),
		on(SectorCommitFailed{}, CommitFailed),
	),
	CommitWait: planOne(
		on(SectorProving{}, FinalizeSector),
		on(SectorCommitFailed{}, CommitFailed),
		on(SectorRetrySubmitCommit{}, SubmitCommit),
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
		on(SectorDealsExpired{}, DealsExpired),
		on(SectorInvalidDealIDs{}, RecoverDealIDs),
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
		on(SectorDealsExpired{}, DealsExpired),
		on(SectorInvalidDealIDs{}, RecoverDealIDs),
	),
	FinalizeFailed: planOne(
		on(SectorRetryFinalize{}, FinalizeSector),
	),
	PackingFailed: planOne(), // TODO: Deprecated, remove
	DealsExpired:  planOne(
	// SectorRemove (global)
	),
	RecoverDealIDs: planOne(
		onReturning(SectorUpdateDealIDs{}),
	),

	// Post-seal

	Proving: planOne(
		on(SectorFaultReported{}, FaultReported),
		on(SectorFaulty{}, Faulty),
	),
	Removing: planOne(
		on(SectorRemoved{}, Removed),
		on(SectorRemoveFailed{}, RemoveFailed),
	),
	RemoveFailed: planOne(
	// SectorRemove (global)
	),
	Faulty: planOne(
		on(SectorFaultReported{}, FaultReported),
	),

	FaultedFinal: final,
	Removed:      final,
}

func (m *Sealing) plan(events []statemachine.Event, state *SectorInfo) (func(statemachine.Context, SectorInfo) error, uint64, error) {
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

	if m.notifee != nil {
		defer func(before SectorInfo) {
			m.notifee(before, *state)
		}(*state) // take safe-ish copy of the before state (except for nested pointers)
	}

	p := fsmPlanners[state.State]
	if p == nil {
		return nil, 0, xerrors.Errorf("planner for state %s not found", state.State)
	}

	processed, err := p(events, state)
	if err != nil {
		return nil, 0, xerrors.Errorf("running planner for state %s failed: %w", state.State, err)
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
		        |   SubmitCommit  |
		        |   |             |
		        |   v             |
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
		return m.handlePacking, processed, nil
	case PreCommit1:
		return m.handlePreCommit1, processed, nil
	case PreCommit2:
		return m.handlePreCommit2, processed, nil
	case PreCommitting:
		return m.handlePreCommitting, processed, nil
	case PreCommitWait:
		return m.handlePreCommitWait, processed, nil
	case WaitSeed:
		return m.handleWaitSeed, processed, nil
	case Committing:
		return m.handleCommitting, processed, nil
	case SubmitCommit:
		return m.handleSubmitCommit, processed, nil
	case CommitWait:
		return m.handleCommitWait, processed, nil
	case FinalizeSector:
		return m.handleFinalizeSector, processed, nil

	// Handled failure modes
	case SealPreCommit1Failed:
		return m.handleSealPrecommit1Failed, processed, nil
	case SealPreCommit2Failed:
		return m.handleSealPrecommit2Failed, processed, nil
	case PreCommitFailed:
		return m.handlePreCommitFailed, processed, nil
	case ComputeProofFailed:
		return m.handleComputeProofFailed, processed, nil
	case CommitFailed:
		return m.handleCommitFailed, processed, nil
	case FinalizeFailed:
		return m.handleFinalizeFailed, processed, nil
	case PackingFailed: // DEPRECATED: remove this for the next reset
		state.State = DealsExpired
		fallthrough
	case DealsExpired:
		return m.handleDealsExpired, processed, nil
	case RecoverDealIDs:
		return m.handleRecoverDealIDs, processed, nil

	// Post-seal
	case Proving:
		return m.handleProvingSector, processed, nil
	case Removing:
		return m.handleRemoving, processed, nil
	case Removed:
		return nil, processed, nil

	case RemoveFailed:
		return m.handleRemoveFailed, processed, nil

		// Faults
	case Faulty:
		return m.handleFaulty, processed, nil
	case FaultReported:
		return m.handleFaultReported, processed, nil

	// Fatal errors
	case UndefinedSectorState:
		log.Error("sector update with undefined state!")
	case FailedUnrecoverable:
		log.Errorf("sector %d failed unrecoverably", state.SectorNumber)
	default:
		log.Errorf("unexpected sector update state: %s", state.State)
	}

	return nil, processed, nil
}

func planCommitting(events []statemachine.Event, state *SectorInfo) (uint64, error) {
	for i, event := range events {
		switch e := event.User.(type) {
		case globalMutator:
			if e.applyGlobal(state) {
				return uint64(i + 1), nil
			}
		case SectorCommitted: // the normal case
			e.apply(state)
			state.State = SubmitCommit
		case SectorSeedReady: // seed changed :/
			if e.SeedEpoch == state.SeedEpoch && bytes.Equal(e.SeedValue, state.SeedValue) {
				log.Warnf("planCommitting: got SectorSeedReady, but the seed didn't change")
				continue // or it didn't!
			}

			log.Warnf("planCommitting: commit Seed changed")
			e.apply(state)
			state.State = Committing
			return uint64(i + 1), nil
		case SectorComputeProofFailed:
			state.State = ComputeProofFailed
		case SectorSealPreCommit1Failed:
			state.State = SealPreCommit1Failed
		case SectorCommitFailed:
			state.State = CommitFailed
		case SectorRetryCommitWait:
			state.State = CommitWait
		default:
			return uint64(i), xerrors.Errorf("planCommitting got event of unknown type %T, events: %+v", event.User, events)
		}
	}
	return uint64(len(events)), nil
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

	m.unsealedInfoMap.lk.Lock()
	defer m.unsealedInfoMap.lk.Unlock()
	for _, sector := range trackedSectors {
		if err := m.sectors.Send(uint64(sector.SectorNumber), SectorRestart{}); err != nil {
			log.Errorf("restarting sector %d: %+v", sector.SectorNumber, err)
		}

		if sector.State == WaitDeals {

			// put the sector in the unsealedInfoMap
			if _, ok := m.unsealedInfoMap.infos[sector.SectorNumber]; ok {
				// something's funky here, but probably safe to move on
				log.Warnf("sector %v was already in the unsealedInfoMap when restarting", sector.SectorNumber)
			} else {
				ui := UnsealedSectorInfo{}
				for _, p := range sector.Pieces {
					if p.DealInfo != nil {
						ui.numDeals++
					}
					ui.stored += p.Piece.Size
					ui.pieceSizes = append(ui.pieceSizes, p.Piece.Size.Unpadded())
				}

				m.unsealedInfoMap.infos[sector.SectorNumber] = ui
			}

			// start a fresh timer for the sector
			if cfg.WaitDealsDelay > 0 {
				timer := time.NewTimer(cfg.WaitDealsDelay)
				go func() {
					<-timer.C
					if err := m.StartPacking(sector.SectorNumber); err != nil {
						log.Errorf("starting sector %d: %+v", sector.SectorNumber, err)
					}
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

func final(events []statemachine.Event, state *SectorInfo) (uint64, error) {
	return 0, xerrors.Errorf("didn't expect any events in state %s, got %+v", state.State, events)
}

func on(mut mutator, next SectorState) func() (mutator, func(*SectorInfo) error) {
	return func() (mutator, func(*SectorInfo) error) {
		return mut, func(state *SectorInfo) error {
			state.State = next
			return nil
		}
	}
}

func onReturning(mut mutator) func() (mutator, func(*SectorInfo) error) {
	return func() (mutator, func(*SectorInfo) error) {
		return mut, func(state *SectorInfo) error {
			if state.Return == "" {
				return xerrors.Errorf("return state not set")
			}

			state.State = SectorState(state.Return)
			state.Return = ""
			return nil
		}
	}
}

func planOne(ts ...func() (mut mutator, next func(*SectorInfo) error)) func(events []statemachine.Event, state *SectorInfo) (uint64, error) {
	return func(events []statemachine.Event, state *SectorInfo) (uint64, error) {
		if gm, ok := events[0].User.(globalMutator); ok {
			gm.applyGlobal(state)
			return 1, nil
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
			return 1, next(state)
		}

		_, ok := events[0].User.(Ignorable)
		if ok {
			return 1, nil
		}

		return 0, xerrors.Errorf("planner for state %s received unexpected event %T (%+v)", state.State, events[0].User, events[0])
	}
}
