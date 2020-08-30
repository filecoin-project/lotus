package paychmgr

import (
	"bytes"
	"context"
	"fmt"
	"sync"

	"golang.org/x/sync/errgroup"

	"github.com/filecoin-project/specs-actors/actors/abi/big"

	"github.com/filecoin-project/specs-actors/actors/builtin"
	init_ "github.com/filecoin-project/specs-actors/actors/builtin/init"
	"github.com/filecoin-project/specs-actors/actors/builtin/paych"
	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"

	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors"
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
	from    address.Address
	to      address.Address
	amt     types.BigInt

	lk sync.Mutex
	// merge parent, if this req is part of a merge
	merge *mergedFundsReq
	// whether the req's context has been cancelled
	active bool
}

func newFundsReq(ctx context.Context, from address.Address, to address.Address, amt types.BigInt) *fundsReq {
	promise := make(chan *paychFundsRes)
	return &fundsReq{
		ctx:     ctx,
		promise: promise,
		from:    from,
		to:      to,
		amt:     amt,
		active:  true,
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

	r.active = false
	m := r.merge

	r.lk.Unlock()

	// If there's a merge parent, tell the merge parent to check if it has any
	// active reqs left
	if m != nil {
		m.checkActive()
	}
}

// isActive indicates whether the req's context has been cancelled
func (r *fundsReq) isActive() bool {
	r.lk.Lock()
	defer r.lk.Unlock()

	return r.active
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
	m := &mergedFundsReq{
		ctx:    ctx,
		cancel: cancel,
		reqs:   reqs,
	}

	for _, r := range m.reqs {
		r.setMergeParent(m)
	}

	// If the requests were all cancelled while being added, cancel the context
	// immediately
	m.checkActive()

	return m
}

// Called when a fundsReq is cancelled
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

func (m *mergedFundsReq) from() address.Address {
	return m.reqs[0].from
}

func (m *mergedFundsReq) to() address.Address {
	return m.reqs[0].to
}

// sum is the sum of the amounts in all requests in the merge
func (m *mergedFundsReq) sum() types.BigInt {
	sum := types.NewInt(0)
	for _, r := range m.reqs {
		if r.isActive() {
			sum = types.BigAdd(sum, r.amt)
		}
	}
	return sum
}

// getPaych ensures that a channel exists between the from and to addresses,
// and adds the given amount of funds.
// If the channel does not exist a create channel message is sent and the
// message CID is returned.
// If the channel does exist an add funds message is sent and both the channel
// address and message CID are returned.
// If there is an in progress operation (create channel / add funds), getPaych
// blocks until the previous operation completes, then returns both the channel
// address and the CID of the new add funds message.
// If an operation returns an error, subsequent waiting operations will still
// be attempted.
func (ca *channelAccessor) getPaych(ctx context.Context, from, to address.Address, amt types.BigInt) (address.Address, cid.Cid, error) {
	// Add the request to add funds to a queue and wait for the result
	freq := newFundsReq(ctx, from, to, amt)
	ca.enqueue(freq)
	select {
	case res := <-freq.promise:
		return res.channel, res.mcid, res.err
	case <-ctx.Done():
		freq.cancel()
		return address.Undef, cid.Undef, ctx.Err()
	}
}

// Queue up an add funds operations
func (ca *channelAccessor) enqueue(task *fundsReq) {
	ca.lk.Lock()
	defer ca.lk.Unlock()

	ca.fundsReqQueue = append(ca.fundsReqQueue, task)
	go ca.processQueue()
}

// Run the operations in the queue
func (ca *channelAccessor) processQueue() {
	ca.lk.Lock()
	defer ca.lk.Unlock()

	// Remove cancelled requests
	ca.filterQueue()

	// If there's nothing in the queue, bail out
	if len(ca.fundsReqQueue) == 0 {
		return
	}

	// Merge all pending requests into one.
	// For example if there are pending requests for 3, 2, 4 then
	// amt = 3 + 2 + 4 = 9
	merged := newMergedFundsReq(ca.fundsReqQueue[:])
	amt := merged.sum()
	if amt.IsZero() {
		// Note: The amount can be zero if requests are cancelled as we're
		// building the mergedFundsReq
		return
	}

	res := ca.processTask(merged.ctx, merged.from(), merged.to(), amt)

	// If the task is waiting on an external event (eg something to appear on
	// chain) it will return nil
	if res == nil {
		// Stop processing the fundsReqQueue and wait. When the event occurs it will
		// call processQueue() again
		return
	}

	// Finished processing so clear the queue
	ca.fundsReqQueue = nil

	// Call the task callback with its results
	merged.onComplete(res)
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
func (ca *channelAccessor) msgWaitComplete(mcid cid.Cid, err error) {
	ca.lk.Lock()
	defer ca.lk.Unlock()

	// Save the message result to the store
	dserr := ca.store.SaveMessageResult(mcid, err)
	if dserr != nil {
		log.Errorf("saving message result: %s", dserr)
	}

	// Inform listeners that the message has completed
	ca.msgListeners.fireMsgComplete(mcid, err)

	// The queue may have been waiting for msg completion to proceed, so
	// process the next queue item
	if len(ca.fundsReqQueue) > 0 {
		go ca.processQueue()
	}
}

// processTask checks the state of the channel and takes appropriate action
// (see description of getPaych).
// Note that processTask may be called repeatedly in the same state, and should
// return nil if there is no state change to be made (eg when waiting for a
// message to be confirmed on chain)
func (ca *channelAccessor) processTask(
	ctx context.Context,
	from address.Address,
	to address.Address,
	amt types.BigInt,
) *paychFundsRes {
	// Get the payment channel for the from/to addresses.
	// Note: It's ok if we get ErrChannelNotTracked. It just means we need to
	// create a channel.
	channelInfo, err := ca.store.OutboundActiveByFromTo(from, to)
	if err != nil && err != ErrChannelNotTracked {
		return &paychFundsRes{err: err}
	}

	// If a channel has not yet been created, create one.
	if channelInfo == nil {
		mcid, err := ca.createPaych(ctx, from, to, amt)
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

	// We need to add more funds, so send an add funds message to
	// cover the amount for this request
	mcid, err := ca.addFunds(ctx, channelInfo, amt)
	if err != nil {
		return &paychFundsRes{err: err}
	}
	return &paychFundsRes{channel: *channelInfo.Channel, mcid: *mcid}
}

// createPaych sends a message to create the channel and returns the message cid
func (ca *channelAccessor) createPaych(ctx context.Context, from, to address.Address, amt types.BigInt) (cid.Cid, error) {
	params, aerr := actors.SerializeParams(&paych.ConstructorParams{From: from, To: to})
	if aerr != nil {
		return cid.Undef, aerr
	}

	enc, aerr := actors.SerializeParams(&init_.ExecParams{
		CodeCID:           builtin.PaymentChannelActorCodeID,
		ConstructorParams: params,
	})
	if aerr != nil {
		return cid.Undef, aerr
	}

	msg := &types.Message{
		To:     builtin.InitActorAddr,
		From:   from,
		Value:  amt,
		Method: builtin.MethodsInit.Exec,
		Params: enc,
	}

	smsg, err := ca.api.MpoolPushMessage(ctx, msg, nil)
	if err != nil {
		return cid.Undef, xerrors.Errorf("initializing paych actor: %w", err)
	}
	mcid := smsg.Cid()

	// Create a new channel in the store
	ci, err := ca.store.CreateChannel(from, to, mcid, amt)
	if err != nil {
		log.Errorf("creating channel: %s", err)
		return cid.Undef, err
	}

	// Wait for the channel to be created on chain
	go ca.waitForPaychCreateMsg(ci.ChannelID, mcid)

	return mcid, nil
}

// waitForPaychCreateMsg waits for mcid to appear on chain and stores the robust address of the
// created payment channel
func (ca *channelAccessor) waitForPaychCreateMsg(channelID string, mcid cid.Cid) {
	err := ca.waitPaychCreateMsg(channelID, mcid)
	ca.msgWaitComplete(mcid, err)
}

func (ca *channelAccessor) waitPaychCreateMsg(channelID string, mcid cid.Cid) error {
	mwait, err := ca.api.StateWaitMsg(ca.waitCtx, mcid, build.MessageConfidence)
	if err != nil {
		log.Errorf("wait msg: %w", err)
		return err
	}

	// If channel creation failed
	if mwait.Receipt.ExitCode != 0 {
		ca.lk.Lock()
		defer ca.lk.Unlock()

		// Channel creation failed, so remove the channel from the datastore
		dserr := ca.store.RemoveChannel(channelID)
		if dserr != nil {
			log.Errorf("failed to remove channel %s: %s", channelID, dserr)
		}

		err := xerrors.Errorf("payment channel creation failed (exit code %d)", mwait.Receipt.ExitCode)
		log.Error(err)
		return err
	}

	var decodedReturn init_.ExecReturn
	err = decodedReturn.UnmarshalCBOR(bytes.NewReader(mwait.Receipt.Return))
	if err != nil {
		log.Error(err)
		return err
	}

	ca.lk.Lock()
	defer ca.lk.Unlock()

	// Store robust address of channel
	ca.mutateChannelInfo(channelID, func(channelInfo *ChannelInfo) {
		channelInfo.Channel = &decodedReturn.RobustAddress
		channelInfo.Amount = channelInfo.PendingAmount
		channelInfo.PendingAmount = big.NewInt(0)
		channelInfo.CreateMsg = nil
	})

	return nil
}

// addFunds sends a message to add funds to the channel and returns the message cid
func (ca *channelAccessor) addFunds(ctx context.Context, channelInfo *ChannelInfo, amt types.BigInt) (*cid.Cid, error) {
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
	ca.mutateChannelInfo(channelInfo.ChannelID, func(ci *ChannelInfo) {
		ci.PendingAmount = amt
		ci.AddFundsMsg = &mcid
	})

	// Store a reference from the message CID to the channel, so that we can
	// look up the channel from the message CID
	err = ca.store.SaveNewMessage(channelInfo.ChannelID, mcid)
	if err != nil {
		log.Errorf("saving add funds message CID %s: %s", mcid, err)
	}

	go ca.waitForAddFundsMsg(channelInfo.ChannelID, mcid)

	return &mcid, nil
}

// waitForAddFundsMsg waits for mcid to appear on chain and returns error, if any
func (ca *channelAccessor) waitForAddFundsMsg(channelID string, mcid cid.Cid) {
	err := ca.waitAddFundsMsg(channelID, mcid)
	ca.msgWaitComplete(mcid, err)
}

func (ca *channelAccessor) waitAddFundsMsg(channelID string, mcid cid.Cid) error {
	mwait, err := ca.api.StateWaitMsg(ca.waitCtx, mcid, build.MessageConfidence)
	if err != nil {
		log.Error(err)
		return err
	}

	if mwait.Receipt.ExitCode != 0 {
		err := xerrors.Errorf("voucher channel creation failed: adding funds (exit code %d)", mwait.Receipt.ExitCode)
		log.Error(err)

		ca.lk.Lock()
		defer ca.lk.Unlock()

		ca.mutateChannelInfo(channelID, func(channelInfo *ChannelInfo) {
			channelInfo.PendingAmount = big.NewInt(0)
			channelInfo.AddFundsMsg = nil
		})

		return err
	}

	ca.lk.Lock()
	defer ca.lk.Unlock()

	// Store updated amount
	ca.mutateChannelInfo(channelID, func(channelInfo *ChannelInfo) {
		channelInfo.Amount = types.BigAdd(channelInfo.Amount, channelInfo.PendingAmount)
		channelInfo.PendingAmount = big.NewInt(0)
		channelInfo.AddFundsMsg = nil
	})

	return nil
}

// Change the state of the channel in the store
func (ca *channelAccessor) mutateChannelInfo(channelID string, mutate func(*ChannelInfo)) {
	channelInfo, err := ca.store.ByChannelID(channelID)

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

	err = ca.store.putChannelInfo(channelInfo)
	if err != nil {
		log.Errorf("Error writing channel info to store: %s", err)
	}
}

// restartPending checks the datastore to see if there are any channels that
// have outstanding create / add funds messages, and if so, waits on the
// messages.
// Outstanding messages can occur if a create / add funds message was sent and
// then the system was shut down or crashed before the result was received.
func (pm *Manager) restartPending() error {
	cis, err := pm.store.WithPendingAddFunds()
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
				go ca.waitForPaychCreateMsg(ci.ChannelID, *ci.CreateMsg)
				return nil
			})
		} else if ci.AddFundsMsg != nil {
			group.Go(func() error {
				ca, err := pm.accessorByAddress(*ci.Channel)
				if err != nil {
					return xerrors.Errorf("error initializing payment channel manager %s: %s", ci.Channel, err)
				}
				go ca.waitForAddFundsMsg(ci.ChannelID, *ci.AddFundsMsg)
				return nil
			})
		}
	}

	return group.Wait()
}

// getPaychWaitReady waits for a the response to the message with the given cid
func (ca *channelAccessor) getPaychWaitReady(ctx context.Context, mcid cid.Cid) (address.Address, error) {
	ca.lk.Lock()

	// First check if the message has completed
	msgInfo, err := ca.store.GetMessage(mcid)
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
		ci, err := ca.store.ByMessageCid(mcid)
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
				ci, err := ca.store.ByMessageCid(mcid)
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
