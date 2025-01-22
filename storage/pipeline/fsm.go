package sealing

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"reflect"
	"runtime"
	"time"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-statemachine"

	"github.com/filecoin-project/lotus/api"
)

var errSectorRemoved = errors.New("sector removed")

func (m *Sealing) Plan(events []statemachine.Event, user interface{}) (interface{}, uint64, error) {
	next, processed, err := m.plan(events, user.(*SectorInfo))
	if err != nil || next == nil {
		if err == errSectorRemoved && os.Getenv("LOTUS_KEEP_REMOVED_FSM_ACTIVE") != "1" {
			return nil, processed, statemachine.ErrTerminated
		}

		l := Log{
			Timestamp: uint64(time.Now().Unix()),
			Message:   fmt.Sprintf("state machine error: %s", err),
			Kind:      fmt.Sprintf("error;%T", err),
		}
		user.(*SectorInfo).logAppend(l)
		return nil, processed, nil
	}

	return func(ctx statemachine.Context, si SectorInfo) (err error) {
		// handle panics
		defer func() {
			if r := recover(); r != nil {
				buf := make([]byte, 1<<16)
				n := runtime.Stack(buf, false)
				buf = buf[:n]

				l := Log{
					Timestamp: uint64(time.Now().Unix()),
					Message:   fmt.Sprintf("panic: %v\n%s", r, buf),
					Kind:      "panic",
				}
				si.logAppend(l)

				err = fmt.Errorf("panic: %v\n%s", r, buf)
			}
		}()

		// execute the next state
		err = next(ctx, si)
		if err != nil {
			log.Errorf("unhandled sector error (%d): %+v", si.SectorNumber, err)
			return nil
		}

		return nil
	}, processed, nil // TODO: This processed event count is not very correct
}

var fsmPlanners = map[SectorState]func(events []statemachine.Event, state *SectorInfo) (uint64, error){
	// external import
	ReceiveSector: planOne(
		onReturning(SectorReceived{}),
	),

	// Sealing

	UndefinedSectorState: planOne(
		on(SectorStart{}, WaitDeals),
		on(SectorStartCC{}, Packing),
		on(SectorReceive{}, ReceiveSector),
	),
	Empty: planOne( // deprecated
		on(SectorAddPiece{}, AddPiece),
		on(SectorStartPacking{}, Packing),
	),
	WaitDeals: planOne(
		on(SectorAddPiece{}, AddPiece),
		on(SectorStartPacking{}, Packing),
	),
	AddPiece: planOne(
		on(SectorPieceAdded{}, WaitDeals),
		apply(SectorStartPacking{}),
		apply(SectorAddPiece{}),
		on(SectorAddPieceFailed{}, AddPieceFailed),
	),
	Packing: planOne(on(SectorPacked{}, GetTicket)),
	GetTicket: planOne(
		on(SectorTicket{}, PreCommit1),
		on(SectorCommitFailed{}, CommitFailed),
	),
	PreCommit1: planOne(
		on(SectorPreCommit1{}, PreCommit2),
		on(SectorSealPreCommit1Failed{}, SealPreCommit1Failed),
		on(SectorDealsExpired{}, DealsExpired),
		on(SectorInvalidDealIDs{}, RecoverDealIDs),
		on(SectorOldTicket{}, GetTicket),
	),
	PreCommit2: planOne(
		on(SectorPreCommit2{}, SubmitPreCommitBatch),
		on(SectorSealPreCommit2Failed{}, SealPreCommit2Failed),
		on(SectorSealPreCommit1Failed{}, SealPreCommit1Failed),
	),
	PreCommitting: planOne(
		on(SectorPreCommitBatch{}, SubmitPreCommitBatch),
		on(SectorPreCommitted{}, PreCommitWait),
		on(SectorSealPreCommit1Failed{}, SealPreCommit1Failed),
		on(SectorChainPreCommitFailed{}, PreCommitFailed),
		on(SectorPreCommitLanded{}, WaitSeed),
		on(SectorDealsExpired{}, DealsExpired),
		on(SectorInvalidDealIDs{}, RecoverDealIDs),
	),
	SubmitPreCommitBatch: planOne(
		on(SectorPreCommitBatchSent{}, PreCommitBatchWait),
		on(SectorSealPreCommit1Failed{}, SealPreCommit1Failed),
		on(SectorChainPreCommitFailed{}, PreCommitFailed),
		on(SectorPreCommitLanded{}, WaitSeed),
		on(SectorDealsExpired{}, DealsExpired),
		on(SectorInvalidDealIDs{}, RecoverDealIDs),
	),
	PreCommitBatchWait: planOne(
		on(SectorChainPreCommitFailed{}, PreCommitFailed),
		on(SectorPreCommitLanded{}, WaitSeed),
		on(SectorRetryPreCommit{}, PreCommitting),
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
	CommitFinalize: planOne(
		on(SectorFinalized{}, SubmitCommitAggregate),
		on(SectorFinalizedAvailable{}, SubmitCommitAggregate),
		on(SectorFinalizeFailed{}, CommitFinalizeFailed),
	),
	SubmitCommit: planOne(
		on(SectorCommitSubmitted{}, CommitWait),
		on(SectorSubmitCommitAggregate{}, SubmitCommitAggregate),
		on(SectorCommitFailed{}, CommitFailed),
	),
	SubmitCommitAggregate: planOne(
		on(SectorCommitAggregateSent{}, CommitAggregateWait),
		on(SectorCommitFailed{}, CommitFailed),
		on(SectorRetrySubmitCommit{}, SubmitCommit),
	),
	CommitWait: planOne(
		on(SectorProving{}, FinalizeSector),
		on(SectorCommitFailed{}, CommitFailed),
		on(SectorRetrySubmitCommit{}, SubmitCommit),
	),
	CommitAggregateWait: planOne(
		on(SectorProving{}, FinalizeSector),
		on(SectorCommitFailed{}, CommitFailed),
		on(SectorRetrySubmitCommit{}, SubmitCommit),
	),

	FinalizeSector: planOne(
		onWithCB(SectorFinalized{}, Proving, maybeNotifyRemoteDone(true, "Proving")),
		onWithCB(SectorFinalizedAvailable{}, Available, maybeNotifyRemoteDone(true, "Available")),
		on(SectorFinalizeFailed{}, FinalizeFailed),
	),

	// Snap deals
	SnapDealsWaitDeals: planOne(
		on(SectorAddPiece{}, SnapDealsAddPiece),
		on(SectorStartPacking{}, SnapDealsPacking),
		on(SectorAbortUpgrade{}, AbortUpgrade),
	),
	SnapDealsAddPiece: planOne(
		on(SectorPieceAdded{}, SnapDealsWaitDeals),
		apply(SectorStartPacking{}),
		apply(SectorAddPiece{}),
		on(SectorAddPieceFailed{}, SnapDealsAddPieceFailed),
		on(SectorAbortUpgrade{}, AbortUpgrade),
	),
	SnapDealsPacking: planOne(
		on(SectorPacked{}, UpdateReplica),
		on(SectorAbortUpgrade{}, AbortUpgrade),
	),
	UpdateReplica: planOne(
		on(SectorReplicaUpdate{}, ProveReplicaUpdate),
		on(SectorUpdateReplicaFailed{}, ReplicaUpdateFailed),
		on(SectorDealsExpired{}, SnapDealsDealsExpired),
		on(SectorInvalidDealIDs{}, SnapDealsRecoverDealIDs),
		on(SectorAbortUpgrade{}, AbortUpgrade),
	),
	ProveReplicaUpdate: planOne(
		on(SectorProveReplicaUpdate{}, FinalizeReplicaUpdate),
		on(SectorProveReplicaUpdateFailed{}, ReplicaUpdateFailed),
		on(SectorDealsExpired{}, SnapDealsDealsExpired),
		on(SectorInvalidDealIDs{}, SnapDealsRecoverDealIDs),
		on(SectorAbortUpgrade{}, AbortUpgrade),
	),
	FinalizeReplicaUpdate: planOne(
		on(SectorFinalized{}, SubmitReplicaUpdate),
		on(SectorFinalizeFailed{}, FinalizeReplicaUpdateFailed),
	),
	SubmitReplicaUpdate: planOne(
		on(SectorReplicaUpdateSubmitted{}, ReplicaUpdateWait),
		on(SectorSubmitReplicaUpdateFailed{}, ReplicaUpdateFailed),
		on(SectorDeadlineImmutable{}, WaitMutable),
	),
	WaitMutable: planOne(
		on(SectorDeadlineMutable{}, SubmitReplicaUpdate),
		on(SectorAbortUpgrade{}, AbortUpgrade),
	),
	ReplicaUpdateWait: planOne(
		on(SectorReplicaUpdateLanded{}, UpdateActivating),
		on(SectorSubmitReplicaUpdateFailed{}, ReplicaUpdateFailed),
		on(SectorAbortUpgrade{}, AbortUpgrade),
	),
	UpdateActivating: planOne(
		on(SectorUpdateActive{}, ReleaseSectorKey),
	),
	ReleaseSectorKey: planOne(
		on(SectorKeyReleased{}, Proving),
		on(SectorReleaseKeyFailed{}, ReleaseSectorKeyFailed),
	),
	// Sealing errors

	AddPieceFailed: planOne(
		on(SectorRetryWaitDeals{}, WaitDeals),
		apply(SectorStartPacking{}),
		apply(SectorAddPiece{}),
	),
	SealPreCommit1Failed: planOne(
		on(SectorRetrySealPreCommit1{}, PreCommit1),
	),
	SealPreCommit2Failed: planOne(
		on(SectorRetrySealPreCommit1{}, PreCommit1),
		on(SectorRetrySealPreCommit2{}, PreCommit2),
	),
	PreCommitFailed: planOne(
		on(SectorRetryPreCommit{}, PreCommitting),
		on(SectorRetryPreCommitWait{}, PreCommitWait),
		on(SectorRetryWaitSeed{}, WaitSeed),
		on(SectorSealPreCommit1Failed{}, SealPreCommit1Failed),
		on(SectorPreCommitLanded{}, WaitSeed),
		onWithCB(SectorDealsExpired{}, DealsExpired, maybeNotifyRemoteDone(false, "DealsExpired")),
		on(SectorInvalidDealIDs{}, RecoverDealIDs),
	),
	ComputeProofFailed: planOne(
		on(SectorRetryComputeProof{}, Committing),
		on(SectorSealPreCommit1Failed{}, SealPreCommit1Failed),
	),
	RemoteCommitFailed: planOne(
		on(SectorRetryComputeProof{}, Committing),
	),
	CommitFinalizeFailed: planOne(
		on(SectorRetryFinalize{}, CommitFinalize),
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
		on(SectorRetrySubmitCommit{}, SubmitCommit),
		onWithCB(SectorDealsExpired{}, DealsExpired, maybeNotifyRemoteDone(false, "DealsExpired")),
		on(SectorInvalidDealIDs{}, RecoverDealIDs),
		onWithCB(SectorTicketExpired{}, Removing, maybeNotifyRemoteDone(false, "Removing")),
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

	// Snap Deals Errors
	SnapDealsAddPieceFailed: planOne(
		on(SectorRetryWaitDeals{}, SnapDealsWaitDeals),
		apply(SectorStartPacking{}),
		apply(SectorAddPiece{}),
		on(SectorAbortUpgrade{}, AbortUpgrade),
	),
	SnapDealsDealsExpired: planOne(
		on(SectorAbortUpgrade{}, AbortUpgrade),
	),
	SnapDealsRecoverDealIDs: planOne(
		on(SectorUpdateDealIDs{}, SubmitReplicaUpdate),
		on(SectorAbortUpgrade{}, AbortUpgrade),
	),
	AbortUpgrade: planOneOrIgnore(
		on(SectorRevertUpgradeToProving{}, Proving),
	),
	ReplicaUpdateFailed: planOne(
		on(SectorRetrySubmitReplicaUpdateWait{}, ReplicaUpdateWait),
		on(SectorRetrySubmitReplicaUpdate{}, SubmitReplicaUpdate),
		on(SectorRetryReplicaUpdate{}, UpdateReplica),
		on(SectorRetryProveReplicaUpdate{}, ProveReplicaUpdate),
		on(SectorInvalidDealIDs{}, SnapDealsRecoverDealIDs),
		on(SectorDealsExpired{}, SnapDealsDealsExpired),
		on(SectorAbortUpgrade{}, AbortUpgrade),
	),
	ReleaseSectorKeyFailed: planOne(
		on(SectorUpdateActive{}, ReleaseSectorKey),
	),
	FinalizeReplicaUpdateFailed: planOne(
		on(SectorRetryFinalize{}, FinalizeReplicaUpdate),
	),

	// Post-seal

	Proving: planOne(
		on(SectorFaultReported{}, FaultReported),
		on(SectorFaulty{}, Faulty),
		on(SectorMarkForUpdate{}, Available),
	),
	Available: planOne(
		on(SectorStartCCUpdate{}, SnapDealsWaitDeals),
		on(SectorAbortUpgrade{}, Proving),
	),
	Terminating: planOne(
		on(SectorTerminating{}, TerminateWait),
		on(SectorTerminateFailed{}, TerminateFailed),
	),
	TerminateWait: planOne(
		on(SectorTerminated{}, TerminateFinality),
		on(SectorTerminateFailed{}, TerminateFailed),
	),
	TerminateFinality: planOne(
		on(SectorTerminateFailed{}, TerminateFailed),
		// SectorRemove (global)
	),
	TerminateFailed: planOne(
	// SectorTerminating (global)
	),
	Removing: planOneOrIgnore(
		on(SectorRemoved{}, Removed),
		on(SectorRemoveFailed{}, RemoveFailed),
	),
	RemoveFailed: planOne(
	// SectorRemove (global)
	),
	Faulty: planOne(
		on(SectorFaultReported{}, FaultReported),
	),

	FaultReported: final, // not really supported right now

	FaultedFinal: final,
	Removed:      finalRemoved,

	FailedUnrecoverable: final,
}

func (state *SectorInfo) logAppend(l Log) {
	if len(state.Log) > 8000 {
		log.Warnw("truncating sector log", "sector", state.SectorNumber)
		state.Log[2000] = Log{
			Timestamp: uint64(time.Now().Unix()),
			Message:   "truncating log (above 8000 entries)",
			Kind:      fmt.Sprintf("truncate"),
		}

		state.Log = append(state.Log[:2000], state.Log[6000:]...)
	}

	state.Log = append(state.Log, l)
}

func (m *Sealing) logEvents(events []statemachine.Event, state *SectorInfo) {
	for _, event := range events {
		log.Debugw("sector event", "sector", state.SectorNumber, "type", fmt.Sprintf("%T", event.User), "event", event.User)

		e, err := json.Marshal(event)
		if err != nil {
			log.Errorf("marshaling event for logging: %+v", err)
			continue
		}

		if event.User == (SectorRestart{}) {
			continue // don't log on every fsm restart
		}

		if len(e) > 8000 {
			e = []byte(string(e[:8000]) + "... truncated")
		}

		l := Log{
			Timestamp: uint64(time.Now().Unix()),
			Message:   string(e),
			Kind:      fmt.Sprintf("event;%T", event.User),
		}

		if err, iserr := event.User.(xerrors.Formatter); iserr {
			l.Trace = fmt.Sprintf("%+v", err)
		}

		state.logAppend(l)
	}
}

func (m *Sealing) plan(events []statemachine.Event, state *SectorInfo) (func(statemachine.Context, SectorInfo) error, uint64, error) {
	/////
	// First process all events

	m.logEvents(events, state)

	if m.notifee != nil {
		defer func(before SectorInfo) {
			m.notifee(before, *state)
		}(*state) // take safe-ish copy of the before state (except for nested pointers)
	}

	p := fsmPlanners[state.State]
	if p == nil {
		if len(events) == 1 {
			if _, ok := events[0].User.(globalMutator); ok {
				p = planOne() // in case we're in a really weird state, allow restart / update state / remove
			}
		}

		if p == nil {
			return nil, 0, xerrors.Errorf("planner for state %s not found", state.State)
		}
	}

	processed, err := p(events, state)
	if err != nil {
		return nil, processed, xerrors.Errorf("running planner for state %s failed: %w", state.State, err)
	}

	/////
	// Now decide what to do next

	/*

				      UndefinedSectorState (start)
				       v                     |
				*<- WaitDeals <-> AddPiece   |
				|   |   /--------------------/
				|   v   v
				*<- Packing <- incoming committed capacity
				|   |
				|   v
				|   GetTicket
				|   |   ^
				|   v   |
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

	*/

	if err := m.onUpdateSector(context.TODO(), state); err != nil {
		log.Errorw("update sector stats", "error", err)
	}

	switch state.State {
	case ReceiveSector:
		return m.handleReceiveSector, processed, nil

	// Happy path
	case Empty:
		fallthrough
	case WaitDeals:
		return m.handleWaitDeals, processed, nil
	case AddPiece:
		return m.handleAddPiece, processed, nil
	case Packing:
		return m.handlePacking, processed, nil
	case GetTicket:
		return m.handleGetTicket, processed, nil
	case PreCommit1:
		return m.handlePreCommit1, processed, nil
	case PreCommit2:
		return m.handlePreCommit2, processed, nil
	case PreCommitting:
		return m.handlePreCommitting, processed, nil
	case SubmitPreCommitBatch:
		return m.handleSubmitPreCommitBatch, processed, nil
	case PreCommitBatchWait:
		fallthrough
	case PreCommitWait:
		return m.handlePreCommitWait, processed, nil
	case WaitSeed:
		return m.handleWaitSeed, processed, nil
	case Committing:
		return m.handleCommitting, processed, nil
	case SubmitCommit:
		return m.handleSubmitCommit, processed, nil
	case SubmitCommitAggregate:
		return m.handleSubmitCommitAggregate, processed, nil
	case CommitAggregateWait:
		fallthrough
	case CommitWait:
		return m.handleCommitWait, processed, nil
	case CommitFinalize:
		fallthrough
	case FinalizeSector:
		return m.handleFinalizeSector, processed, nil

	// Snap deals updates
	case SnapDealsWaitDeals:
		return m.handleWaitDeals, processed, nil
	case SnapDealsAddPiece:
		return m.handleAddPiece, processed, nil
	case SnapDealsPacking:
		return m.handlePacking, processed, nil
	case UpdateReplica:
		return m.handleReplicaUpdate, processed, nil
	case ProveReplicaUpdate:
		return m.handleProveReplicaUpdate, processed, nil
	case SubmitReplicaUpdate:
		return m.handleSubmitReplicaUpdate, processed, nil
	case WaitMutable:
		return m.handleWaitMutable, processed, nil
	case ReplicaUpdateWait:
		return m.handleReplicaUpdateWait, processed, nil
	case FinalizeReplicaUpdate:
		return m.handleFinalizeReplicaUpdate, processed, nil
	case UpdateActivating:
		return m.handleUpdateActivating, processed, nil
	case ReleaseSectorKey:
		return m.handleReleaseSectorKey, processed, nil

	// Handled failure modes
	case AddPieceFailed:
		return m.handleAddPieceFailed, processed, nil
	case SealPreCommit1Failed:
		return m.handleSealPrecommit1Failed, processed, nil
	case SealPreCommit2Failed:
		return m.handleSealPrecommit2Failed, processed, nil
	case PreCommitFailed:
		return m.handlePreCommitFailed, processed, nil
	case ComputeProofFailed:
		return m.handleComputeProofFailed, processed, nil
	case RemoteCommitFailed:
		return m.handleRemoteCommitFailed, processed, nil
	case CommitFailed:
		return m.handleCommitFailed, processed, nil
	case CommitFinalizeFailed:
		fallthrough
	case FinalizeFailed:
		return m.handleFinalizeFailed, processed, nil
	case PackingFailed: // DEPRECATED: remove this for the next reset
		state.State = DealsExpired
		fallthrough
	case DealsExpired:
		return m.handleDealsExpired, processed, nil
	case RecoverDealIDs:
		return m.HandleRecoverDealIDs, processed, nil

	// Snap Deals failure modes
	case SnapDealsAddPieceFailed:
		return m.handleAddPieceFailed, processed, nil

	case SnapDealsDealsExpired:
		return m.handleDealsExpiredSnapDeals, processed, nil
	case SnapDealsRecoverDealIDs:
		return m.handleSnapDealsRecoverDealIDs, processed, nil
	case ReplicaUpdateFailed:
		return m.handleSubmitReplicaUpdateFailed, processed, nil
	case ReleaseSectorKeyFailed:
		return m.handleReleaseSectorKeyFailed, 0, err
	case FinalizeReplicaUpdateFailed:
		return m.handleFinalizeFailed, processed, nil
	case AbortUpgrade:
		return m.handleAbortUpgrade, processed, nil

	// Post-seal
	case Proving:
		return m.handleProvingSector, processed, nil
	case Available:
		return m.handleAvailableSector, processed, nil
	case Terminating:
		return m.handleTerminating, processed, nil
	case TerminateWait:
		return m.handleTerminateWait, processed, nil
	case TerminateFinality:
		return m.handleTerminateFinality, processed, nil
	case TerminateFailed:
		return m.handleTerminateFailed, processed, nil
	case Removing:
		return m.handleRemoving, processed, nil
	case Removed:
		return nil, processed, errSectorRemoved

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
		return nil, processed, xerrors.Errorf("sector update with undefined state")
	case FailedUnrecoverable:
		log.Errorf("sector %d failed unrecoverably", state.SectorNumber)
		return nil, processed, xerrors.Errorf("sector %d failed unrecoverably", state.SectorNumber)
	default:
		log.Errorf("unexpected sector update state: %s", state.State)
		return nil, processed, xerrors.Errorf("unexpected sector update state: %s", state.State)
	}
}

func (m *Sealing) onUpdateSector(ctx context.Context, state *SectorInfo) error {
	if m.getConfig == nil {
		return nil // tests
	}

	cfg, err := m.getConfig()
	if err != nil {
		return xerrors.Errorf("getting config: %w", err)
	}

	shouldUpdateInput := m.stats.updateSector(ctx, cfg, m.minerSectorID(state.SectorNumber), state.State)

	// trigger more input processing when we've dipped below max sealing limits
	if shouldUpdateInput {
		sp, err := m.currentSealProof(ctx)
		if err != nil {
			return xerrors.Errorf("getting seal proof type: %w", err)
		}

		go func() {
			m.inputLk.Lock()
			defer m.inputLk.Unlock()

			if err := m.updateInput(ctx, sp); err != nil {
				log.Errorf("%+v", err)
			}
		}()
	}

	return nil
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
			state.State = SubmitCommitAggregate
		case SectorProofReady: // early finalize
			e.apply(state)
			state.State = CommitFinalize
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
		case SectorRemoteCommit1Failed, SectorRemoteCommit2Failed:
			state.State = RemoteCommitFailed
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
	defer m.startupWait.Done()

	trackedSectors, err := m.ListSectors()
	if err != nil {
		log.Errorf("loading sector list: %+v", err)
	}

	for _, sector := range trackedSectors {
		if err := m.sectors.Send(uint64(sector.SectorNumber), SectorRestart{}); err != nil {
			log.Errorf("restarting sector %d: %+v", sector.SectorNumber, err)
		}
	}

	// TODO: Grab on-chain sector set and diff with trackedSectors

	return nil
}

func (m *Sealing) ForceSectorState(ctx context.Context, id abi.SectorNumber, state SectorState) error {
	m.startupWait.Wait()
	return m.sectors.Send(id, SectorForceState{state})
}

// as sector has been removed, it's no needs to care about later events,
// just returns length of events as `processed` is ok.
func finalRemoved(events []statemachine.Event, state *SectorInfo) (uint64, error) {
	return uint64(len(events)), nil
}

func final(events []statemachine.Event, state *SectorInfo) (uint64, error) {
	if len(events) > 0 {
		if gm, ok := events[0].User.(globalMutator); ok {
			gm.applyGlobal(state)
			return 1, nil
		}
	}

	return 0, xerrors.Errorf("didn't expect any events in state %s, got %+v", state.State, events)
}

func on(mut mutator, next SectorState) func() (mutator, func(*SectorInfo) (bool, error)) {
	return func() (mutator, func(*SectorInfo) (bool, error)) {
		return mut, func(state *SectorInfo) (bool, error) {
			state.State = next
			return false, nil
		}
	}
}

func onWithCB(mut mutator, next SectorState, cb func(info *SectorInfo)) func() (mutator, func(*SectorInfo) (bool, error)) {
	return func() (mutator, func(*SectorInfo) (bool, error)) {
		return mut, func(state *SectorInfo) (bool, error) {
			cb(state)
			state.State = next
			return false, nil
		}
	}
}

// like `on`, but doesn't change state
func apply(mut mutator) func() (mutator, func(*SectorInfo) (bool, error)) {
	return func() (mutator, func(*SectorInfo) (bool, error)) {
		return mut, func(state *SectorInfo) (bool, error) {
			return true, nil
		}
	}
}

func onReturning(mut mutator) func() (mutator, func(*SectorInfo) (bool, error)) {
	return func() (mutator, func(*SectorInfo) (bool, error)) {
		return mut, func(state *SectorInfo) (bool, error) {
			if state.Return == "" {
				return false, xerrors.Errorf("return state not set")
			}

			state.State = SectorState(state.Return)
			state.Return = ""
			return false, nil
		}
	}
}

func planOne(ts ...func() (mut mutator, next func(*SectorInfo) (more bool, err error))) func(events []statemachine.Event, state *SectorInfo) (uint64, error) {
	return func(events []statemachine.Event, state *SectorInfo) (uint64, error) {
	eloop:
		for i, event := range events {
			if gm, ok := event.User.(globalMutator); ok {
				gm.applyGlobal(state)
				return uint64(i + 1), nil
			}

			for _, t := range ts {
				mut, next := t()

				if reflect.TypeOf(event.User) != reflect.TypeOf(mut) {
					continue
				}

				if err, iserr := event.User.(error); iserr {
					log.Warnf("sector %d got error event %T: %+v", state.SectorNumber, event.User, err)
				}

				event.User.(mutator).apply(state)
				more, err := next(state)
				if err != nil || !more {
					return uint64(i + 1), err
				}

				continue eloop
			}

			_, ok := event.User.(Ignorable)
			if ok {
				continue
			}

			return uint64(i + 1), xerrors.Errorf("planner for state %s received unexpected event %T (%+v)", state.State, event.User, event)
		}

		return uint64(len(events)), nil
	}
}

// planOneOrIgnore but ignores unhandled states without erroring, this prevents the need to handle all possible events creating
// error during forced override
func planOneOrIgnore(ts ...func() (mut mutator, next func(*SectorInfo) (more bool, err error))) func(events []statemachine.Event, state *SectorInfo) (uint64, error) {
	f := planOne(ts...)
	return func(events []statemachine.Event, state *SectorInfo) (uint64, error) {
		cnt, err := f(events, state)
		if err != nil {
			log.Warnf("planOneOrIgnore: ignoring error from planOne: %s", err)
		}
		return cnt, nil
	}
}

// maybeNotifyRemoteDone will send sealing-done notification to the RemoteSealingDone
// if the RemoteSealingDoneEndpoint is set. If RemoteSealingDoneEndpoint is not set,
// this is no-op
func maybeNotifyRemoteDone(success bool, state string) func(*SectorInfo) {
	return func(sector *SectorInfo) {
		if sector.RemoteSealingDoneEndpoint == "" {
			return
		}

		reqData := api.RemoteSealingDoneParams{
			Successful:    success,
			State:         state,
			CommitMessage: sector.CommitMessage,
		}
		reqBody, err := json.Marshal(&reqData)
		if err != nil {
			log.Errorf("marshaling remote done notification request params: %s", err)
			return
		}

		req, err := http.NewRequest("POST", sector.RemoteSealingDoneEndpoint, bytes.NewReader(reqBody))
		if err != nil {
			log.Errorf("creating new remote done notification request: %s", err)
			return
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			log.Errorf("sending remote done notification: %s", err)
			return
		}

		defer resp.Body.Close() //nolint:errcheck

		if resp.StatusCode != http.StatusOK {
			log.Errorf("remote done notification received non-200 http response %s", resp.Status)
			return
		}
	}
}
