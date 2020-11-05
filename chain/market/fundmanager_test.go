package market

import (
	"bytes"
	"context"
	"sync"
	"testing"
	"time"

	"github.com/filecoin-project/lotus/chain/actors/builtin/market"

	"github.com/filecoin-project/go-state-types/abi"

	tutils "github.com/filecoin-project/specs-actors/v2/support/testing"

	"github.com/filecoin-project/lotus/chain/wallet"

	ds "github.com/ipfs/go-datastore"
	ds_sync "github.com/ipfs/go-datastore/sync"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/ipfs/go-cid"
)

// TestFundManagerBasic verifies that the basic fund manager operations work
func TestFundManagerBasic(t *testing.T) {
	s := setup(t)
	defer s.fm.Stop()

	// Reserve 10
	// balance:  0 -> 10
	// reserved: 0 -> 10
	amt := abi.NewTokenAmount(10)
	sentinel, err := s.fm.Reserve(s.ctx, s.acctAddr, amt)
	require.NoError(t, err)

	msg := s.mockApi.getSentMessage(cid.Cid(sentinel))
	checkMessageFields(t, msg, s.walletAddr, s.acctAddr, amt)

	s.mockApi.completeMsg(cid.Cid(sentinel))
	err = s.fm.Wait(s.ctx, sentinel)
	require.NoError(t, err)

	// Reserve 7
	// balance:  10 -> 17
	// reserved: 10 -> 17
	amt = abi.NewTokenAmount(7)
	sentinel, err = s.fm.Reserve(s.ctx, s.acctAddr, amt)
	require.NoError(t, err)

	msg = s.mockApi.getSentMessage(cid.Cid(sentinel))
	checkMessageFields(t, msg, s.walletAddr, s.acctAddr, amt)

	s.mockApi.completeMsg(cid.Cid(sentinel))
	err = s.fm.Wait(s.ctx, sentinel)
	require.NoError(t, err)

	// Release 5
	// balance:  17
	// reserved: 17 -> 12
	amt = abi.NewTokenAmount(5)
	err = s.fm.Release(s.ctx, s.acctAddr, amt)
	require.NoError(t, err)

	// Withdraw 2
	// balance:  17 -> 15
	// reserved: 12
	amt = abi.NewTokenAmount(2)
	sentinel, err = s.fm.Withdraw(s.ctx, s.acctAddr, amt)
	require.NoError(t, err)

	msg = s.mockApi.getSentMessage(cid.Cid(sentinel))
	checkMessageFields(t, msg, s.acctAddr, s.walletAddr, amt)

	s.mockApi.completeMsg(cid.Cid(sentinel))
	err = s.fm.Wait(s.ctx, sentinel)
	require.NoError(t, err)

	// Reserve 3
	// balance:  15
	// reserved: 12 -> 15
	// Note: reserved (15) is <= balance (15) so should not send on-chain
	// message
	msgCount := s.mockApi.messageCount()
	amt = abi.NewTokenAmount(3)
	sentinel, err = s.fm.Reserve(s.ctx, s.acctAddr, amt)
	require.NoError(t, err)
	require.Equal(t, msgCount, s.mockApi.messageCount())
	require.Equal(t, sentinel, waitSentinelUndef)

	// Reserve 1
	// balance:  15 -> 16
	// reserved: 15 -> 16
	// Note: reserved (16) is above balance (15) so *should* send on-chain
	// message to top up balance
	amt = abi.NewTokenAmount(1)
	topUp := abi.NewTokenAmount(1)
	sentinel, err = s.fm.Reserve(s.ctx, s.acctAddr, amt)
	require.NoError(t, err)

	s.mockApi.completeMsg(cid.Cid(sentinel))
	msg = s.mockApi.getSentMessage(cid.Cid(sentinel))
	checkMessageFields(t, msg, s.walletAddr, s.acctAddr, topUp)

	// Withdraw 1
	// balance:  16
	// reserved: 16
	// Note: Expect failure because there is no available balance to withdraw:
	// balance - reserved = 16 - 16 = 0
	amt = abi.NewTokenAmount(1)
	sentinel, err = s.fm.Withdraw(s.ctx, s.acctAddr, amt)
	require.Error(t, err)
}

// TestFundManagerParallel verifies that operations can be run in parallel
func TestFundManagerParallel(t *testing.T) {
	s := setup(t)
	defer s.fm.Stop()

	// Reserve 10
	amt := abi.NewTokenAmount(10)
	sentinelReserve10, err := s.fm.Reserve(s.ctx, s.acctAddr, amt)
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
		_, err := s.fm.Withdraw(s.ctx, s.acctAddr, amt)
		withdrawReady <- err
	}()

	reserveSentinels := make(chan waitSentinel)

	// Reserve 3
	go func() {
		amt := abi.NewTokenAmount(3)
		sentinelReserve3, err := s.fm.Reserve(s.ctx, s.acctAddr, amt)
		require.NoError(t, err)
		reserveSentinels <- sentinelReserve3
	}()

	// Reserve 5
	go func() {
		amt := abi.NewTokenAmount(5)
		sentinelReserve5, err := s.fm.Reserve(s.ctx, s.acctAddr, amt)
		require.NoError(t, err)
		reserveSentinels <- sentinelReserve5
	}()

	// Release 2
	go func() {
		amt := abi.NewTokenAmount(2)
		err = s.fm.Release(s.ctx, s.acctAddr, amt)
		require.NoError(t, err)
	}()

	// Everything is queued up
	<-queueReady

	// Complete the "Reserve 10" message
	s.mockApi.completeMsg(cid.Cid(sentinelReserve10))
	msg := s.mockApi.getSentMessage(cid.Cid(sentinelReserve10))
	checkMessageFields(t, msg, s.walletAddr, s.acctAddr, abi.NewTokenAmount(10))

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
	s.mockApi.completeMsg(cid.Cid(rs1))
	msg = s.mockApi.getSentMessage(cid.Cid(rs1))

	// "Reserve 3" +3
	// "Reserve 5" +5
	// "Release 2" -2
	// Result:      6
	checkMessageFields(t, msg, s.walletAddr, s.acctAddr, abi.NewTokenAmount(6))

	// Expect withdraw to fail because not enough available funds
	err = <-withdrawReady
	require.Error(t, err)
}

// TestFundManagerWithdrawal verifies that as many withdraw operations as
// possible are processed
func TestFundManagerWithdrawal(t *testing.T) {
	s := setup(t)
	defer s.fm.Stop()

	// Reserve 10
	amt := abi.NewTokenAmount(10)
	sentinelReserve10, err := s.fm.Reserve(s.ctx, s.acctAddr, amt)
	require.NoError(t, err)

	// Complete the "Reserve 10" message
	s.mockApi.completeMsg(cid.Cid(sentinelReserve10))

	// Release 10
	err = s.fm.Release(s.ctx, s.acctAddr, amt)
	require.NoError(t, err)

	// Available 10
	// Withdraw 6
	// Expect success
	amt = abi.NewTokenAmount(6)
	sentinelWithdraw, err := s.fm.Withdraw(s.ctx, s.acctAddr, amt)
	require.NoError(t, err)

	s.mockApi.completeMsg(cid.Cid(sentinelWithdraw))
	err = s.fm.Wait(s.ctx, sentinelWithdraw)
	require.NoError(t, err)

	// Available 4
	// Withdraw 4
	// Expect success
	amt = abi.NewTokenAmount(4)
	sentinelWithdraw, err = s.fm.Withdraw(s.ctx, s.acctAddr, amt)
	require.NoError(t, err)

	s.mockApi.completeMsg(cid.Cid(sentinelWithdraw))
	err = s.fm.Wait(s.ctx, sentinelWithdraw)
	require.NoError(t, err)

	// Available 0
	// Withdraw 1
	// Expect FAIL
	amt = abi.NewTokenAmount(1)
	sentinelWithdraw, err = s.fm.Withdraw(s.ctx, s.acctAddr, amt)
	require.Error(t, err)
}

// TestFundManagerRestart verifies that waiting for incomplete requests resumes
// on restart
func TestFundManagerRestart(t *testing.T) {
	s := setup(t)
	defer s.fm.Stop()

	acctAddr2 := tutils.NewActorAddr(t, "addr2")

	// Address 1: Reserve 10
	amt := abi.NewTokenAmount(10)
	sentinelAddr1, err := s.fm.Reserve(s.ctx, s.acctAddr, amt)
	require.NoError(t, err)

	msg := s.mockApi.getSentMessage(cid.Cid(sentinelAddr1))
	checkMessageFields(t, msg, s.walletAddr, s.acctAddr, amt)

	// Address 2: Reserve 7
	amt2 := abi.NewTokenAmount(7)
	sentinelAddr2Res7, err := s.fm.Reserve(s.ctx, acctAddr2, amt2)
	require.NoError(t, err)

	msg2 := s.mockApi.getSentMessage(cid.Cid(sentinelAddr2Res7))
	checkMessageFields(t, msg2, s.walletAddr, acctAddr2, amt2)

	// Complete "Address 1: Reserve 10"
	s.mockApi.completeMsg(cid.Cid(sentinelAddr1))
	err = s.fm.Wait(s.ctx, sentinelAddr1)
	require.NoError(t, err)

	// Give the completed state a moment to be stored before restart
	time.Sleep(time.Millisecond * 10)

	// Restart
	mockApiAfter := s.mockApi
	fmAfter := NewFundManager(mockApiAfter, s.ds, s.walletAddr)
	fmAfter.Start()

	amt3 := abi.NewTokenAmount(9)
	reserveSentinel := make(chan waitSentinel)
	go func() {
		// Address 2: Reserve 9
		sentinel3, err := fmAfter.Reserve(s.ctx, acctAddr2, amt3)
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
	mockApiAfter.completeMsg(cid.Cid(sentinelAddr2Res7))
	err = fmAfter.Wait(s.ctx, sentinelAddr2Res7)
	require.NoError(t, err)

	// Expect waiting message to now be sent
	sentinel3 := <-reserveSentinel
	msg3 := mockApiAfter.getSentMessage(cid.Cid(sentinel3))
	checkMessageFields(t, msg3, s.walletAddr, acctAddr2, amt3)
}

type scaffold struct {
	ctx        context.Context
	ds         *ds_sync.MutexDatastore
	walletAddr address.Address
	acctAddr   address.Address
	mockApi    *mockFundManagerAPI
	fm         *FundManager
}

func setup(t *testing.T) *scaffold {
	ctx := context.Background()

	w, err := wallet.NewWallet(wallet.NewMemKeyStore())
	if err != nil {
		t.Fatal(err)
	}

	walletAddr, err := w.WalletNew(context.Background(), types.KTSecp256k1)
	if err != nil {
		t.Fatal(err)
	}

	acctAddr := tutils.NewActorAddr(t, "addr")

	mockApi := newMockFundManagerAPI(walletAddr)
	ds := ds_sync.MutexWrap(ds.NewMapDatastore())
	fm := NewFundManager(mockApi, ds, walletAddr)
	return &scaffold{
		ctx:        ctx,
		ds:         ds,
		walletAddr: walletAddr,
		acctAddr:   acctAddr,
		mockApi:    mockApi,
		fm:         fm,
	}
}

func checkMessageFields(t *testing.T, msg *types.Message, from address.Address, to address.Address, amt abi.TokenAmount) {
	require.Equal(t, from, msg.From)
	require.Equal(t, market.Address, msg.To)
	require.Equal(t, amt, msg.Value)

	var paramsTo address.Address
	err := paramsTo.UnmarshalCBOR(bytes.NewReader(msg.Params))
	require.NoError(t, err)
	require.Equal(t, to, paramsTo)
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
		if pmsg.msg.Message.From == mapi.wallet {
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
			escrowAcct := pmsg.msg.Message.From
			escrow := mapi.getEscrow(escrowAcct)
			before := escrow
			escrow = types.BigSub(escrow, pmsg.msg.Message.Value)
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

func (mapi *mockFundManagerAPI) StateWaitMsg(ctx context.Context, c cid.Cid, confidence uint64) (*api.MsgLookup, error) {
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
