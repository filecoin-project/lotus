package market

import (
	"context"
	"sync"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/builtin/market"

	"github.com/filecoin-project/lotus/build"

	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	logging "github.com/ipfs/go-log"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/node/impl/full"
	"go.uber.org/fx"
)

var log = logging.Logger("market_adapter")

// API is the fx dependencies need to run a fund manager
type FundManagerAPI struct {
	fx.In

	full.StateAPI
	full.MpoolAPI
}

// fundManagerAPI is the specific methods called by the FundManager
type fundManagerAPI interface {
	MpoolPushMessage(context.Context, *types.Message, *api.MessageSendSpec) (*types.SignedMessage, error)
	StateMarketBalance(context.Context, address.Address, types.TipSetKey) (api.MarketBalance, error)
	StateWaitMsg(ctx context.Context, cid cid.Cid, confidence uint64) (*api.MsgLookup, error)
}

// FundManager keeps track of funds in a set of addresses
type FundManager struct {
	ctx      context.Context
	shutdown context.CancelFunc
	api      fundManagerAPI
	wallet   address.Address
	str      *Store

	lk          sync.Mutex
	fundedAddrs map[address.Address]*fundedAddress
}

type waitSentinel cid.Cid

var waitSentinelUndef = waitSentinel(cid.Undef)

func NewFundManager(api fundManagerAPI, ds datastore.Batching, wallet address.Address) *FundManager {
	ctx, cancel := context.WithCancel(context.Background())
	return &FundManager{
		ctx:         ctx,
		shutdown:    cancel,
		api:         api,
		wallet:      wallet,
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
	return fm.str.forEach(func(state *FundedAddressState) {
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

// Reserve adds amt to `reserved`. If there is not enough available funds for
// the address, submits a message on chain to top up available funds.
func (fm *FundManager) Reserve(ctx context.Context, addr address.Address, amt abi.TokenAmount) (waitSentinel, error) {
	return fm.getFundedAddress(addr).reserve(ctx, amt)
}

// Subtract from `reserved`.
func (fm *FundManager) Release(ctx context.Context, addr address.Address, amt abi.TokenAmount) error {
	return fm.getFundedAddress(addr).release(ctx, amt)
}

// Withdraw unreserved funds. Only succeeds if there are enough unreserved
// funds for the address.
func (fm *FundManager) Withdraw(ctx context.Context, addr address.Address, amt abi.TokenAmount) (waitSentinel, error) {
	return fm.getFundedAddress(addr).withdraw(ctx, amt)
}

// Waits for a reserve or withdraw to complete.
func (fm *FundManager) Wait(ctx context.Context, sentinel waitSentinel) error {
	_, err := fm.api.StateWaitMsg(ctx, cid.Cid(sentinel), build.MessageConfidence)
	return err
}

// FundedAddressState keeps track of the state of an address with funds in the
// datastore
type FundedAddressState struct {
	// Wallet is the wallet from which funds are added to the address
	Wallet address.Address
	Addr   address.Address
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

	lk    sync.Mutex
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
			Wallet:      fm.wallet,
			Addr:        addr,
			AmtReserved: abi.NewTokenAmount(0),
		},
	}
}

// If there is a in-progress on-chain message, don't submit any more messages
// on chain until it completes
func (a *fundedAddress) start() {
	a.lk.Lock()
	defer a.lk.Unlock()

	if a.state.MsgCid != nil {
		a.debugf("restart: wait for %s", a.state.MsgCid)
		a.startWaitForResults(*a.state.MsgCid)
	}
}

func (a *fundedAddress) reserve(ctx context.Context, amt abi.TokenAmount) (waitSentinel, error) {
	return a.requestAndWait(ctx, amt, &a.reservations)
}

func (a *fundedAddress) release(ctx context.Context, amt abi.TokenAmount) error {
	_, err := a.requestAndWait(ctx, amt, &a.releases)
	return err
}

func (a *fundedAddress) withdraw(ctx context.Context, amt abi.TokenAmount) (waitSentinel, error) {
	return a.requestAndWait(ctx, amt, &a.withdrawals)
}

func (a *fundedAddress) requestAndWait(ctx context.Context, amt abi.TokenAmount, reqs *[]*fundRequest) (waitSentinel, error) {
	// Create a request and add it to the request queue
	req := newFundRequest(ctx, amt)

	a.lk.Lock()
	*reqs = append(*reqs, req)
	a.lk.Unlock()

	// Process the queue
	go a.process()

	// Wait for the results
	select {
	case <-ctx.Done():
		return waitSentinelUndef, ctx.Err()
	case r := <-req.Result:
		return waitSentinel(r.msgCid), r.err
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
		if done {
			a.onProcessStartListener = nil
		}
	}

	// Check if we're still waiting for the response to a message
	if a.state.MsgCid != nil {
		return
	}

	// Check if there's anything to do
	if len(a.reservations) == 0 && len(a.releases) == 0 && len(a.withdrawals) == 0 {
		return
	}

	res, _ := a.processRequests()

	a.reservations = filterOutProcessedReqs(a.reservations)
	a.releases = filterOutProcessedReqs(a.releases)
	a.withdrawals = filterOutProcessedReqs(a.withdrawals)

	a.applyStateChange(res)
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
func (a *fundedAddress) applyStateChange(res *processResult) {
	a.state.MsgCid = res.msgCid
	a.state.AmtReserved = res.amtReserved
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
	err := a.str.save(a.state)
	if err != nil {
		log.Errorf("saving state to store for addr %s: %w", a.state.Addr, err)
	}
}

// The result of processing the request queues
type processResult struct {
	// The new reserved amount
	amtReserved abi.TokenAmount
	// The message cid, if a message was pushed
	msgCid *cid.Cid
}

// process request queues and return the resulting changes to state
func (a *fundedAddress) processRequests() (pr *processResult, prerr error) {
	// If there's an error, mark reserve requests as errored
	defer func() {
		if prerr != nil {
			for _, req := range a.reservations {
				req.Complete(cid.Undef, prerr)
			}
		}
	}()

	// Start with the reserved amount in state
	reserved := a.state.AmtReserved

	// Add the amount of each reserve request
	for _, req := range a.reservations {
		amt := req.Amount()
		a.debugf("reserve %d", amt)
		reserved = types.BigAdd(reserved, amt)
	}

	// Subtract the amount of each release request
	for _, req := range a.releases {
		amt := req.Amount()
		a.debugf("release %d", amt)
		reserved = types.BigSub(reserved, amt)

		// Mark release as complete
		req.Complete(cid.Undef, nil)
	}

	// If reserved amount is negative, set it to zero
	if reserved.LessThan(abi.NewTokenAmount(0)) {
		reserved = abi.NewTokenAmount(0)
	}

	res := &processResult{amtReserved: reserved}

	// Work out the amount to add to the balance
	toAdd := abi.NewTokenAmount(0)

	// If the new reserved amount is greater than the existing amount
	if reserved.GreaterThan(a.state.AmtReserved) {
		a.debugf("reserved %d > state.AmtReserved %d", reserved, a.state.AmtReserved)

		// Get available funds for address
		avail, err := a.env.AvailableFunds(a.ctx, a.state.Addr)
		if err != nil {
			return res, err
		}

		// amount to add = new reserved amount - available
		toAdd = types.BigSub(reserved, avail)
		a.debugf("reserved %d - avail %d = %d", reserved, avail, toAdd)
	}

	// If there's nothing to add to the balance
	if toAdd.LessThanEqual(abi.NewTokenAmount(0)) {
		// Mark reserve requests as complete
		for _, req := range a.reservations {
			req.Complete(cid.Undef, nil)
		}

		// Process withdrawals
		return a.processWithdrawals(reserved)
	}

	// Add funds to address
	a.debugf("add funds %d", toAdd)
	addFundsCid, err := a.env.AddFunds(a.ctx, a.state.Wallet, a.state.Addr, toAdd)
	if err != nil {
		return res, err
	}

	// Mark reserve requests as complete
	for _, req := range a.reservations {
		req.Complete(addFundsCid, nil)
	}

	// Start waiting for results (async)
	defer a.startWaitForResults(addFundsCid)

	// Save the message CID to state
	res.msgCid = &addFundsCid
	return res, nil
}

// process withdrawal queue
func (a *fundedAddress) processWithdrawals(reserved abi.TokenAmount) (pr *processResult, prerr error) {
	// If there's an error, mark withdrawal requests as errored
	defer func() {
		if prerr != nil {
			for _, req := range a.withdrawals {
				req.Complete(cid.Undef, prerr)
			}
		}
	}()

	res := &processResult{
		amtReserved: reserved,
	}

	// Get the net available balance
	avail, err := a.env.AvailableFunds(a.ctx, a.state.Addr)
	if err != nil {
		return res, err
	}

	netAvail := types.BigSub(avail, reserved)

	// Fit as many withdrawals as possible into the available balance, and fail
	// the rest
	withdrawalAmt := abi.NewTokenAmount(0)
	allowedAmt := abi.NewTokenAmount(0)
	allowed := make([]*fundRequest, 0, len(a.withdrawals))
	for _, req := range a.withdrawals {
		amt := req.Amount()
		withdrawalAmt = types.BigAdd(withdrawalAmt, amt)
		if withdrawalAmt.LessThanEqual(netAvail) {
			a.debugf("withdraw %d", amt)
			allowed = append(allowed, req)
			allowedAmt = types.BigAdd(allowedAmt, amt)
		} else {
			err := xerrors.Errorf("insufficient funds for withdrawal %d", amt)
			a.debugf("%s", err)
			req.Complete(cid.Undef, err)
		}
	}

	// Check if there is anything to withdraw
	if allowedAmt.Equals(abi.NewTokenAmount(0)) {
		// Mark allowed requests as complete
		for _, req := range allowed {
			req.Complete(cid.Undef, nil)
		}
		return res, nil
	}

	// Withdraw funds
	a.debugf("withdraw funds %d", allowedAmt)
	withdrawFundsCid, err := a.env.WithdrawFunds(a.ctx, a.state.Wallet, a.state.Addr, allowedAmt)
	if err != nil {
		return res, err
	}

	// Mark allowed requests as complete
	for _, req := range allowed {
		req.Complete(withdrawFundsCid, nil)
	}

	// Start waiting for results of message (async)
	defer a.startWaitForResults(withdrawFundsCid)

	// Save the message CID to state
	res.msgCid = &withdrawFundsCid
	return res, nil
}

// asynchonously wait for results of message
func (a *fundedAddress) startWaitForResults(msgCid cid.Cid) {
	go func() {
		err := a.env.WaitMsg(a.ctx, msgCid)
		if err != nil {
			// We don't really care about the results here, we're just waiting
			// so as to only process one on-chain message at a time
			log.Errorf("waiting for results of message %s for addr %s: %w", msgCid, a.state.Addr, err)
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
	Result    chan reqResult
}

func newFundRequest(ctx context.Context, amt abi.TokenAmount) *fundRequest {
	return &fundRequest{
		ctx:       ctx,
		amt:       amt,
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

func (frp *fundRequest) Equals(other *fundRequest) bool {
	return frp == other
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
	return env.sendFunds(ctx, wallet, addr, amt)
}

func (env *fundManagerEnvironment) WithdrawFunds(
	ctx context.Context,
	wallet address.Address,
	addr address.Address,
	amt abi.TokenAmount,
) (cid.Cid, error) {
	return env.sendFunds(ctx, addr, wallet, amt)
}

func (env *fundManagerEnvironment) sendFunds(
	ctx context.Context,
	from address.Address,
	to address.Address,
	amt abi.TokenAmount,
) (cid.Cid, error) {
	params, err := actors.SerializeParams(&to)
	if err != nil {
		return cid.Undef, err
	}

	smsg, aerr := env.api.MpoolPushMessage(ctx, &types.Message{
		To:     market.Address,
		From:   from,
		Value:  amt,
		Method: market.Methods.AddBalance,
		Params: params,
	}, nil)

	if aerr != nil {
		return cid.Undef, aerr
	}

	return smsg.Cid(), nil
}

func (env *fundManagerEnvironment) WaitMsg(ctx context.Context, c cid.Cid) error {
	_, err := env.api.StateWaitMsg(ctx, c, build.MessageConfidence)
	return err
}
