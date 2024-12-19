package market

import (
	"bytes"
	"context"
	"sync"
	"testing"
	"time"

	"github.com/ipfs/go-cid"
	ds "github.com/ipfs/go-datastore"
	ds_sync "github.com/ipfs/go-datastore/sync"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	markettypes "github.com/filecoin-project/go-state-types/builtin/v9/market"
	tutils "github.com/filecoin-project/specs-actors/v2/support/testing"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/actors/builtin/market"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/wallet"
)

// TestFundManagerBasic verifies that the basic fund manager operations work
func TestFundManagerBasic(t *testing.T) {
	s := setup(t)
	defer s.fm.Stop()

	// Reserve 10
	// balance:  0 -> 10
	// reserved: 0 -> 10
	amt := abi.NewTokenAmount(10)
	sentinel, err := s.fm.Reserve(s.ctx, s.walletAddr, s.acctAddr, amt)
	require.NoError(t, err)

	msg := s.mockApi.getSentMessage(sentinel)
	checkAddMessageFields(t, msg, s.walletAddr, s.acctAddr, amt)

	s.mockApi.completeMsg(sentinel)

	// Reserve 7
	// balance:  10 -> 17
	// reserved: 10 -> 17
	amt = abi.NewTokenAmount(7)
	sentinel, err = s.fm.Reserve(s.ctx, s.walletAddr, s.acctAddr, amt)
	require.NoError(t, err)

	msg = s.mockApi.getSentMessage(sentinel)
	checkAddMessageFields(t, msg, s.walletAddr, s.acctAddr, amt)

	s.mockApi.completeMsg(sentinel)

	// Release 5
	// balance:  17
	// reserved: 17 -> 12
	amt = abi.NewTokenAmount(5)
	err = s.fm.Release(s.acctAddr, amt)
	require.NoError(t, err)

	// Withdraw 2
	// balance:  17 -> 15
	// reserved: 12
	amt = abi.NewTokenAmount(2)
	sentinel, err = s.fm.Withdraw(s.ctx, s.walletAddr, s.acctAddr, amt)
	require.NoError(t, err)

	msg = s.mockApi.getSentMessage(sentinel)
	checkWithdrawMessageFields(t, msg, s.walletAddr, s.acctAddr, amt)

	s.mockApi.completeMsg(sentinel)

	// Reserve 3
	// balance:  15
	// reserved: 12 -> 15
	// Note: reserved (15) is <= balance (15) so should not send on-chain
	// message
	msgCount := s.mockApi.messageCount()
	amt = abi.NewTokenAmount(3)
	sentinel, err = s.fm.Reserve(s.ctx, s.walletAddr, s.acctAddr, amt)
	require.NoError(t, err)
	require.Equal(t, msgCount, s.mockApi.messageCount())
	require.Equal(t, sentinel, cid.Undef)

	// Reserve 1
	// balance:  15 -> 16
	// reserved: 15 -> 16
	// Note: reserved (16) is above balance (15) so *should* send on-chain
	// message to top up balance
	amt = abi.NewTokenAmount(1)
	topUp := abi.NewTokenAmount(1)
	sentinel, err = s.fm.Reserve(s.ctx, s.walletAddr, s.acctAddr, amt)
	require.NoError(t, err)

	s.mockApi.completeMsg(sentinel)
	msg = s.mockApi.getSentMessage(sentinel)
	checkAddMessageFields(t, msg, s.walletAddr, s.acctAddr, topUp)

	// Withdraw 1
	// balance:  16
	// reserved: 16
	// Note: Expect failure because there is no available balance to withdraw:
	// balance - reserved = 16 - 16 = 0
	amt = abi.NewTokenAmount(1)
	_, err = s.fm.Withdraw(s.ctx, s.walletAddr, s.acctAddr, amt)
	require.Error(t, err)
}

// TestFundManagerParallel verifies that operations can be run in parallel
func TestFundManagerParallel(t *testing.T) {
	s := setup(t)
	defer s.fm.Stop()

	// Reserve 10
	amt := abi.NewTokenAmount(10)
	sentinelReserve10, err := s.fm.Reserve(s.ctx, s.walletAddr, s.acctAddr, amt)
	require.NoError(t, err)

	// Wait until all the subsequent requests are queued up
	queueReady := make(chan struct{})
	fa := s.fm.getFundedAddress(s.acctAddr)
	fa.onProcessStart(func() bool {
		if len(fa.withdrawals) == 1 && len(fa.reservations) == 2 && len(fa.releases) == 1 {
			close(queueReady)
			return true
		}
		return false
	})

	// Withdraw 5 (should not run until after reserves / releases)
	withdrawReady := make(chan error)
	go func() {
		amt = abi.NewTokenAmount(5)
		_, err := s.fm.Withdraw(s.ctx, s.walletAddr, s.acctAddr, amt)
		withdrawReady <- err
	}()

	reserveSentinels := make(chan cid.Cid)

	// Reserve 3
	go func() {
		amt := abi.NewTokenAmount(3)
		sentinelReserve3, err := s.fm.Reserve(s.ctx, s.walletAddr, s.acctAddr, amt)
		require.NoError(t, err)
		reserveSentinels <- sentinelReserve3
	}()

	// Reserve 5
	go func() {
		amt := abi.NewTokenAmount(5)
		sentinelReserve5, err := s.fm.Reserve(s.ctx, s.walletAddr, s.acctAddr, amt)
		require.NoError(t, err)
		reserveSentinels <- sentinelReserve5
	}()

	// Release 2
	go func() {
		amt := abi.NewTokenAmount(2)
		err = s.fm.Release(s.acctAddr, amt)
		require.NoError(t, err)
	}()

	// Everything is queued up
	<-queueReady

	// Complete the "Reserve 10" message
	s.mockApi.completeMsg(sentinelReserve10)
	msg := s.mockApi.getSentMessage(sentinelReserve10)
	checkAddMessageFields(t, msg, s.walletAddr, s.acctAddr, abi.NewTokenAmount(10))

	// The other requests should now be combined and be submitted on-chain as
	// a single message
	rs1 := <-reserveSentinels
	rs2 := <-reserveSentinels
	require.Equal(t, rs1, rs2)

	// Withdraw should not have been called yet, because reserve / release
	// requests run first
	select {
	case <-withdrawReady:
		require.Fail(t, "Withdraw should run after reserve / release")
	default:
	}

	// Complete the message
	s.mockApi.completeMsg(rs1)
	msg = s.mockApi.getSentMessage(rs1)

	// "Reserve 3" +3
	// "Reserve 5" +5
	// "Release 2" -2
	// Result:      6
	checkAddMessageFields(t, msg, s.walletAddr, s.acctAddr, abi.NewTokenAmount(6))

	// Expect withdraw to fail because not enough available funds
	err = <-withdrawReady
	require.Error(t, err)
}

// TestFundManagerReserveByWallet verifies that reserve requests are grouped by wallet
func TestFundManagerReserveByWallet(t *testing.T) {
	s := setup(t)
	defer s.fm.Stop()

	walletAddrA, err := s.wllt.WalletNew(context.Background(), types.KTSecp256k1)
	require.NoError(t, err)
	walletAddrB, err := s.wllt.WalletNew(context.Background(), types.KTSecp256k1)
	require.NoError(t, err)

	// Wait until all the reservation requests are queued up
	walletAQueuedUp := make(chan struct{})
	queueReady := make(chan struct{})
	fa := s.fm.getFundedAddress(s.acctAddr)
	fa.onProcessStart(func() bool {
		if len(fa.reservations) == 1 {
			close(walletAQueuedUp)
		}
		if len(fa.reservations) == 3 {
			close(queueReady)
			return true
		}
		return false
	})

	type reserveResult struct {
		ws  cid.Cid
		err error
	}
	results := make(chan *reserveResult)

	amtA1 := abi.NewTokenAmount(1)
	go func() {
		// Wallet A: Reserve 1
		sentinelA1, err := s.fm.Reserve(s.ctx, walletAddrA, s.acctAddr, amtA1)
		results <- &reserveResult{
			ws:  sentinelA1,
			err: err,
		}
	}()

	amtB1 := abi.NewTokenAmount(2)
	amtB2 := abi.NewTokenAmount(3)
	go func() {
		// Wait for reservation for wallet A to be queued up
		<-walletAQueuedUp

		// Wallet B: Reserve 2
		go func() {
			sentinelB1, err := s.fm.Reserve(s.ctx, walletAddrB, s.acctAddr, amtB1)
			results <- &reserveResult{
				ws:  sentinelB1,
				err: err,
			}
		}()

		// Wallet B: Reserve 3
		sentinelB2, err := s.fm.Reserve(s.ctx, walletAddrB, s.acctAddr, amtB2)
		results <- &reserveResult{
			ws:  sentinelB2,
			err: err,
		}
	}()

	// All reservation requests are queued up
	<-queueReady

	resA := <-results
	sentinelA1 := resA.ws

	// Should send to wallet A
	msg := s.mockApi.getSentMessage(sentinelA1)
	checkAddMessageFields(t, msg, walletAddrA, s.acctAddr, amtA1)

	// Complete wallet A message
	s.mockApi.completeMsg(sentinelA1)

	resB1 := <-results
	resB2 := <-results
	require.NoError(t, resB1.err)
	require.NoError(t, resB2.err)
	sentinelB1 := resB1.ws
	sentinelB2 := resB2.ws

	// Should send different message to wallet B
	require.NotEqual(t, sentinelA1, sentinelB1)
	// Should be single message combining amount 1 and 2
	require.Equal(t, sentinelB1, sentinelB2)
	msg = s.mockApi.getSentMessage(sentinelB1)
	checkAddMessageFields(t, msg, walletAddrB, s.acctAddr, types.BigAdd(amtB1, amtB2))
}

// TestFundManagerWithdrawalLimit verifies that as many withdraw operations as
// possible are processed
func TestFundManagerWithdrawalLimit(t *testing.T) {
	s := setup(t)
	defer s.fm.Stop()

	// Reserve 10
	amt := abi.NewTokenAmount(10)
	sentinelReserve10, err := s.fm.Reserve(s.ctx, s.walletAddr, s.acctAddr, amt)
	require.NoError(t, err)

	// Complete the "Reserve 10" message
	s.mockApi.completeMsg(sentinelReserve10)

	// Release 10
	err = s.fm.Release(s.acctAddr, amt)
	require.NoError(t, err)

	// Queue up withdraw requests
	queueReady := make(chan struct{})
	fa := s.fm.getFundedAddress(s.acctAddr)
	withdrawalReqTotal := 3
	withdrawalReqEnqueued := 0
	withdrawalReqQueue := make(chan func(), withdrawalReqTotal)
	fa.onProcessStart(func() bool {
		// If a new withdrawal request was enqueued
		if len(fa.withdrawals) > withdrawalReqEnqueued {
			withdrawalReqEnqueued++

			// Pop the next request and run it
			select {
			case fn := <-withdrawalReqQueue:
				go fn()
			default:
			}
		}
		// Once all the requests have arrived, we're ready to process the queue
		if withdrawalReqEnqueued == withdrawalReqTotal {
			close(queueReady)
			return true
		}
		return false
	})

	type withdrawResult struct {
		reqIndex int
		ws       cid.Cid
		err      error
	}
	withdrawRes := make(chan *withdrawResult)

	// Queue up three "Withdraw 5" requests
	enqueuedCount := 0
	for i := 0; i < withdrawalReqTotal; i++ {
		withdrawalReqQueue <- func() {
			idx := enqueuedCount
			enqueuedCount++

			amt := abi.NewTokenAmount(5)
			ws, err := s.fm.Withdraw(s.ctx, s.walletAddr, s.acctAddr, amt)
			withdrawRes <- &withdrawResult{reqIndex: idx, ws: ws, err: err}
		}
	}
	// Start the first request
	fn := <-withdrawalReqQueue
	go fn()

	// All withdrawal requests are queued up and ready to be processed
	<-queueReady

	// Organize results in request order
	results := make([]*withdrawResult, withdrawalReqTotal)
	for i := 0; i < 3; i++ {
		res := <-withdrawRes
		results[res.reqIndex] = res
	}

	// Available 10
	// Withdraw 5
	// Expect Success
	require.NoError(t, results[0].err)
	// Available 5
	// Withdraw 5
	// Expect Success
	require.NoError(t, results[1].err)
	// Available 0
	// Withdraw 5
	// Expect FAIL
	require.Error(t, results[2].err)

	// Expect withdrawal requests that fit under reserved amount to be combined
	// into a single message on-chain
	require.Equal(t, results[0].ws, results[1].ws)
}

// TestFundManagerWithdrawByWallet verifies that withdraw requests are grouped by wallet
func TestFundManagerWithdrawByWallet(t *testing.T) {
	s := setup(t)
	defer s.fm.Stop()

	walletAddrA, err := s.wllt.WalletNew(context.Background(), types.KTSecp256k1)
	require.NoError(t, err)
	walletAddrB, err := s.wllt.WalletNew(context.Background(), types.KTSecp256k1)
	require.NoError(t, err)

	// Reserve 10
	reserveAmt := abi.NewTokenAmount(10)
	sentinelReserve, err := s.fm.Reserve(s.ctx, s.walletAddr, s.acctAddr, reserveAmt)
	require.NoError(t, err)
	s.mockApi.completeMsg(sentinelReserve)

	time.Sleep(10 * time.Millisecond)

	// Release 10
	err = s.fm.Release(s.acctAddr, reserveAmt)
	require.NoError(t, err)

	type withdrawResult struct {
		ws  cid.Cid
		err error
	}
	results := make(chan *withdrawResult)

	// Wait until withdrawals are queued up
	walletAQueuedUp := make(chan struct{})
	queueReady := make(chan struct{})
	withdrawalCount := 0
	fa := s.fm.getFundedAddress(s.acctAddr)
	fa.onProcessStart(func() bool {
		if len(fa.withdrawals) == withdrawalCount {
			return false
		}
		withdrawalCount = len(fa.withdrawals)

		if withdrawalCount == 1 {
			close(walletAQueuedUp)
		} else if withdrawalCount == 3 {
			close(queueReady)
			return true
		}
		return false
	})

	amtA1 := abi.NewTokenAmount(1)
	go func() {
		// Wallet A: Withdraw 1
		sentinelA1, err := s.fm.Withdraw(s.ctx, walletAddrA, s.acctAddr, amtA1)
		results <- &withdrawResult{
			ws:  sentinelA1,
			err: err,
		}
	}()

	amtB1 := abi.NewTokenAmount(2)
	amtB2 := abi.NewTokenAmount(3)
	go func() {
		// Wait until withdraw for wallet A is queued up
		<-walletAQueuedUp

		// Wallet B: Withdraw 2
		go func() {
			sentinelB1, err := s.fm.Withdraw(s.ctx, walletAddrB, s.acctAddr, amtB1)
			results <- &withdrawResult{
				ws:  sentinelB1,
				err: err,
			}
		}()

		// Wallet B: Withdraw 3
		sentinelB2, err := s.fm.Withdraw(s.ctx, walletAddrB, s.acctAddr, amtB2)
		results <- &withdrawResult{
			ws:  sentinelB2,
			err: err,
		}
	}()

	// Withdrawals are queued up
	<-queueReady

	// Should withdraw from wallet A first
	resA1 := <-results
	sentinelA1 := resA1.ws
	msg := s.mockApi.getSentMessage(sentinelA1)
	checkWithdrawMessageFields(t, msg, walletAddrA, s.acctAddr, amtA1)

	// Complete wallet A message
	s.mockApi.completeMsg(sentinelA1)

	resB1 := <-results
	resB2 := <-results
	require.NoError(t, resB1.err)
	require.NoError(t, resB2.err)
	sentinelB1 := resB1.ws
	sentinelB2 := resB2.ws

	// Should send different message for wallet B from wallet A
	require.NotEqual(t, sentinelA1, sentinelB1)
	// Should be single message combining amount 1 and 2
	require.Equal(t, sentinelB1, sentinelB2)
	msg = s.mockApi.getSentMessage(sentinelB1)
	checkWithdrawMessageFields(t, msg, walletAddrB, s.acctAddr, types.BigAdd(amtB1, amtB2))
}

// TestFundManagerRestart verifies that waiting for incomplete requests resumes
// on restart
func TestFundManagerRestart(t *testing.T) {
	s := setup(t)
	defer s.fm.Stop()

	acctAddr2 := tutils.NewActorAddr(t, "addr2")

	// Address 1: Reserve 10
	amt := abi.NewTokenAmount(10)
	sentinelAddr1, err := s.fm.Reserve(s.ctx, s.walletAddr, s.acctAddr, amt)
	require.NoError(t, err)

	msg := s.mockApi.getSentMessage(sentinelAddr1)
	checkAddMessageFields(t, msg, s.walletAddr, s.acctAddr, amt)

	// Address 2: Reserve 7
	amt2 := abi.NewTokenAmount(7)
	sentinelAddr2Res7, err := s.fm.Reserve(s.ctx, s.walletAddr, acctAddr2, amt2)
	require.NoError(t, err)

	msg2 := s.mockApi.getSentMessage(sentinelAddr2Res7)
	checkAddMessageFields(t, msg2, s.walletAddr, acctAddr2, amt2)

	// Complete "Address 1: Reserve 10"
	s.mockApi.completeMsg(sentinelAddr1)

	// Give the completed state a moment to be stored before restart
	time.Sleep(time.Millisecond * 10)

	// Restart
	mockApiAfter := s.mockApi
	fmAfter := newFundManager(mockApiAfter, s.ds)
	err = fmAfter.Start()
	require.NoError(t, err)

	amt3 := abi.NewTokenAmount(9)
	reserveSentinel := make(chan cid.Cid)
	go func() {
		// Address 2: Reserve 9
		sentinel3, err := fmAfter.Reserve(s.ctx, s.walletAddr, acctAddr2, amt3)
		require.NoError(t, err)
		reserveSentinel <- sentinel3
	}()

	// Expect no message to be sent, because still waiting for previous
	// message "Address 2: Reserve 7" to complete on-chain
	select {
	case <-reserveSentinel:
		require.Fail(t, "Expected no message to be sent")
	case <-time.After(10 * time.Millisecond):
	}

	// Complete "Address 2: Reserve 7"
	mockApiAfter.completeMsg(sentinelAddr2Res7)

	// Expect waiting message to now be sent
	sentinel3 := <-reserveSentinel
	msg3 := mockApiAfter.getSentMessage(sentinel3)
	checkAddMessageFields(t, msg3, s.walletAddr, acctAddr2, amt3)
}

// TestFundManagerReleaseAfterPublish verifies that release is successful in
// the following scenario:
// 1. Deal A adds 5 to addr1:                     reserved  0 ->  5    available  0 ->  5
// 2. Deal B adds 7 to addr1:                     reserved  5 -> 12    available  5 -> 12
// 3. Deal B completes, reducing addr1 by 7:      reserved       12    available 12 ->  5
// 4. Deal A releases 5 from addr1:               reserved 12 ->  7    available        5
func TestFundManagerReleaseAfterPublish(t *testing.T) {
	s := setup(t)
	defer s.fm.Stop()

	// Deal A: Reserve 5
	// balance:  0 -> 5
	// reserved: 0 -> 5
	amt := abi.NewTokenAmount(5)
	sentinel, err := s.fm.Reserve(s.ctx, s.walletAddr, s.acctAddr, amt)
	require.NoError(t, err)
	s.mockApi.completeMsg(sentinel)

	// Deal B: Reserve 7
	// balance:  5 -> 12
	// reserved: 5 -> 12
	amt = abi.NewTokenAmount(7)
	sentinel, err = s.fm.Reserve(s.ctx, s.walletAddr, s.acctAddr, amt)
	require.NoError(t, err)
	s.mockApi.completeMsg(sentinel)

	// Deal B: Publish (removes Deal B amount from balance)
	// balance:  12 -> 5
	// reserved: 12
	amt = abi.NewTokenAmount(7)
	s.mockApi.publish(s.acctAddr, amt)

	// Deal A: Release 5
	// balance:  5
	// reserved: 12 -> 7
	amt = abi.NewTokenAmount(5)
	err = s.fm.Release(s.acctAddr, amt)
	require.NoError(t, err)

	// Deal B: Release 7
	// balance:  5
	// reserved: 12 -> 7
	amt = abi.NewTokenAmount(5)
	err = s.fm.Release(s.acctAddr, amt)
	require.NoError(t, err)
}

type scaffold struct {
	ctx        context.Context
	ds         *ds_sync.MutexDatastore
	wllt       *wallet.LocalWallet
	walletAddr address.Address
	acctAddr   address.Address
	mockApi    *mockFundManagerAPI
	fm         *FundManager
}

func setup(t *testing.T) *scaffold {
	ctx := context.Background()

	wllt, err := wallet.NewWallet(wallet.NewMemKeyStore())
	if err != nil {
		t.Fatal(err)
	}

	walletAddr, err := wllt.WalletNew(context.Background(), types.KTSecp256k1)
	if err != nil {
		t.Fatal(err)
	}

	acctAddr := tutils.NewActorAddr(t, "addr")

	mockApi := newMockFundManagerAPI(walletAddr)
	dstore := ds_sync.MutexWrap(ds.NewMapDatastore())
	fm := newFundManager(mockApi, dstore)
	return &scaffold{
		ctx:        ctx,
		ds:         dstore,
		wllt:       wllt,
		walletAddr: walletAddr,
		acctAddr:   acctAddr,
		mockApi:    mockApi,
		fm:         fm,
	}
}

func checkAddMessageFields(t *testing.T, msg *types.Message, from address.Address, to address.Address, amt abi.TokenAmount) {
	require.Equal(t, from, msg.From)
	require.Equal(t, market.Address, msg.To)
	require.Equal(t, amt, msg.Value)

	var paramsTo address.Address
	err := paramsTo.UnmarshalCBOR(bytes.NewReader(msg.Params))
	require.NoError(t, err)
	require.Equal(t, to, paramsTo)
}

func checkWithdrawMessageFields(t *testing.T, msg *types.Message, from address.Address, addr address.Address, amt abi.TokenAmount) {
	require.Equal(t, from, msg.From)
	require.Equal(t, market.Address, msg.To)
	require.Equal(t, abi.NewTokenAmount(0), msg.Value)

	var params markettypes.WithdrawBalanceParams
	err := params.UnmarshalCBOR(bytes.NewReader(msg.Params))
	require.NoError(t, err)
	require.Equal(t, addr, params.ProviderOrClientAddress)
	require.Equal(t, amt, params.Amount)
}

type sentMsg struct {
	msg   *types.SignedMessage
	ready chan struct{}
}

type mockFundManagerAPI struct {
	wallet address.Address

	lk            sync.Mutex
	escrow        map[address.Address]abi.TokenAmount
	sentMsgs      map[cid.Cid]*sentMsg
	completedMsgs map[cid.Cid]struct{}
	waitingFor    map[cid.Cid]chan struct{}
}

func newMockFundManagerAPI(wallet address.Address) *mockFundManagerAPI {
	return &mockFundManagerAPI{
		wallet:        wallet,
		escrow:        make(map[address.Address]abi.TokenAmount),
		sentMsgs:      make(map[cid.Cid]*sentMsg),
		completedMsgs: make(map[cid.Cid]struct{}),
		waitingFor:    make(map[cid.Cid]chan struct{}),
	}
}

func (mapi *mockFundManagerAPI) MpoolPushMessage(ctx context.Context, message *types.Message, spec *api.MessageSendSpec) (*types.SignedMessage, error) {
	mapi.lk.Lock()
	defer mapi.lk.Unlock()

	smsg := &types.SignedMessage{Message: *message}
	mapi.sentMsgs[smsg.Cid()] = &sentMsg{msg: smsg, ready: make(chan struct{})}

	return smsg, nil
}

func (mapi *mockFundManagerAPI) getSentMessage(c cid.Cid) *types.Message {
	mapi.lk.Lock()
	defer mapi.lk.Unlock()

	for i := 0; i < 1000; i++ {
		if pending, ok := mapi.sentMsgs[c]; ok {
			return &pending.msg.Message
		}
		time.Sleep(time.Millisecond)
	}
	panic("expected message to be sent")
}

func (mapi *mockFundManagerAPI) messageCount() int {
	mapi.lk.Lock()
	defer mapi.lk.Unlock()

	return len(mapi.sentMsgs)
}

func (mapi *mockFundManagerAPI) completeMsg(msgCid cid.Cid) {
	mapi.lk.Lock()

	pmsg, ok := mapi.sentMsgs[msgCid]
	if ok {
		if pmsg.msg.Message.Method == market.Methods.AddBalance {
			var escrowAcct address.Address
			err := escrowAcct.UnmarshalCBOR(bytes.NewReader(pmsg.msg.Message.Params))
			if err != nil {
				panic(err)
			}

			escrow := mapi.getEscrow(escrowAcct)
			before := escrow
			escrow = types.BigAdd(escrow, pmsg.msg.Message.Value)
			mapi.escrow[escrowAcct] = escrow
			log.Debugf("%s:   escrow %d -> %d", escrowAcct, before, escrow)
		} else {
			var params markettypes.WithdrawBalanceParams
			err := params.UnmarshalCBOR(bytes.NewReader(pmsg.msg.Message.Params))
			if err != nil {
				panic(err)
			}
			escrowAcct := params.ProviderOrClientAddress

			escrow := mapi.getEscrow(escrowAcct)
			before := escrow
			escrow = types.BigSub(escrow, params.Amount)
			mapi.escrow[escrowAcct] = escrow
			log.Debugf("%s:   escrow %d -> %d", escrowAcct, before, escrow)
		}
	}

	mapi.completedMsgs[msgCid] = struct{}{}

	ready, ok := mapi.waitingFor[msgCid]

	mapi.lk.Unlock()

	if ok {
		close(ready)
	}
}

func (mapi *mockFundManagerAPI) StateMarketBalance(ctx context.Context, a address.Address, key types.TipSetKey) (api.MarketBalance, error) {
	mapi.lk.Lock()
	defer mapi.lk.Unlock()

	return api.MarketBalance{
		Locked: abi.NewTokenAmount(0),
		Escrow: mapi.getEscrow(a),
	}, nil
}

func (mapi *mockFundManagerAPI) getEscrow(a address.Address) abi.TokenAmount {
	escrow := mapi.escrow[a]
	if escrow.Nil() {
		return abi.NewTokenAmount(0)
	}
	return escrow
}

func (mapi *mockFundManagerAPI) publish(addr address.Address, amt abi.TokenAmount) {
	mapi.lk.Lock()
	defer mapi.lk.Unlock()

	escrow := mapi.escrow[addr]
	if escrow.Nil() {
		return
	}
	escrow = types.BigSub(escrow, amt)
	if escrow.LessThan(abi.NewTokenAmount(0)) {
		escrow = abi.NewTokenAmount(0)
	}
	mapi.escrow[addr] = escrow
}

func (mapi *mockFundManagerAPI) StateWaitMsg(ctx context.Context, c cid.Cid, confidence uint64, limit abi.ChainEpoch, allowReplaced bool) (*api.MsgLookup, error) {
	res := &api.MsgLookup{
		Message: c,
		Receipt: types.MessageReceipt{
			ExitCode: 0,
			Return:   nil,
		},
	}
	ready := make(chan struct{})

	mapi.lk.Lock()
	_, ok := mapi.completedMsgs[c]
	if !ok {
		mapi.waitingFor[c] = ready
	}
	mapi.lk.Unlock()

	if !ok {
		select {
		case <-ctx.Done():
		case <-ready:
		}
	}
	return res, nil
}
