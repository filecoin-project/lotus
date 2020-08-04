package paychmgr

import (
	"context"
	"sync"
	"testing"
	"time"

	cborrpc "github.com/filecoin-project/go-cbor-util"

	init_ "github.com/filecoin-project/specs-actors/actors/builtin/init"

	"github.com/filecoin-project/specs-actors/actors/builtin"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/types"

	"github.com/filecoin-project/go-address"

	"github.com/filecoin-project/specs-actors/actors/abi/big"
	tutils "github.com/filecoin-project/specs-actors/support/testing"
	"github.com/ipfs/go-cid"
	ds "github.com/ipfs/go-datastore"
	ds_sync "github.com/ipfs/go-datastore/sync"

	"github.com/stretchr/testify/require"
)

type waitingCall struct {
	response chan types.MessageReceipt
}

type mockPaychAPI struct {
	lk           sync.Mutex
	messages     map[cid.Cid]*types.SignedMessage
	waitingCalls []*waitingCall
}

func newMockPaychAPI() *mockPaychAPI {
	return &mockPaychAPI{
		messages: make(map[cid.Cid]*types.SignedMessage),
	}
}

func (pchapi *mockPaychAPI) StateWaitMsg(ctx context.Context, msg cid.Cid, confidence uint64) (*api.MsgLookup, error) {
	response := make(chan types.MessageReceipt)

	pchapi.lk.Lock()
	pchapi.waitingCalls = append(pchapi.waitingCalls, &waitingCall{response: response})
	pchapi.lk.Unlock()

	receipt := <-response

	return &api.MsgLookup{Receipt: receipt}, nil
}

func (pchapi *mockPaychAPI) MpoolPushMessage(ctx context.Context, msg *types.Message) (*types.SignedMessage, error) {
	pchapi.lk.Lock()
	defer pchapi.lk.Unlock()

	smsg := &types.SignedMessage{Message: *msg}
	pchapi.messages[smsg.Cid()] = smsg
	return smsg, nil
}

func (pchapi *mockPaychAPI) pushedMessages(c cid.Cid) *types.SignedMessage {
	pchapi.lk.Lock()
	defer pchapi.lk.Unlock()

	return pchapi.messages[c]
}

func (pchapi *mockPaychAPI) pushedMessageCount() int {
	pchapi.lk.Lock()
	defer pchapi.lk.Unlock()

	return len(pchapi.messages)
}

func (pchapi *mockPaychAPI) finishWaitingCalls(receipt types.MessageReceipt) {
	pchapi.lk.Lock()
	defer pchapi.lk.Unlock()

	for _, call := range pchapi.waitingCalls {
		call.response <- receipt
	}
	pchapi.waitingCalls = nil
}

func (pchapi *mockPaychAPI) close() {
	pchapi.finishWaitingCalls(types.MessageReceipt{
		ExitCode: 0,
		Return:   []byte{},
	})
}

func testChannelResponse(t *testing.T, ch address.Address) types.MessageReceipt {
	createChannelRet := init_.ExecReturn{
		IDAddress:     ch,
		RobustAddress: ch,
	}
	createChannelRetBytes, err := cborrpc.Dump(&createChannelRet)
	require.NoError(t, err)
	createChannelResponse := types.MessageReceipt{
		ExitCode: 0,
		Return:   createChannelRetBytes,
	}
	return createChannelResponse
}

// TestPaychGetCreateChannelMsg tests that GetPaych sends a message to create
// a new channel with the correct funds
func TestPaychGetCreateChannelMsg(t *testing.T) {
	ctx := context.Background()
	store := NewStore(ds_sync.MutexWrap(ds.NewMapDatastore()))

	from := tutils.NewIDAddr(t, 101)
	to := tutils.NewIDAddr(t, 102)

	sm := newMockStateManager()
	pchapi := newMockPaychAPI()
	defer pchapi.close()

	mgr, err := newManager(sm, store, pchapi)
	require.NoError(t, err)

	amt := big.NewInt(10)
	ch, mcid, err := mgr.GetPaych(ctx, from, to, amt)
	require.NoError(t, err)
	require.Equal(t, address.Undef, ch)

	pushedMsg := pchapi.pushedMessages(mcid)
	require.Equal(t, from, pushedMsg.Message.From)
	require.Equal(t, builtin.InitActorAddr, pushedMsg.Message.To)
	require.Equal(t, amt, pushedMsg.Message.Value)
}

// TestPaychGetCreateChannelThenAddFunds tests creating a channel and then
// adding funds to it
func TestPaychGetCreateChannelThenAddFunds(t *testing.T) {
	ctx := context.Background()
	store := NewStore(ds_sync.MutexWrap(ds.NewMapDatastore()))

	ch := tutils.NewIDAddr(t, 100)
	from := tutils.NewIDAddr(t, 101)
	to := tutils.NewIDAddr(t, 102)

	sm := newMockStateManager()
	pchapi := newMockPaychAPI()
	defer pchapi.close()

	mgr, err := newManager(sm, store, pchapi)
	require.NoError(t, err)

	// Send create message for a channel with value 10
	amt := big.NewInt(10)
	_, createMsgCid, err := mgr.GetPaych(ctx, from, to, amt)
	require.NoError(t, err)

	// Should have no channels yet (message sent but channel not created)
	cis, err := mgr.ListChannels()
	require.NoError(t, err)
	require.Len(t, cis, 0)

	// 1. Set up create channel response (sent in response to WaitForMsg())
	response := testChannelResponse(t, ch)

	done := make(chan struct{})
	go func() {
		defer close(done)

		// 2. Request add funds - should block until create channel has completed
		amt2 := big.NewInt(5)
		ch2, addFundsMsgCid, err := mgr.GetPaych(ctx, from, to, amt2)

		// 4. This GetPaych should return after create channel from first
		//    GetPaych completes
		require.NoError(t, err)

		// Expect the channel to be the same
		require.Equal(t, ch, ch2)
		// Expect add funds message CID to be different to create message cid
		require.NotEqual(t, createMsgCid, addFundsMsgCid)

		// Should have one channel, whose address is the channel that was created
		cis, err := mgr.ListChannels()
		require.NoError(t, err)
		require.Len(t, cis, 1)
		require.Equal(t, ch, cis[0])

		// Amount should be amount sent to first GetPaych (to create
		// channel).
		// PendingAmount should be amount sent in second GetPaych
		// (second GetPaych triggered add funds, which has not yet been confirmed)
		ci, err := mgr.GetChannelInfo(ch)
		require.NoError(t, err)
		require.EqualValues(t, 10, ci.Amount.Int64())
		require.EqualValues(t, 5, ci.PendingAmount.Int64())
		require.Nil(t, ci.CreateMsg)

		// Trigger add funds confirmation
		pchapi.finishWaitingCalls(types.MessageReceipt{ExitCode: 0})

		time.Sleep(time.Millisecond * 10)

		// Should still have one channel
		cis, err = mgr.ListChannels()
		require.NoError(t, err)
		require.Len(t, cis, 1)
		require.Equal(t, ch, cis[0])

		// Channel amount should include last amount sent to GetPaych
		ci, err = mgr.GetChannelInfo(ch)
		require.NoError(t, err)
		require.EqualValues(t, 15, ci.Amount.Int64())
		require.EqualValues(t, 0, ci.PendingAmount.Int64())
		require.Nil(t, ci.AddFundsMsg)
	}()

	// Give the go routine above a moment to run
	time.Sleep(time.Millisecond * 10)

	// 3. Send create channel response
	pchapi.finishWaitingCalls(response)

	<-done
}

// TestPaychGetCreateChannelWithErrorThenCreateAgain tests that if an
// operation is queued up behind a create channel operation, and the create
// channel fails, then the waiting operation can succeed.
func TestPaychGetCreateChannelWithErrorThenCreateAgain(t *testing.T) {
	ctx := context.Background()
	store := NewStore(ds_sync.MutexWrap(ds.NewMapDatastore()))

	from := tutils.NewIDAddr(t, 101)
	to := tutils.NewIDAddr(t, 102)

	sm := newMockStateManager()
	pchapi := newMockPaychAPI()
	defer pchapi.close()

	mgr, err := newManager(sm, store, pchapi)
	require.NoError(t, err)

	// Send create message for a channel
	amt := big.NewInt(10)
	_, _, err = mgr.GetPaych(ctx, from, to, amt)
	require.NoError(t, err)

	// 1. Set up create channel response (sent in response to WaitForMsg())
	//    This response indicates an error.
	errResponse := types.MessageReceipt{
		ExitCode: 1, // error
		Return:   []byte{},
	}

	done := make(chan struct{})
	go func() {
		defer close(done)

		// 2. Should block until create channel has completed.
		//    Because first channel create fails, this request
		//    should be for channel create.
		amt2 := big.NewInt(5)
		ch2, _, err := mgr.GetPaych(ctx, from, to, amt2)
		require.NoError(t, err)
		require.Equal(t, address.Undef, ch2)

		time.Sleep(time.Millisecond * 10)

		// 4. Send a success response
		ch := tutils.NewIDAddr(t, 100)
		successResponse := testChannelResponse(t, ch)
		pchapi.finishWaitingCalls(successResponse)

		time.Sleep(time.Millisecond * 10)

		// Should have one channel, whose address is the channel that was created
		cis, err := mgr.ListChannels()
		require.NoError(t, err)
		require.Len(t, cis, 1)
		require.Equal(t, ch, cis[0])

		ci, err := mgr.GetChannelInfo(ch)
		require.NoError(t, err)
		require.Equal(t, amt2, ci.Amount)
	}()

	// Give the go routine above a moment to run
	time.Sleep(time.Millisecond * 10)

	// 3. Send error response to first channel create
	pchapi.finishWaitingCalls(errResponse)

	<-done
}

// TestPaychGetRecoverAfterError tests that after a create channel fails, the
// next attempt to create channel can succeed.
func TestPaychGetRecoverAfterError(t *testing.T) {
	ctx := context.Background()
	store := NewStore(ds_sync.MutexWrap(ds.NewMapDatastore()))

	ch := tutils.NewIDAddr(t, 100)
	from := tutils.NewIDAddr(t, 101)
	to := tutils.NewIDAddr(t, 102)

	sm := newMockStateManager()
	pchapi := newMockPaychAPI()
	defer pchapi.close()

	mgr, err := newManager(sm, store, pchapi)
	require.NoError(t, err)

	// Send create message for a channel
	amt := big.NewInt(10)
	_, _, err = mgr.GetPaych(ctx, from, to, amt)
	require.NoError(t, err)

	time.Sleep(time.Millisecond * 10)

	// Send error create channel response
	pchapi.finishWaitingCalls(types.MessageReceipt{
		ExitCode: 1, // error
		Return:   []byte{},
	})

	time.Sleep(time.Millisecond * 10)

	// Send create message for a channel again
	amt2 := big.NewInt(7)
	_, _, err = mgr.GetPaych(ctx, from, to, amt2)
	require.NoError(t, err)

	time.Sleep(time.Millisecond * 10)

	// Send success create channel response
	response := testChannelResponse(t, ch)
	pchapi.finishWaitingCalls(response)

	time.Sleep(time.Millisecond * 10)

	// Should have one channel, whose address is the channel that was created
	cis, err := mgr.ListChannels()
	require.NoError(t, err)
	require.Len(t, cis, 1)
	require.Equal(t, ch, cis[0])

	ci, err := mgr.GetChannelInfo(ch)
	require.NoError(t, err)
	require.Equal(t, amt2, ci.Amount)
	require.EqualValues(t, 0, ci.PendingAmount.Int64())
	require.Nil(t, ci.CreateMsg)
}

// TestPaychGetRecoverAfterAddFundsError tests that after an add funds fails, the
// next attempt to add funds can succeed.
func TestPaychGetRecoverAfterAddFundsError(t *testing.T) {
	ctx := context.Background()
	store := NewStore(ds_sync.MutexWrap(ds.NewMapDatastore()))

	ch := tutils.NewIDAddr(t, 100)
	from := tutils.NewIDAddr(t, 101)
	to := tutils.NewIDAddr(t, 102)

	sm := newMockStateManager()
	pchapi := newMockPaychAPI()
	defer pchapi.close()

	mgr, err := newManager(sm, store, pchapi)
	require.NoError(t, err)

	// Send create message for a channel
	amt := big.NewInt(10)
	_, _, err = mgr.GetPaych(ctx, from, to, amt)
	require.NoError(t, err)

	time.Sleep(time.Millisecond * 10)

	// Send success create channel response
	response := testChannelResponse(t, ch)
	pchapi.finishWaitingCalls(response)

	time.Sleep(time.Millisecond * 10)

	// Send add funds message for channel
	amt2 := big.NewInt(5)
	_, _, err = mgr.GetPaych(ctx, from, to, amt2)
	require.NoError(t, err)

	time.Sleep(time.Millisecond * 10)

	// Send error add funds response
	pchapi.finishWaitingCalls(types.MessageReceipt{
		ExitCode: 1, // error
		Return:   []byte{},
	})

	time.Sleep(time.Millisecond * 10)

	// Should have one channel, whose address is the channel that was created
	cis, err := mgr.ListChannels()
	require.NoError(t, err)
	require.Len(t, cis, 1)
	require.Equal(t, ch, cis[0])

	ci, err := mgr.GetChannelInfo(ch)
	require.NoError(t, err)
	require.Equal(t, amt, ci.Amount)
	require.EqualValues(t, 0, ci.PendingAmount.Int64())
	require.Nil(t, ci.CreateMsg)
	require.Nil(t, ci.AddFundsMsg)

	// Send add funds message for channel again
	amt3 := big.NewInt(2)
	_, _, err = mgr.GetPaych(ctx, from, to, amt3)
	require.NoError(t, err)

	time.Sleep(time.Millisecond * 10)

	// Send success add funds response
	pchapi.finishWaitingCalls(types.MessageReceipt{
		ExitCode: 0,
		Return:   []byte{},
	})

	time.Sleep(time.Millisecond * 10)

	// Should have one channel, whose address is the channel that was created
	cis, err = mgr.ListChannels()
	require.NoError(t, err)
	require.Len(t, cis, 1)
	require.Equal(t, ch, cis[0])

	// Amount should include amount for successful add funds msg
	ci, err = mgr.GetChannelInfo(ch)
	require.NoError(t, err)
	require.Equal(t, amt.Int64()+amt3.Int64(), ci.Amount.Int64())
	require.EqualValues(t, 0, ci.PendingAmount.Int64())
	require.Nil(t, ci.CreateMsg)
	require.Nil(t, ci.AddFundsMsg)
}

// TestPaychGetRestartAfterCreateChannelMsg tests that if the system stops
// right after the create channel message is sent, the channel will be
// created when the system restarts.
func TestPaychGetRestartAfterCreateChannelMsg(t *testing.T) {
	ctx := context.Background()
	store := NewStore(ds_sync.MutexWrap(ds.NewMapDatastore()))

	ch := tutils.NewIDAddr(t, 100)
	from := tutils.NewIDAddr(t, 101)
	to := tutils.NewIDAddr(t, 102)

	sm := newMockStateManager()
	pchapi := newMockPaychAPI()

	mgr, err := newManager(sm, store, pchapi)
	require.NoError(t, err)

	// Send create message for a channel with value 10
	amt := big.NewInt(10)
	_, createMsgCid, err := mgr.GetPaych(ctx, from, to, amt)
	require.NoError(t, err)

	// Simulate shutting down lotus
	pchapi.close()

	// Create a new manager with the same datastore
	sm2 := newMockStateManager()
	pchapi2 := newMockPaychAPI()
	defer pchapi2.close()

	mgr2, err := newManager(sm2, store, pchapi2)
	require.NoError(t, err)

	// Should have no channels yet (message sent but channel not created)
	cis, err := mgr2.ListChannels()
	require.NoError(t, err)
	require.Len(t, cis, 0)

	// 1. Set up create channel response (sent in response to WaitForMsg())
	response := testChannelResponse(t, ch)

	done := make(chan struct{})
	go func() {
		defer close(done)

		// 2. Request add funds - should block until create channel has completed
		amt2 := big.NewInt(5)
		ch2, addFundsMsgCid, err := mgr2.GetPaych(ctx, from, to, amt2)

		// 4. This GetPaych should return after create channel from first
		//    GetPaych completes
		require.NoError(t, err)

		// Expect the channel to have been created
		require.Equal(t, ch, ch2)
		// Expect add funds message CID to be different to create message cid
		require.NotEqual(t, createMsgCid, addFundsMsgCid)

		// Should have one channel, whose address is the channel that was created
		cis, err := mgr2.ListChannels()
		require.NoError(t, err)
		require.Len(t, cis, 1)
		require.Equal(t, ch, cis[0])

		// Amount should be amount sent to first GetPaych (to create
		// channel).
		// PendingAmount should be amount sent in second GetPaych
		// (second GetPaych triggered add funds, which has not yet been confirmed)
		ci, err := mgr2.GetChannelInfo(ch)
		require.NoError(t, err)
		require.EqualValues(t, 10, ci.Amount.Int64())
		require.EqualValues(t, 5, ci.PendingAmount.Int64())
		require.Nil(t, ci.CreateMsg)
	}()

	// Give the go routine above a moment to run
	time.Sleep(time.Millisecond * 10)

	// 3. Send create channel response
	pchapi2.finishWaitingCalls(response)

	<-done
}

// TestPaychGetRestartAfterAddFundsMsg tests that if the system stops
// right after the add funds message is sent, the add funds will be
// processed when the system restarts.
func TestPaychGetRestartAfterAddFundsMsg(t *testing.T) {
	ctx := context.Background()
	store := NewStore(ds_sync.MutexWrap(ds.NewMapDatastore()))

	ch := tutils.NewIDAddr(t, 100)
	from := tutils.NewIDAddr(t, 101)
	to := tutils.NewIDAddr(t, 102)

	sm := newMockStateManager()
	pchapi := newMockPaychAPI()

	mgr, err := newManager(sm, store, pchapi)
	require.NoError(t, err)

	// Send create message for a channel
	amt := big.NewInt(10)
	_, _, err = mgr.GetPaych(ctx, from, to, amt)
	require.NoError(t, err)

	time.Sleep(time.Millisecond * 10)

	// Send success create channel response
	response := testChannelResponse(t, ch)
	pchapi.finishWaitingCalls(response)

	time.Sleep(time.Millisecond * 10)

	// Send add funds message for channel
	amt2 := big.NewInt(5)
	_, _, err = mgr.GetPaych(ctx, from, to, amt2)
	require.NoError(t, err)

	time.Sleep(time.Millisecond * 10)

	// Simulate shutting down lotus
	pchapi.close()

	// Create a new manager with the same datastore
	sm2 := newMockStateManager()
	pchapi2 := newMockPaychAPI()
	defer pchapi2.close()

	time.Sleep(time.Millisecond * 10)

	mgr2, err := newManager(sm2, store, pchapi2)
	require.NoError(t, err)

	// Send success add funds response
	pchapi2.finishWaitingCalls(types.MessageReceipt{
		ExitCode: 0,
		Return:   []byte{},
	})

	time.Sleep(time.Millisecond * 10)

	// Should have one channel, whose address is the channel that was created
	cis, err := mgr2.ListChannels()
	require.NoError(t, err)
	require.Len(t, cis, 1)
	require.Equal(t, ch, cis[0])

	// Amount should include amount for successful add funds msg
	ci, err := mgr2.GetChannelInfo(ch)
	require.NoError(t, err)
	require.Equal(t, amt.Int64()+amt2.Int64(), ci.Amount.Int64())
	require.EqualValues(t, 0, ci.PendingAmount.Int64())
	require.Nil(t, ci.CreateMsg)
	require.Nil(t, ci.AddFundsMsg)
}

// TestPaychGetWait tests that GetPaychWaitReady correctly waits for the
// channel to be created or funds to be added
func TestPaychGetWait(t *testing.T) {
	ctx := context.Background()
	store := NewStore(ds_sync.MutexWrap(ds.NewMapDatastore()))

	from := tutils.NewIDAddr(t, 101)
	to := tutils.NewIDAddr(t, 102)

	sm := newMockStateManager()
	pchapi := newMockPaychAPI()
	defer pchapi.close()

	mgr, err := newManager(sm, store, pchapi)
	require.NoError(t, err)

	// 1. Get
	amt := big.NewInt(10)
	_, mcid, err := mgr.GetPaych(ctx, from, to, amt)
	require.NoError(t, err)

	done := make(chan address.Address)
	go func() {
		// 2. Wait till ready
		ch, err := mgr.GetPaychWaitReady(ctx, mcid)
		require.NoError(t, err)

		done <- ch
	}()

	time.Sleep(time.Millisecond * 10)

	// 3. Send response
	expch := tutils.NewIDAddr(t, 100)
	response := testChannelResponse(t, expch)
	pchapi.finishWaitingCalls(response)

	time.Sleep(time.Millisecond * 10)

	ch := <-done
	require.Equal(t, expch, ch)

	// 4. Wait again - message has already been received so should
	//    return immediately
	ch, err = mgr.GetPaychWaitReady(ctx, mcid)
	require.NoError(t, err)
	require.Equal(t, expch, ch)

	// Request add funds
	amt2 := big.NewInt(15)
	_, addFundsMsgCid, err := mgr.GetPaych(ctx, from, to, amt2)
	require.NoError(t, err)

	time.Sleep(time.Millisecond * 10)

	go func() {
		// 5. Wait for add funds
		ch, err := mgr.GetPaychWaitReady(ctx, addFundsMsgCid)
		require.NoError(t, err)
		require.Equal(t, expch, ch)

		done <- ch
	}()

	time.Sleep(time.Millisecond * 10)

	// 6. Send add funds response
	addFundsResponse := types.MessageReceipt{
		ExitCode: 0,
		Return:   []byte{},
	}
	pchapi.finishWaitingCalls(addFundsResponse)

	<-done
}

// TestPaychGetWaitErr tests that GetPaychWaitReady correctly handles errors
func TestPaychGetWaitErr(t *testing.T) {
	ctx := context.Background()
	store := NewStore(ds_sync.MutexWrap(ds.NewMapDatastore()))

	from := tutils.NewIDAddr(t, 101)
	to := tutils.NewIDAddr(t, 102)

	sm := newMockStateManager()
	pchapi := newMockPaychAPI()
	defer pchapi.close()

	mgr, err := newManager(sm, store, pchapi)
	require.NoError(t, err)

	// 1. Create channel
	amt := big.NewInt(10)
	_, mcid, err := mgr.GetPaych(ctx, from, to, amt)
	require.NoError(t, err)

	done := make(chan address.Address)
	go func() {
		defer close(done)

		// 2. Wait for channel to be created
		_, err := mgr.GetPaychWaitReady(ctx, mcid)

		// 4. Channel creation should have failed
		require.NotNil(t, err)

		// 5. Call wait again with the same message CID
		_, err = mgr.GetPaychWaitReady(ctx, mcid)

		// 6. Should return immediately with the same error
		require.NotNil(t, err)
	}()

	// Give the wait a moment to start before sending response
	time.Sleep(time.Millisecond * 10)

	// 3. Send error response to create channel
	response := types.MessageReceipt{
		ExitCode: 1, // error
		Return:   []byte{},
	}
	pchapi.finishWaitingCalls(response)

	<-done
}

// TestPaychGetWaitCtx tests that GetPaychWaitReady returns early if the context
// is cancelled
func TestPaychGetWaitCtx(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	store := NewStore(ds_sync.MutexWrap(ds.NewMapDatastore()))

	from := tutils.NewIDAddr(t, 101)
	to := tutils.NewIDAddr(t, 102)

	sm := newMockStateManager()
	pchapi := newMockPaychAPI()
	defer pchapi.close()

	mgr, err := newManager(sm, store, pchapi)
	require.NoError(t, err)

	amt := big.NewInt(10)
	_, mcid, err := mgr.GetPaych(ctx, from, to, amt)
	require.NoError(t, err)

	// When the context is cancelled, should unblock wait
	go func() {
		time.Sleep(time.Millisecond * 10)
		cancel()
	}()

	_, err = mgr.GetPaychWaitReady(ctx, mcid)
	require.Error(t, ctx.Err(), err)
}
