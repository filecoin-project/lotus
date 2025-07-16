package paychmgr

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"

	"github.com/ipfs/go-cid"
	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/big"
	init2 "github.com/filecoin-project/specs-actors/v2/actors/builtin/init"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/types"
)

// paychFundsRes is the response to a create channel or add funds request
type paychFundsRes struct {
	channel address.Address
	mcid    cid.Cid
	err     error
}

// fundsReq is a request to create a channel or add funds to a channel
type fundsReq struct {
	ctx     context.Context
	promise chan *paychFundsRes
	amt     types.BigInt
	opts    GetOpts

	lk sync.Mutex
	// merge parent, if this req is part of a merge
	merge *mergedFundsReq
}

func newFundsReq(ctx context.Context, amt types.BigInt, opts GetOpts) *fundsReq {
	promise := make(chan *paychFundsRes, 1)
	return &fundsReq{
		ctx:     ctx,
		promise: promise,
		amt:     amt,
		opts:    opts,
	}
}

// onComplete is called when the funds request has been executed
func (r *fundsReq) onComplete(res *paychFundsRes) {
	select {
	case <-r.ctx.Done():
	case r.promise <- res:
	}
}

// cancel is called when the req's context is cancelled
func (r *fundsReq) cancel() {
	r.lk.Lock()
	defer r.lk.Unlock()

	// If there's a merge parent, tell the merge parent to check if it has any
	// active reqs left
	if r.merge != nil {
		r.merge.checkActive()
	}
}

// isActive indicates whether the req's context has been cancelled
func (r *fundsReq) isActive() bool {
	return r.ctx.Err() == nil
}

// setMergeParent sets the merge that this req is part of
func (r *fundsReq) setMergeParent(m *mergedFundsReq) {
	r.lk.Lock()
	defer r.lk.Unlock()

	r.merge = m
}

// mergedFundsReq merges together multiple add funds requests that are queued
// up, so that only one message is sent for all the requests (instead of one
// message for each request)
type mergedFundsReq struct {
	ctx    context.Context
	cancel context.CancelFunc
	reqs   []*fundsReq
}

func newMergedFundsReq(reqs []*fundsReq) *mergedFundsReq {
	ctx, cancel := context.WithCancel(context.Background())

	rqs := make([]*fundsReq, len(reqs))
	copy(rqs, reqs)
	m := &mergedFundsReq{
		ctx:    ctx,
		cancel: cancel,
		reqs:   rqs,
	}

	for _, r := range m.reqs {
		r.setMergeParent(m)
	}

	sort.Slice(m.reqs, func(i, j int) bool {
		if m.reqs[i].opts.OffChain != m.reqs[j].opts.OffChain { // off-chain first
			return m.reqs[i].opts.OffChain
		}

		if m.reqs[i].opts.Reserve != m.reqs[j].opts.Reserve { // non-reserve after off-chain
			return m.reqs[i].opts.Reserve
		}

		// sort by amount asc (reducing latency for smaller requests)
		return m.reqs[i].amt.LessThan(m.reqs[j].amt)
	})

	// If the requests were all cancelled while being added, cancel the context
	// immediately
	m.checkActive()

	return m
}

// checkActive is called when a fundsReq is cancelled
func (m *mergedFundsReq) checkActive() {
	// Check if there are any active fundsReqs
	for _, r := range m.reqs {
		if r.isActive() {
			return
		}
	}

	// If all fundsReqs have been cancelled, cancel the context
	m.cancel()
}

// onComplete is called when the queue has executed the mergeFundsReq.
// Calls onComplete on each fundsReq in the mergeFundsReq.
func (m *mergedFundsReq) onComplete(res *paychFundsRes) {
	for _, r := range m.reqs {
		if r.isActive() {
			r.onComplete(res)
		}
	}
}

// sum is the sum of the amounts in all requests in the merge
func (m *mergedFundsReq) sum() (types.BigInt, types.BigInt) {
	sum := types.NewInt(0)
	avail := types.NewInt(0)

	for _, r := range m.reqs {
		if r.isActive() {
			sum = types.BigAdd(sum, r.amt)
			if !r.opts.Reserve {
				avail = types.BigAdd(avail, r.amt)
			}
		}
	}

	return sum, avail
}

// completeAmount completes first non-reserving requests up to the available amount
func (m *mergedFundsReq) completeAmount(avail types.BigInt, channelInfo *ChannelInfo) (*paychFundsRes, types.BigInt, types.BigInt) {
	used, failed := types.NewInt(0), types.NewInt(0)
	next := 0

	// order: [offchain+reserve, !offchain+reserve, !offchain+!reserve]
	for i, r := range m.reqs {
		if !r.opts.Reserve {
			// non-reserving request are put after reserving requests, so we are done here
			break
		}

		// don't try to fill inactive requests
		if !r.isActive() {
			continue
		}

		if r.amt.GreaterThan(types.BigSub(avail, used)) {
			// requests are sorted by amount ascending, so if we hit this, there aren't any more requests we can fill

			if r.opts.OffChain {
				// can't fill, so OffChain want an error
				if r.isActive() {
					failed = types.BigAdd(failed, r.amt)
					r.onComplete(&paychFundsRes{
						channel: *channelInfo.Channel,
						err:     xerrors.Errorf("not enough funds available in the payment channel %s; add funds with 'lotus paych add-funds %s %s %s'", channelInfo.Channel, channelInfo.from(), channelInfo.to(), types.FIL(r.amt).Unitless()),
					})
				}
				next = i + 1
				continue
			}

			break
		}

		used = types.BigAdd(used, r.amt)
		r.onComplete(&paychFundsRes{channel: *channelInfo.Channel})
		next = i + 1
	}

	m.reqs = m.reqs[next:]
	if len(m.reqs) == 0 {
		return &paychFundsRes{channel: *channelInfo.Channel}, used, failed
	}
	return nil, used, failed
}

func (m *mergedFundsReq) failOffChainNoChannel(from, to address.Address) (*paychFundsRes, types.BigInt) {
	next := 0
	freed := types.NewInt(0)

	for i, r := range m.reqs {
		if !r.opts.OffChain {
			break
		}

		freed = types.BigAdd(freed, r.amt)
		if !r.isActive() {
			continue
		}
		r.onComplete(&paychFundsRes{err: xerrors.Errorf("payment channel doesn't exist, create with 'lotus paych add-funds %s %s %s'", from, to, types.FIL(r.amt).Unitless())})
		next = i + 1
	}

	m.reqs = m.reqs[next:]
	if len(m.reqs) == 0 {
		return &paychFundsRes{err: xerrors.Errorf("payment channel doesn't exist, create with 'lotus paych add-funds %s %s 0'", from, to)}, freed
	}

	return nil, freed
}

// getPaych ensures that a channel exists between the from and to addresses,
// and reserves (or adds as available) the given amount of funds.
// If the channel does not exist a create channel message is sent and the
// message CID is returned.
// If the channel does exist an add funds message is sent and both the channel
// address and message CID are returned.
// If there is an in progress operation (create channel / add funds), getPaych
// blocks until the previous operation completes, then returns both the channel
// address and the CID of the new add funds message.
// If an operation returns an error, subsequent waiting operations will still
// be attempted.
func (ca *channelAccessor) getPaych(ctx context.Context, amt types.BigInt, opts GetOpts) (address.Address, cid.Cid, error) {
	// Add the request to add funds to a queue and wait for the result
	freq := newFundsReq(ctx, amt, opts)
	ca.enqueue(ctx, freq)
	select {
	case res := <-freq.promise:
		return res.channel, res.mcid, res.err
	case <-ctx.Done():
		freq.cancel()
		return address.Undef, cid.Undef, ctx.Err()
	}
}

// Queue up an add funds operation
func (ca *channelAccessor) enqueue(ctx context.Context, task *fundsReq) {
	ca.lk.Lock()
	defer ca.lk.Unlock()

	ca.fundsReqQueue = append(ca.fundsReqQueue, task)
	go ca.processQueue(ctx, "") // nolint: errcheck
}

// Run the operations in the queue
func (ca *channelAccessor) processQueue(ctx context.Context, channelID string) (*api.ChannelAvailableFunds, error) {
	ca.lk.Lock()
	defer ca.lk.Unlock()

	// Remove cancelled requests
	ca.filterQueue()

	// If there's nothing in the queue, bail out
	if len(ca.fundsReqQueue) == 0 {
		return ca.currentAvailableFunds(ctx, channelID, types.NewInt(0))
	}

	// Merge all pending requests into one.
	// For example if there are pending requests for 3, 2, 4 then
	// amt = 3 + 2 + 4 = 9
	merged := newMergedFundsReq(ca.fundsReqQueue)
	amt, avail := merged.sum()
	if amt.IsZero() {
		// Note: The amount can be zero if requests are cancelled as we're
		// building the mergedFundsReq
		return ca.currentAvailableFunds(ctx, channelID, amt)
	}

	res := ca.processTask(merged, amt, avail)

	// If the task is waiting on an external event (eg something to appear on
	// chain) it will return nil
	if res == nil {
		// Stop processing the fundsReqQueue and wait. When the event occurs it will
		// call processQueue() again
		return ca.currentAvailableFunds(ctx, channelID, amt)
	}

	// Finished processing so clear the queue
	ca.fundsReqQueue = nil

	// Call the task callback with its results
	merged.onComplete(res)

	return ca.currentAvailableFunds(ctx, channelID, types.NewInt(0))
}

// filterQueue filters cancelled requests out of the queue
func (ca *channelAccessor) filterQueue() {
	if len(ca.fundsReqQueue) == 0 {
		return
	}

	// Remove cancelled requests
	i := 0
	for _, r := range ca.fundsReqQueue {
		if r.isActive() {
			ca.fundsReqQueue[i] = r
			i++
		}
	}

	// Allow GC of remaining slice elements
	for rem := i; rem < len(ca.fundsReqQueue); rem++ {
		ca.fundsReqQueue[i] = nil
	}

	// Resize slice
	ca.fundsReqQueue = ca.fundsReqQueue[:i]
}

// queueSize is the size of the funds request queue (used by tests)
func (ca *channelAccessor) queueSize() int {
	ca.lk.Lock()
	defer ca.lk.Unlock()

	return len(ca.fundsReqQueue)
}

// msgWaitComplete is called when the message for a previous task is confirmed
// or there is an error.
func (ca *channelAccessor) msgWaitComplete(ctx context.Context, mcid cid.Cid, err error) {
	// if context is canceled, should Not mark message to 'bad', just return.
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return
	}

	ca.lk.Lock()
	defer ca.lk.Unlock()

	// Save the message result to the store
	dserr := ca.store.SaveMessageResult(ctx, mcid, err)
	if dserr != nil {
		log.Errorf("saving message result: %s", dserr)
	}

	// Inform listeners that the message has completed
	ca.msgListeners.fireMsgComplete(mcid, err)

	// The queue may have been waiting for msg completion to proceed, so
	// process the next queue item
	if len(ca.fundsReqQueue) > 0 {
		go ca.processQueue(ctx, "") // nolint: errcheck
	}
}

func (ca *channelAccessor) currentAvailableFunds(ctx context.Context, channelID string, queuedAmt types.BigInt) (*api.ChannelAvailableFunds, error) {
	if len(channelID) == 0 {
		return nil, nil
	}

	channelInfo, err := ca.store.ByChannelID(ctx, channelID)
	if err != nil {
		return nil, err
	}

	// The channel may have a pending create or add funds message
	waitSentinel := channelInfo.CreateMsg
	if waitSentinel == nil {
		waitSentinel = channelInfo.AddFundsMsg
	}

	// Get the total amount redeemed by vouchers.
	// This includes vouchers that have been submitted, and vouchers that are
	// in the datastore but haven't yet been submitted.
	totalRedeemed := types.NewInt(0)
	if channelInfo.Channel != nil {
		ch := *channelInfo.Channel
		_, pchState, err := ca.sa.loadPaychActorState(ca.chctx, ch)
		if err != nil {
			return nil, err
		}

		laneStates, err := ca.laneState(ctx, pchState, ch)
		if err != nil {
			return nil, err
		}

		for _, ls := range laneStates {
			r, err := ls.Redeemed()
			if err != nil {
				return nil, err
			}
			totalRedeemed = types.BigAdd(totalRedeemed, r)
		}
	}

	return &api.ChannelAvailableFunds{
		Channel:             channelInfo.Channel,
		From:                channelInfo.from(),
		To:                  channelInfo.to(),
		ConfirmedAmt:        channelInfo.Amount,
		PendingAmt:          channelInfo.PendingAmount,
		NonReservedAmt:      channelInfo.AvailableAmount,
		PendingAvailableAmt: channelInfo.PendingAvailableAmount,
		PendingWaitSentinel: waitSentinel,
		QueuedAmt:           queuedAmt,
		VoucherReedeemedAmt: totalRedeemed,
	}, nil
}

// processTask checks the state of the channel and takes appropriate action
// (see description of getPaych).
// Note that processTask may be called repeatedly in the same state, and should
// return nil if there is no state change to be made (eg when waiting for a
// message to be confirmed on chain)
func (ca *channelAccessor) processTask(merged *mergedFundsReq, amt, avail types.BigInt) *paychFundsRes {
	ctx := merged.ctx

	// Get the payment channel for the from/to addresses.
	// Note: It's ok if we get ErrChannelNotTracked. It just means we need to
	// create a channel.
	channelInfo, err := ca.store.OutboundActiveByFromTo(ctx, ca.api, ca.from, ca.to)
	if err != nil && err != ErrChannelNotTracked {
		return &paychFundsRes{err: err}
	}

	// If a channel has not yet been created, create one.
	if channelInfo == nil {
		res, freed := merged.failOffChainNoChannel(ca.from, ca.to)
		if res != nil {
			return res
		}
		amt = types.BigSub(amt, freed)

		mcid, err := ca.createPaych(ctx, amt, avail)
		if err != nil {
			return &paychFundsRes{err: err}
		}

		return &paychFundsRes{mcid: mcid}
	}

	// If the create channel message has been sent but the channel hasn't
	// been created on chain yet
	if channelInfo.CreateMsg != nil {
		// Wait for the channel to be created before trying again
		return nil
	}

	// If an add funds message was sent to the chain but hasn't been confirmed
	// on chain yet
	if channelInfo.AddFundsMsg != nil {
		// Wait for the add funds message to be confirmed before trying again
		return nil
	}

	// Try to fill requests using available funds, without going to the chain
	res, amt := ca.completeAvailable(ctx, merged, channelInfo, amt, avail)

	if res != nil || amt.LessThanEqual(types.NewInt(0)) {
		return res
	}

	// We need to add more funds, so send an add funds message to
	// cover the amount for this request
	mcid, err := ca.addFunds(ctx, channelInfo, amt, avail)
	if err != nil {
		return &paychFundsRes{err: err}
	}
	return &paychFundsRes{channel: *channelInfo.Channel, mcid: *mcid}
}

// createPaych sends a message to create the channel and returns the message cid
func (ca *channelAccessor) createPaych(ctx context.Context, amt, avail types.BigInt) (cid.Cid, error) {
	mb, err := ca.messageBuilder(ctx, ca.from)
	if err != nil {
		return cid.Undef, err
	}
	msg, err := mb.Create(ca.to, amt)
	if err != nil {
		return cid.Undef, err
	}

	smsg, err := ca.api.MpoolPushMessage(ctx, msg, nil)
	if err != nil {
		return cid.Undef, xerrors.Errorf("initializing paych actor: %w", err)
	}
	mcid := smsg.Cid()

	// Create a new channel in the store
	ci, err := ca.store.CreateChannel(ctx, ca.from, ca.to, mcid, amt, avail)
	if err != nil {
		log.Errorf("creating channel: %s", err)
		return cid.Undef, err
	}

	// Wait for the channel to be created on chain
	go ca.waitForPaychCreateMsg(ctx, ci.ChannelID, mcid)

	return mcid, nil
}

// waitForPaychCreateMsg waits for mcid to appear on chain and stores the robust address of the
// created payment channel
func (ca *channelAccessor) waitForPaychCreateMsg(ctx context.Context, channelID string, mcid cid.Cid) {
	err := ca.waitPaychCreateMsg(ctx, channelID, mcid)
	ca.msgWaitComplete(ctx, mcid, err)
}

func (ca *channelAccessor) waitPaychCreateMsg(ctx context.Context, channelID string, mcid cid.Cid) error {
	mwait, err := ca.api.StateWaitMsg(ca.chctx, mcid, buildconstants.MessageConfidence, api.LookbackNoLimit, true)
	if err != nil {
		log.Errorf("wait msg: %v", err)
		return err
	}

	// If channel creation failed
	if mwait.Receipt.ExitCode.IsError() {
		ca.lk.Lock()
		defer ca.lk.Unlock()

		// Channel creation failed, so remove the channel from the datastore
		dserr := ca.store.RemoveChannel(ctx, channelID)
		if dserr != nil {
			log.Errorf("failed to remove channel %s: %s", channelID, dserr)
		}

		err := xerrors.Errorf("payment channel creation failed (exit code %d)", mwait.Receipt.ExitCode)
		log.Error(err)
		return err
	}

	// TODO: ActorUpgrade abstract over this.
	// This "works" because it hasn't changed from v0 to v2, but we still
	// need an abstraction here.
	var decodedReturn init2.ExecReturn
	err = decodedReturn.UnmarshalCBOR(bytes.NewReader(mwait.Receipt.Return))
	if err != nil {
		log.Error(err)
		return err
	}

	ca.lk.Lock()
	defer ca.lk.Unlock()

	// Store robust address of channel
	ca.mutateChannelInfo(ctx, channelID, func(channelInfo *ChannelInfo) {
		channelInfo.Channel = &decodedReturn.RobustAddress
		channelInfo.Amount = channelInfo.PendingAmount
		channelInfo.AvailableAmount = channelInfo.PendingAvailableAmount
		channelInfo.PendingAmount = big.NewInt(0)
		channelInfo.PendingAvailableAmount = big.NewInt(0)
		channelInfo.CreateMsg = nil
	})

	return nil
}

// completeAvailable fills reserving fund requests using already available funds, without interacting with the chain
func (ca *channelAccessor) completeAvailable(ctx context.Context, merged *mergedFundsReq, channelInfo *ChannelInfo, amt, av types.BigInt) (*paychFundsRes, types.BigInt) {
	toReserve := types.BigSub(amt, av)
	avail := types.NewInt(0)

	// reserve at most what we need
	ca.mutateChannelInfo(ctx, channelInfo.ChannelID, func(ci *ChannelInfo) {
		avail = ci.AvailableAmount
		if avail.GreaterThan(toReserve) {
			avail = toReserve
		}
		ci.AvailableAmount = big.Sub(ci.AvailableAmount, avail)
	})

	res, used, failed := merged.completeAmount(avail, channelInfo)

	// return any unused reserved funds (e.g. from cancelled requests)
	ca.mutateChannelInfo(ctx, channelInfo.ChannelID, func(ci *ChannelInfo) {
		ci.AvailableAmount = types.BigAdd(ci.AvailableAmount, types.BigSub(avail, used))
	})

	return res, types.BigSub(amt, types.BigAdd(used, failed))
}

// addFunds sends a message to add funds to the channel and returns the message cid
func (ca *channelAccessor) addFunds(ctx context.Context, channelInfo *ChannelInfo, amt, avail types.BigInt) (*cid.Cid, error) {
	msg := &types.Message{
		To:     *channelInfo.Channel,
		From:   channelInfo.Control,
		Value:  amt,
		Method: 0,
	}

	smsg, err := ca.api.MpoolPushMessage(ctx, msg, nil)
	if err != nil {
		return nil, err
	}
	mcid := smsg.Cid()

	// Store the add funds message CID on the channel
	ca.mutateChannelInfo(ctx, channelInfo.ChannelID, func(ci *ChannelInfo) {
		ci.PendingAmount = amt
		ci.PendingAvailableAmount = avail
		ci.AddFundsMsg = &mcid
	})

	// Store a reference from the message CID to the channel, so that we can
	// look up the channel from the message CID
	err = ca.store.SaveNewMessage(ctx, channelInfo.ChannelID, mcid)
	if err != nil {
		log.Errorf("saving add funds message CID %s: %s", mcid, err)
	}

	go ca.waitForAddFundsMsg(ctx, channelInfo.ChannelID, mcid)

	return &mcid, nil
}

// TODO func (ca *channelAccessor) freeFunds(ctx context.Context, channelInfo *ChannelInfo, amt, avail types.BigInt) (*cid.Cid, error) {

// waitForAddFundsMsg waits for mcid to appear on chain and returns error, if any
func (ca *channelAccessor) waitForAddFundsMsg(ctx context.Context, channelID string, mcid cid.Cid) {
	err := ca.waitAddFundsMsg(ctx, channelID, mcid)
	ca.msgWaitComplete(ctx, mcid, err)
}

func (ca *channelAccessor) waitAddFundsMsg(ctx context.Context, channelID string, mcid cid.Cid) error {
	mwait, err := ca.api.StateWaitMsg(ca.chctx, mcid, buildconstants.MessageConfidence, api.LookbackNoLimit, true)
	if err != nil {
		log.Error(err)
		return err
	}

	if mwait.Receipt.ExitCode.IsError() {
		err := xerrors.Errorf("voucher channel creation failed: adding funds (exit code %d)", mwait.Receipt.ExitCode)
		log.Error(err)

		ca.lk.Lock()
		defer ca.lk.Unlock()

		ca.mutateChannelInfo(ctx, channelID, func(channelInfo *ChannelInfo) {
			channelInfo.PendingAmount = big.NewInt(0)
			channelInfo.PendingAvailableAmount = big.NewInt(0)
			channelInfo.AddFundsMsg = nil
		})

		return err
	}

	ca.lk.Lock()
	defer ca.lk.Unlock()

	// Store updated amount
	ca.mutateChannelInfo(ctx, channelID, func(channelInfo *ChannelInfo) {
		channelInfo.Amount = types.BigAdd(channelInfo.Amount, channelInfo.PendingAmount)
		channelInfo.AvailableAmount = types.BigAdd(channelInfo.AvailableAmount, channelInfo.PendingAvailableAmount)
		channelInfo.PendingAmount = big.NewInt(0)
		channelInfo.PendingAvailableAmount = big.NewInt(0)
		channelInfo.AddFundsMsg = nil
	})

	return nil
}

// Change the state of the channel in the store
func (ca *channelAccessor) mutateChannelInfo(ctx context.Context, channelID string, mutate func(*ChannelInfo)) {
	channelInfo, err := ca.store.ByChannelID(ctx, channelID)

	// If there's an error reading or writing to the store just log an error.
	// For now we're assuming it's unlikely to happen in practice.
	// Later we may want to implement a transactional approach, whereby
	// we record to the store that we're going to send a message, send
	// the message, and then record that the message was sent.
	if err != nil {
		log.Errorf("Error reading channel info from store: %s", err)
		return
	}

	mutate(channelInfo)

	err = ca.store.putChannelInfo(ctx, channelInfo)
	if err != nil {
		log.Errorf("Error writing channel info to store: %s", err)
	}
}

// restartPending checks the datastore to see if there are any channels that
// have outstanding create / add funds messages, and if so, waits on the
// messages.
// Outstanding messages can occur if a create / add funds message was sent and
// then the system was shut down or crashed before the result was received.
func (pm *Manager) restartPending(ctx context.Context) error {
	cis, err := pm.store.WithPendingAddFunds(ctx)
	if err != nil {
		return err
	}

	group := errgroup.Group{}
	for _, chanInfo := range cis {
		ci := chanInfo
		if ci.CreateMsg != nil {
			group.Go(func() error {
				ca, err := pm.accessorByFromTo(ci.Control, ci.Target)
				if err != nil {
					return xerrors.Errorf("error initializing payment channel manager %s -> %s: %s", ci.Control, ci.Target, err)
				}
				go ca.waitForPaychCreateMsg(ctx, ci.ChannelID, *ci.CreateMsg)
				return nil
			})
		} else if ci.AddFundsMsg != nil {
			group.Go(func() error {
				ca, err := pm.accessorByAddress(ctx, *ci.Channel)
				if err != nil {
					return xerrors.Errorf("error initializing payment channel manager %s: %s", ci.Channel, err)
				}
				go ca.waitForAddFundsMsg(ctx, ci.ChannelID, *ci.AddFundsMsg)
				return nil
			})
		}
	}

	return group.Wait()
}

// getPaychWaitReady waits for the response to the message with the given cid
func (ca *channelAccessor) getPaychWaitReady(ctx context.Context, mcid cid.Cid) (address.Address, error) {
	ca.lk.Lock()

	// First check if the message has completed
	msgInfo, err := ca.store.GetMessage(ctx, mcid)
	if err != nil {
		ca.lk.Unlock()

		return address.Undef, err
	}

	// If the create channel / add funds message failed, return an error
	if len(msgInfo.Err) > 0 {
		ca.lk.Unlock()

		return address.Undef, xerrors.New(msgInfo.Err)
	}

	// If the message has completed successfully
	if msgInfo.Received {
		ca.lk.Unlock()

		// Get the channel address
		ci, err := ca.store.ByMessageCid(ctx, mcid)
		if err != nil {
			return address.Undef, err
		}

		if ci.Channel == nil {
			panic(fmt.Sprintf("create / add funds message %s succeeded but channelInfo.Channel is nil", mcid))
		}
		return *ci.Channel, nil
	}

	// The message hasn't completed yet so wait for it to complete
	promise := ca.msgPromise(ctx, mcid)

	// Unlock while waiting
	ca.lk.Unlock()

	select {
	case res := <-promise:
		return res.channel, res.err
	case <-ctx.Done():
		return address.Undef, ctx.Err()
	}
}

type onMsgRes struct {
	channel address.Address
	err     error
}

// msgPromise returns a channel that receives the result of the message with
// the given CID
func (ca *channelAccessor) msgPromise(ctx context.Context, mcid cid.Cid) chan onMsgRes {
	promise := make(chan onMsgRes)
	triggerUnsub := make(chan struct{})
	unsub := ca.msgListeners.onMsgComplete(mcid, func(err error) {
		close(triggerUnsub)

		// Use a go-routine so as not to block the event handler loop
		go func() {
			res := onMsgRes{err: err}
			if res.err == nil {
				// Get the channel associated with the message cid
				ci, err := ca.store.ByMessageCid(ctx, mcid)
				if err != nil {
					res.err = err
				} else {
					res.channel = *ci.Channel
				}
			}

			// Pass the result to the caller
			select {
			case promise <- res:
			case <-ctx.Done():
			}
		}()
	})

	// Unsubscribe when the message is received or the context is done
	go func() {
		select {
		case <-ctx.Done():
		case <-triggerUnsub:
		}

		unsub()
	}()

	return promise
}

func (ca *channelAccessor) availableFunds(ctx context.Context, channelID string) (*api.ChannelAvailableFunds, error) {
	return ca.processQueue(ctx, channelID)
}
