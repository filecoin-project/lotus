package paychmgr

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/ipfs/go-cid"
	ds "github.com/ipfs/go-datastore"
	ds_sync "github.com/ipfs/go-datastore/sync"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	cborrpc "github.com/filecoin-project/go-cbor-util"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/specs-actors/v2/actors/builtin"
	init2 "github.com/filecoin-project/specs-actors/v2/actors/builtin/init"
	tutils "github.com/filecoin-project/specs-actors/v2/support/testing"

	lotusinit "github.com/filecoin-project/lotus/chain/actors/builtin/init"
	"github.com/filecoin-project/lotus/chain/actors/builtin/paych"
	paychmock "github.com/filecoin-project/lotus/chain/actors/builtin/paych/mock"
	"github.com/filecoin-project/lotus/chain/types"
)

var onChainReserve = GetOpts{
	Reserve:  true,
	OffChain: false,
}
var onChainNoReserve = GetOpts{
	Reserve:  false,
	OffChain: false,
}
var offChainReserve = GetOpts{
	Reserve:  true,
	OffChain: true,
}
var offChainNoReserve = GetOpts{
	Reserve:  false,
	OffChain: true,
}

func testChannelResponse(t *testing.T, ch address.Address) types.MessageReceipt {
	createChannelRet := init2.ExecReturn{
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
	ch, mcid, err := mgr.GetPaych(ctx, from, to, amt, onChainReserve)
	require.NoError(t, err)
	require.Equal(t, address.Undef, ch)

	pushedMsg := mock.pushedMessages(mcid)
	require.Equal(t, from, pushedMsg.Message.From)
	require.Equal(t, lotusinit.Address, pushedMsg.Message.To)
	require.Equal(t, amt, pushedMsg.Message.Value)
}

func TestPaychGetOffchainNoReserveFails(t *testing.T) {
	ctx := context.Background()
	store := NewStore(ds_sync.MutexWrap(ds.NewMapDatastore()))

	from := tutils.NewIDAddr(t, 101)
	to := tutils.NewIDAddr(t, 102)

	mock := newMockManagerAPI()
	defer mock.close()

	mgr, err := newManager(store, mock)
	require.NoError(t, err)

	amt := big.NewInt(10)
	_, _, err = mgr.GetPaych(ctx, from, to, amt, offChainNoReserve)
	require.Error(t, err)
}

func TestPaychGetCreateOffchainReserveFails(t *testing.T) {
	ctx := context.Background()
	store := NewStore(ds_sync.MutexWrap(ds.NewMapDatastore()))

	from := tutils.NewIDAddr(t, 101)
	to := tutils.NewIDAddr(t, 102)

	mock := newMockManagerAPI()
	defer mock.close()

	mgr, err := newManager(store, mock)
	require.NoError(t, err)

	amt := big.NewInt(10)
	_, _, err = mgr.GetPaych(ctx, from, to, amt, offChainReserve)
	require.Error(t, err)
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

	act := &types.Actor{
		Code:    builtin.AccountActorCodeID,
		Head:    cid.Cid{},
		Nonce:   0,
		Balance: types.NewInt(20),
	}
	mock.setPaychState(ch, act, paychmock.NewMockPayChState(from, to, abi.ChainEpoch(0), make(map[uint64]paych.LaneState)))

	mgr, err := newManager(store, mock)
	require.NoError(t, err)

	// Send create message for a channel with value 10
	amt := big.NewInt(10)
	_, createMsgCid, err := mgr.GetPaych(ctx, from, to, amt, onChainReserve)
	require.NoError(t, err)

	// Should have no channels yet (message sent but channel not created)
	cis, err := mgr.ListChannels(ctx)
	require.NoError(t, err)
	require.Len(t, cis, 0)

	// 1. Set up create channel response (sent in response to WaitForMsg())
	response := testChannelResponse(t, ch)

	done := make(chan struct{})
	go func() {
		defer close(done)

		// 2. Request add funds - should block until create channel has completed
		amt2 := big.NewInt(5)
		ch2, addFundsMsgCid, err := mgr.GetPaych(ctx, from, to, amt2, onChainReserve)

		// 4. This GetPaych should return after create channel from first
		//    GetPaych completes
		require.NoError(t, err)

		// Expect the channel to be the same
		require.Equal(t, ch, ch2)
		// Expect add funds message CID to be different to create message cid
		require.NotEqual(t, createMsgCid, addFundsMsgCid)

		// Should have one channel, whose address is the channel that was created
		cis, err := mgr.ListChannels(ctx)
		require.NoError(t, err)
		require.Len(t, cis, 1)
		require.Equal(t, ch, cis[0])

		// Amount should be amount sent to first GetPaych (to create
		// channel).
		// PendingAmount should be amount sent in second GetPaych
		// (second GetPaych triggered add funds, which has not yet been confirmed)
		ci, err := mgr.GetChannelInfo(ctx, ch)
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
		cis, err = mgr.ListChannels(ctx)
		require.NoError(t, err)
		require.Len(t, cis, 1)
		require.Equal(t, ch, cis[0])

		// Channel amount should include last amount sent to GetPaych
		ci, err = mgr.GetChannelInfo(ctx, ch)
		require.NoError(t, err)
		require.EqualValues(t, 15, ci.Amount.Int64())
		require.EqualValues(t, 0, ci.PendingAmount.Int64())
		require.Nil(t, ci.AddFundsMsg)
	}()

	// 3. Send create channel response
	mock.receiveMsgResponse(createMsgCid, response)

	<-done
}

func TestPaychGetCreatePrefundedChannelThenAddFunds(t *testing.T) {
	ctx := context.Background()
	store := NewStore(ds_sync.MutexWrap(ds.NewMapDatastore()))

	ch := tutils.NewIDAddr(t, 100)
	from := tutils.NewIDAddr(t, 101)
	to := tutils.NewIDAddr(t, 102)

	mock := newMockManagerAPI()
	defer mock.close()

	act := &types.Actor{
		Code:    builtin.AccountActorCodeID,
		Head:    cid.Cid{},
		Nonce:   0,
		Balance: types.NewInt(20),
	}
	mock.setPaychState(ch, act, paychmock.NewMockPayChState(from, to, abi.ChainEpoch(0), make(map[uint64]paych.LaneState)))

	mgr, err := newManager(store, mock)
	require.NoError(t, err)

	// Send create message for a channel with value 10
	amt := big.NewInt(10)
	_, createMsgCid, err := mgr.GetPaych(ctx, from, to, amt, onChainNoReserve)
	require.NoError(t, err)

	// Should have no channels yet (message sent but channel not created)
	cis, err := mgr.ListChannels(ctx)
	require.NoError(t, err)
	require.Len(t, cis, 0)

	// 1. Set up create channel response (sent in response to WaitForMsg())
	response := testChannelResponse(t, ch)

	done := make(chan struct{})
	go func() {
		defer close(done)

		// 2. Request add funds - shouldn't block
		amt2 := big.NewInt(3)
		ch2, addFundsMsgCid, err := mgr.GetPaych(ctx, from, to, amt2, offChainReserve)

		// 4. This GetPaych should return after create channel from first
		//    GetPaych completes
		require.NoError(t, err)

		// Expect the channel to be the same
		require.Equal(t, ch, ch2)
		require.Equal(t, cid.Undef, addFundsMsgCid)

		// Should have one channel, whose address is the channel that was created
		cis, err := mgr.ListChannels(ctx)
		require.NoError(t, err)
		require.Len(t, cis, 1)
		require.Equal(t, ch, cis[0])

		// Amount should be amount sent to first GetPaych (to create
		// channel).
		// PendingAmount should be zero, AvailableAmount should be Amount minus what we requested

		ci, err := mgr.GetChannelInfo(ctx, ch)
		require.NoError(t, err)
		require.EqualValues(t, 10, ci.Amount.Int64())
		require.EqualValues(t, 0, ci.PendingAmount.Int64())
		require.EqualValues(t, 7, ci.AvailableAmount.Int64())
		require.Nil(t, ci.CreateMsg)
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
	_, mcid1, err := mgr.GetPaych(ctx, from, to, amt, onChainReserve)
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
		ch2, mcid2, err := mgr.GetPaych(ctx, from, to, amt2, onChainReserve)
		require.NoError(t, err)
		require.Equal(t, address.Undef, ch2)

		// 4. Send a success response
		ch := tutils.NewIDAddr(t, 100)
		successResponse := testChannelResponse(t, ch)
		mock.receiveMsgResponse(mcid2, successResponse)

		_, err = mgr.GetPaychWaitReady(ctx, mcid2)
		require.NoError(t, err)

		// Should have one channel, whose address is the channel that was created
		cis, err := mgr.ListChannels(ctx)
		require.NoError(t, err)
		require.Len(t, cis, 1)
		require.Equal(t, ch, cis[0])

		ci, err := mgr.GetChannelInfo(ctx, ch)
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
	_, mcid, err := mgr.GetPaych(ctx, from, to, amt, onChainReserve)
	require.NoError(t, err)

	// Send error create channel response
	mock.receiveMsgResponse(mcid, types.MessageReceipt{
		ExitCode: 1, // error
		Return:   []byte{},
	})

	// Send create message for a channel again
	amt2 := big.NewInt(7)
	_, mcid2, err := mgr.GetPaych(ctx, from, to, amt2, onChainReserve)
	require.NoError(t, err)

	// Send success create channel response
	response := testChannelResponse(t, ch)
	mock.receiveMsgResponse(mcid2, response)

	_, err = mgr.GetPaychWaitReady(ctx, mcid2)
	require.NoError(t, err)

	// Should have one channel, whose address is the channel that was created
	cis, err := mgr.ListChannels(ctx)
	require.NoError(t, err)
	require.Len(t, cis, 1)
	require.Equal(t, ch, cis[0])

	ci, err := mgr.GetChannelInfo(ctx, ch)
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

	act := &types.Actor{
		Code:    builtin.AccountActorCodeID,
		Head:    cid.Cid{},
		Nonce:   0,
		Balance: types.NewInt(20),
	}
	mock.setPaychState(ch, act, paychmock.NewMockPayChState(from, to, abi.ChainEpoch(0), make(map[uint64]paych.LaneState)))

	mgr, err := newManager(store, mock)
	require.NoError(t, err)

	// Send create message for a channel
	amt := big.NewInt(10)
	_, mcid1, err := mgr.GetPaych(ctx, from, to, amt, onChainReserve)
	require.NoError(t, err)

	// Send success create channel response
	response := testChannelResponse(t, ch)
	mock.receiveMsgResponse(mcid1, response)

	// Send add funds message for channel
	amt2 := big.NewInt(5)
	_, mcid2, err := mgr.GetPaych(ctx, from, to, amt2, onChainReserve)
	require.NoError(t, err)

	// Send error add funds response
	mock.receiveMsgResponse(mcid2, types.MessageReceipt{
		ExitCode: 1, // error
		Return:   []byte{},
	})

	_, err = mgr.GetPaychWaitReady(ctx, mcid2)
	require.Error(t, err)

	// Should have one channel, whose address is the channel that was created
	cis, err := mgr.ListChannels(ctx)
	require.NoError(t, err)
	require.Len(t, cis, 1)
	require.Equal(t, ch, cis[0])

	ci, err := mgr.GetChannelInfo(ctx, ch)
	require.NoError(t, err)
	require.Equal(t, amt, ci.Amount)
	require.EqualValues(t, 0, ci.PendingAmount.Int64())
	require.Nil(t, ci.CreateMsg)
	require.Nil(t, ci.AddFundsMsg)

	// Send add funds message for channel again
	amt3 := big.NewInt(2)
	_, mcid3, err := mgr.GetPaych(ctx, from, to, amt3, onChainReserve)
	require.NoError(t, err)

	// Send success add funds response
	mock.receiveMsgResponse(mcid3, types.MessageReceipt{
		ExitCode: 0,
		Return:   []byte{},
	})

	_, err = mgr.GetPaychWaitReady(ctx, mcid3)
	require.NoError(t, err)

	// Should have one channel, whose address is the channel that was created
	cis, err = mgr.ListChannels(ctx)
	require.NoError(t, err)
	require.Len(t, cis, 1)
	require.Equal(t, ch, cis[0])

	// Amount should include amount for successful add funds msg
	ci, err = mgr.GetChannelInfo(ctx, ch)
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

	var createMsgCid cid.Cid
	{
		mock := newMockManagerAPI()

		mgr, err := newManager(store, mock)
		require.NoError(t, err)

		// Send create message for a channel with value 10
		amt := big.NewInt(10)
		_, createMsgCid, err = mgr.GetPaych(ctx, from, to, amt, onChainReserve)
		require.NoError(t, err)

		// Simulate shutting down system
		mock.close()
	}

	// Create a new manager with the same datastore
	mock2 := newMockManagerAPI()
	defer mock2.close()

	act := &types.Actor{
		Code:    builtin.AccountActorCodeID,
		Head:    cid.Cid{},
		Nonce:   0,
		Balance: types.NewInt(20),
	}
	mock2.setPaychState(ch, act, paychmock.NewMockPayChState(from, to, abi.ChainEpoch(0), make(map[uint64]paych.LaneState)))

	mgr2, err := newManager(store, mock2)
	require.NoError(t, err)

	// Should have no channels yet (message sent but channel not created)
	cis, err := mgr2.ListChannels(ctx)
	require.NoError(t, err)
	require.Len(t, cis, 0)

	// 1. Set up create channel response (sent in response to WaitForMsg())
	response := testChannelResponse(t, ch)

	done := make(chan struct{})
	go func() {
		defer close(done)

		// 2. Request add funds - should block until create channel has completed
		amt2 := big.NewInt(5)
		ch2, addFundsMsgCid, err := mgr2.GetPaych(ctx, from, to, amt2, onChainReserve)

		// 4. This GetPaych should return after create channel from first
		//    GetPaych completes
		require.NoError(t, err)

		// Expect the channel to have been created
		require.Equal(t, ch, ch2)
		// Expect add funds message CID to be different to create message cid
		require.NotEqual(t, createMsgCid, addFundsMsgCid)

		// Should have one channel, whose address is the channel that was created
		cis, err := mgr2.ListChannels(ctx)
		require.NoError(t, err)
		require.Len(t, cis, 1)
		require.Equal(t, ch, cis[0])

		// Amount should be amount sent to first GetPaych (to create
		// channel).
		// PendingAmount should be amount sent in second GetPaych
		// (second GetPaych triggered add funds, which has not yet been confirmed)
		ci, err := mgr2.GetChannelInfo(ctx, ch)
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

	act := &types.Actor{
		Code:    builtin.AccountActorCodeID,
		Head:    cid.Cid{},
		Nonce:   0,
		Balance: types.NewInt(20),
	}
	mock.setPaychState(ch, act, paychmock.NewMockPayChState(from, to, abi.ChainEpoch(0), make(map[uint64]paych.LaneState)))

	mgr, err := newManager(store, mock)
	require.NoError(t, err)

	// Send create message for a channel
	amt := big.NewInt(10)
	_, mcid1, err := mgr.GetPaych(ctx, from, to, amt, onChainReserve)
	require.NoError(t, err)

	// Send success create channel response
	response := testChannelResponse(t, ch)
	mock.receiveMsgResponse(mcid1, response)

	// Send add funds message for channel
	amt2 := big.NewInt(5)
	_, mcid2, err := mgr.GetPaych(ctx, from, to, amt2, onChainReserve)
	require.NoError(t, err)

	// Simulate shutting down system
	require.NoError(t, mgr.Stop())

	// Create a new manager with the same datastore
	mock2 := newMockManagerAPI()
	defer mock2.close()

	mock2.setPaychState(ch, act, paychmock.NewMockPayChState(from, to, abi.ChainEpoch(0), make(map[uint64]paych.LaneState)))

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
	cis, err := mgr2.ListChannels(ctx)
	require.NoError(t, err)
	require.Len(t, cis, 1)
	require.Equal(t, ch, cis[0])

	// Amount should include amount for successful add funds msg
	ci, err := mgr2.GetChannelInfo(ctx, ch)
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
	expch := tutils.NewIDAddr(t, 100)

	mock := newMockManagerAPI()
	defer mock.close()

	act := &types.Actor{
		Code:    builtin.AccountActorCodeID,
		Head:    cid.Cid{},
		Nonce:   0,
		Balance: types.NewInt(20),
	}
	mock.setPaychState(expch, act, paychmock.NewMockPayChState(from, to, abi.ChainEpoch(0), make(map[uint64]paych.LaneState)))

	mgr, err := newManager(store, mock)
	require.NoError(t, err)

	// 1. Get
	amt := big.NewInt(10)
	_, createMsgCid, err := mgr.GetPaych(ctx, from, to, amt, onChainReserve)
	require.NoError(t, err)

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
	_, addFundsMsgCid, err := mgr.GetPaych(ctx, from, to, amt2, onChainReserve)
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
	_, mcid, err := mgr.GetPaych(ctx, from, to, amt, onChainReserve)
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
	_, mcid, err := mgr.GetPaych(ctx, from, to, amt, onChainReserve)
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

	act := &types.Actor{
		Code:    builtin.AccountActorCodeID,
		Head:    cid.Cid{},
		Nonce:   0,
		Balance: types.NewInt(20),
	}
	mock.setPaychState(ch, act, paychmock.NewMockPayChState(from, to, abi.ChainEpoch(0), make(map[uint64]paych.LaneState)))

	mgr, err := newManager(store, mock)
	require.NoError(t, err)

	// Send create message for a channel with value 10
	createAmt := big.NewInt(10)
	_, createMsgCid, err := mgr.GetPaych(ctx, from, to, createAmt, onChainReserve)
	require.NoError(t, err)

	// Queue up two add funds requests behind create channel
	var addFundsSent sync.WaitGroup
	addFundsSent.Add(2)

	addFundsAmt1 := big.NewInt(5)
	addFundsAmt2 := big.NewInt(3)
	var addFundsCh1 address.Address
	var addFundsCh2 address.Address
	var addFundsMcid1 cid.Cid
	var addFundsMcid2 cid.Cid
	go func() {
		defer addFundsSent.Done()

		// Request add funds - should block until create channel has completed
		var err error
		addFundsCh1, addFundsMcid1, err = mgr.GetPaych(ctx, from, to, addFundsAmt1, onChainReserve)
		require.NoError(t, err)
	}()

	go func() {
		defer addFundsSent.Done()

		// Request add funds again - should merge with waiting add funds request
		var err error
		addFundsCh2, addFundsMcid2, err = mgr.GetPaych(ctx, from, to, addFundsAmt2, onChainReserve)
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
	require.Equal(t, lotusinit.Address, createMsg.Message.To)
	require.Equal(t, createAmt, createMsg.Message.Value)

	// Check merged add funds amount is the sum of the individual
	// amounts
	addFundsMsg := mock.pushedMessages(addFundsMcid1)
	require.Equal(t, from, addFundsMsg.Message.From)
	require.Equal(t, ch, addFundsMsg.Message.To)
	require.Equal(t, types.BigAdd(addFundsAmt1, addFundsAmt2), addFundsMsg.Message.Value)
}

func TestPaychGetMergePrefundAndReserve(t *testing.T) {
	ctx := context.Background()
	store := NewStore(ds_sync.MutexWrap(ds.NewMapDatastore()))

	ch := tutils.NewIDAddr(t, 100)
	from := tutils.NewIDAddr(t, 101)
	to := tutils.NewIDAddr(t, 102)

	mock := newMockManagerAPI()
	defer mock.close()

	act := &types.Actor{
		Code:    builtin.AccountActorCodeID,
		Head:    cid.Cid{},
		Nonce:   0,
		Balance: types.NewInt(20),
	}
	mock.setPaychState(ch, act, paychmock.NewMockPayChState(from, to, abi.ChainEpoch(0), make(map[uint64]paych.LaneState)))

	mgr, err := newManager(store, mock)
	require.NoError(t, err)

	// Send create message for a channel with value 10
	createAmt := big.NewInt(10)
	_, createMsgCid, err := mgr.GetPaych(ctx, from, to, createAmt, onChainReserve)
	require.NoError(t, err)

	// Queue up two add funds requests behind create channel
	var addFundsSent sync.WaitGroup
	addFundsSent.Add(2)

	addFundsAmt1 := big.NewInt(5) // 1 prefunds
	addFundsAmt2 := big.NewInt(3) // 2 reserves
	var addFundsCh1 address.Address
	var addFundsCh2 address.Address
	var addFundsMcid1 cid.Cid
	var addFundsMcid2 cid.Cid
	go func() {
		defer addFundsSent.Done()

		// Request add funds - should block until create channel has completed
		var err error
		addFundsCh1, addFundsMcid1, err = mgr.GetPaych(ctx, from, to, addFundsAmt1, onChainNoReserve)
		require.NoError(t, err)
	}()

	go func() {
		defer addFundsSent.Done()

		// Request add funds again - should merge with waiting add funds request
		var err error
		addFundsCh2, addFundsMcid2, err = mgr.GetPaych(ctx, from, to, addFundsAmt2, onChainReserve)
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
	require.Equal(t, lotusinit.Address, createMsg.Message.To)
	require.Equal(t, createAmt, createMsg.Message.Value)

	// Check merged add funds amount is the sum of the individual
	// amounts
	addFundsMsg := mock.pushedMessages(addFundsMcid1)
	require.Equal(t, from, addFundsMsg.Message.From)
	require.Equal(t, ch, addFundsMsg.Message.To)
	require.Equal(t, types.BigAdd(addFundsAmt1, addFundsAmt2), addFundsMsg.Message.Value)
}

func TestPaychGetMergePrefundAndReservePrefunded(t *testing.T) {
	ctx := context.Background()
	store := NewStore(ds_sync.MutexWrap(ds.NewMapDatastore()))

	ch := tutils.NewIDAddr(t, 100)
	from := tutils.NewIDAddr(t, 101)
	to := tutils.NewIDAddr(t, 102)

	mock := newMockManagerAPI()
	defer mock.close()

	act := &types.Actor{
		Code:    builtin.AccountActorCodeID,
		Head:    cid.Cid{},
		Nonce:   0,
		Balance: types.NewInt(20),
	}
	mock.setPaychState(ch, act, paychmock.NewMockPayChState(from, to, abi.ChainEpoch(0), make(map[uint64]paych.LaneState)))

	mgr, err := newManager(store, mock)
	require.NoError(t, err)

	// Send create message for a channel with value 10
	createAmt := big.NewInt(10)
	_, createMsgCid, err := mgr.GetPaych(ctx, from, to, createAmt, onChainNoReserve)
	require.NoError(t, err)

	// Queue up two add funds requests behind create channel
	var addFundsSent sync.WaitGroup
	addFundsSent.Add(2)

	addFundsAmt1 := big.NewInt(5) // 1 prefunds
	addFundsAmt2 := big.NewInt(3) // 2 reserves
	var addFundsCh1 address.Address
	var addFundsCh2 address.Address
	var addFundsMcid1 cid.Cid
	var addFundsMcid2 cid.Cid
	go func() {
		defer addFundsSent.Done()

		// Request add funds - should block until create channel has completed
		var err error
		addFundsCh1, addFundsMcid1, err = mgr.GetPaych(ctx, from, to, addFundsAmt1, onChainNoReserve)
		require.NoError(t, err)
	}()

	go func() {
		defer addFundsSent.Done()

		// Request add funds again - should merge with waiting add funds request
		var err error
		addFundsCh2, addFundsMcid2, err = mgr.GetPaych(ctx, from, to, addFundsAmt2, onChainReserve)
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
	require.NotEqual(t, cid.Undef, addFundsMcid1)
	require.Equal(t, cid.Undef, addFundsMcid2)

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
	require.Equal(t, lotusinit.Address, createMsg.Message.To)
	require.Equal(t, createAmt, createMsg.Message.Value)

	// Check merged add funds amount is the sum of the individual
	// amounts
	addFundsMsg := mock.pushedMessages(addFundsMcid1)
	require.Equal(t, from, addFundsMsg.Message.From)
	require.Equal(t, ch, addFundsMsg.Message.To)
	require.Equal(t, addFundsAmt1, addFundsMsg.Message.Value)
}

func TestPaychGetMergePrefundAndReservePrefundedOneOffchain(t *testing.T) {
	ctx := context.Background()
	store := NewStore(ds_sync.MutexWrap(ds.NewMapDatastore()))

	ch := tutils.NewIDAddr(t, 100)
	from := tutils.NewIDAddr(t, 101)
	to := tutils.NewIDAddr(t, 102)

	mock := newMockManagerAPI()
	defer mock.close()

	act := &types.Actor{
		Code:    builtin.AccountActorCodeID,
		Head:    cid.Cid{},
		Nonce:   0,
		Balance: types.NewInt(20),
	}
	mock.setPaychState(ch, act, paychmock.NewMockPayChState(from, to, abi.ChainEpoch(0), make(map[uint64]paych.LaneState)))

	mgr, err := newManager(store, mock)
	require.NoError(t, err)

	// Send create message for a channel with value 10
	createAmt := big.NewInt(10)
	_, createMsgCid, err := mgr.GetPaych(ctx, from, to, createAmt, onChainNoReserve)
	require.NoError(t, err)

	// Queue up two add funds requests behind create channel
	var addFundsSent sync.WaitGroup
	addFundsSent.Add(2)

	addFundsAmt1 := big.NewInt(5) // 1 reserves
	addFundsAmt2 := big.NewInt(3) // 2 reserves
	var addFundsCh1 address.Address
	var addFundsCh2 address.Address
	var addFundsMcid1 cid.Cid
	var addFundsMcid2 cid.Cid
	go func() {
		defer addFundsSent.Done()

		// Request add funds - should block until create channel has completed
		var err error
		addFundsCh1, addFundsMcid1, err = mgr.GetPaych(ctx, from, to, addFundsAmt1, offChainReserve)
		require.NoError(t, err)
	}()

	go func() {
		defer addFundsSent.Done()

		// Request add funds again - should merge with waiting add funds request
		var err error
		addFundsCh2, addFundsMcid2, err = mgr.GetPaych(ctx, from, to, addFundsAmt2, onChainReserve)
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
	require.Equal(t, cid.Undef, addFundsMcid1)
	require.Equal(t, cid.Undef, addFundsMcid2)

	// Make sure that one create channel message was sent
	require.Equal(t, 1, mock.pushedMessageCount())

	// Check create message amount is correct
	createMsg := mock.pushedMessages(createMsgCid)
	require.Equal(t, from, createMsg.Message.From)
	require.Equal(t, lotusinit.Address, createMsg.Message.To)
	require.Equal(t, createAmt, createMsg.Message.Value)
}

func TestPaychGetMergePrefundAndReservePrefundedBothOffchainOneFail(t *testing.T) {
	ctx := context.Background()
	store := NewStore(ds_sync.MutexWrap(ds.NewMapDatastore()))

	ch := tutils.NewIDAddr(t, 100)
	from := tutils.NewIDAddr(t, 101)
	to := tutils.NewIDAddr(t, 102)

	mock := newMockManagerAPI()
	defer mock.close()

	act := &types.Actor{
		Code:    builtin.AccountActorCodeID,
		Head:    cid.Cid{},
		Nonce:   0,
		Balance: types.NewInt(20),
	}
	mock.setPaychState(ch, act, paychmock.NewMockPayChState(from, to, abi.ChainEpoch(0), make(map[uint64]paych.LaneState)))

	mgr, err := newManager(store, mock)
	require.NoError(t, err)

	// Send create message for a channel with value 10
	createAmt := big.NewInt(10)
	_, createMsgCid, err := mgr.GetPaych(ctx, from, to, createAmt, onChainNoReserve)
	require.NoError(t, err)

	// Queue up two add funds requests behind create channel
	var addFundsSent sync.WaitGroup
	addFundsSent.Add(2)

	addFundsAmt1 := big.NewInt(5) // 1 reserves
	addFundsAmt2 := big.NewInt(6) // 2 reserves too much
	var addFundsCh1 address.Address
	var addFundsCh2 address.Address
	var addFundsMcid1 cid.Cid
	var addFundsMcid2 cid.Cid
	go func() {
		defer addFundsSent.Done()

		// Request add funds - should block until create channel has completed
		var err error
		addFundsCh1, addFundsMcid1, err = mgr.GetPaych(ctx, from, to, addFundsAmt1, offChainReserve)
		require.NoError(t, err)
	}()

	go func() {
		defer addFundsSent.Done()

		// Request add funds again - should merge with waiting add funds request
		var err error
		addFundsCh2, addFundsMcid2, err = mgr.GetPaych(ctx, from, to, addFundsAmt2, offChainReserve)
		require.Error(t, err)
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
	require.Equal(t, cid.Undef, addFundsMcid1)
	require.Equal(t, cid.Undef, addFundsMcid2)

	// Make sure that one create channel message was sent
	require.Equal(t, 1, mock.pushedMessageCount())

	// Check create message amount is correct
	createMsg := mock.pushedMessages(createMsgCid)
	require.Equal(t, from, createMsg.Message.From)
	require.Equal(t, lotusinit.Address, createMsg.Message.To)
	require.Equal(t, createAmt, createMsg.Message.Value)
}

func TestPaychGetMergePrefundAndReserveOneOffchainOneFail(t *testing.T) {
	ctx := context.Background()
	store := NewStore(ds_sync.MutexWrap(ds.NewMapDatastore()))

	ch := tutils.NewIDAddr(t, 100)
	from := tutils.NewIDAddr(t, 101)
	to := tutils.NewIDAddr(t, 102)

	mock := newMockManagerAPI()
	defer mock.close()

	act := &types.Actor{
		Code:    builtin.AccountActorCodeID,
		Head:    cid.Cid{},
		Nonce:   0,
		Balance: types.NewInt(20),
	}
	mock.setPaychState(ch, act, paychmock.NewMockPayChState(from, to, abi.ChainEpoch(0), make(map[uint64]paych.LaneState)))

	mgr, err := newManager(store, mock)
	require.NoError(t, err)

	// Send create message for a channel with value 10
	createAmt := big.NewInt(10)
	_, createMsgCid, err := mgr.GetPaych(ctx, from, to, createAmt, onChainReserve)
	require.NoError(t, err)

	// Queue up two add funds requests behind create channel
	var addFundsSent sync.WaitGroup
	addFundsSent.Add(2)

	addFundsAmt1 := big.NewInt(5) // 1 reserves
	addFundsAmt2 := big.NewInt(6) // 2 reserves
	var addFundsCh1 address.Address
	var addFundsCh2 address.Address
	var addFundsMcid1 cid.Cid
	var addFundsMcid2 cid.Cid
	go func() {
		defer addFundsSent.Done()

		// Request add funds - should block until create channel has completed
		var err error
		addFundsCh1, addFundsMcid1, err = mgr.GetPaych(ctx, from, to, addFundsAmt1, onChainReserve)
		require.NoError(t, err)
	}()

	go func() {
		defer addFundsSent.Done()

		// Request add funds again - should merge with waiting add funds request
		var err error
		addFundsCh2, addFundsMcid2, err = mgr.GetPaych(ctx, from, to, addFundsAmt2, offChainReserve)
		require.Error(t, err)
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
	require.NotEqual(t, cid.Undef, addFundsMcid1)
	require.Equal(t, cid.Undef, addFundsMcid2)

	// Make sure that one create channel message was sent
	require.Equal(t, 2, mock.pushedMessageCount())

	// Check create message amount is correct
	createMsg := mock.pushedMessages(createMsgCid)
	require.Equal(t, from, createMsg.Message.From)
	require.Equal(t, lotusinit.Address, createMsg.Message.To)
	require.Equal(t, createAmt, createMsg.Message.Value)

	// Check merged add funds amount is the sum of the individual
	// amounts
	addFundsMsg := mock.pushedMessages(addFundsMcid1)
	require.Equal(t, from, addFundsMsg.Message.From)
	require.Equal(t, ch, addFundsMsg.Message.To)
	require.Equal(t, addFundsAmt1, addFundsMsg.Message.Value)
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

	act := &types.Actor{
		Code:    builtin.AccountActorCodeID,
		Head:    cid.Cid{},
		Nonce:   0,
		Balance: types.NewInt(20),
	}
	mock.setPaychState(ch, act, paychmock.NewMockPayChState(from, to, abi.ChainEpoch(0), make(map[uint64]paych.LaneState)))

	mgr, err := newManager(store, mock)
	require.NoError(t, err)

	// Send create message for a channel with value 10
	createAmt := big.NewInt(10)
	_, createMsgCid, err := mgr.GetPaych(ctx, from, to, createAmt, onChainReserve)
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
		_, _, addFundsErr1 = mgr.GetPaych(addFundsCtx1, from, to, addFundsAmt1, onChainReserve)
	}()

	go func() {
		defer addFundsSent.Done()

		// Request add funds again - should merge with waiting add funds request
		var err error
		addFundsCh2, addFundsMcid2, err = mgr.GetPaych(ctx, from, to, addFundsAmt2, onChainReserve)
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
	require.Equal(t, lotusinit.Address, createMsg.Message.To)
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
	_, createMsgCid, err := mgr.GetPaych(ctx, from, to, createAmt, onChainReserve)
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
		_, _, addFundsErr1 = mgr.GetPaych(addFundsCtx1, from, to, big.NewInt(5), onChainReserve)
	}()

	go func() {
		defer addFundsSent.Done()

		// Request add funds again - should merge with waiting add funds request
		_, _, addFundsErr2 = mgr.GetPaych(addFundsCtx2, from, to, big.NewInt(3), onChainReserve)
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
	require.Equal(t, lotusinit.Address, createMsg.Message.To)
	require.Equal(t, createAmt, createMsg.Message.Value)
}

// TestPaychAvailableFunds tests that PaychAvailableFunds returns the correct
// channel state
func TestPaychAvailableFunds(t *testing.T) {
	ctx := context.Background()
	store := NewStore(ds_sync.MutexWrap(ds.NewMapDatastore()))

	fromKeyPrivate, fromKeyPublic := testGenerateKeyPair(t)
	ch := tutils.NewIDAddr(t, 100)
	from := tutils.NewSECP256K1Addr(t, string(fromKeyPublic))
	to := tutils.NewIDAddr(t, 102)
	fromAcct := tutils.NewActorAddr(t, "fromAct")
	toAcct := tutils.NewActorAddr(t, "toAct")

	mock := newMockManagerAPI()
	defer mock.close()

	mgr, err := newManager(store, mock)
	require.NoError(t, err)

	// No channel created yet so available funds should be all zeroes
	av, err := mgr.AvailableFundsByFromTo(ctx, from, to)
	require.NoError(t, err)
	require.Nil(t, av.Channel)
	require.Nil(t, av.PendingWaitSentinel)
	require.EqualValues(t, 0, av.ConfirmedAmt.Int64())
	require.EqualValues(t, 0, av.PendingAmt.Int64())
	require.EqualValues(t, 0, av.QueuedAmt.Int64())
	require.EqualValues(t, 0, av.VoucherReedeemedAmt.Int64())

	// Send create message for a channel with value 10
	createAmt := big.NewInt(10)
	_, createMsgCid, err := mgr.GetPaych(ctx, from, to, createAmt, onChainReserve)
	require.NoError(t, err)

	// Available funds should reflect create channel message sent
	av, err = mgr.AvailableFundsByFromTo(ctx, from, to)
	require.NoError(t, err)
	require.Nil(t, av.Channel)
	require.EqualValues(t, 0, av.ConfirmedAmt.Int64())
	require.EqualValues(t, createAmt, av.PendingAmt)
	require.EqualValues(t, 0, av.QueuedAmt.Int64())
	require.EqualValues(t, 0, av.VoucherReedeemedAmt.Int64())
	// Should now have a pending wait sentinel
	require.NotNil(t, av.PendingWaitSentinel)

	// Queue up an add funds request behind create channel
	var addFundsSent sync.WaitGroup
	addFundsSent.Add(1)

	addFundsAmt := big.NewInt(5)
	var addFundsMcid cid.Cid
	go func() {
		defer addFundsSent.Done()

		// Request add funds - should block until create channel has completed
		var err error
		_, addFundsMcid, err = mgr.GetPaych(ctx, from, to, addFundsAmt, onChainReserve)
		require.NoError(t, err)
	}()

	// Wait for add funds request to be queued up
	waitForQueueSize(t, mgr, from, to, 1)

	// Available funds should now include queued funds
	av, err = mgr.AvailableFundsByFromTo(ctx, from, to)
	require.NoError(t, err)
	require.Nil(t, av.Channel)
	require.NotNil(t, av.PendingWaitSentinel)
	require.EqualValues(t, 0, av.ConfirmedAmt.Int64())
	// create amount is still pending
	require.EqualValues(t, createAmt, av.PendingAmt)
	// queued amount now includes add funds amount
	require.EqualValues(t, addFundsAmt, av.QueuedAmt)
	require.EqualValues(t, 0, av.VoucherReedeemedAmt.Int64())

	// Create channel in state
	mock.setAccountAddress(fromAcct, from)
	mock.setAccountAddress(toAcct, to)
	act := &types.Actor{
		Code:    builtin.AccountActorCodeID,
		Head:    cid.Cid{},
		Nonce:   0,
		Balance: createAmt,
	}
	mock.setPaychState(ch, act, paychmock.NewMockPayChState(fromAcct, toAcct, abi.ChainEpoch(0), make(map[uint64]paych.LaneState)))
	// Send create channel response
	response := testChannelResponse(t, ch)
	mock.receiveMsgResponse(createMsgCid, response)

	// Wait for create channel response
	chres, err := mgr.GetPaychWaitReady(ctx, *av.PendingWaitSentinel)
	require.NoError(t, err)
	require.Equal(t, ch, chres)

	// Wait for add funds request to be sent
	addFundsSent.Wait()

	// Available funds should now include the channel and also a wait sentinel
	// for the add funds message
	av, err = mgr.AvailableFunds(ctx, ch)
	require.NoError(t, err)
	require.NotNil(t, av.Channel)
	require.NotNil(t, av.PendingWaitSentinel)
	// create amount is now confirmed
	require.EqualValues(t, createAmt, av.ConfirmedAmt)
	// add funds amount it now pending
	require.EqualValues(t, addFundsAmt, av.PendingAmt)
	require.EqualValues(t, 0, av.QueuedAmt.Int64())
	require.EqualValues(t, 0, av.VoucherReedeemedAmt.Int64())

	// Send success add funds response
	mock.receiveMsgResponse(addFundsMcid, types.MessageReceipt{
		ExitCode: 0,
		Return:   []byte{},
	})

	// Wait for add funds response
	_, err = mgr.GetPaychWaitReady(ctx, *av.PendingWaitSentinel)
	require.NoError(t, err)

	// Available funds should no longer have a wait sentinel
	av, err = mgr.AvailableFunds(ctx, ch)
	require.NoError(t, err)
	require.NotNil(t, av.Channel)
	require.Nil(t, av.PendingWaitSentinel)
	// confirmed amount now includes create and add funds amounts
	require.EqualValues(t, types.BigAdd(createAmt, addFundsAmt), av.ConfirmedAmt)
	require.EqualValues(t, 0, av.PendingAmt.Int64())
	require.EqualValues(t, 0, av.QueuedAmt.Int64())
	require.EqualValues(t, 0, av.VoucherReedeemedAmt.Int64())

	// Add some vouchers
	voucherAmt1 := types.NewInt(3)
	voucher := createTestVoucher(t, ch, 1, 1, voucherAmt1, fromKeyPrivate)
	_, err = mgr.AddVoucherOutbound(ctx, ch, voucher, nil, types.NewInt(0))
	require.NoError(t, err)

	voucherAmt2 := types.NewInt(2)
	voucher = createTestVoucher(t, ch, 2, 1, voucherAmt2, fromKeyPrivate)
	_, err = mgr.AddVoucherOutbound(ctx, ch, voucher, nil, types.NewInt(0))
	require.NoError(t, err)

	av, err = mgr.AvailableFunds(ctx, ch)
	require.NoError(t, err)
	require.NotNil(t, av.Channel)
	require.Nil(t, av.PendingWaitSentinel)
	require.EqualValues(t, types.BigAdd(createAmt, addFundsAmt), av.ConfirmedAmt)
	require.EqualValues(t, 0, av.PendingAmt.Int64())
	require.EqualValues(t, 0, av.QueuedAmt.Int64())
	// voucher redeemed amount now includes vouchers
	require.EqualValues(t, types.BigAdd(voucherAmt1, voucherAmt2), av.VoucherReedeemedAmt)
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
