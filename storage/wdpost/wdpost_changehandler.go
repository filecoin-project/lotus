package wdpost

import (
	"context"
	"sync"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/dline"

	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/types"
)

const (
	SubmitConfidence    = 4
	ChallengeConfidence = 1
)

type CompleteGeneratePoSTCb func(posts []miner.SubmitWindowedPoStParams, err error)
type CompleteSubmitPoSTCb func(err error)

// WdPoStCommands is the subset of the WindowPoStScheduler + full node APIs used
// by the changeHandler to execute actions and query state.
type WdPoStCommands interface {
	StateMinerProvingDeadline(context.Context, address.Address, types.TipSetKey) (*dline.Info, error)

	startGeneratePoST(ctx context.Context, ts *types.TipSet, deadline *dline.Info, onComplete CompleteGeneratePoSTCb) context.CancelFunc
	startSubmitPoST(ctx context.Context, ts *types.TipSet, deadline *dline.Info, posts []miner.SubmitWindowedPoStParams, onComplete CompleteSubmitPoSTCb) context.CancelFunc
	onAbort(ts *types.TipSet, deadline *dline.Info)
	recordPoStFailure(err error, ts *types.TipSet, deadline *dline.Info)
}

type changeHandler struct {
	api        WdPoStCommands
	actor      address.Address
	proveHdlr  *proveHandler
	submitHdlr *submitHandler
}

func newChangeHandler(api WdPoStCommands, actor address.Address) *changeHandler {
	posts := newPostsCache()
	p := newProver(api, posts)
	s := newSubmitter(api, posts)
	return &changeHandler{api: api, actor: actor, proveHdlr: p, submitHdlr: s}
}

func (ch *changeHandler) start() {
	go ch.proveHdlr.run()
	go ch.submitHdlr.run()
}

func (ch *changeHandler) update(ctx context.Context, revert *types.TipSet, advance *types.TipSet) error {
	// Get the current deadline period
	di, err := ch.api.StateMinerProvingDeadline(ctx, ch.actor, advance.Key())
	if err != nil {
		return err
	}

	if !di.PeriodStarted() {
		return nil // not proving anything yet
	}

	hc := &headChange{
		ctx:     ctx,
		revert:  revert,
		advance: advance,
		di:      di,
	}

	select {
	case ch.proveHdlr.hcs <- hc:
	case <-ch.proveHdlr.shutdownCtx.Done():
	case <-ctx.Done():
	}

	select {
	case ch.submitHdlr.hcs <- hc:
	case <-ch.submitHdlr.shutdownCtx.Done():
	case <-ctx.Done():
	}

	return nil
}

func (ch *changeHandler) shutdown() {
	ch.proveHdlr.shutdown()
	ch.submitHdlr.shutdown()
}

func (ch *changeHandler) currentTSDI() (*types.TipSet, *dline.Info) {
	return ch.submitHdlr.currentTSDI()
}

// postsCache keeps a cache of PoSTs for each proving window
type postsCache struct {
	added chan *postInfo
	lk    sync.RWMutex
	cache map[abi.ChainEpoch][]miner.SubmitWindowedPoStParams
}

func newPostsCache() *postsCache {
	return &postsCache{
		added: make(chan *postInfo, 16),
		cache: make(map[abi.ChainEpoch][]miner.SubmitWindowedPoStParams),
	}
}

func (c *postsCache) add(di *dline.Info, posts []miner.SubmitWindowedPoStParams) {
	c.lk.Lock()
	defer c.lk.Unlock()

	// TODO: clear cache entries older than chain finality
	c.cache[di.Open] = posts

	c.added <- &postInfo{
		di:    di,
		posts: posts,
	}
}

func (c *postsCache) get(di *dline.Info) ([]miner.SubmitWindowedPoStParams, bool) {
	c.lk.RLock()
	defer c.lk.RUnlock()

	posts, ok := c.cache[di.Open]
	return posts, ok
}

type headChange struct {
	ctx     context.Context
	revert  *types.TipSet
	advance *types.TipSet
	di      *dline.Info
}

type currentPost struct {
	di    *dline.Info
	abort context.CancelFunc
}

type postResult struct {
	ts       *types.TipSet
	currPost *currentPost
	posts    []miner.SubmitWindowedPoStParams
	err      error
}

// proveHandler generates proofs
type proveHandler struct {
	api   WdPoStCommands
	posts *postsCache

	postResults chan *postResult
	hcs         chan *headChange

	current *currentPost

	shutdownCtx context.Context
	shutdown    context.CancelFunc

	// Used for testing
	processedHeadChanges chan *headChange
	processedPostResults chan *postResult
}

func newProver(
	api WdPoStCommands,
	posts *postsCache,
) *proveHandler {
	ctx, cancel := context.WithCancel(context.Background())
	return &proveHandler{
		api:         api,
		posts:       posts,
		postResults: make(chan *postResult),
		hcs:         make(chan *headChange),
		shutdownCtx: ctx,
		shutdown:    cancel,
	}
}

func (p *proveHandler) run() {
	// Abort proving on shutdown
	defer func() {
		if p.current != nil {
			p.current.abort()
		}
	}()

	for p.shutdownCtx.Err() == nil {
		select {
		case <-p.shutdownCtx.Done():
			return

		case hc := <-p.hcs:
			// Head changed
			p.processHeadChange(hc.ctx, hc.advance, hc.di)
			if p.processedHeadChanges != nil {
				p.processedHeadChanges <- hc
			}

		case res := <-p.postResults:
			// Proof generation complete
			p.processPostResult(res)
			if p.processedPostResults != nil {
				p.processedPostResults <- res
			}
		}
	}
}

func (p *proveHandler) processHeadChange(ctx context.Context, newTS *types.TipSet, di *dline.Info) {
	// If the post window has expired, abort the current proof
	if p.current != nil && newTS.Height() >= p.current.di.Close {
		// Cancel the context on the current proof
		p.current.abort()

		// Clear out the reference to the proof so that we can immediately
		// start generating a new proof, without having to worry about state
		// getting clobbered when the abort completes
		p.current = nil
	}

	// Only generate one proof at a time
	if p.current != nil {
		return
	}

	// If the proof for the current post window has been generated, check the
	// next post window
	_, complete := p.posts.get(di)
	for complete {
		di = NextDeadline(di)
		_, complete = p.posts.get(di)
	}

	// Check if the chain is above the Challenge height for the post window
	if newTS.Height() < di.Challenge+ChallengeConfidence {
		return
	}

	p.current = &currentPost{di: di}
	curr := p.current
	p.current.abort = p.api.startGeneratePoST(ctx, newTS, di, func(posts []miner.SubmitWindowedPoStParams, err error) {
		p.postResults <- &postResult{ts: newTS, currPost: curr, posts: posts, err: err}
	})
}

func (p *proveHandler) processPostResult(res *postResult) {
	di := res.currPost.di
	if res.err != nil {
		// Proving failed so inform the API
		p.api.recordPoStFailure(res.err, res.ts, di)
		log.Warnf("Aborted window post Proving (Deadline: %+v)", di)
		p.api.onAbort(res.ts, di)

		// Check if the current post has already been aborted
		if p.current == res.currPost {
			// If the current post was not already aborted, setting it to nil
			// marks it as complete so that a new post can be started
			p.current = nil
		}
		return
	}

	// Completed processing this proving window
	p.current = nil

	// Add the proofs to the cache
	p.posts.add(di, res.posts)
}

type submitResult struct {
	pw  *postWindow
	err error
}

type SubmitState string

const (
	SubmitStateStart      SubmitState = "SubmitStateStart"
	SubmitStateSubmitting SubmitState = "SubmitStateSubmitting"
	SubmitStateComplete   SubmitState = "SubmitStateComplete"
)

type postWindow struct {
	ts          *types.TipSet
	di          *dline.Info
	submitState SubmitState
	abort       context.CancelFunc
}

type postInfo struct {
	di    *dline.Info
	posts []miner.SubmitWindowedPoStParams
}

// submitHandler submits proofs on-chain
type submitHandler struct {
	api   WdPoStCommands
	posts *postsCache

	submitResults chan *submitResult
	hcs           chan *headChange

	postWindows       map[abi.ChainEpoch]*postWindow
	getPostWindowReqs chan *getPWReq

	shutdownCtx context.Context
	shutdown    context.CancelFunc

	currentCtx context.Context
	currentTS  *types.TipSet
	currentDI  *dline.Info
	getTSDIReq chan chan *tsdi

	// Used for testing
	processedHeadChanges   chan *headChange
	processedSubmitResults chan *submitResult
	processedPostReady     chan *postInfo
}

func newSubmitter(
	api WdPoStCommands,
	posts *postsCache,
) *submitHandler {
	ctx, cancel := context.WithCancel(context.Background())
	return &submitHandler{
		api:               api,
		posts:             posts,
		submitResults:     make(chan *submitResult),
		hcs:               make(chan *headChange),
		postWindows:       make(map[abi.ChainEpoch]*postWindow),
		getPostWindowReqs: make(chan *getPWReq),
		getTSDIReq:        make(chan chan *tsdi),
		shutdownCtx:       ctx,
		shutdown:          cancel,
	}
}

func (s *submitHandler) run() {
	// On shutdown, abort in-progress submits
	defer func() {
		for _, pw := range s.postWindows {
			if pw.abort != nil {
				pw.abort()
			}
		}
	}()

	for s.shutdownCtx.Err() == nil {
		select {
		case <-s.shutdownCtx.Done():
			return

		case hc := <-s.hcs:
			// Head change
			s.processHeadChange(hc.ctx, hc.revert, hc.advance, hc.di)
			if s.processedHeadChanges != nil {
				s.processedHeadChanges <- hc
			}

		case pi := <-s.posts.added:
			// Proof generated
			s.processPostReady(pi)
			if s.processedPostReady != nil {
				s.processedPostReady <- pi
			}

		case res := <-s.submitResults:
			// Submit complete
			s.processSubmitResult(res)
			if s.processedSubmitResults != nil {
				s.processedSubmitResults <- res
			}

		case pwreq := <-s.getPostWindowReqs:
			// used by getPostWindow() to sync with run loop
			pwreq.out <- s.postWindows[pwreq.di.Open]

		case out := <-s.getTSDIReq:
			// used by currentTSDI() to sync with run loop
			out <- &tsdi{ts: s.currentTS, di: s.currentDI}
		}
	}
}

// processHeadChange is called when the chain head changes
func (s *submitHandler) processHeadChange(ctx context.Context, revert *types.TipSet, advance *types.TipSet, di *dline.Info) {
	s.currentCtx = ctx
	s.currentTS = advance
	s.currentDI = di

	// Start tracking the current post window if we're not already
	// TODO: clear post windows older than chain finality
	if _, ok := s.postWindows[di.Open]; !ok {
		s.postWindows[di.Open] = &postWindow{
			di:          di,
			ts:          advance,
			submitState: SubmitStateStart,
		}
	}

	// Apply the change to all post windows
	for _, pw := range s.postWindows {
		s.processHeadChangeForPW(ctx, revert, advance, pw)
	}
}

func (s *submitHandler) processHeadChangeForPW(ctx context.Context, revert *types.TipSet, advance *types.TipSet, pw *postWindow) {
	revertedToPrevDL := revert != nil && revert.Height() < pw.di.Open
	expired := advance.Height() >= pw.di.Close

	// If the chain was reverted back to the previous deadline, or if the post
	// window has expired, abort submit
	if pw.submitState == SubmitStateSubmitting && (revertedToPrevDL || expired) {
		// Replace the aborted postWindow with a new one so that we can
		// submit again at any time without the state getting clobbered
		// when the abort completes
		abort := pw.abort
		if abort != nil {
			pw = &postWindow{
				di:          pw.di,
				ts:          advance,
				submitState: SubmitStateStart,
			}
			s.postWindows[pw.di.Open] = pw

			// Abort the current submit
			abort()
		}
	} else if pw.submitState == SubmitStateComplete && revertedToPrevDL {
		// If submit for this deadline has completed, but the chain was
		// reverted back to the previous deadline, reset the submit state to the
		// starting state, so that it can be resubmitted
		pw.submitState = SubmitStateStart
	}

	// Submit the proof to chain if the proof has been generated and the chain
	// height is above confidence
	s.submitIfReady(ctx, advance, pw)
}

// processPostReady is called when a proof generation completes
func (s *submitHandler) processPostReady(pi *postInfo) {
	pw, ok := s.postWindows[pi.di.Open]
	if ok {
		s.submitIfReady(s.currentCtx, s.currentTS, pw)
	}
}

// submitIfReady submits a proof if the chain is high enough and the proof
// has been generated for this deadline
func (s *submitHandler) submitIfReady(ctx context.Context, advance *types.TipSet, pw *postWindow) {
	// If the window has expired, there's nothing more to do.
	if advance.Height() >= pw.di.Close {
		return
	}

	// Check if we're already submitting, or already completed submit
	if pw.submitState != SubmitStateStart {
		return
	}

	// Check if we've reached the confidence height to submit
	if advance.Height() < pw.di.Open+SubmitConfidence {
		return
	}

	// Check if the proofs have been generated for this deadline
	posts, ok := s.posts.get(pw.di)
	if !ok {
		return
	}

	// If there was nothing to prove, move straight to the complete state
	if len(posts) == 0 {
		pw.submitState = SubmitStateComplete
		return
	}

	// Start submitting post
	pw.submitState = SubmitStateSubmitting
	pw.abort = s.api.startSubmitPoST(ctx, advance, pw.di, posts, func(err error) {
		s.submitResults <- &submitResult{pw: pw, err: err}
	})
}

// processSubmitResult is called with the response to a submit
func (s *submitHandler) processSubmitResult(res *submitResult) {
	if res.err != nil {
		// Submit failed so inform the API and go back to the start state
		s.api.recordPoStFailure(res.err, res.pw.ts, res.pw.di)
		log.Warnf("Aborted window post Submitting (Deadline: %+v)", res.pw.di)
		s.api.onAbort(res.pw.ts, res.pw.di)

		res.pw.submitState = SubmitStateStart
		return
	}

	// Submit succeeded so move to complete state
	res.pw.submitState = SubmitStateComplete
}

type tsdi struct {
	ts *types.TipSet
	di *dline.Info
}

func (s *submitHandler) currentTSDI() (*types.TipSet, *dline.Info) {
	out := make(chan *tsdi)
	s.getTSDIReq <- out
	res := <-out
	return res.ts, res.di
}

type getPWReq struct {
	di  *dline.Info
	out chan *postWindow
}

func (s *submitHandler) getPostWindow(di *dline.Info) *postWindow {
	out := make(chan *postWindow)
	s.getPostWindowReqs <- &getPWReq{di: di, out: out}
	return <-out
}

// NextDeadline gets deadline info for the subsequent deadline
func NextDeadline(currentDeadline *dline.Info) *dline.Info {
	periodStart := currentDeadline.PeriodStart
	newDeadline := currentDeadline.Index + 1
	if newDeadline == miner.WPoStPeriodDeadlines {
		newDeadline = 0
		periodStart = periodStart + miner.WPoStProvingPeriod()
	}

	return NewDeadlineInfo(periodStart, newDeadline, currentDeadline.CurrentEpoch)
}

func NewDeadlineInfo(periodStart abi.ChainEpoch, deadlineIdx uint64, currEpoch abi.ChainEpoch) *dline.Info {
	return dline.NewInfo(periodStart, deadlineIdx, currEpoch, miner.WPoStPeriodDeadlines, miner.WPoStProvingPeriod(), miner.WPoStChallengeWindow(), miner.WPoStChallengeLookback, miner.FaultDeclarationCutoff)
}
