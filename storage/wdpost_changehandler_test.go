package storage

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	tutils "github.com/filecoin-project/specs-actors/support/testing"

	"github.com/filecoin-project/go-state-types/crypto"

	"github.com/ipfs/go-cid"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/dline"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/types"
)

var dummyCid cid.Cid

func init() {
	dummyCid, _ = cid.Parse("bafkqaaa")
}

type proveRes struct {
	posts []miner.SubmitWindowedPoStParams
	err   error
}

type postStatus string

const (
	postStatusStart    postStatus = "postStatusStart"
	postStatusProving  postStatus = "postStatusProving"
	postStatusComplete postStatus = "postStatusComplete"
)

type mockAPI struct {
	ch            *changeHandler
	deadline      *dline.Info
	proveResult   chan *proveRes
	submitResult  chan error
	onStateChange chan struct{}

	tsLock sync.RWMutex
	ts     map[types.TipSetKey]*types.TipSet

	abortCalledLock sync.RWMutex
	abortCalled     bool

	statesLk   sync.RWMutex
	postStates map[abi.ChainEpoch]postStatus
}

func newMockAPI() *mockAPI {
	return &mockAPI{
		proveResult:   make(chan *proveRes),
		onStateChange: make(chan struct{}),
		submitResult:  make(chan error),
		postStates:    make(map[abi.ChainEpoch]postStatus),
		ts:            make(map[types.TipSetKey]*types.TipSet),
	}
}

func (m *mockAPI) makeTs(t *testing.T, h abi.ChainEpoch) *types.TipSet {
	m.tsLock.Lock()
	defer m.tsLock.Unlock()

	ts := makeTs(t, h)
	m.ts[ts.Key()] = ts
	return ts
}

func (m *mockAPI) setDeadline(di *dline.Info) {
	m.tsLock.Lock()
	defer m.tsLock.Unlock()

	m.deadline = di
}

func (m *mockAPI) getDeadline(currentEpoch abi.ChainEpoch) *dline.Info {
	close := miner.WPoStChallengeWindow - 1
	dlIdx := uint64(0)
	for close < currentEpoch {
		close += miner.WPoStChallengeWindow
		dlIdx++
	}
	return NewDeadlineInfo(0, dlIdx, currentEpoch)
}

func (m *mockAPI) StateMinerProvingDeadline(ctx context.Context, address address.Address, key types.TipSetKey) (*dline.Info, error) {
	m.tsLock.RLock()
	defer m.tsLock.RUnlock()

	ts, ok := m.ts[key]
	if !ok {
		panic(fmt.Sprintf("unexpected tipset key %s", key))
	}

	if m.deadline != nil {
		m.deadline.CurrentEpoch = ts.Height()
		return m.deadline, nil
	}

	return m.getDeadline(ts.Height()), nil
}

func (m *mockAPI) startGeneratePoST(
	ctx context.Context,
	ts *types.TipSet,
	deadline *dline.Info,
	completeGeneratePoST CompleteGeneratePoSTCb,
) context.CancelFunc {
	ctx, cancel := context.WithCancel(ctx)

	m.statesLk.Lock()
	defer m.statesLk.Unlock()
	m.postStates[deadline.Open] = postStatusProving

	go func() {
		defer cancel()

		select {
		case psRes := <-m.proveResult:
			m.statesLk.Lock()
			{
				if psRes.err == nil {
					m.postStates[deadline.Open] = postStatusComplete
				} else {
					m.postStates[deadline.Open] = postStatusStart
				}
			}
			m.statesLk.Unlock()
			completeGeneratePoST(psRes.posts, psRes.err)
		case <-ctx.Done():
			completeGeneratePoST(nil, ctx.Err())
		}
	}()

	return cancel
}

func (m *mockAPI) getPostStatus(di *dline.Info) postStatus {
	m.statesLk.RLock()
	defer m.statesLk.RUnlock()

	status, ok := m.postStates[di.Open]
	if ok {
		return status
	}
	return postStatusStart
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

func (m *mockAPI) onAbort(ts *types.TipSet, deadline *dline.Info) {
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

func (m *mockAPI) setChangeHandler(ch *changeHandler) {
	m.ch = ch
}

// TestChangeHandlerBasic verifies we can generate a proof and submit it
func TestChangeHandlerBasic(t *testing.T) {
	s := makeScaffolding(t)
	mock := s.mock

	defer s.ch.shutdown()
	s.ch.start()

	// Trigger a head change
	currentEpoch := abi.ChainEpoch(1)
	go triggerHeadAdvance(t, s, currentEpoch)

	// Should start proving
	<-s.ch.proveHdlr.processedHeadChanges
	di := mock.getDeadline(currentEpoch)
	require.Equal(t, postStatusProving, s.mock.getPostStatus(di))

	// Submitter doesn't have anything to do yet
	<-s.ch.submitHdlr.processedHeadChanges
	require.Equal(t, SubmitStateStart, s.submitState(di))

	// Send a response to the call to generate proofs
	posts := []miner.SubmitWindowedPoStParams{{Deadline: di.Index}}
	mock.proveResult <- &proveRes{posts: posts}

	// Should move to proving complete
	<-s.ch.proveHdlr.processedPostResults
	require.Equal(t, postStatusComplete, s.mock.getPostStatus(di))

	// Move to the correct height to submit the proof
	currentEpoch = 1 + SubmitConfidence
	go triggerHeadAdvance(t, s, currentEpoch)

	// Should move to submitting state
	<-s.ch.submitHdlr.processedHeadChanges
	di = mock.getDeadline(currentEpoch)
	require.Equal(t, SubmitStateSubmitting, s.submitState(di))

	// Send a response to the submit call
	mock.submitResult <- nil

	// Should move to the complete state
	<-s.ch.submitHdlr.processedSubmitResults
	require.Equal(t, SubmitStateComplete, s.submitState(di))
}

// TestChangeHandlerFromProvingToSubmittingNoHeadChange tests that when the
// chain is already advanced past the confidence interval, we should move from
// proving to submitting without a head change in between.
func TestChangeHandlerFromProvingToSubmittingNoHeadChange(t *testing.T) {
	s := makeScaffolding(t)
	mock := s.mock

	// Monitor submit handler's processing of incoming postInfo
	s.ch.submitHdlr.processedPostReady = make(chan *postInfo)

	defer s.ch.shutdown()
	s.ch.start()

	// Trigger a head change
	currentEpoch := abi.ChainEpoch(1)
	go triggerHeadAdvance(t, s, currentEpoch)

	// Should start proving
	<-s.ch.proveHdlr.processedHeadChanges
	di := mock.getDeadline(currentEpoch)
	require.Equal(t, postStatusProving, s.mock.getPostStatus(di))

	// Submitter doesn't have anything to do yet
	<-s.ch.submitHdlr.processedHeadChanges
	require.Equal(t, SubmitStateStart, s.submitState(di))

	// Trigger a head change that advances the chain beyond the submit
	// confidence
	currentEpoch = 1 + SubmitConfidence
	go triggerHeadAdvance(t, s, currentEpoch)

	// Should be no change to state yet
	<-s.ch.proveHdlr.processedHeadChanges
	require.Equal(t, postStatusProving, s.mock.getPostStatus(di))
	<-s.ch.submitHdlr.processedHeadChanges
	require.Equal(t, SubmitStateStart, s.submitState(di))

	// Send a response to the call to generate proofs
	posts := []miner.SubmitWindowedPoStParams{{Deadline: di.Index}}
	mock.proveResult <- &proveRes{posts: posts}

	// Should move to proving complete
	<-s.ch.proveHdlr.processedPostResults
	di = mock.getDeadline(currentEpoch)
	require.Equal(t, postStatusComplete, s.mock.getPostStatus(di))

	// Should move directly to submitting state with no further head changes
	<-s.ch.submitHdlr.processedPostReady
	require.Equal(t, SubmitStateSubmitting, s.submitState(di))
}

// TestChangeHandlerFromProvingEmptyProofsToComplete tests that when there are no
// proofs generated we should not submit anything to chain but submit state
// should move to completed
func TestChangeHandlerFromProvingEmptyProofsToComplete(t *testing.T) {
	s := makeScaffolding(t)
	mock := s.mock

	// Monitor submit handler's processing of incoming postInfo
	s.ch.submitHdlr.processedPostReady = make(chan *postInfo)

	defer s.ch.shutdown()
	s.ch.start()

	// Trigger a head change
	currentEpoch := abi.ChainEpoch(1)
	go triggerHeadAdvance(t, s, currentEpoch)

	// Should start proving
	<-s.ch.proveHdlr.processedHeadChanges
	di := mock.getDeadline(currentEpoch)
	require.Equal(t, postStatusProving, s.mock.getPostStatus(di))

	// Submitter doesn't have anything to do yet
	<-s.ch.submitHdlr.processedHeadChanges
	require.Equal(t, SubmitStateStart, s.submitState(di))

	// Trigger a head change that advances the chain beyond the submit
	// confidence
	currentEpoch = 1 + SubmitConfidence
	go triggerHeadAdvance(t, s, currentEpoch)

	// Should be no change to state yet
	<-s.ch.proveHdlr.processedHeadChanges
	require.Equal(t, postStatusProving, s.mock.getPostStatus(di))
	<-s.ch.submitHdlr.processedHeadChanges
	require.Equal(t, SubmitStateStart, s.submitState(di))

	// Send a response to the call to generate proofs with an empty proofs array
	posts := []miner.SubmitWindowedPoStParams{}
	mock.proveResult <- &proveRes{posts: posts}

	// Should move to proving complete
	<-s.ch.proveHdlr.processedPostResults
	di = mock.getDeadline(currentEpoch)
	require.Equal(t, postStatusComplete, s.mock.getPostStatus(di))

	// Should move directly to submitting complete state
	<-s.ch.submitHdlr.processedPostReady
	require.Equal(t, SubmitStateComplete, s.submitState(di))
}

// TestChangeHandlerDontStartUntilProvingPeriod tests that the handler
// ignores updates until the proving period has been reached.
func TestChangeHandlerDontStartUntilProvingPeriod(t *testing.T) {
	s := makeScaffolding(t)
	mock := s.mock

	periodStart := miner.WPoStProvingPeriod
	dlIdx := uint64(1)
	currentEpoch := abi.ChainEpoch(10)
	di := NewDeadlineInfo(periodStart, dlIdx, currentEpoch)
	mock.setDeadline(di)

	defer s.ch.shutdown()
	s.ch.start()

	// Trigger a head change
	go triggerHeadAdvance(t, s, currentEpoch)

	// Nothing should happen because the proving period has not started
	select {
	case <-s.ch.proveHdlr.processedHeadChanges:
		require.Fail(t, "unexpected prove change")
	case <-s.ch.submitHdlr.processedHeadChanges:
		require.Fail(t, "unexpected submit change")
	case <-time.After(10 * time.Millisecond):
	}

	// Advance the head to the next proving period's first epoch
	currentEpoch = periodStart + miner.WPoStChallengeWindow
	di = NewDeadlineInfo(periodStart, dlIdx, currentEpoch)
	mock.setDeadline(di)
	go triggerHeadAdvance(t, s, currentEpoch)

	// Should start proving
	<-s.ch.proveHdlr.processedHeadChanges
	require.Equal(t, postStatusProving, s.mock.getPostStatus(di))
}

// TestChangeHandlerStartProvingNextDeadline verifies that the proof handler
// starts proving the next deadline after the current one
func TestChangeHandlerStartProvingNextDeadline(t *testing.T) {
	s := makeScaffolding(t)
	mock := s.mock

	defer s.ch.shutdown()
	s.ch.start()

	// Trigger a head change
	currentEpoch := abi.ChainEpoch(1)
	go triggerHeadAdvance(t, s, currentEpoch+ChallengeConfidence)

	// Should start proving
	<-s.ch.proveHdlr.processedHeadChanges
	di := mock.getDeadline(currentEpoch)
	require.Equal(t, postStatusProving, s.mock.getPostStatus(di))

	// Trigger a head change that advances the chain beyond the submit
	// confidence
	currentEpoch = 1 + SubmitConfidence
	go triggerHeadAdvance(t, s, currentEpoch+ChallengeConfidence)

	// Should be no change to state yet
	<-s.ch.proveHdlr.processedHeadChanges
	require.Equal(t, postStatusProving, s.mock.getPostStatus(di))

	// Send a response to the call to generate proofs
	posts := []miner.SubmitWindowedPoStParams{{Deadline: di.Index}}
	mock.proveResult <- &proveRes{posts: posts}

	// Should move to proving complete
	<-s.ch.proveHdlr.processedPostResults
	di = mock.getDeadline(currentEpoch)
	require.Equal(t, postStatusComplete, s.mock.getPostStatus(di))

	// Trigger head change that advances the chain to the Challenge epoch for
	// the next deadline
	go func() {
		di = nextDeadline(di)
		currentEpoch = di.Challenge + ChallengeConfidence
		triggerHeadAdvance(t, s, currentEpoch)
	}()

	// Should start generating next window's proof
	<-s.ch.proveHdlr.processedHeadChanges
	require.Equal(t, postStatusProving, s.mock.getPostStatus(di))
}

// TestChangeHandlerProvingRounds verifies we can generate several rounds of
// proofs as the chain head advances
func TestChangeHandlerProvingRounds(t *testing.T) {
	s := makeScaffolding(t)
	mock := s.mock

	defer s.ch.shutdown()
	s.ch.start()

	completeProofIndex := abi.ChainEpoch(10)
	for currentEpoch := abi.ChainEpoch(1); currentEpoch < miner.WPoStChallengeWindow*5; currentEpoch++ {
		// Trigger a head change
		di := mock.getDeadline(currentEpoch)
		go triggerHeadAdvance(t, s, currentEpoch+ChallengeConfidence)

		// Wait for prover to process head change
		<-s.ch.proveHdlr.processedHeadChanges

		completeProofEpoch := di.Open + completeProofIndex
		next := nextDeadline(di)
		//fmt.Println("epoch", currentEpoch, s.mock.getPostStatus(di), "next", s.mock.getPostStatus(next))
		if currentEpoch >= next.Challenge {
			require.Equal(t, postStatusComplete, s.mock.getPostStatus(di))
			// At the next deadline's challenge epoch, should start proving
			// for that epoch
			require.Equal(t, postStatusProving, s.mock.getPostStatus(next))
		} else if currentEpoch > completeProofEpoch {
			// After proving for the round is complete, should be in complete state
			require.Equal(t, postStatusComplete, s.mock.getPostStatus(di))
			require.Equal(t, postStatusStart, s.mock.getPostStatus(next))
		} else {
			// Until proving completes, should be in the proving state
			require.Equal(t, postStatusProving, s.mock.getPostStatus(di))
			require.Equal(t, postStatusStart, s.mock.getPostStatus(next))
		}

		// Wait for submitter to process head change
		<-s.ch.submitHdlr.processedHeadChanges

		completeSubmitEpoch := completeProofEpoch + 1
		//fmt.Println("epoch", currentEpoch, s.submitState(di))
		if currentEpoch > completeSubmitEpoch {
			require.Equal(t, SubmitStateComplete, s.submitState(di))
		} else if currentEpoch > completeProofEpoch {
			require.Equal(t, SubmitStateSubmitting, s.submitState(di))
		} else {
			require.Equal(t, SubmitStateStart, s.submitState(di))
		}

		if currentEpoch == completeProofEpoch {
			// Send a response to the call to generate proofs
			posts := []miner.SubmitWindowedPoStParams{{Deadline: di.Index}}
			mock.proveResult <- &proveRes{posts: posts}

			// Should move to proving complete
			<-s.ch.proveHdlr.processedPostResults
			require.Equal(t, postStatusComplete, s.mock.getPostStatus(di))
		}

		if currentEpoch == completeSubmitEpoch {
			// Send a response to the submit call
			mock.submitResult <- nil

			// Should move to the complete state
			<-s.ch.submitHdlr.processedSubmitResults
			require.Equal(t, SubmitStateComplete, s.submitState(di))
		}
	}
}

// TestChangeHandlerProvingErrorRecovery verifies that the proof handler
// recovers correctly from an error
func TestChangeHandlerProvingErrorRecovery(t *testing.T) {
	s := makeScaffolding(t)
	mock := s.mock

	defer s.ch.shutdown()
	s.ch.start()

	// Trigger a head change
	currentEpoch := abi.ChainEpoch(1)
	go triggerHeadAdvance(t, s, currentEpoch)

	// Should start proving
	<-s.ch.proveHdlr.processedHeadChanges
	di := mock.getDeadline(currentEpoch)
	require.Equal(t, postStatusProving, s.mock.getPostStatus(di))

	// Send an error response to the call to generate proofs
	mock.proveResult <- &proveRes{err: fmt.Errorf("err")}

	// Should abort and then move to start state
	<-s.ch.proveHdlr.processedPostResults
	require.Equal(t, postStatusStart, s.mock.getPostStatus(di))

	// Trigger a head change
	go triggerHeadAdvance(t, s, currentEpoch)

	// Should start proving
	<-s.ch.proveHdlr.processedHeadChanges
	require.Equal(t, postStatusProving, s.mock.getPostStatus(di))

	// Send a success response to the call to generate proofs
	posts := []miner.SubmitWindowedPoStParams{{Deadline: di.Index}}
	mock.proveResult <- &proveRes{posts: posts}

	// Should move to proving complete
	<-s.ch.proveHdlr.processedPostResults
	require.Equal(t, postStatusComplete, s.mock.getPostStatus(di))
}

// TestChangeHandlerSubmitErrorRecovery verifies that the submit handler
// recovers correctly from an error
func TestChangeHandlerSubmitErrorRecovery(t *testing.T) {
	s := makeScaffolding(t)
	mock := s.mock

	defer s.ch.shutdown()
	s.ch.start()

	// Trigger a head change
	currentEpoch := abi.ChainEpoch(1)
	go triggerHeadAdvance(t, s, currentEpoch)

	// Should start proving
	<-s.ch.proveHdlr.processedHeadChanges
	di := mock.getDeadline(currentEpoch)
	require.Equal(t, postStatusProving, s.mock.getPostStatus(di))

	// Submitter doesn't have anything to do yet
	<-s.ch.submitHdlr.processedHeadChanges
	require.Equal(t, SubmitStateStart, s.submitState(di))

	// Send a response to the call to generate proofs
	posts := []miner.SubmitWindowedPoStParams{{Deadline: di.Index}}
	mock.proveResult <- &proveRes{posts: posts}

	// Should move to proving complete
	<-s.ch.proveHdlr.processedPostResults
	require.Equal(t, postStatusComplete, s.mock.getPostStatus(di))

	// Move to the correct height to submit the proof
	currentEpoch = 1 + SubmitConfidence
	go triggerHeadAdvance(t, s, currentEpoch)

	// Read from prover incoming channel (so as not to block)
	<-s.ch.proveHdlr.processedHeadChanges

	// Should move to submitting state
	<-s.ch.submitHdlr.processedHeadChanges
	di = mock.getDeadline(currentEpoch)
	require.Equal(t, SubmitStateSubmitting, s.submitState(di))

	// Send an error response to the call to submit
	mock.submitResult <- fmt.Errorf("err")

	// Should abort and then move back to the start state
	<-s.ch.submitHdlr.processedSubmitResults
	require.Equal(t, SubmitStateStart, s.submitState(di))
	require.True(t, mock.wasAbortCalled())

	// Trigger another head change
	go triggerHeadAdvance(t, s, currentEpoch)

	// Read from prover incoming channel (so as not to block)
	<-s.ch.proveHdlr.processedHeadChanges

	// Should move to submitting state
	<-s.ch.submitHdlr.processedHeadChanges
	di = mock.getDeadline(currentEpoch)
	require.Equal(t, SubmitStateSubmitting, s.submitState(di))

	// Send a response to the submit call
	mock.submitResult <- nil

	// Should move to the complete state
	<-s.ch.submitHdlr.processedSubmitResults
	require.Equal(t, SubmitStateComplete, s.submitState(di))
}

// TestChangeHandlerProveExpiry verifies that the prove handler
// behaves correctly on expiry
func TestChangeHandlerProveExpiry(t *testing.T) {
	s := makeScaffolding(t)
	mock := s.mock

	defer s.ch.shutdown()
	s.ch.start()

	// Trigger a head change
	currentEpoch := abi.ChainEpoch(1)
	go triggerHeadAdvance(t, s, currentEpoch)

	// Should start proving
	<-s.ch.proveHdlr.processedHeadChanges
	di := mock.getDeadline(currentEpoch)
	require.Equal(t, postStatusProving, s.mock.getPostStatus(di))

	// Move to a height that expires the current proof
	currentEpoch = miner.WPoStChallengeWindow
	di = mock.getDeadline(currentEpoch)
	go triggerHeadAdvance(t, s, currentEpoch)

	// Should trigger an abort and start proving for the new deadline
	<-s.ch.proveHdlr.processedHeadChanges
	require.Equal(t, postStatusProving, s.mock.getPostStatus(di))
	<-s.ch.proveHdlr.processedPostResults
	require.True(t, mock.wasAbortCalled())

	// Send a response to the call to generate proofs
	posts := []miner.SubmitWindowedPoStParams{{Deadline: di.Index}}
	mock.proveResult <- &proveRes{posts: posts}

	// Should move to proving complete
	<-s.ch.proveHdlr.processedPostResults
	require.Equal(t, postStatusComplete, s.mock.getPostStatus(di))
}

// TestChangeHandlerSubmitExpiry verifies that the submit handler
// behaves correctly on expiry
func TestChangeHandlerSubmitExpiry(t *testing.T) {
	s := makeScaffolding(t)
	mock := s.mock

	// Ignore prove handler head change processing for this test
	s.ch.proveHdlr.processedHeadChanges = nil

	defer s.ch.shutdown()
	s.ch.start()

	// Trigger a head change
	currentEpoch := abi.ChainEpoch(1)
	go triggerHeadAdvance(t, s, currentEpoch)

	// Submitter doesn't have anything to do yet
	<-s.ch.submitHdlr.processedHeadChanges
	di := mock.getDeadline(currentEpoch)
	require.Equal(t, SubmitStateStart, s.submitState(di))

	// Send a response to the call to generate proofs
	posts := []miner.SubmitWindowedPoStParams{{Deadline: di.Index}}
	mock.proveResult <- &proveRes{posts: posts}

	// Should move to proving complete
	<-s.ch.proveHdlr.processedPostResults
	require.Equal(t, postStatusComplete, s.mock.getPostStatus(di))

	// Move to the correct height to submit the proof
	currentEpoch = 1 + SubmitConfidence
	go triggerHeadAdvance(t, s, currentEpoch)

	// Should move to submitting state
	<-s.ch.submitHdlr.processedHeadChanges
	di = mock.getDeadline(currentEpoch)
	require.Equal(t, SubmitStateSubmitting, s.submitState(di))

	// Move to a height that expires the submit
	currentEpoch = miner.WPoStChallengeWindow
	di = mock.getDeadline(currentEpoch)
	go triggerHeadAdvance(t, s, currentEpoch)

	// Should trigger an abort and move back to start state
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()

		<-s.ch.submitHdlr.processedSubmitResults
		require.True(t, mock.wasAbortCalled())
	}()

	go func() {
		defer wg.Done()

		<-s.ch.submitHdlr.processedHeadChanges
		require.Equal(t, SubmitStateStart, s.submitState(di))
	}()

	wg.Wait()
}

// TestChangeHandlerProveRevert verifies that the prove handler
// behaves correctly on revert
func TestChangeHandlerProveRevert(t *testing.T) {
	s := makeScaffolding(t)
	mock := s.mock

	defer s.ch.shutdown()
	s.ch.start()

	// Trigger a head change
	currentEpoch := miner.WPoStChallengeWindow
	go triggerHeadAdvance(t, s, currentEpoch)

	// Should start proving
	<-s.ch.proveHdlr.processedHeadChanges
	di := mock.getDeadline(currentEpoch)
	require.Equal(t, postStatusProving, s.mock.getPostStatus(di))

	// Trigger a revert to the previous epoch
	revertEpoch := di.Open - 5
	go triggerHeadChange(t, s, revertEpoch, currentEpoch)

	// Should be no change
	<-s.ch.proveHdlr.processedHeadChanges
	require.Equal(t, postStatusProving, s.mock.getPostStatus(di))

	// Send a response to the call to generate proofs
	posts := []miner.SubmitWindowedPoStParams{{Deadline: di.Index}}
	mock.proveResult <- &proveRes{posts: posts}

	// Should move to proving complete
	<-s.ch.proveHdlr.processedPostResults
	require.Equal(t, postStatusComplete, s.mock.getPostStatus(di))
	require.False(t, mock.wasAbortCalled())
}

// TestChangeHandlerSubmittingRevert verifies that the submit handler
// behaves correctly when there's a revert from the submitting state
func TestChangeHandlerSubmittingRevert(t *testing.T) {
	s := makeScaffolding(t)
	mock := s.mock

	// Ignore prove handler head change processing for this test
	s.ch.proveHdlr.processedHeadChanges = nil

	defer s.ch.shutdown()
	s.ch.start()

	// Trigger a head change
	currentEpoch := miner.WPoStChallengeWindow
	go triggerHeadAdvance(t, s, currentEpoch)

	// Submitter doesn't have anything to do yet
	<-s.ch.submitHdlr.processedHeadChanges
	di := mock.getDeadline(currentEpoch)
	require.Equal(t, SubmitStateStart, s.submitState(di))

	// Send a response to the call to generate proofs
	posts := []miner.SubmitWindowedPoStParams{{Deadline: di.Index}}
	mock.proveResult <- &proveRes{posts: posts}

	// Should move to proving complete
	<-s.ch.proveHdlr.processedPostResults
	require.Equal(t, postStatusComplete, s.mock.getPostStatus(di))

	// Move to the correct height to submit the proof
	currentEpoch = currentEpoch + 1 + SubmitConfidence
	go triggerHeadAdvance(t, s, currentEpoch)

	// Should move to submitting state
	<-s.ch.submitHdlr.processedHeadChanges
	di = mock.getDeadline(currentEpoch)
	require.Equal(t, SubmitStateSubmitting, s.submitState(di))

	// Trigger a revert to the previous epoch
	revertEpoch := di.Open - 5
	go triggerHeadChange(t, s, revertEpoch, currentEpoch)

	var wg sync.WaitGroup
	wg.Add(2)

	// Should trigger an abort
	go func() {
		defer wg.Done()

		<-s.ch.submitHdlr.processedSubmitResults
		require.True(t, mock.wasAbortCalled())
	}()

	// Should resubmit current epoch
	go func() {
		defer wg.Done()

		<-s.ch.submitHdlr.processedHeadChanges
		require.Equal(t, SubmitStateSubmitting, s.submitState(di))
	}()

	wg.Wait()

	// Send a response to the resubmit call
	mock.submitResult <- nil

	// Should move to the complete state
	<-s.ch.submitHdlr.processedSubmitResults
	require.Equal(t, SubmitStateComplete, s.submitState(di))
}

// TestChangeHandlerSubmitCompleteRevert verifies that the submit handler
// behaves correctly when there's a revert from the submit complete state
func TestChangeHandlerSubmitCompleteRevert(t *testing.T) {
	s := makeScaffolding(t)
	mock := s.mock

	// Ignore prove handler head change processing for this test
	s.ch.proveHdlr.processedHeadChanges = nil

	defer s.ch.shutdown()
	s.ch.start()

	// Trigger a head change
	currentEpoch := miner.WPoStChallengeWindow
	go triggerHeadAdvance(t, s, currentEpoch)

	// Submitter doesn't have anything to do yet
	<-s.ch.submitHdlr.processedHeadChanges
	di := mock.getDeadline(currentEpoch)
	require.Equal(t, SubmitStateStart, s.submitState(di))

	// Send a response to the call to generate proofs
	posts := []miner.SubmitWindowedPoStParams{{Deadline: di.Index}}
	mock.proveResult <- &proveRes{posts: posts}

	// Should move to proving complete
	<-s.ch.proveHdlr.processedPostResults
	require.Equal(t, postStatusComplete, s.mock.getPostStatus(di))

	// Move to the correct height to submit the proof
	currentEpoch = currentEpoch + 1 + SubmitConfidence
	go triggerHeadAdvance(t, s, currentEpoch)

	// Should move to submitting state
	<-s.ch.submitHdlr.processedHeadChanges
	di = mock.getDeadline(currentEpoch)
	require.Equal(t, SubmitStateSubmitting, s.submitState(di))

	// Send a response to the resubmit call
	mock.submitResult <- nil

	// Should move to the complete state
	<-s.ch.submitHdlr.processedSubmitResults
	require.Equal(t, SubmitStateComplete, s.submitState(di))

	// Trigger a revert to the previous epoch
	revertEpoch := di.Open - 5
	go triggerHeadChange(t, s, revertEpoch, currentEpoch)

	// Should resubmit current epoch
	<-s.ch.submitHdlr.processedHeadChanges
	require.Equal(t, SubmitStateSubmitting, s.submitState(di))

	// Send a response to the resubmit call
	mock.submitResult <- nil

	// Should move to the complete state
	<-s.ch.submitHdlr.processedSubmitResults
	require.Equal(t, SubmitStateComplete, s.submitState(di))
}

// TestChangeHandlerSubmitRevertTwoEpochs verifies that the submit handler
// behaves correctly when the revert is two epochs deep
func TestChangeHandlerSubmitRevertTwoEpochs(t *testing.T) {
	s := makeScaffolding(t)
	mock := s.mock

	// Ignore prove handler head change processing for this test
	s.ch.proveHdlr.processedHeadChanges = nil

	defer s.ch.shutdown()
	s.ch.start()

	// Trigger a head change
	currentEpoch := miner.WPoStChallengeWindow
	go triggerHeadAdvance(t, s, currentEpoch)

	// Submitter doesn't have anything to do yet
	<-s.ch.submitHdlr.processedHeadChanges
	diE1 := mock.getDeadline(currentEpoch)
	require.Equal(t, SubmitStateStart, s.submitState(diE1))

	// Send a response to the call to generate proofs
	posts := []miner.SubmitWindowedPoStParams{{Deadline: diE1.Index}}
	mock.proveResult <- &proveRes{posts: posts}

	// Should move to proving complete
	<-s.ch.proveHdlr.processedPostResults
	require.Equal(t, postStatusComplete, s.mock.getPostStatus(diE1))

	// Move to the challenge epoch for the next deadline
	diE2 := nextDeadline(diE1)
	currentEpoch = diE2.Challenge + ChallengeConfidence
	go triggerHeadAdvance(t, s, currentEpoch)

	// Should move to submitting state for epoch 1
	<-s.ch.submitHdlr.processedHeadChanges
	diE1 = mock.getDeadline(currentEpoch)
	require.Equal(t, SubmitStateSubmitting, s.submitState(diE1))

	// Send a response to the submit call for epoch 1
	mock.submitResult <- nil

	// Should move to the complete state for epoch 1
	<-s.ch.submitHdlr.processedSubmitResults
	require.Equal(t, SubmitStateComplete, s.submitState(diE1))

	// Should start proving epoch 2
	// Send a response to the call to generate proofs
	postsE2 := []miner.SubmitWindowedPoStParams{{Deadline: diE2.Index}}
	mock.proveResult <- &proveRes{posts: postsE2}

	// Should move to proving complete for epoch 2
	<-s.ch.proveHdlr.processedPostResults
	require.Equal(t, postStatusComplete, s.mock.getPostStatus(diE2))

	// Move to the correct height to submit the proof for epoch 2
	currentEpoch = diE2.Open + 1 + SubmitConfidence
	go triggerHeadAdvance(t, s, currentEpoch)

	// Should move to submitting state for epoch 2
	<-s.ch.submitHdlr.processedHeadChanges
	diE2 = mock.getDeadline(currentEpoch)
	require.Equal(t, SubmitStateSubmitting, s.submitState(diE2))

	// Trigger a revert through two epochs (from epoch 2 to epoch 0)
	revertEpoch := diE1.Open - 5
	go triggerHeadChange(t, s, revertEpoch, currentEpoch)

	var wg sync.WaitGroup
	wg.Add(2)

	// Should trigger an abort
	go func() {
		defer wg.Done()

		<-s.ch.submitHdlr.processedSubmitResults
		require.True(t, mock.wasAbortCalled())
	}()

	go func() {
		defer wg.Done()

		<-s.ch.submitHdlr.processedHeadChanges

		// Should reset epoch 1 (that is expired) to start state
		require.Equal(t, SubmitStateStart, s.submitState(diE1))
		// Should resubmit epoch 2
		require.Equal(t, SubmitStateSubmitting, s.submitState(diE2))
	}()

	wg.Wait()

	// Send a response to the resubmit call for epoch 2
	mock.submitResult <- nil

	// Should move to the complete state for epoch 2
	<-s.ch.submitHdlr.processedSubmitResults
	require.Equal(t, SubmitStateComplete, s.submitState(diE2))
}

// TestChangeHandlerSubmitRevertAdvanceLess verifies that the submit handler
// behaves correctly when the revert is two epochs deep and the advance is
// to a lower height than before
func TestChangeHandlerSubmitRevertAdvanceLess(t *testing.T) {
	s := makeScaffolding(t)
	mock := s.mock

	// Ignore prove handler head change processing for this test
	s.ch.proveHdlr.processedHeadChanges = nil

	defer s.ch.shutdown()
	s.ch.start()

	// Trigger a head change
	currentEpoch := miner.WPoStChallengeWindow
	go triggerHeadAdvance(t, s, currentEpoch)

	// Submitter doesn't have anything to do yet
	<-s.ch.submitHdlr.processedHeadChanges
	diE1 := mock.getDeadline(currentEpoch)
	require.Equal(t, SubmitStateStart, s.submitState(diE1))

	// Send a response to the call to generate proofs
	posts := []miner.SubmitWindowedPoStParams{{Deadline: diE1.Index}}
	mock.proveResult <- &proveRes{posts: posts}

	// Should move to proving complete
	<-s.ch.proveHdlr.processedPostResults
	require.Equal(t, postStatusComplete, s.mock.getPostStatus(diE1))

	// Move to the challenge epoch for the next deadline
	diE2 := nextDeadline(diE1)
	currentEpoch = diE2.Challenge + ChallengeConfidence
	go triggerHeadAdvance(t, s, currentEpoch)

	// Should move to submitting state for epoch 1
	<-s.ch.submitHdlr.processedHeadChanges
	diE1 = mock.getDeadline(currentEpoch)
	require.Equal(t, SubmitStateSubmitting, s.submitState(diE1))

	// Send a response to the submit call for epoch 1
	mock.submitResult <- nil

	// Should move to the complete state for epoch 1
	<-s.ch.submitHdlr.processedSubmitResults
	require.Equal(t, SubmitStateComplete, s.submitState(diE1))

	// Should start proving epoch 2
	// Send a response to the call to generate proofs
	postsE2 := []miner.SubmitWindowedPoStParams{{Deadline: diE2.Index}}
	mock.proveResult <- &proveRes{posts: postsE2}

	// Should move to proving complete for epoch 2
	<-s.ch.proveHdlr.processedPostResults
	require.Equal(t, postStatusComplete, s.mock.getPostStatus(diE2))

	// Move to the correct height to submit the proof for epoch 2
	currentEpoch = diE2.Open + 1 + SubmitConfidence
	go triggerHeadAdvance(t, s, currentEpoch)

	// Should move to submitting state for epoch 2
	<-s.ch.submitHdlr.processedHeadChanges
	diE2 = mock.getDeadline(currentEpoch)
	require.Equal(t, SubmitStateSubmitting, s.submitState(diE2))

	// Trigger a revert through two epochs (from epoch 2 to epoch 0)
	// then advance to the previous epoch (to epoch 1)
	revertEpoch := diE1.Open - 5
	currentEpoch = diE2.Open - 1
	go triggerHeadChange(t, s, revertEpoch, currentEpoch)

	var wg sync.WaitGroup
	wg.Add(2)

	// Should trigger an abort
	go func() {
		defer wg.Done()

		<-s.ch.submitHdlr.processedSubmitResults
		require.True(t, mock.wasAbortCalled())
	}()

	go func() {
		defer wg.Done()

		<-s.ch.submitHdlr.processedHeadChanges

		// Should resubmit epoch 1
		require.Equal(t, SubmitStateSubmitting, s.submitState(diE1))
		// Should reset epoch 2 to start state
		require.Equal(t, SubmitStateStart, s.submitState(diE2))
	}()

	wg.Wait()

	// Send a response to the resubmit call for epoch 1
	mock.submitResult <- nil

	// Should move to the complete state for epoch 1
	<-s.ch.submitHdlr.processedSubmitResults
	require.Equal(t, SubmitStateComplete, s.submitState(diE1))
}

type smScaffolding struct {
	ctx  context.Context
	mock *mockAPI
	ch   *changeHandler
}

func makeScaffolding(t *testing.T) *smScaffolding {
	ctx := context.Background()
	actor := tutils.NewActorAddr(t, "actor")
	mock := newMockAPI()
	ch := newChangeHandler(mock, actor)
	mock.setChangeHandler(ch)

	ch.proveHdlr.processedHeadChanges = make(chan *headChange)
	ch.proveHdlr.processedPostResults = make(chan *postResult)

	ch.submitHdlr.processedHeadChanges = make(chan *headChange)
	ch.submitHdlr.processedSubmitResults = make(chan *submitResult)

	return &smScaffolding{
		ctx:  ctx,
		mock: mock,
		ch:   ch,
	}
}

func triggerHeadAdvance(t *testing.T, s *smScaffolding, height abi.ChainEpoch) {
	ts := s.mock.makeTs(t, height)
	err := s.ch.update(s.ctx, nil, ts)
	require.NoError(t, err)
}

func triggerHeadChange(t *testing.T, s *smScaffolding, revertHeight, advanceHeight abi.ChainEpoch) {
	tsRev := s.mock.makeTs(t, revertHeight)
	tsAdv := s.mock.makeTs(t, advanceHeight)
	err := s.ch.update(s.ctx, tsRev, tsAdv)
	require.NoError(t, err)
}

func (s *smScaffolding) submitState(di *dline.Info) SubmitState {
	return s.ch.submitHdlr.getPostWindow(di).submitState
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
