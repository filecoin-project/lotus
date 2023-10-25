package lpwindow

import (
	"context"
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/builtin/v9/miner"
	"github.com/filecoin-project/go-state-types/dline"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/storage/wdpost"
)

type changeHandler struct {
	api       WDPoStAPI
	actor     address.Address
	proveHdlr *proveHandler
}

func newChangeHandler(api WDPoStAPI, actor address.Address) *changeHandler {
	p := newProver(api)
	return &changeHandler{api: api, actor: actor, proveHdlr: p}
}

func (ch *changeHandler) start() {
	go ch.proveHdlr.run()
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

type proveHandler struct {
	api   WdPoStCommands
	posts *postsCache

	postResults chan *postResult
	hcs         chan *headChange

	current *currentPost

	shutdownCtx context.Context
	shutdown    context.CancelFunc
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

func newProver(
	api WDPoStAPI,
) *proveHandler {
	ctx, cancel := context.WithCancel(context.Background())
	return &proveHandler{
		api:         api,
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

		case res := <-p.postResults:
			// Proof generation complete
			p.processPostResult(res)
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
		di = wdpost.NextDeadline(di)
		_, complete = p.posts.get(di)
	}

	// Check if the chain is above the Challenge height for the post window
	if newTS.Height() < di.Challenge+wdpost.ChallengeConfidence {
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
