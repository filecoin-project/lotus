package market

import (
	"context"
	"fmt"
	"sync"

	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	logging "github.com/ipfs/go-log/v2"
	"go.uber.org/fx"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/builtin"
	"github.com/filecoin-project/go-state-types/builtin/v9/market"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/node/impl/full"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
)

var log = logging.Logger("market_adapter")

// FundManagerAPI is the fx dependencies need to run a fund manager
type FundManagerAPI struct {
	fx.In

	full.StateAPI
	full.MpoolAPI
}

// fundManagerAPI is the specific methods called by the FundManager
// (used by the tests)
type fundManagerAPI interface {
	MpoolPushMessage(context.Context, *types.Message, *api.MessageSendSpec) (*types.SignedMessage, error)
	StateMarketBalance(context.Context, address.Address, types.TipSetKey) (api.MarketBalance, error)
	StateWaitMsg(ctx context.Context, cid cid.Cid, confidence uint64, limit abi.ChainEpoch, allowReplaced bool) (*api.MsgLookup, error)
}

// FundManager keeps track of funds in a set of addresses
type FundManager struct {
	ctx      context.Context
	shutdown context.CancelFunc
	api      fundManagerAPI
	str      *Store

	lk          sync.Mutex
	fundedAddrs map[address.Address]*fundedAddress
}

func NewFundManager(lc fx.Lifecycle, api FundManagerAPI, ds dtypes.MetadataDS) *FundManager {
	fm := newFundManager(&api, ds)
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			return fm.Start()
		},
		OnStop: func(ctx context.Context) error {
			fm.Stop()
			return nil
		},
	})
	return fm
}

// newFundManager is used by the tests
func newFundManager(api fundManagerAPI, ds datastore.Batching) *FundManager {
	ctx, cancel := context.WithCancel(context.Background())
	return &FundManager{
		ctx:         ctx,
		shutdown:    cancel,
		api:         api,
		str:         newStore(ds),
		fundedAddrs: make(map[address.Address]*fundedAddress),
	}
}

func (fm *FundManager) Stop() {
	fm.shutdown()
}

func (fm *FundManager) Start() error {
	fm.lk.Lock()
	defer fm.lk.Unlock()

	// TODO:
	// To save memory:
	// - in State() only load addresses with in-progress messages
	// - load the others just-in-time from getFundedAddress
	// - delete(fm.fundedAddrs, addr) when the queue has been processed
	return fm.str.forEach(fm.ctx, func(state *FundedAddressState) {
		fa := newFundedAddress(fm, state.Addr)
		fa.state = state
		fm.fundedAddrs[fa.state.Addr] = fa
		fa.start()
	})
}

// Creates a fundedAddress if it doesn't already exist, and returns it
func (fm *FundManager) getFundedAddress(addr address.Address) *fundedAddress {
	fm.lk.Lock()
	defer fm.lk.Unlock()

	fa, ok := fm.fundedAddrs[addr]
	if !ok {
		fa = newFundedAddress(fm, addr)
		fm.fundedAddrs[addr] = fa
	}
	return fa
}

// Reserve adds amt to `reserved`. If there are not enough available funds for
// the address, submits a message on chain to top up available funds.
// Returns the cid of the message that was submitted on chain, or cid.Undef if
// the required funds were already available.
func (fm *FundManager) Reserve(ctx context.Context, wallet, addr address.Address, amt abi.TokenAmount) (cid.Cid, error) {
	return fm.getFundedAddress(addr).reserve(ctx, wallet, amt)
}

// Release subtracts from `reserved`.
func (fm *FundManager) Release(addr address.Address, amt abi.TokenAmount) error {
	return fm.getFundedAddress(addr).release(amt)
}

// Withdraw unreserved funds. Only succeeds if there are enough unreserved
// funds for the address.
// Returns the cid of the message that was submitted on chain.
func (fm *FundManager) Withdraw(ctx context.Context, wallet, addr address.Address, amt abi.TokenAmount) (cid.Cid, error) {
	return fm.getFundedAddress(addr).withdraw(ctx, wallet, amt)
}

// GetReserved returns the amount that is currently reserved for the address
func (fm *FundManager) GetReserved(addr address.Address) abi.TokenAmount {
	return fm.getFundedAddress(addr).getReserved()
}

// FundedAddressState keeps track of the state of an address with funds in the
// datastore
type FundedAddressState struct {
	Addr address.Address
	// AmtReserved is the amount that must be kept in the address (cannot be
	// withdrawn)
	AmtReserved abi.TokenAmount
	// MsgCid is the cid of an in-progress on-chain message
	MsgCid *cid.Cid
}

// fundedAddress keeps track of the state and request queues for a
// particular address
type fundedAddress struct {
	ctx context.Context
	env *fundManagerEnvironment
	str *Store

	lk    sync.RWMutex
	state *FundedAddressState

	// Note: These request queues are ephemeral, they are not saved to store
	reservations []*fundRequest
	releases     []*fundRequest
	withdrawals  []*fundRequest

	// Used by the tests
	onProcessStartListener func() bool
}

func newFundedAddress(fm *FundManager, addr address.Address) *fundedAddress {
	return &fundedAddress{
		ctx: fm.ctx,
		env: &fundManagerEnvironment{api: fm.api},
		str: fm.str,
		state: &FundedAddressState{
			Addr:        addr,
			AmtReserved: abi.NewTokenAmount(0),
		},
	}
}

// If there is an in-progress on-chain message, don't submit any more messages
// on chain until it completes
func (a *fundedAddress) start() {
	a.lk.Lock()
	defer a.lk.Unlock()

	if a.state.MsgCid != nil {
		a.debugf("restart: wait for %s", a.state.MsgCid)
		a.startWaitForResults(*a.state.MsgCid)
	}
}

func (a *fundedAddress) getReserved() abi.TokenAmount {
	a.lk.RLock()
	defer a.lk.RUnlock()

	return a.state.AmtReserved
}

func (a *fundedAddress) reserve(ctx context.Context, wallet address.Address, amt abi.TokenAmount) (cid.Cid, error) {
	return a.requestAndWait(ctx, wallet, amt, &a.reservations)
}

func (a *fundedAddress) release(amt abi.TokenAmount) error {
	_, err := a.requestAndWait(context.Background(), address.Undef, amt, &a.releases)
	return err
}

func (a *fundedAddress) withdraw(ctx context.Context, wallet address.Address, amt abi.TokenAmount) (cid.Cid, error) {
	return a.requestAndWait(ctx, wallet, amt, &a.withdrawals)
}

func (a *fundedAddress) requestAndWait(ctx context.Context, wallet address.Address, amt abi.TokenAmount, reqs *[]*fundRequest) (cid.Cid, error) {
	// Create a request and add it to the request queue
	req := newFundRequest(ctx, wallet, amt)

	a.lk.Lock()
	*reqs = append(*reqs, req)
	a.lk.Unlock()

	// Process the queue
	go a.process()

	// Wait for the results
	select {
	case <-ctx.Done():
		return cid.Undef, ctx.Err()
	case r := <-req.Result:
		return r.msgCid, r.err
	}
}

// Used by the tests
func (a *fundedAddress) onProcessStart(fn func() bool) {
	a.lk.Lock()
	defer a.lk.Unlock()

	a.onProcessStartListener = fn
}

// Process queued requests
func (a *fundedAddress) process() {
	a.lk.Lock()
	defer a.lk.Unlock()

	// Used by the tests
	if a.onProcessStartListener != nil {
		done := a.onProcessStartListener()
		if !done {
			return
		}
		a.onProcessStartListener = nil
	}

	// Check if we're still waiting for the response to a message
	if a.state.MsgCid != nil {
		return
	}

	// Check if there's anything to do
	haveReservations := len(a.reservations) > 0 || len(a.releases) > 0
	haveWithdrawals := len(a.withdrawals) > 0
	if !haveReservations && !haveWithdrawals {
		return
	}

	// Process reservations / releases
	if haveReservations {
		res, err := a.processReservations(a.reservations, a.releases)
		if err == nil {
			a.applyStateChange(res.msgCid, res.amtReserved)
		}
		a.reservations = filterOutProcessedReqs(a.reservations)
		a.releases = filterOutProcessedReqs(a.releases)
	}

	// If there was no message sent on chain by adding reservations, and all
	// reservations have completed processing, process withdrawals
	if haveWithdrawals && a.state.MsgCid == nil && len(a.reservations) == 0 {
		withdrawalCid, err := a.processWithdrawals(a.withdrawals)
		if err == nil && withdrawalCid != cid.Undef {
			a.applyStateChange(&withdrawalCid, types.EmptyInt)
		}
		a.withdrawals = filterOutProcessedReqs(a.withdrawals)
	}

	// If a message was sent on-chain
	if a.state.MsgCid != nil {
		// Start waiting for results of message (async)
		a.startWaitForResults(*a.state.MsgCid)
	}

	// Process any remaining queued requests
	go a.process()
}

// Filter out completed requests
func filterOutProcessedReqs(reqs []*fundRequest) []*fundRequest {
	filtered := make([]*fundRequest, 0, len(reqs))
	for _, req := range reqs {
		if !req.Completed() {
			filtered = append(filtered, req)
		}
	}
	return filtered
}

// Apply the results of processing queues and save to the datastore
func (a *fundedAddress) applyStateChange(msgCid *cid.Cid, amtReserved abi.TokenAmount) {
	a.state.MsgCid = msgCid
	if !amtReserved.Nil() {
		a.state.AmtReserved = amtReserved
	}
	a.saveState()
}

// Clear the pending message cid so that a new message can be sent
func (a *fundedAddress) clearWaitState() {
	a.state.MsgCid = nil
	a.saveState()
}

// Save state to datastore
func (a *fundedAddress) saveState() {
	// Not much we can do if saving to the datastore fails, just log
	err := a.str.save(a.ctx, a.state)
	if err != nil {
		log.Errorf("saving state to store for addr %s: %v", a.state.Addr, err)
	}
}

// The result of processing the reservation / release queues
type processResult struct {
	// Requests that completed without adding funds
	covered []*fundRequest
	// Requests that added funds
	added []*fundRequest

	// The new reserved amount
	amtReserved abi.TokenAmount
	// The message cid, if a message was submitted on-chain
	msgCid *cid.Cid
}

// process reservations and releases, and return the resulting changes to state
func (a *fundedAddress) processReservations(reservations []*fundRequest, releases []*fundRequest) (pr *processResult, prerr error) {
	// When the function returns
	defer func() {
		// If there's an error, mark all requests as errored
		if prerr != nil {
			for _, req := range append(reservations, releases...) {
				req.Complete(cid.Undef, prerr)
			}
			return
		}

		// Complete all release requests
		for _, req := range releases {
			req.Complete(cid.Undef, nil)
		}

		// Complete all requests that were covered by released amounts
		for _, req := range pr.covered {
			req.Complete(cid.Undef, nil)
		}

		// If a message was sent
		if pr.msgCid != nil {
			// Complete all add funds requests
			for _, req := range pr.added {
				req.Complete(*pr.msgCid, nil)
			}
		}
	}()

	// Split reservations into those that are covered by released amounts,
	// and those to add to the reserved amount.
	// Note that we process requests from the same wallet in batches. So some
	// requests may not be included in covered if they don't match the first
	// covered request's wallet. These will be processed on a subsequent
	// invocation of processReservations.
	toCancel, toAdd, reservedDelta := splitReservations(reservations, releases)

	// Apply the reserved delta to the reserved amount
	reserved := types.BigAdd(a.state.AmtReserved, reservedDelta)
	if reserved.LessThan(abi.NewTokenAmount(0)) {
		reserved = abi.NewTokenAmount(0)
	}
	res := &processResult{
		amtReserved: reserved,
		covered:     toCancel,
	}

	// Work out the amount to add to the balance
	amtToAdd := abi.NewTokenAmount(0)
	if len(toAdd) > 0 && reserved.GreaterThan(abi.NewTokenAmount(0)) {
		// Get available funds for address
		avail, err := a.env.AvailableFunds(a.ctx, a.state.Addr)
		if err != nil {
			return res, err
		}

		// amount to add = new reserved amount - available
		amtToAdd = types.BigSub(reserved, avail)
		a.debugf("reserved %d - avail %d = to add %d", reserved, avail, amtToAdd)
	}

	// If there's nothing to add to the balance, bail out
	if amtToAdd.LessThanEqual(abi.NewTokenAmount(0)) {
		res.covered = append(res.covered, toAdd...)
		return res, nil
	}

	// Add funds to address
	a.debugf("add funds %d", amtToAdd)
	addFundsCid, err := a.env.AddFunds(a.ctx, toAdd[0].Wallet, a.state.Addr, amtToAdd)
	if err != nil {
		return res, err
	}

	// Mark reservation requests as complete
	res.added = toAdd

	// Save the message CID to state
	res.msgCid = &addFundsCid
	return res, nil
}

// Split reservations into those that are under the total release amount
// (covered) and those that exceed it (to add).
// Note that we process requests from the same wallet in batches. So some
// requests may not be included in covered if they don't match the first
// covered request's wallet.
func splitReservations(reservations []*fundRequest, releases []*fundRequest) ([]*fundRequest, []*fundRequest, abi.TokenAmount) {
	toCancel := make([]*fundRequest, 0, len(reservations))
	toAdd := make([]*fundRequest, 0, len(reservations))
	toAddAmt := abi.NewTokenAmount(0)

	// Sum release amounts
	releaseAmt := abi.NewTokenAmount(0)
	for _, req := range releases {
		releaseAmt = types.BigAdd(releaseAmt, req.Amount())
	}

	// We only want to combine requests that come from the same wallet
	batchWallet := address.Undef
	for _, req := range reservations {
		amt := req.Amount()

		// If the amount to add to the reserve is cancelled out by a release
		if amt.LessThanEqual(releaseAmt) {
			// Cancel the request and update the release total
			releaseAmt = types.BigSub(releaseAmt, amt)
			toCancel = append(toCancel, req)
			continue
		}

		// The amount to add is greater that the release total so we want
		// to send an add funds request

		// The first time the wallet will be undefined
		if batchWallet == address.Undef {
			batchWallet = req.Wallet
		}
		// If this request's wallet is the same as the batch wallet,
		// the requests will be combined
		if batchWallet == req.Wallet {
			delta := types.BigSub(amt, releaseAmt)
			toAddAmt = types.BigAdd(toAddAmt, delta)
			releaseAmt = abi.NewTokenAmount(0)
			toAdd = append(toAdd, req)
		}
	}

	// The change in the reserved amount is "amount to add" - "amount to release"
	reservedDelta := types.BigSub(toAddAmt, releaseAmt)

	return toCancel, toAdd, reservedDelta
}

// process withdrawal queue
func (a *fundedAddress) processWithdrawals(withdrawals []*fundRequest) (msgCid cid.Cid, prerr error) {
	// If there's an error, mark all withdrawal requests as errored
	defer func() {
		if prerr != nil {
			for _, req := range withdrawals {
				req.Complete(cid.Undef, prerr)
			}
		}
	}()

	// Get the net available balance
	avail, err := a.env.AvailableFunds(a.ctx, a.state.Addr)
	if err != nil {
		return cid.Undef, err
	}

	netAvail := types.BigSub(avail, a.state.AmtReserved)

	// Fit as many withdrawals as possible into the available balance, and fail
	// the rest
	withdrawalAmt := abi.NewTokenAmount(0)
	allowedAmt := abi.NewTokenAmount(0)
	allowed := make([]*fundRequest, 0, len(withdrawals))
	var batchWallet address.Address
	for _, req := range withdrawals {
		amt := req.Amount()
		if amt.IsZero() {
			// If the context for the request was cancelled, bail out
			req.Complete(cid.Undef, err)
			continue
		}

		// If the amount would exceed the available amount, complete the
		// request with an error
		newWithdrawalAmt := types.BigAdd(withdrawalAmt, amt)
		if newWithdrawalAmt.GreaterThan(netAvail) {
			msg := fmt.Sprintf("insufficient funds for withdrawal of %s: ", types.FIL(amt))
			msg += fmt.Sprintf("net available (%s) = available (%s) - reserved (%s)",
				types.FIL(types.BigSub(netAvail, withdrawalAmt)), types.FIL(avail), types.FIL(a.state.AmtReserved))
			if !withdrawalAmt.IsZero() {
				msg += fmt.Sprintf(" - queued withdrawals (%s)", types.FIL(withdrawalAmt))
			}
			err := xerrors.Errorf(msg)
			a.debugf("%s", err)
			req.Complete(cid.Undef, err)
			continue
		}

		// If this is the first allowed withdrawal request in this batch, save
		// its wallet address
		if batchWallet == address.Undef {
			batchWallet = req.Wallet
		}
		// If the request wallet doesn't match the batch wallet, bail out
		// (the withdrawal will be processed after the current batch has
		// completed)
		if req.Wallet != batchWallet {
			continue
		}

		// Include this withdrawal request in the batch
		withdrawalAmt = newWithdrawalAmt
		a.debugf("withdraw %d", amt)
		allowed = append(allowed, req)
		allowedAmt = types.BigAdd(allowedAmt, amt)
	}

	// Check if there is anything to withdraw.
	// Note that if the context for a request is cancelled,
	// req.Amount() returns zero
	if allowedAmt.Equals(abi.NewTokenAmount(0)) {
		// Mark allowed requests as complete
		for _, req := range allowed {
			req.Complete(cid.Undef, nil)
		}
		return cid.Undef, nil
	}

	// Withdraw funds
	a.debugf("withdraw funds %d", allowedAmt)
	withdrawFundsCid, err := a.env.WithdrawFunds(a.ctx, allowed[0].Wallet, a.state.Addr, allowedAmt)
	if err != nil {
		return cid.Undef, err
	}

	// Mark allowed requests as complete
	for _, req := range allowed {
		req.Complete(withdrawFundsCid, nil)
	}

	// Save the message CID to state
	return withdrawFundsCid, nil
}

// asynchronously wait for results of message
func (a *fundedAddress) startWaitForResults(msgCid cid.Cid) {
	go func() {
		err := a.env.WaitMsg(a.ctx, msgCid)
		if err != nil {
			// We don't really care about the results here, we're just waiting
			// so as to only process one on-chain message at a time
			log.Errorf("waiting for results of message %s for addr %s: %v", msgCid, a.state.Addr, err)
		}

		a.lk.Lock()
		a.debugf("complete wait")
		a.clearWaitState()
		a.lk.Unlock()

		a.process()
	}()
}

func (a *fundedAddress) debugf(args ...interface{}) {
	fmtStr := args[0].(string)
	args = args[1:]
	log.Debugf(a.state.Addr.String()+": "+fmtStr, args...)
}

// The result of a fund request
type reqResult struct {
	msgCid cid.Cid
	err    error
}

// A request to change funds
type fundRequest struct {
	ctx       context.Context
	amt       abi.TokenAmount
	completed chan struct{}
	Wallet    address.Address
	Result    chan reqResult
}

func newFundRequest(ctx context.Context, wallet address.Address, amt abi.TokenAmount) *fundRequest {
	return &fundRequest{
		ctx:       ctx,
		amt:       amt,
		Wallet:    wallet,
		Result:    make(chan reqResult),
		completed: make(chan struct{}),
	}
}

// Amount returns zero if the context has expired
func (frp *fundRequest) Amount() abi.TokenAmount {
	if frp.ctx.Err() != nil {
		return abi.NewTokenAmount(0)
	}
	return frp.amt
}

// Complete is called with the message CID when the funds request has been
// started or with the error if there was an error
func (frp *fundRequest) Complete(msgCid cid.Cid, err error) {
	select {
	case <-frp.completed:
	case <-frp.ctx.Done():
	case frp.Result <- reqResult{msgCid: msgCid, err: err}:
	}
	close(frp.completed)
}

// Completed indicates if Complete has already been called
func (frp *fundRequest) Completed() bool {
	select {
	case <-frp.completed:
		return true
	default:
		return false
	}
}

// fundManagerEnvironment simplifies some API calls
type fundManagerEnvironment struct {
	api fundManagerAPI
}

func (env *fundManagerEnvironment) AvailableFunds(ctx context.Context, addr address.Address) (abi.TokenAmount, error) {
	bal, err := env.api.StateMarketBalance(ctx, addr, types.EmptyTSK)
	if err != nil {
		return abi.NewTokenAmount(0), err
	}

	return types.BigSub(bal.Escrow, bal.Locked), nil
}

func (env *fundManagerEnvironment) AddFunds(
	ctx context.Context,
	wallet address.Address,
	addr address.Address,
	amt abi.TokenAmount,
) (cid.Cid, error) {
	params, err := actors.SerializeParams(&addr)
	if err != nil {
		return cid.Undef, err
	}

	smsg, aerr := env.api.MpoolPushMessage(ctx, &types.Message{
		To:     builtin.StorageMarketActorAddr,
		From:   wallet,
		Value:  amt,
		Method: builtin.MethodsMarket.AddBalance,
		Params: params,
	}, nil)

	if aerr != nil {
		return cid.Undef, aerr
	}

	return smsg.Cid(), nil
}

func (env *fundManagerEnvironment) WithdrawFunds(
	ctx context.Context,
	wallet address.Address,
	addr address.Address,
	amt abi.TokenAmount,
) (cid.Cid, error) {
	params, err := actors.SerializeParams(&market.WithdrawBalanceParams{
		ProviderOrClientAddress: addr,
		Amount:                  amt,
	})
	if err != nil {
		return cid.Undef, xerrors.Errorf("serializing params: %w", err)
	}

	smsg, aerr := env.api.MpoolPushMessage(ctx, &types.Message{
		To:     builtin.StorageMarketActorAddr,
		From:   wallet,
		Value:  types.NewInt(0),
		Method: builtin.MethodsMarket.WithdrawBalance,
		Params: params,
	}, nil)

	if aerr != nil {
		return cid.Undef, aerr
	}

	return smsg.Cid(), nil
}

func (env *fundManagerEnvironment) WaitMsg(ctx context.Context, c cid.Cid) error {
	_, err := env.api.StateWaitMsg(ctx, c, buildconstants.MessageConfidence, api.LookbackNoLimit, true)
	return err
}
