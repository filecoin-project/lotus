package storage

import (
	"context"
	"sync"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/dline"
	"github.com/filecoin-project/lotus/chain/types"

	"github.com/filecoin-project/specs-actors/actors/builtin/miner"
)

type CompleteGeneratePoSTCb func(posts []miner.SubmitWindowedPoStParams, err error)
type CompleteSubmitPoSTCb func(err error)

type stateMachineAPI interface {
	StateMinerProvingDeadline(context.Context, address.Address, types.TipSetKey) (*dline.Info, error)
	startGeneratePoST(ctx context.Context, ts *types.TipSet, deadline *dline.Info, onComplete CompleteGeneratePoSTCb) context.CancelFunc
	startSubmitPoST(ctx context.Context, ts *types.TipSet, deadline *dline.Info, posts []miner.SubmitWindowedPoStParams, onComplete CompleteSubmitPoSTCb) context.CancelFunc
	onAbort(ts *types.TipSet, deadline *dline.Info, state PoSTStatus)
	failPost(err error, ts *types.TipSet, deadline *dline.Info)
}

// SubmitConfidence is the height above the deadline period Open at which to
// submit PoST
const SubmitConfidence = 4 // TODO: config

// PoSTStatus is the current state of the PoST
type PoSTStatus string

const (
	PoSTStatusStart           PoSTStatus = "PoSTStatusStart"
	PoSTStatusProving         PoSTStatus = "PoSTStatusProving"
	PoSTStatusProvingComplete PoSTStatus = "PoSTStatusProvingComplete"
	PoSTStatusSubmitting      PoSTStatus = "PoSTStatusSubmitting"
	PoSTStatusComplete        PoSTStatus = "PoSTStatusComplete"
)

func (ps PoSTStatus) String() string {
	return string(ps)
}

// FailSubmitReason enumerates the reason a PoST submit might be aborted
type FailSubmitReason string

const (
	FailSubmitReasonShutdown FailSubmitReason = "FailSubmitReasonShutdown"
	FailSubmitReasonExpire   FailSubmitReason = "FailSubmitReasonExpire"
	FailSubmitReasonReorg    FailSubmitReason = "FailSubmitReasonReorg"
	FailSubmitReasonError    FailSubmitReason = "FailSubmitReasonError"
)

func (r FailSubmitReason) Undefined() bool {
	return r == ""
}

type stateMachine struct {
	api          stateMachineAPI
	actor        address.Address
	stateChanges chan PoSTStatus

	lk          sync.RWMutex
	state       PoSTStatus
	deadline    *dline.Info
	posts       []miner.SubmitWindowedPoStParams
	abortProof  context.CancelFunc
	abortSubmit func(FailSubmitReason)
	currentTS   *types.TipSet
	currentCtx  context.Context
}

func newStateMachine(api stateMachineAPI, actor address.Address, stateChanges chan PoSTStatus) *stateMachine {
	return &stateMachine{
		api:          api,
		actor:        actor,
		state:        PoSTStatusStart,
		stateChanges: stateChanges,
	}
}

func (a *stateMachine) setState(state PoSTStatus) {
	a.state = state
	if a.stateChanges != nil {
		a.stateChanges <- state
	}
}

// startGeneratePoST triggers the process of generating PoST
func (a *stateMachine) startGeneratePoST(deadline *dline.Info) {
	a.deadline = deadline
	a.setState(PoSTStatusProving)

	ts := a.currentTS
	abort := a.api.startGeneratePoST(a.currentCtx, ts, deadline, func(posts []miner.SubmitWindowedPoStParams, err error) {
		a.lk.Lock()
		defer a.lk.Unlock()

		if err != nil {
			a.failProving(err, ts, deadline)
			return
		}
		a.completeGeneratePoST(posts)
	})
	a.abortProof = abort
}

// failProving is called when proving fails or is aborted
func (a *stateMachine) failProving(err error, ts *types.TipSet, deadline *dline.Info) {
	a.api.failPost(err, ts, deadline)
	log.Warnf("Aborting Window PoSt %s (Deadline: %+v)", PoSTStatusProving, deadline)
	a.api.onAbort(ts, deadline, PoSTStatusProving)

	// Reset to the starting state
	a.setState(PoSTStatusStart)
}

// completeGeneratePoST is called when PoST was generated successfully
func (a *stateMachine) completeGeneratePoST(posts []miner.SubmitWindowedPoStParams) {
	a.posts = posts

	// If there was nothing to prove, move straight to the complete state
	if len(a.posts) == 0 {
		a.setState(PoSTStatusComplete)
		return
	}

	// If there is something to prove, move to proving complete
	a.setState(PoSTStatusProvingComplete)

	// Generating the proof has completed, so trigger a state change that will
	// check if we're at the right height to submit the proof
	a.runStateChange()
}

// startSubmitPoST triggers submitting the PoST to chain
func (a *stateMachine) startSubmitPoST() {
	a.setState(PoSTStatusSubmitting)

	// failReason will be set if submit is aborted, eg if the state machine
	// shuts down or the current PoST expires
	var failReason FailSubmitReason

	ts := a.currentTS
	deadline := a.deadline
	abort := a.api.startSubmitPoST(a.currentCtx, ts, deadline, a.posts, func(err error) {
		a.lk.Lock()
		defer a.lk.Unlock()

		if err != nil {
			// If there was an error, and it wasn't because the submit was
			// aborted, set the fail reason to be because of an error
			if failReason.Undefined() {
				failReason = FailSubmitReasonError
			}
			a.failSubmit(err, ts, deadline, failReason)
			return
		}
		a.setState(PoSTStatusComplete)
	})
	a.abortSubmit = func(r FailSubmitReason) {
		// Record the reason for aborting, then abort
		failReason = r
		abort()
	}
}

// failSubmit is called when proof submit fails or is aborted
func (a *stateMachine) failSubmit(err error, ts *types.TipSet, deadline *dline.Info, failReason FailSubmitReason) {
	a.api.failPost(err, ts, deadline)
	log.Warnf("Aborting Window PoSt %s (Deadline: %+v)", PoSTStatusSubmitting, deadline)
	a.api.onAbort(ts, deadline, PoSTStatusSubmitting)

	switch {
	case failReason == FailSubmitReasonExpire || failReason == FailSubmitReasonShutdown:
		// If the proving deadline has expired, or the state machine has shut
		// down, there's nothing more to be done with this proof, so go back to
		// the start state
		a.setState(PoSTStatusStart)
	case failReason == FailSubmitReasonError || failReason == FailSubmitReasonReorg:
		// If there was an error submitting, or the chain reorged, we can still
		// use the proof so go back to the proving complete state
		a.setState(PoSTStatusProvingComplete)
	}

	// If the abort was because of expiry, trigger a state change so that the
	// next proof can start generating immediately.
	// If the abort was because of a chain reorg, trigger a state change so
	// that the proof can be resubmitted immediately.
	if failReason == FailSubmitReasonExpire || failReason == FailSubmitReasonReorg {
		a.runStateChange()
	}
}

// HeadChange is called when the chain head changes
func (a *stateMachine) HeadChange(ctx context.Context, newTS *types.TipSet, reorged bool) error {
	a.lk.Lock()
	defer a.lk.Unlock()

	a.currentCtx = ctx
	a.currentTS = newTS

	return a.stateChange(reorged)
}

// runStateChange is called from within the state machine
func (a *stateMachine) runStateChange() {
	err := a.stateChange(false)
	if err != nil {
		log.Errorf("handling state change in window post state machine: %+v", err)
	}
}

// stateChange checks the current state and takes appropriate action
func (a *stateMachine) stateChange(reorged bool) error {
	ctx := a.currentCtx
	newTS := a.currentTS

	// Get the current proving deadline info
	diNew, err := a.api.StateMinerProvingDeadline(ctx, a.actor, newTS.Key())
	if err != nil {
		return err
	}

	// If the current proving period hasn't started yet, bail out
	if !diNew.PeriodStarted() {
		return nil // not proving anything yet
	}

	// If the active PoST has expired, abort it
	newHeight := newTS.Height()
	expired := a.deadline != nil && newHeight >= a.deadline.Close
	inActiveState := a.state == PoSTStatusProving || a.state == PoSTStatusProvingComplete || a.state == PoSTStatusSubmitting
	if expired && inActiveState {
		stateWhenAborted := a.state
		a.abort(newTS, diNew, FailSubmitReasonExpire)

		// If the state was proving or submitting, we need to return here and
		// wait until the abort has been processed.
		// If the state was proving complete, we can safely continue.
		if stateWhenAborted != PoSTStatusProvingComplete {
			return nil
		}
	}

	// If proof generation for the current deadline has not started, start it
	if a.state == PoSTStatusStart {
		a.startGeneratePoST(diNew)
		return nil
	}

	// If the proof has been generated, and it's not already being submitted,
	// submit it
	reachedSubmitHeight := a.deadline != nil && newHeight > a.deadline.Open+SubmitConfidence
	if reachedSubmitHeight && a.state == PoSTStatusProvingComplete {
		a.startSubmitPoST()
		return nil
	}

	// If the proof is being submitted, and the chain has reorged, abort the
	// submit
	if a.state == PoSTStatusSubmitting && reorged {
		a.abortSubmit(FailSubmitReasonReorg)
		return nil
	}

	// If the proof has been submitted
	if a.state == PoSTStatusComplete {
		// Check if the current height is within the challenge period for the
		// next deadline
		diNext := nextDeadline(diNew)
		if newHeight < diNext.Challenge {
			return nil
		}

		// Start generating the proof for the next deadline
		a.startGeneratePoST(diNext)

		return nil
	}

	// Nothing to do, wait for next state event
	return nil
}

// Shutdown stops the state machine
func (a *stateMachine) Shutdown() {
	a.lk.Lock()
	defer a.lk.Unlock()

	a.abort(a.currentTS, a.deadline, FailSubmitReasonShutdown)
}

// abort is called when the state machine is shut down or when the current
// deadline expires
// returns true if an active state was aborted, false if it was an idle (start
// or end) state
func (a *stateMachine) abort(ts *types.TipSet, deadline *dline.Info, reason FailSubmitReason) {
	// If the window post is being proved or submitted, abort those operations
	// and wait for them to clean up and move onto the next state
	switch a.state {
	case PoSTStatusProving:
		a.abortProof()
		return
	case PoSTStatusSubmitting:
		a.abortSubmit(reason)
		return
	}

	// If the window post is not idle (eg in PoSTStatusProvingComplete)
	if a.state == PoSTStatusProvingComplete {
		// Log a warning and inform the API that proving was aborted
		log.Warnf("Aborting Window PoSt (Deadline: %+v)", deadline)
		a.api.onAbort(ts, deadline, a.state)
	}

	// Reset to the starting state
	a.setState(PoSTStatusStart)
}

// CurrentState returns the current PoSTStatus
func (a *stateMachine) CurrentState() PoSTStatus {
	a.lk.RLock()
	defer a.lk.RUnlock()

	return a.state
}

// CurrentTSDL gets the current tipset and deadline info
func (a *stateMachine) CurrentTSDL() (*types.TipSet, *dline.Info) {
	a.lk.RLock()
	defer a.lk.RUnlock()

	var ts *types.TipSet
	if a.currentTS != nil {
		cp := *a.currentTS
		ts = &cp
	}

	var dl *dline.Info
	if a.deadline != nil {
		cp := *a.deadline
		dl = &cp
	}

	return ts, dl
}

// nextDeadline gets deadline info for the subsequent deadline
func nextDeadline(currentDeadline *dline.Info) *dline.Info {
	periodStart := currentDeadline.PeriodStart
	newDeadline := currentDeadline.Index + 1
	if newDeadline == miner.WPoStPeriodDeadlines {
		newDeadline = 0
		periodStart = periodStart + miner.WPoStProvingPeriod
	}

	return miner.NewDeadlineInfo(periodStart, newDeadline, currentDeadline.CurrentEpoch)
}
