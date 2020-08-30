package paychmgr

import (
	"context"
	"sync"
	"testing"
	"time"

	cborrpc "github.com/filecoin-project/go-cbor-util"

	init_ "github.com/filecoin-project/specs-actors/actors/builtin/init"

	"github.com/filecoin-project/specs-actors/actors/builtin"

	"github.com/filecoin-project/lotus/chain/types"

	"github.com/filecoin-project/go-address"

	"github.com/filecoin-project/specs-actors/actors/abi/big"
	tutils "github.com/filecoin-project/specs-actors/support/testing"
	"github.com/ipfs/go-cid"
	ds "github.com/ipfs/go-datastore"
	ds_sync "github.com/ipfs/go-datastore/sync"

	"github.com/stretchr/testify/require"
)

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

	mock := newMockManagerAPI()
	defer mock.close()

	mgr, err := newManager(store, mock)
	require.NoError(t, err)

	amt := big.NewInt(10)
	ch, mcid, err := mgr.GetPaych(ctx, from, to, amt)
	require.NoError(t, err)
	require.Equal(t, address.Undef, ch)

	pushedMsg := mock.pushedMessages(mcid)
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

	mock := newMockManagerAPI()
	defer mock.close()

	mgr, err := newManager(store, mock)
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
		mock.receiveMsgResponse(addFundsMsgCid, types.MessageReceipt{ExitCode: 0})

		// Wait for add funds confirmation to be processed by manager
		_, err = mgr.GetPaychWaitReady(ctx, addFundsMsgCid)
		require.NoError(t, err)

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

	// 3. Send create channel response
	mock.receiveMsgResponse(createMsgCid, response)

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

	mock := newMockManagerAPI()
	defer mock.close()

	mgr, err := newManager(store, mock)
	require.NoError(t, err)

	// Send create message for a channel
	amt := big.NewInt(10)
	_, mcid1, err := mgr.GetPaych(ctx, from, to, amt)
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
		//    should be for channel create again.
		amt2 := big.NewInt(5)
		ch2, mcid2, err := mgr.GetPaych(ctx, from, to, amt2)
		require.NoError(t, err)
		require.Equal(t, address.Undef, ch2)

		// 4. Send a success response
		ch := tutils.NewIDAddr(t, 100)
		successResponse := testChannelResponse(t, ch)
		mock.receiveMsgResponse(mcid2, successResponse)

		_, err = mgr.GetPaychWaitReady(ctx, mcid2)
		require.NoError(t, err)

		// Should have one channel, whose address is the channel that was created
		cis, err := mgr.ListChannels()
		require.NoError(t, err)
		require.Len(t, cis, 1)
		require.Equal(t, ch, cis[0])

		ci, err := mgr.GetChannelInfo(ch)
		require.NoError(t, err)
		require.Equal(t, amt2, ci.Amount)
	}()

	// 3. Send error response to first channel create
	mock.receiveMsgResponse(mcid1, errResponse)

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

	mock := newMockManagerAPI()
	defer mock.close()

	mgr, err := newManager(store, mock)
	require.NoError(t, err)

	// Send create message for a channel
	amt := big.NewInt(10)
	_, mcid, err := mgr.GetPaych(ctx, from, to, amt)
	require.NoError(t, err)

	// Send error create channel response
	mock.receiveMsgResponse(mcid, types.MessageReceipt{
		ExitCode: 1, // error
		Return:   []byte{},
	})

	// Send create message for a channel again
	amt2 := big.NewInt(7)
	_, mcid2, err := mgr.GetPaych(ctx, from, to, amt2)
	require.NoError(t, err)

	// Send success create channel response
	response := testChannelResponse(t, ch)
	mock.receiveMsgResponse(mcid2, response)

	_, err = mgr.GetPaychWaitReady(ctx, mcid2)
	require.NoError(t, err)

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

	mock := newMockManagerAPI()
	defer mock.close()

	mgr, err := newManager(store, mock)
	require.NoError(t, err)

	// Send create message for a channel
	amt := big.NewInt(10)
	_, mcid1, err := mgr.GetPaych(ctx, from, to, amt)
	require.NoError(t, err)

	// Send success create channel response
	response := testChannelResponse(t, ch)
	mock.receiveMsgResponse(mcid1, response)

	// Send add funds message for channel
	amt2 := big.NewInt(5)
	_, mcid2, err := mgr.GetPaych(ctx, from, to, amt2)
	require.NoError(t, err)

	// Send error add funds response
	mock.receiveMsgResponse(mcid2, types.MessageReceipt{
		ExitCode: 1, // error
		Return:   []byte{},
	})

	_, err = mgr.GetPaychWaitReady(ctx, mcid2)
	require.Error(t, err)

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
	_, mcid3, err := mgr.GetPaych(ctx, from, to, amt3)
	require.NoError(t, err)

	// Send success add funds response
	mock.receiveMsgResponse(mcid3, types.MessageReceipt{
		ExitCode: 0,
		Return:   []byte{},
	})

	_, err = mgr.GetPaychWaitReady(ctx, mcid3)
	require.NoError(t, err)

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

	mock := newMockManagerAPI()

	mgr, err := newManager(store, mock)
	require.NoError(t, err)

	// Send create message for a channel with value 10
	amt := big.NewInt(10)
	_, createMsgCid, err := mgr.GetPaych(ctx, from, to, amt)
	require.NoError(t, err)

	// Simulate shutting down system
	mock.close()

	// Create a new manager with the same datastore
	mock2 := newMockManagerAPI()
	defer mock2.close()

	mgr2, err := newManager(store, mock2)
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

	// 3. Send create channel response
	mock2.receiveMsgResponse(createMsgCid, response)

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

	mock := newMockManagerAPI()

	mgr, err := newManager(store, mock)
	require.NoError(t, err)

	// Send create message for a channel
	amt := big.NewInt(10)
	_, mcid1, err := mgr.GetPaych(ctx, from, to, amt)
	require.NoError(t, err)

	// Send success create channel response
	response := testChannelResponse(t, ch)
	mock.receiveMsgResponse(mcid1, response)

	// Send add funds message for channel
	amt2 := big.NewInt(5)
	_, mcid2, err := mgr.GetPaych(ctx, from, to, amt2)
	require.NoError(t, err)

	// Simulate shutting down system
	mock.close()

	// Create a new manager with the same datastore
	mock2 := newMockManagerAPI()
	defer mock2.close()

	mgr2, err := newManager(store, mock2)
	require.NoError(t, err)

	// Send success add funds response
	mock2.receiveMsgResponse(mcid2, types.MessageReceipt{
		ExitCode: 0,
		Return:   []byte{},
	})

	_, err = mgr2.GetPaychWaitReady(ctx, mcid2)
	require.NoError(t, err)

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

	mock := newMockManagerAPI()
	defer mock.close()

	mgr, err := newManager(store, mock)
	require.NoError(t, err)

	// 1. Get
	amt := big.NewInt(10)
	_, createMsgCid, err := mgr.GetPaych(ctx, from, to, amt)
	require.NoError(t, err)

	expch := tutils.NewIDAddr(t, 100)
	go func() {
		// 3. Send response
		response := testChannelResponse(t, expch)
		mock.receiveMsgResponse(createMsgCid, response)
	}()

	// 2. Wait till ready
	ch, err := mgr.GetPaychWaitReady(ctx, createMsgCid)
	require.NoError(t, err)
	require.Equal(t, expch, ch)

	// 4. Wait again - message has already been received so should
	//    return immediately
	ch, err = mgr.GetPaychWaitReady(ctx, createMsgCid)
	require.NoError(t, err)
	require.Equal(t, expch, ch)

	// Request add funds
	amt2 := big.NewInt(15)
	_, addFundsMsgCid, err := mgr.GetPaych(ctx, from, to, amt2)
	require.NoError(t, err)

	go func() {
		// 6. Send add funds response
		addFundsResponse := types.MessageReceipt{
			ExitCode: 0,
			Return:   []byte{},
		}
		mock.receiveMsgResponse(addFundsMsgCid, addFundsResponse)
	}()

	// 5. Wait for add funds
	ch, err = mgr.GetPaychWaitReady(ctx, addFundsMsgCid)
	require.NoError(t, err)
	require.Equal(t, expch, ch)
}

// TestPaychGetWaitErr tests that GetPaychWaitReady correctly handles errors
func TestPaychGetWaitErr(t *testing.T) {
	ctx := context.Background()
	store := NewStore(ds_sync.MutexWrap(ds.NewMapDatastore()))

	from := tutils.NewIDAddr(t, 101)
	to := tutils.NewIDAddr(t, 102)

	mock := newMockManagerAPI()
	defer mock.close()

	mgr, err := newManager(store, mock)
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

	// 3. Send error response to create channel
	response := types.MessageReceipt{
		ExitCode: 1, // error
		Return:   []byte{},
	}
	mock.receiveMsgResponse(mcid, response)

	<-done
}

// TestPaychGetWaitCtx tests that GetPaychWaitReady returns early if the context
// is cancelled
func TestPaychGetWaitCtx(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	store := NewStore(ds_sync.MutexWrap(ds.NewMapDatastore()))

	from := tutils.NewIDAddr(t, 101)
	to := tutils.NewIDAddr(t, 102)

	mock := newMockManagerAPI()
	defer mock.close()

	mgr, err := newManager(store, mock)
	require.NoError(t, err)

	amt := big.NewInt(10)
	_, mcid, err := mgr.GetPaych(ctx, from, to, amt)
	require.NoError(t, err)

	// When the context is cancelled, should unblock wait
	go func() {
		cancel()
	}()

	_, err = mgr.GetPaychWaitReady(ctx, mcid)
	require.Error(t, ctx.Err(), err)
}

// TestPaychGetMergeAddFunds tests that if a create channel is in
// progress and two add funds are queued up behind it, the two add funds
// will be merged
func TestPaychGetMergeAddFunds(t *testing.T) {
	ctx := context.Background()
	store := NewStore(ds_sync.MutexWrap(ds.NewMapDatastore()))

	ch := tutils.NewIDAddr(t, 100)
	from := tutils.NewIDAddr(t, 101)
	to := tutils.NewIDAddr(t, 102)

	mock := newMockManagerAPI()
	defer mock.close()

	mgr, err := newManager(store, mock)
	require.NoError(t, err)

	// Send create message for a channel with value 10
	createAmt := big.NewInt(10)
	_, createMsgCid, err := mgr.GetPaych(ctx, from, to, createAmt)
	require.NoError(t, err)

	// Queue up two add funds requests behind create channel
	//var addFundsQueuedUp sync.WaitGroup
	//addFundsQueuedUp.Add(2)
	var addFundsSent sync.WaitGroup
	addFundsSent.Add(2)

	addFundsAmt1 := big.NewInt(5)
	addFundsAmt2 := big.NewInt(3)
	var addFundsCh1 address.Address
	var addFundsCh2 address.Address
	var addFundsMcid1 cid.Cid
	var addFundsMcid2 cid.Cid
	go func() {
		//go addFundsQueuedUp.Done()
		defer addFundsSent.Done()

		// Request add funds - should block until create channel has completed
		addFundsCh1, addFundsMcid1, err = mgr.GetPaych(ctx, from, to, addFundsAmt1)
		require.NoError(t, err)
	}()

	go func() {
		//go addFundsQueuedUp.Done()
		defer addFundsSent.Done()

		// Request add funds again - should merge with waiting add funds request
		addFundsCh2, addFundsMcid2, err = mgr.GetPaych(ctx, from, to, addFundsAmt2)
		require.NoError(t, err)
	}()
	// Wait for add funds requests to be queued up
	waitForQueueSize(t, mgr, from, to, 2)

	// Send create channel response
	response := testChannelResponse(t, ch)
	mock.receiveMsgResponse(createMsgCid, response)

	// Wait for create channel response
	chres, err := mgr.GetPaychWaitReady(ctx, createMsgCid)
	require.NoError(t, err)
	require.Equal(t, ch, chres)

	// Wait for add funds requests to be sent
	addFundsSent.Wait()

	// Expect add funds requests to have same channel as create channel and
	// same message cid as each other (because they should have been merged)
	require.Equal(t, ch, addFundsCh1)
	require.Equal(t, ch, addFundsCh2)
	require.Equal(t, addFundsMcid1, addFundsMcid2)

	// Send success add funds response
	mock.receiveMsgResponse(addFundsMcid1, types.MessageReceipt{
		ExitCode: 0,
		Return:   []byte{},
	})

	// Wait for add funds response
	addFundsCh, err := mgr.GetPaychWaitReady(ctx, addFundsMcid1)
	require.NoError(t, err)
	require.Equal(t, ch, addFundsCh)

	// Make sure that one create channel message and one add funds message was
	// sent
	require.Equal(t, 2, mock.pushedMessageCount())

	// Check create message amount is correct
	createMsg := mock.pushedMessages(createMsgCid)
	require.Equal(t, from, createMsg.Message.From)
	require.Equal(t, builtin.InitActorAddr, createMsg.Message.To)
	require.Equal(t, createAmt, createMsg.Message.Value)

	// Check merged add funds amount is the sum of the individual
	// amounts
	addFundsMsg := mock.pushedMessages(addFundsMcid1)
	require.Equal(t, from, addFundsMsg.Message.From)
	require.Equal(t, ch, addFundsMsg.Message.To)
	require.Equal(t, types.BigAdd(addFundsAmt1, addFundsAmt2), addFundsMsg.Message.Value)
}

// TestPaychGetMergeAddFundsCtxCancelOne tests that when a queued add funds
// request is cancelled, its amount is removed from the total merged add funds
func TestPaychGetMergeAddFundsCtxCancelOne(t *testing.T) {
	ctx := context.Background()
	store := NewStore(ds_sync.MutexWrap(ds.NewMapDatastore()))

	ch := tutils.NewIDAddr(t, 100)
	from := tutils.NewIDAddr(t, 101)
	to := tutils.NewIDAddr(t, 102)

	mock := newMockManagerAPI()
	defer mock.close()

	mgr, err := newManager(store, mock)
	require.NoError(t, err)

	// Send create message for a channel with value 10
	createAmt := big.NewInt(10)
	_, createMsgCid, err := mgr.GetPaych(ctx, from, to, createAmt)
	require.NoError(t, err)

	// Queue up two add funds requests behind create channel
	var addFundsSent sync.WaitGroup
	addFundsSent.Add(2)

	addFundsAmt1 := big.NewInt(5)
	addFundsAmt2 := big.NewInt(3)
	var addFundsCh2 address.Address
	var addFundsMcid2 cid.Cid
	var addFundsErr1 error
	addFundsCtx1, cancelAddFundsCtx1 := context.WithCancel(ctx)
	go func() {
		defer addFundsSent.Done()

		// Request add funds - should block until create channel has completed
		_, _, addFundsErr1 = mgr.GetPaych(addFundsCtx1, from, to, addFundsAmt1)
	}()

	go func() {
		defer addFundsSent.Done()

		// Request add funds again - should merge with waiting add funds request
		addFundsCh2, addFundsMcid2, err = mgr.GetPaych(ctx, from, to, addFundsAmt2)
		require.NoError(t, err)
	}()
	// Wait for add funds requests to be queued up
	waitForQueueSize(t, mgr, from, to, 2)

	// Cancel the first add funds request
	cancelAddFundsCtx1()

	// Send create channel response
	response := testChannelResponse(t, ch)
	mock.receiveMsgResponse(createMsgCid, response)

	// Wait for create channel response
	chres, err := mgr.GetPaychWaitReady(ctx, createMsgCid)
	require.NoError(t, err)
	require.Equal(t, ch, chres)

	// Wait for add funds requests to be sent
	addFundsSent.Wait()

	// Expect first add funds request to have been cancelled
	require.NotNil(t, addFundsErr1)
	require.Equal(t, ch, addFundsCh2)

	// Send success add funds response
	mock.receiveMsgResponse(addFundsMcid2, types.MessageReceipt{
		ExitCode: 0,
		Return:   []byte{},
	})

	// Wait for add funds response
	addFundsCh, err := mgr.GetPaychWaitReady(ctx, addFundsMcid2)
	require.NoError(t, err)
	require.Equal(t, ch, addFundsCh)

	// Make sure that one create channel message and one add funds message was
	// sent
	require.Equal(t, 2, mock.pushedMessageCount())

	// Check create message amount is correct
	createMsg := mock.pushedMessages(createMsgCid)
	require.Equal(t, from, createMsg.Message.From)
	require.Equal(t, builtin.InitActorAddr, createMsg.Message.To)
	require.Equal(t, createAmt, createMsg.Message.Value)

	// Check merged add funds amount only includes the second add funds amount
	// (because first was cancelled)
	addFundsMsg := mock.pushedMessages(addFundsMcid2)
	require.Equal(t, from, addFundsMsg.Message.From)
	require.Equal(t, ch, addFundsMsg.Message.To)
	require.Equal(t, addFundsAmt2, addFundsMsg.Message.Value)
}

// TestPaychGetMergeAddFundsCtxCancelAll tests that when all queued add funds
// requests are cancelled, no add funds message is sent
func TestPaychGetMergeAddFundsCtxCancelAll(t *testing.T) {
	ctx := context.Background()
	store := NewStore(ds_sync.MutexWrap(ds.NewMapDatastore()))

	ch := tutils.NewIDAddr(t, 100)
	from := tutils.NewIDAddr(t, 101)
	to := tutils.NewIDAddr(t, 102)

	mock := newMockManagerAPI()
	defer mock.close()

	mgr, err := newManager(store, mock)
	require.NoError(t, err)

	// Send create message for a channel with value 10
	createAmt := big.NewInt(10)
	_, createMsgCid, err := mgr.GetPaych(ctx, from, to, createAmt)
	require.NoError(t, err)

	// Queue up two add funds requests behind create channel
	var addFundsSent sync.WaitGroup
	addFundsSent.Add(2)

	var addFundsErr1 error
	var addFundsErr2 error
	addFundsCtx1, cancelAddFundsCtx1 := context.WithCancel(ctx)
	addFundsCtx2, cancelAddFundsCtx2 := context.WithCancel(ctx)
	go func() {
		defer addFundsSent.Done()

		// Request add funds - should block until create channel has completed
		_, _, addFundsErr1 = mgr.GetPaych(addFundsCtx1, from, to, big.NewInt(5))
	}()

	go func() {
		defer addFundsSent.Done()

		// Request add funds again - should merge with waiting add funds request
		_, _, addFundsErr2 = mgr.GetPaych(addFundsCtx2, from, to, big.NewInt(3))
		require.NoError(t, err)
	}()
	// Wait for add funds requests to be queued up
	waitForQueueSize(t, mgr, from, to, 2)

	// Cancel all add funds requests
	cancelAddFundsCtx1()
	cancelAddFundsCtx2()

	// Send create channel response
	response := testChannelResponse(t, ch)
	mock.receiveMsgResponse(createMsgCid, response)

	// Wait for create channel response
	chres, err := mgr.GetPaychWaitReady(ctx, createMsgCid)
	require.NoError(t, err)
	require.Equal(t, ch, chres)

	// Wait for add funds requests to error out
	addFundsSent.Wait()

	require.NotNil(t, addFundsErr1)
	require.NotNil(t, addFundsErr2)

	// Make sure that just the create channel message was sent
	require.Equal(t, 1, mock.pushedMessageCount())

	// Check create message amount is correct
	createMsg := mock.pushedMessages(createMsgCid)
	require.Equal(t, from, createMsg.Message.From)
	require.Equal(t, builtin.InitActorAddr, createMsg.Message.To)
	require.Equal(t, createAmt, createMsg.Message.Value)
}

// waitForQueueSize waits for the funds request queue to be of the given size
func waitForQueueSize(t *testing.T, mgr *Manager, from address.Address, to address.Address, size int) {
	ca, err := mgr.accessorByFromTo(from, to)
	require.NoError(t, err)

	for {
		if ca.queueSize() == size {
			return
		}

		time.Sleep(time.Millisecond)
	}
}
