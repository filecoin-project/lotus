package storage

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/filecoin-project/go-state-types/crypto"

	"github.com/ipfs/go-cid"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/dline"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/specs-actors/actors/builtin/miner"
	tutils "github.com/filecoin-project/specs-actors/support/testing"
)

var dummyCid cid.Cid

func init() {
	dummyCid, _ = cid.Parse("bafkqaaa")
}

type proveRes struct {
	posts []miner.SubmitWindowedPoStParams
	err   error
}

type mockAPI struct {
	sm            *stateMachine
	periodStart   abi.ChainEpoch
	deadlineIdx   uint64
	proveResult   chan *proveRes
	submitResult  chan error
	onStateChange chan struct{}

	tsLock sync.RWMutex
	ts     *types.TipSet

	abortCalledLock sync.RWMutex
	abortCalled     bool
}

func newMockAPI() *mockAPI {
	return &mockAPI{
		proveResult:   make(chan *proveRes),
		onStateChange: make(chan struct{}),
		submitResult:  make(chan error),
		periodStart:   abi.ChainEpoch(0),
	}
}

func (m *mockAPI) makeTs(t *testing.T, h abi.ChainEpoch) *types.TipSet {
	m.tsLock.Lock()
	defer m.tsLock.Unlock()

	m.ts = makeTs(t, h)
	return m.ts
}

func makeTs(t *testing.T, h abi.ChainEpoch) *types.TipSet {
	var parents []cid.Cid
	msgcid := dummyCid

	a, _ := address.NewFromString("t00")
	b, _ := address.NewFromString("t02")
	var ts, err = types.NewTipSet([]*types.BlockHeader{
		{
			Height: h,
			Miner:  a,

			Parents: parents,

			Ticket: &types.Ticket{VRFProof: []byte{byte(h % 2)}},

			ParentStateRoot:       dummyCid,
			Messages:              msgcid,
			ParentMessageReceipts: dummyCid,

			BlockSig:     &crypto.Signature{Type: crypto.SigTypeBLS},
			BLSAggregate: &crypto.Signature{Type: crypto.SigTypeBLS},
		},
		{
			Height: h,
			Miner:  b,

			Parents: parents,

			Ticket: &types.Ticket{VRFProof: []byte{byte((h + 1) % 2)}},

			ParentStateRoot:       dummyCid,
			Messages:              msgcid,
			ParentMessageReceipts: dummyCid,

			BlockSig:     &crypto.Signature{Type: crypto.SigTypeBLS},
			BLSAggregate: &crypto.Signature{Type: crypto.SigTypeBLS},
		},
	})

	require.NoError(t, err)

	return ts
}

func (m *mockAPI) setDeadlineParams(periodStart abi.ChainEpoch, deadlineIdx uint64) {
	m.periodStart = periodStart
	m.deadlineIdx = deadlineIdx
}

func (m *mockAPI) StateMinerProvingDeadline(ctx context.Context, address address.Address, key types.TipSetKey) (*dline.Info, error) {
	m.tsLock.RLock()
	defer m.tsLock.RUnlock()

	currentEpoch := m.ts.Height()
	deadline := miner.NewDeadlineInfo(m.periodStart, m.deadlineIdx, currentEpoch)
	return deadline, nil
}

func (m *mockAPI) startGeneratePoST(
	ctx context.Context,
	ts *types.TipSet,
	deadline *dline.Info,
	completeGeneratePoST CompleteGeneratePoSTCb,
) context.CancelFunc {
	ctx, cancel := context.WithCancel(ctx)

	go func() {
		defer cancel()

		select {
		case psRes := <-m.proveResult:
			completeGeneratePoST(psRes.posts, psRes.err)
		case <-ctx.Done():
			completeGeneratePoST(nil, ctx.Err())
		}
	}()

	return cancel
}

func (m *mockAPI) startSubmitPoST(
	ctx context.Context,
	ts *types.TipSet,
	deadline *dline.Info,
	posts []miner.SubmitWindowedPoStParams,
	completeSubmitPoST CompleteSubmitPoSTCb,
) context.CancelFunc {
	ctx, cancel := context.WithCancel(ctx)

	go func() {
		defer cancel()

		select {
		case err := <-m.submitResult:
			completeSubmitPoST(err)
		case <-ctx.Done():
			completeSubmitPoST(ctx.Err())
		}
	}()

	return cancel
}

func (m *mockAPI) onAbort(ts *types.TipSet, deadline *dline.Info, state PoSTStatus) {
	m.abortCalledLock.Lock()
	defer m.abortCalledLock.Unlock()
	m.abortCalled = true
}

func (m *mockAPI) wasAbortCalled() bool {
	m.abortCalledLock.RLock()
	defer m.abortCalledLock.RUnlock()
	return m.abortCalled
}

func (m *mockAPI) failPost(err error, ts *types.TipSet, deadline *dline.Info) {
}

func (m *mockAPI) setStateMachine(sm *stateMachine) {
	m.sm = sm
}

// TestStateMachine tests that we move from each state to the next as expected
func TestStateMachine(t *testing.T) {
	periodStart := abi.ChainEpoch(0)
	deadlineIdx := uint64(0)
	s := makeScaffolding(t, periodStart, deadlineIdx)
	ctx := s.ctx
	sm := s.sm
	mock := s.mock

	// Starting state
	require.Equal(t, PoSTStatusStart, sm.CurrentState())

	// Trigger a head change
	go triggerHeadChange(t, s, abi.ChainEpoch(1))

	// Should start proving
	currentState := <-s.stateChanges
	require.Equal(t, PoSTStatusProving, currentState)

	// Send a response to the call to generate proofs
	posts := []miner.SubmitWindowedPoStParams{{Deadline: deadlineIdx}}
	mock.proveResult <- &proveRes{posts: posts}

	// Should move to proving complete
	currentState = <-s.stateChanges
	require.Equal(t, PoSTStatusProvingComplete, currentState)

	// Should not advance from PoSTStatusProvingComplete until the chain has
	// reached sufficient height
	select {
	case <-s.stateChanges:
	case <-time.After(10 * time.Millisecond):
	}
	require.Equal(t, PoSTStatusProvingComplete, currentState)

	// Move to the correct height to submit the proof
	go func() {
		ts := mock.makeTs(t, 1+SubmitConfidence)
		err := sm.HeadChange(ctx, ts, false)
		require.NoError(t, err)
	}()

	// Should move to submitting state
	currentState = <-s.stateChanges
	require.Equal(t, PoSTStatusSubmitting, currentState)

	// Send a response to the submit call
	mock.submitResult <- nil

	// Should move to the complete state
	currentState = <-s.stateChanges
	require.Equal(t, PoSTStatusComplete, currentState)
}

// TestStateMachineFromProvingToSubmittingNoHeadChange tests that when the
// chain is already advanced past the confidence interval, we should move from
// proving to submitting without stopping at prove complete.
func TestStateMachineFromProvingToSubmittingNoHeadChange(t *testing.T) {
	periodStart := abi.ChainEpoch(0)
	deadlineIdx := uint64(0)
	s := makeScaffolding(t, periodStart, deadlineIdx)
	ctx := s.ctx
	sm := s.sm
	mock := s.mock

	// Starting state
	require.Equal(t, PoSTStatusStart, sm.CurrentState())

	// Trigger a head change
	go triggerHeadChange(t, s, abi.ChainEpoch(1))

	// Should move to proving state
	currentState := <-s.stateChanges
	require.Equal(t, PoSTStatusProving, currentState)

	// Trigger a head change that advances the chain beyond the submit
	// confidence
	ts := mock.makeTs(t, 1+SubmitConfidence)
	err := sm.HeadChange(ctx, ts, false)
	require.NoError(t, err)

	// Send a reply to the call to generate proofs
	posts := []miner.SubmitWindowedPoStParams{{Deadline: deadlineIdx}}
	mock.proveResult <- &proveRes{posts: posts}

	// Should move to the proving complete state
	currentState = <-s.stateChanges
	require.Equal(t, PoSTStatusProvingComplete, currentState)

	// Should move directly to submitting state with no further head changes
	currentState = <-s.stateChanges
	require.Equal(t, PoSTStatusSubmitting, currentState)
}

// TestStateMachineFromProvingEmptyProofsToComplete tests that when there are no
// proofs generated we should move directly from proving to the complete
// state (without submitting anything to chain)
func TestStateMachineFromProvingEmptyProofsToComplete(t *testing.T) {
	periodStart := abi.ChainEpoch(0)
	deadlineIdx := uint64(0)
	s := makeScaffolding(t, periodStart, deadlineIdx)
	ctx := s.ctx
	sm := s.sm
	mock := s.mock

	// Starting state
	require.Equal(t, PoSTStatusStart, sm.CurrentState())

	// Trigger a head change
	go triggerHeadChange(t, s, abi.ChainEpoch(1))

	// Should move to proving state
	currentState := <-s.stateChanges
	require.Equal(t, PoSTStatusProving, currentState)

	// Trigger a head change that advances the chain beyond the submit
	// confidence
	ts := mock.makeTs(t, 1+SubmitConfidence)
	err := sm.HeadChange(ctx, ts, false)
	require.NoError(t, err)

	// Send a reply to the call to generate proofs with an empty proofs array
	posts := []miner.SubmitWindowedPoStParams{}
	mock.proveResult <- &proveRes{posts: posts}

	// Should move directly to the complete state
	currentState = <-s.stateChanges
	require.Equal(t, PoSTStatusComplete, currentState)
}

// TestStateMachineDontStartUntilProvingPeriod tests that the state machine
// doesn't start until the proving period has been reached.
func TestStateMachineDontStartUntilProvingPeriod(t *testing.T) {
	periodStart := miner.WPoStProvingPeriod
	deadlineIdx := uint64(1)
	s := makeScaffolding(t, periodStart, deadlineIdx)
	ctx := s.ctx
	sm := s.sm
	mock := s.mock

	// Start state
	require.Equal(t, PoSTStatusStart, sm.CurrentState())

	// Trigger a head change
	go triggerHeadChange(t, s, abi.ChainEpoch(10))

	// Nothing should happen because the proving period has not started
	select {
	case <-s.stateChanges:
	case <-time.After(10 * time.Millisecond):
	}
	require.Equal(t, PoSTStatusStart, sm.CurrentState())

	// Advance the head to the proving period
	go func() {
		ts := mock.makeTs(t, miner.WPoStProvingPeriod)
		err := sm.HeadChange(ctx, ts, false)
		require.NoError(t, err)
	}()

	// Should start proving
	currentState := <-s.stateChanges
	require.Equal(t, PoSTStatusProving, currentState)
}

func TestStateMachineStartProvingNextDeadline(t *testing.T) {
	periodStart := abi.ChainEpoch(0)
	deadlineIdx := uint64(0)
	s := makeScaffolding(t, periodStart, deadlineIdx)
	ctx := s.ctx
	sm := s.sm
	mock := s.mock

	// Starting state
	require.Equal(t, PoSTStatusStart, sm.CurrentState())

	// Trigger a head change that advances the chain beyond the submit
	// confidence
	currentEpoch := abi.ChainEpoch(1 + SubmitConfidence)
	go triggerHeadChange(t, s, currentEpoch)

	// Should move to proving state
	currentState := <-s.stateChanges
	require.Equal(t, PoSTStatusProving, currentState)

	// Send a reply to the call to generate proofs
	posts := []miner.SubmitWindowedPoStParams{{Deadline: deadlineIdx}}
	mock.proveResult <- &proveRes{posts: posts}

	// Should move to the proving complete state
	currentState = <-s.stateChanges
	require.Equal(t, PoSTStatusProvingComplete, currentState)

	// Should move to submitting state
	currentState = <-s.stateChanges
	require.Equal(t, PoSTStatusSubmitting, currentState)

	// Send a response to the submit call
	mock.submitResult <- nil

	// Should move to the complete state
	currentState = <-s.stateChanges
	require.Equal(t, PoSTStatusComplete, currentState)

	// Trigger head change that advances the chain to the Challenge epoch for
	// the next deadline
	go func() {
		current := miner.NewDeadlineInfo(periodStart, deadlineIdx, currentEpoch)
		next := nextDeadline(current)
		ts := mock.makeTs(t, next.Challenge)
		err := sm.HeadChange(ctx, ts, false)
		require.NoError(t, err)
	}()

	currentState = <-s.stateChanges
	require.Equal(t, PoSTStatusProving, currentState)
}

func TestStateMachineProvingErrorRecovery(t *testing.T) {
	periodStart := abi.ChainEpoch(0)
	deadlineIdx := uint64(0)

	s := makeScaffolding(t, periodStart, deadlineIdx)
	sm := s.sm
	mock := s.mock

	// Starting state
	require.Equal(t, PoSTStatusStart, sm.CurrentState())

	// Trigger a head change
	go triggerHeadChange(t, s, abi.ChainEpoch(1))

	// Should start proving
	currentState := <-s.stateChanges
	require.Equal(t, PoSTStatusProving, currentState)

	// Send an error response to the call to generate proofs
	mock.proveResult <- &proveRes{err: fmt.Errorf("err")}

	// Should abort and then move to start state
	currentState = <-s.stateChanges
	require.Equal(t, PoSTStatusStart, currentState)
	require.True(t, mock.wasAbortCalled())

	// Trigger a head change
	go triggerHeadChange(t, s, abi.ChainEpoch(1))

	// Should start proving
	currentState = <-s.stateChanges
	require.Equal(t, PoSTStatusProving, currentState)

	// Send a reply to the call to generate proofs
	posts := []miner.SubmitWindowedPoStParams{{Deadline: deadlineIdx}}
	mock.proveResult <- &proveRes{posts: posts}

	// Should move to the proving complete state
	currentState = <-s.stateChanges
	require.Equal(t, PoSTStatusProvingComplete, currentState)
}

func TestStateMachineSubmitErrorRecovery(t *testing.T) {
	periodStart := abi.ChainEpoch(0)
	deadlineIdx := uint64(0)

	s := makeScaffolding(t, periodStart, deadlineIdx)
	sm := s.sm
	mock := s.mock

	// Starting state
	require.Equal(t, PoSTStatusStart, sm.CurrentState())

	// Trigger a head change
	submitConfidenceHeight := abi.ChainEpoch(1 + SubmitConfidence)
	go triggerHeadChange(t, s, submitConfidenceHeight)

	// Should start proving
	currentState := <-s.stateChanges
	require.Equal(t, PoSTStatusProving, currentState)

	// Send a reply to the call to generate proofs
	posts := []miner.SubmitWindowedPoStParams{{Deadline: deadlineIdx}}
	mock.proveResult <- &proveRes{posts: posts}

	// Should move to the proving complete state
	currentState = <-s.stateChanges
	require.Equal(t, PoSTStatusProvingComplete, currentState)

	// Should move to the submitting state
	currentState = <-s.stateChanges
	require.Equal(t, PoSTStatusSubmitting, currentState)

	// Send an error response to the call to submit
	mock.submitResult <- fmt.Errorf("err")

	// Should abort and then move back to proving complete state
	currentState = <-s.stateChanges
	require.Equal(t, PoSTStatusProvingComplete, currentState)
	require.True(t, mock.wasAbortCalled())

	// Should not advance from PoSTStatusProvingComplete until the next state
	// change is triggered
	select {
	case <-s.stateChanges:
	case <-time.After(10 * time.Millisecond):
	}
	require.Equal(t, PoSTStatusProvingComplete, currentState)

	// Trigger a state change
	go triggerHeadChange(t, s, submitConfidenceHeight)

	// Should move to submitting state
	currentState = <-s.stateChanges
	require.Equal(t, PoSTStatusSubmitting, currentState)

	// Send a response to the submit call
	mock.submitResult <- nil

	// Should move to the complete state
	currentState = <-s.stateChanges
	require.Equal(t, PoSTStatusComplete, currentState)

}

// Move through states until we reach the desired state
func advanceThroughStatesUntil(
	t *testing.T,
	s *smScaffolding,
	at PoSTStatus,
	triggerProvingEpoch abi.ChainEpoch,
	triggerSubmitEpoch abi.ChainEpoch,
) {
	ctx := s.ctx
	sm := s.sm
	mock := s.mock

	go func() {
		ts := mock.makeTs(t, triggerProvingEpoch)
		err := sm.HeadChange(ctx, ts, false)
		require.NoError(t, err)
	}()

	<-s.stateChanges
	require.Equal(t, PoSTStatusProving, sm.CurrentState())
	if at == PoSTStatusProving {
		return
	}

	posts := []miner.SubmitWindowedPoStParams{{Deadline: s.deadlineIdx}}
	mock.proveResult <- &proveRes{posts: posts}

	<-s.stateChanges
	require.Equal(t, PoSTStatusProvingComplete, sm.CurrentState())
	if at == PoSTStatusProvingComplete {
		return
	}

	go func() {
		ts := mock.makeTs(t, triggerSubmitEpoch)
		err := sm.HeadChange(ctx, ts, false)
		require.NoError(t, err)
	}()

	<-s.stateChanges
	require.Equal(t, PoSTStatusSubmitting, sm.CurrentState())
	if at == PoSTStatusSubmitting {
		return
	}

	mock.submitResult <- nil

	<-s.stateChanges
	require.Equal(t, PoSTStatusComplete, sm.CurrentState())
	if at == PoSTStatusComplete {
		return
	}

	panic("in unexpected state")
}

// TestStateMachineExpire verifies that when the current deadline expires, we
// move to the correct subsequent state
func TestStateMachineExpire(t *testing.T) {
	deadlineCurrentEpoch := abi.ChainEpoch(1)
	deadlineSubmitEpoch := abi.ChainEpoch(1 + SubmitConfidence)
	epochProvokingExpiry := deadlineCurrentEpoch + miner.WPoStChallengeWindow

	tcases := []struct {
		expireAt        PoSTStatus
		expectNextState PoSTStatus
		expectOnAbort   bool
	}{{
		// If expiry is triggered when we're in the proving state, expect to
		// abort
		expireAt:      PoSTStatusProving,
		expectOnAbort: true,
	}, {
		// If expiry is triggered when we're in the proving complete state,
		// expect to abort then start proving for the next deadline
		expireAt:        PoSTStatusProvingComplete,
		expectOnAbort:   true,
		expectNextState: PoSTStatusProving,
	}, {
		// If expiry is triggered when we're in the submitting state, expect to
		// abort then start proving for the next deadline
		expireAt:        PoSTStatusSubmitting,
		expectOnAbort:   true,
		expectNextState: PoSTStatusProving,
	}, {
		// If expiry is triggered when we're already in the complete state,
		// expect to ignore the expiry and start proving for the next deadline
		expireAt:        PoSTStatusComplete,
		expectOnAbort:   false,
		expectNextState: PoSTStatusProving,
	}}

	for _, tcase := range tcases {
		tcase := tcase
		t.Run("expiry in state"+tcase.expireAt.String(), func(t *testing.T) {
			periodStart := abi.ChainEpoch(0)
			deadlineIdx := uint64(0)
			s := makeScaffolding(t, periodStart, deadlineIdx)

			// Move to the state at which expiry is going to triggered
			advanceThroughStatesUntil(t, s, tcase.expireAt, deadlineCurrentEpoch, deadlineSubmitEpoch)

			// Trigger expiry
			go func() {
				expiryTS := s.mock.makeTs(t, epochProvokingExpiry)
				err := s.sm.HeadChange(s.ctx, expiryTS, false)
				require.NoError(t, err)
			}()

			// Get the next state transition
			currentState := <-s.stateChanges

			// Check if abort was expected
			require.Equal(t, tcase.expectOnAbort, s.mock.wasAbortCalled())
			if tcase.expectOnAbort {
				// After an expiry we should always move back to the start state
				require.Equal(t, PoSTStatusStart, currentState)
				if tcase.expectNextState != "" {
					// From some states we expect to move to a subsequent state
					// after an abort
					currentState = <-s.stateChanges
					require.Equal(t, tcase.expectNextState, currentState)
				}
			} else {
				// If there wasn't an abort, check that the next state is correct
				require.Equal(t, tcase.expectNextState, currentState)
			}
		})
	}
}

// TestStateMachineShutdown verifies that the state machine can be shut down
// correctly from any state
func TestStateMachineShutdown(t *testing.T) {
	deadlineCurrentEpoch := abi.ChainEpoch(1)
	deadlineSubmitEpoch := abi.ChainEpoch(1 + SubmitConfidence)

	tcases := []struct {
		shutdownAt    PoSTStatus
		expectOnAbort bool
	}{{
		shutdownAt:    PoSTStatusProving,
		expectOnAbort: true,
	}, {
		shutdownAt:    PoSTStatusProvingComplete,
		expectOnAbort: true,
	}, {
		shutdownAt:    PoSTStatusSubmitting,
		expectOnAbort: true,
	}, {
		shutdownAt:    PoSTStatusComplete,
		expectOnAbort: false,
	}}

	for _, tcase := range tcases {
		tcase := tcase
		t.Run("shutdown from state"+tcase.shutdownAt.String(), func(t *testing.T) {
			periodStart := abi.ChainEpoch(0)
			deadlineIdx := uint64(0)
			s := makeScaffolding(t, periodStart, deadlineIdx)

			// Move to the state at which expiry is going to triggered
			advanceThroughStatesUntil(t, s, tcase.shutdownAt, deadlineCurrentEpoch, deadlineSubmitEpoch)

			// Trigger shutdown
			go func() {
				s.sm.Shutdown()
			}()

			// Get the next state transition
			currentState := <-s.stateChanges

			// Check if abort was expected
			require.Equal(t, tcase.expectOnAbort, s.mock.wasAbortCalled())
			// After shutdown we should move back to the start state
			require.Equal(t, PoSTStatusStart, currentState)
		})
	}
}

func TestStateMachineRevert(t *testing.T) {
	periodStart := abi.ChainEpoch(0)
	deadlineIdx := uint64(1)
	lastEpochInPreviousDeadline := miner.WPoStChallengeWindow - 1
	firstEpochInDeadline := miner.WPoStChallengeWindow + 1
	submitConfidenceEpoch := miner.WPoStChallengeWindow + 1 + SubmitConfidence

	advanceAndContinue := func(s *smScaffolding) {
		// Should not change state
		select {
		case <-s.stateChanges:
			t.Fatal("Did not expect state change")
		case <-time.After(10 * time.Millisecond):
		}

		// Trigger a head change that advances to the first epoch in the
		// target deadline
		go triggerHeadChange(t, s, firstEpochInDeadline)

		// Should not change state because we've already moved beyond the
		// start state
		select {
		case <-s.stateChanges:
			t.Fatal("Did not expect state change")
		case <-time.After(10 * time.Millisecond):
		}
	}

	atSubmitting := func(s *smScaffolding) {
		// Expect submit to be aborted and to move back to the proving
		// complete state
		currentState := <-s.stateChanges
		require.Equal(t, PoSTStatusProvingComplete, currentState)

		// Trigger a head change that advances to the first epoch in the
		// target deadline
		go triggerHeadChange(t, s, firstEpochInDeadline)

		// Should not change state because we've previously moved beyond the
		// start state
		select {
		case <-s.stateChanges:
			t.Fatal("Did not expect state change")
		case <-time.After(10 * time.Millisecond):
		}

		// Trigger a head change that advances to the submit confidence epoch
		go triggerHeadChange(t, s, submitConfidenceEpoch)

		// Should move to submitting state
		currentState = <-s.stateChanges
		require.Equal(t, PoSTStatusSubmitting, currentState)
	}

	triggerRevert := func(s *smScaffolding, afterRevert func(s *smScaffolding)) {
		// Trigger a head change that reverts to the previous epoch
		go func() {
			ts := s.mock.makeTs(t, lastEpochInPreviousDeadline)
			err := s.sm.HeadChange(s.ctx, ts, true)
			require.NoError(t, err)
		}()

		afterRevert(s)
	}

	triggerRevertAt := func(at PoSTStatus, afterRevert func(s *smScaffolding)) {
		s := makeScaffolding(t, periodStart, deadlineIdx)
		ctx := s.ctx
		sm := s.sm
		mock := s.mock

		// Starting state
		require.Equal(t, PoSTStatusStart, sm.CurrentState())

		// Trigger a head change
		go triggerHeadChange(t, s, firstEpochInDeadline)

		// Should start proving
		currentState := <-s.stateChanges
		require.Equal(t, PoSTStatusProving, currentState)
		if at == currentState {
			triggerRevert(s, afterRevert)
		}

		// Send a response to the call to generate proofs
		posts := []miner.SubmitWindowedPoStParams{{Deadline: deadlineIdx}}
		mock.proveResult <- &proveRes{posts: posts}

		// Should move to proving complete
		currentState = <-s.stateChanges
		require.Equal(t, PoSTStatusProvingComplete, currentState)
		if at == currentState {
			triggerRevert(s, afterRevert)
		}

		// Should not advance from PoSTStatusProvingComplete until the chain has
		// reached sufficient height
		select {
		case <-s.stateChanges:
		case <-time.After(10 * time.Millisecond):
		}
		require.Equal(t, PoSTStatusProvingComplete, currentState)

		// Move to the correct height to submit the proof
		go func() {
			ts := mock.makeTs(t, submitConfidenceEpoch)
			err := sm.HeadChange(ctx, ts, false)
			require.NoError(t, err)
		}()

		// Should move to submitting state
		currentState = <-s.stateChanges
		require.Equal(t, PoSTStatusSubmitting, currentState)
		if at == currentState {
			triggerRevert(s, afterRevert)
		}

		// Send a response to the submit call
		mock.submitResult <- nil

		// Should move to the complete state
		currentState = <-s.stateChanges
		require.Equal(t, PoSTStatusComplete, currentState)
		if at == currentState {
			triggerRevert(s, afterRevert)
		}
	}

	tcases := []struct {
		revertAt    PoSTStatus
		afterRevert func(*smScaffolding)
	}{{
		revertAt:    PoSTStatusProving,
		afterRevert: advanceAndContinue,
	}, {
		revertAt:    PoSTStatusProvingComplete,
		afterRevert: advanceAndContinue,
	}, {
		revertAt:    PoSTStatusSubmitting,
		afterRevert: atSubmitting,
	}, {
		revertAt:    PoSTStatusComplete,
		afterRevert: advanceAndContinue,
	}}

	for _, tcase := range tcases {
		tcase := tcase
		t.Run("revert from state"+tcase.revertAt.String(), func(t *testing.T) {
			triggerRevertAt(tcase.revertAt, tcase.afterRevert)
		})
	}
}

type smScaffolding struct {
	ctx          context.Context
	mock         *mockAPI
	sm           *stateMachine
	periodStart  abi.ChainEpoch
	deadlineIdx  uint64
	stateChanges chan PoSTStatus
}

func makeScaffolding(t *testing.T, periodStart abi.ChainEpoch, deadlineIdx uint64) *smScaffolding {
	ctx := context.Background()
	actor := tutils.NewActorAddr(t, "actor")
	mock := newMockAPI()
	stateChanges := make(chan PoSTStatus)
	sm := newStateMachine(mock, actor, stateChanges)
	mock.setStateMachine(sm)
	mock.setDeadlineParams(periodStart, deadlineIdx)
	return &smScaffolding{
		ctx:          ctx,
		mock:         mock,
		stateChanges: stateChanges,
		sm:           sm,
		periodStart:  periodStart,
		deadlineIdx:  deadlineIdx,
	}
}

func triggerHeadChange(t *testing.T, s *smScaffolding, height abi.ChainEpoch) {
	ts := s.mock.makeTs(t, height)
	err := s.sm.HeadChange(s.ctx, ts, false)
	require.NoError(t, err)
}
