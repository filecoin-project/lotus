package paychmgr

import (
	"context"
	"testing"

	"github.com/filecoin-project/specs-actors/actors/builtin"
	"github.com/filecoin-project/specs-actors/actors/util/adt"
	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/lotus/lib/sigs"
	"github.com/filecoin-project/specs-actors/actors/crypto"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/specs-actors/actors/abi/big"

	"github.com/filecoin-project/specs-actors/actors/abi"
	tutils "github.com/filecoin-project/specs-actors/support/testing"

	"github.com/filecoin-project/specs-actors/actors/builtin/paych"

	"github.com/filecoin-project/specs-actors/actors/builtin/account"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/chain/types"

	ds "github.com/ipfs/go-datastore"
	ds_sync "github.com/ipfs/go-datastore/sync"
)

func TestCheckVoucherValid(t *testing.T) {
	ctx := context.Background()

	fromKeyPrivate, fromKeyPublic := testGenerateKeyPair(t)
	toKeyPrivate, toKeyPublic := testGenerateKeyPair(t)
	randKeyPrivate, _ := testGenerateKeyPair(t)

	ch := tutils.NewIDAddr(t, 100)
	from := tutils.NewSECP256K1Addr(t, string(fromKeyPublic))
	to := tutils.NewSECP256K1Addr(t, string(toKeyPublic))
	fromAcct := tutils.NewActorAddr(t, "fromAct")
	toAcct := tutils.NewActorAddr(t, "toAct")

	mock := newMockManagerAPI()
	mock.setAccountState(fromAcct, account.State{Address: from})
	mock.setAccountState(toAcct, account.State{Address: to})

	tcases := []struct {
		name          string
		expectError   bool
		key           []byte
		actorBalance  big.Int
		toSend        big.Int
		voucherAmount big.Int
		voucherLane   uint64
		voucherNonce  uint64
		laneStates    map[uint64]paych.LaneState
	}{{
		name:          "passes when voucher amount < balance",
		key:           fromKeyPrivate,
		actorBalance:  big.NewInt(10),
		toSend:        big.NewInt(0),
		voucherAmount: big.NewInt(5),
	}, {
		name:          "fails when funds too low",
		expectError:   true,
		key:           fromKeyPrivate,
		actorBalance:  big.NewInt(5),
		toSend:        big.NewInt(0),
		voucherAmount: big.NewInt(10),
	}, {
		name:          "fails when invalid signature",
		expectError:   true,
		key:           randKeyPrivate,
		actorBalance:  big.NewInt(10),
		toSend:        big.NewInt(0),
		voucherAmount: big.NewInt(5),
	}, {
		name:          "fails when signed by channel To account (instead of From account)",
		expectError:   true,
		key:           toKeyPrivate,
		actorBalance:  big.NewInt(10),
		toSend:        big.NewInt(0),
		voucherAmount: big.NewInt(5),
	}, {
		name:          "fails when nonce too low",
		expectError:   true,
		key:           fromKeyPrivate,
		actorBalance:  big.NewInt(10),
		toSend:        big.NewInt(0),
		voucherAmount: big.NewInt(5),
		voucherLane:   1,
		voucherNonce:  2,
		laneStates: map[uint64]paych.LaneState{
			1: {
				Redeemed: big.NewInt(2),
				Nonce:    3,
			},
		},
	}, {
		name:          "passes when nonce higher",
		key:           fromKeyPrivate,
		actorBalance:  big.NewInt(10),
		toSend:        big.NewInt(0),
		voucherAmount: big.NewInt(5),
		voucherLane:   1,
		voucherNonce:  3,
		laneStates: map[uint64]paych.LaneState{
			1: {
				Redeemed: big.NewInt(2),
				Nonce:    2,
			},
		},
	}, {
		name:          "passes when nonce for different lane",
		key:           fromKeyPrivate,
		actorBalance:  big.NewInt(10),
		toSend:        big.NewInt(0),
		voucherAmount: big.NewInt(5),
		voucherLane:   2,
		voucherNonce:  2,
		laneStates: map[uint64]paych.LaneState{
			1: {
				Redeemed: big.NewInt(2),
				Nonce:    3,
			},
		},
	}, {
		name:          "fails when voucher has higher nonce but lower value than lane state",
		expectError:   true,
		key:           fromKeyPrivate,
		actorBalance:  big.NewInt(10),
		toSend:        big.NewInt(0),
		voucherAmount: big.NewInt(5),
		voucherLane:   1,
		voucherNonce:  3,
		laneStates: map[uint64]paych.LaneState{
			1: {
				Redeemed: big.NewInt(6),
				Nonce:    2,
			},
		},
	}, {
		name:          "fails when voucher + ToSend > balance",
		expectError:   true,
		key:           fromKeyPrivate,
		actorBalance:  big.NewInt(10),
		toSend:        big.NewInt(9),
		voucherAmount: big.NewInt(2),
	}, {
		// voucher supersedes lane 1 redeemed so
		// lane 1 effective redeemed = voucher amount
		//
		// required balance = toSend + total redeemed
		//                  = 1 + 6 (lane1)
		//                  = 7
		// So required balance: 7 < actor balance: 10
		name:          "passes when voucher + total redeemed <= balance",
		key:           fromKeyPrivate,
		actorBalance:  big.NewInt(10),
		toSend:        big.NewInt(1),
		voucherAmount: big.NewInt(6),
		voucherLane:   1,
		voucherNonce:  2,
		laneStates: map[uint64]paych.LaneState{
			// Lane 1 (same as voucher lane 1)
			1: {
				Redeemed: big.NewInt(4),
				Nonce:    1,
			},
		},
	}, {
		// required balance = toSend + total redeemed
		//                  = 1 + 4 (lane 2) + 6 (voucher lane 1)
		//                  = 11
		// So required balance: 11 > actor balance: 10
		name:          "fails when voucher + total redeemed > balance",
		expectError:   true,
		key:           fromKeyPrivate,
		actorBalance:  big.NewInt(10),
		toSend:        big.NewInt(1),
		voucherAmount: big.NewInt(6),
		voucherLane:   1,
		voucherNonce:  1,
		laneStates: map[uint64]paych.LaneState{
			// Lane 2 (different from voucher lane 1)
			2: {
				Redeemed: big.NewInt(4),
				Nonce:    1,
			},
		},
	}}

	for _, tcase := range tcases {
		tcase := tcase
		t.Run(tcase.name, func(t *testing.T) {
			store := NewStore(ds_sync.MutexWrap(ds.NewMapDatastore()))

			// Create an actor for the channel with the test case balance
			act := &types.Actor{
				Code:    builtin.AccountActorCodeID,
				Head:    cid.Cid{},
				Nonce:   0,
				Balance: tcase.actorBalance,
			}

			// Set the state of the channel's lanes
			laneStates, err := mock.storeLaneStates(tcase.laneStates)
			require.NoError(t, err)

			mock.setPaychState(ch, act, paych.State{
				From:            fromAcct,
				To:              toAcct,
				ToSend:          tcase.toSend,
				SettlingAt:      abi.ChainEpoch(0),
				MinSettleHeight: abi.ChainEpoch(0),
				LaneStates:      laneStates,
			})

			// Create a manager
			mgr, err := newManager(store, mock)
			require.NoError(t, err)

			// Add channel To address to wallet
			mock.addWalletAddress(to)

			// Create a signed voucher
			sv := createTestVoucher(t, ch, tcase.voucherLane, tcase.voucherNonce, tcase.voucherAmount, tcase.key)

			// Check the voucher's validity
			err = mgr.CheckVoucherValid(ctx, ch, sv)
			if tcase.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestCheckVoucherValidCountingAllLanes(t *testing.T) {
	ctx := context.Background()

	fromKeyPrivate, fromKeyPublic := testGenerateKeyPair(t)

	ch := tutils.NewIDAddr(t, 100)
	from := tutils.NewSECP256K1Addr(t, string(fromKeyPublic))
	to := tutils.NewSECP256K1Addr(t, "secpTo")
	fromAcct := tutils.NewActorAddr(t, "fromAct")
	toAcct := tutils.NewActorAddr(t, "toAct")
	minDelta := big.NewInt(0)

	mock := newMockManagerAPI()
	mock.setAccountState(fromAcct, account.State{Address: from})
	mock.setAccountState(toAcct, account.State{Address: to})

	store := NewStore(ds_sync.MutexWrap(ds.NewMapDatastore()))

	actorBalance := big.NewInt(10)
	toSend := big.NewInt(1)
	laneStates := map[uint64]paych.LaneState{
		1: {
			Nonce:    1,
			Redeemed: big.NewInt(3),
		},
		2: {
			Nonce:    1,
			Redeemed: big.NewInt(4),
		},
	}

	act := &types.Actor{
		Code:    builtin.AccountActorCodeID,
		Head:    cid.Cid{},
		Nonce:   0,
		Balance: actorBalance,
	}

	lsCid, err := mock.storeLaneStates(laneStates)
	require.NoError(t, err)
	mock.setPaychState(ch, act, paych.State{
		From:            fromAcct,
		To:              toAcct,
		ToSend:          toSend,
		SettlingAt:      abi.ChainEpoch(0),
		MinSettleHeight: abi.ChainEpoch(0),
		LaneStates:      lsCid,
	})

	mgr, err := newManager(store, mock)
	require.NoError(t, err)

	// Add channel To address to wallet
	mock.addWalletAddress(to)

	//
	// Should not be possible to add a voucher with a value such that
	// <total lane Redeemed> + toSend > <actor balance>
	//
	// lane 1 redeemed:                   3
	// voucher amount (lane 1):           6
	// lane 1 redeemed (with voucher):    6
	//
	// Lane 1:             6
	// Lane 2:             4
	// toSend:             1
	//                     --
	// total:              11
	//
	// actor balance is 10 so total is too high.
	//
	voucherLane := uint64(1)
	voucherNonce := uint64(2)
	voucherAmount := big.NewInt(6)
	sv := createTestVoucher(t, ch, voucherLane, voucherNonce, voucherAmount, fromKeyPrivate)
	err = mgr.CheckVoucherValid(ctx, ch, sv)
	require.Error(t, err)

	//
	// lane 1 redeemed:                   3
	// voucher amount (lane 1):           4
	// lane 1 redeemed (with voucher):    4
	//
	// Lane 1:             4
	// Lane 2:             4
	// toSend:             1
	//                     --
	// total:              9
	//
	// actor balance is 10 so total is ok.
	//
	voucherAmount = big.NewInt(4)
	sv = createTestVoucher(t, ch, voucherLane, voucherNonce, voucherAmount, fromKeyPrivate)
	err = mgr.CheckVoucherValid(ctx, ch, sv)
	require.NoError(t, err)

	// Add voucher to lane 1, so Lane 1 effective redeemed
	// (with first voucher) is now 4
	_, err = mgr.AddVoucherOutbound(ctx, ch, sv, nil, minDelta)
	require.NoError(t, err)

	//
	// lane 1 redeemed:                   4
	// voucher amount (lane 1):           6
	// lane 1 redeemed (with voucher):    6
	//
	// Lane 1:             6
	// Lane 2:             4
	// toSend:             1
	//                     --
	// total:              11
	//
	// actor balance is 10 so total is too high.
	//
	voucherNonce++
	voucherAmount = big.NewInt(6)
	sv = createTestVoucher(t, ch, voucherLane, voucherNonce, voucherAmount, fromKeyPrivate)
	err = mgr.CheckVoucherValid(ctx, ch, sv)
	require.Error(t, err)

	//
	// lane 1 redeemed:                   4
	// voucher amount (lane 1):           5
	// lane 1 redeemed (with voucher):    5
	//
	// Lane 1:             5
	// Lane 2:             4
	// toSend:             1
	//                     --
	// total:              10
	//
	// actor balance is 10 so total is ok.
	//
	voucherAmount = big.NewInt(5)
	sv = createTestVoucher(t, ch, voucherLane, voucherNonce, voucherAmount, fromKeyPrivate)
	err = mgr.CheckVoucherValid(ctx, ch, sv)
	require.NoError(t, err)
}

func TestAddVoucherDelta(t *testing.T) {
	ctx := context.Background()

	// Set up a manager with a single payment channel
	mgr, ch, fromKeyPrivate := testSetupMgrWithChannel(ctx, t)

	voucherLane := uint64(1)

	// Expect error when adding a voucher whose amount is less than minDelta
	minDelta := big.NewInt(2)
	nonce := uint64(1)
	voucherAmount := big.NewInt(1)
	sv := createTestVoucher(t, ch, voucherLane, nonce, voucherAmount, fromKeyPrivate)
	_, err := mgr.AddVoucherOutbound(ctx, ch, sv, nil, minDelta)
	require.Error(t, err)

	// Expect success when adding a voucher whose amount is equal to minDelta
	nonce++
	voucherAmount = big.NewInt(2)
	sv = createTestVoucher(t, ch, voucherLane, nonce, voucherAmount, fromKeyPrivate)
	delta, err := mgr.AddVoucherOutbound(ctx, ch, sv, nil, minDelta)
	require.NoError(t, err)
	require.EqualValues(t, delta.Int64(), 2)

	// Check that delta is correct when there's an existing voucher
	nonce++
	voucherAmount = big.NewInt(5)
	sv = createTestVoucher(t, ch, voucherLane, nonce, voucherAmount, fromKeyPrivate)
	delta, err = mgr.AddVoucherOutbound(ctx, ch, sv, nil, minDelta)
	require.NoError(t, err)
	require.EqualValues(t, delta.Int64(), 3)

	// Check that delta is correct when voucher added to a different lane
	nonce = uint64(1)
	voucherAmount = big.NewInt(6)
	voucherLane = uint64(2)
	sv = createTestVoucher(t, ch, voucherLane, nonce, voucherAmount, fromKeyPrivate)
	delta, err = mgr.AddVoucherOutbound(ctx, ch, sv, nil, minDelta)
	require.NoError(t, err)
	require.EqualValues(t, delta.Int64(), 6)
}

func TestAddVoucherNextLane(t *testing.T) {
	ctx := context.Background()

	// Set up a manager with a single payment channel
	mgr, ch, fromKeyPrivate := testSetupMgrWithChannel(ctx, t)

	minDelta := big.NewInt(0)
	voucherAmount := big.NewInt(2)

	// Add a voucher in lane 2
	nonce := uint64(1)
	voucherLane := uint64(2)
	sv := createTestVoucher(t, ch, voucherLane, nonce, voucherAmount, fromKeyPrivate)
	_, err := mgr.AddVoucherOutbound(ctx, ch, sv, nil, minDelta)
	require.NoError(t, err)

	ci, err := mgr.GetChannelInfo(ch)
	require.NoError(t, err)
	require.EqualValues(t, ci.NextLane, 3)

	// Allocate a lane (should be lane 3)
	lane, err := mgr.AllocateLane(ch)
	require.NoError(t, err)
	require.EqualValues(t, lane, 3)

	ci, err = mgr.GetChannelInfo(ch)
	require.NoError(t, err)
	require.EqualValues(t, ci.NextLane, 4)

	// Add a voucher in lane 1
	voucherLane = uint64(1)
	sv = createTestVoucher(t, ch, voucherLane, nonce, voucherAmount, fromKeyPrivate)
	_, err = mgr.AddVoucherOutbound(ctx, ch, sv, nil, minDelta)
	require.NoError(t, err)

	ci, err = mgr.GetChannelInfo(ch)
	require.NoError(t, err)
	require.EqualValues(t, ci.NextLane, 4)

	// Add a voucher in lane 7
	voucherLane = uint64(7)
	sv = createTestVoucher(t, ch, voucherLane, nonce, voucherAmount, fromKeyPrivate)
	_, err = mgr.AddVoucherOutbound(ctx, ch, sv, nil, minDelta)
	require.NoError(t, err)

	ci, err = mgr.GetChannelInfo(ch)
	require.NoError(t, err)
	require.EqualValues(t, ci.NextLane, 8)
}

func TestAllocateLane(t *testing.T) {
	ctx := context.Background()

	// Set up a manager with a single payment channel
	mgr, ch, _ := testSetupMgrWithChannel(ctx, t)

	// First lane should be 0
	lane, err := mgr.AllocateLane(ch)
	require.NoError(t, err)
	require.EqualValues(t, lane, 0)

	// Next lane should be 1
	lane, err = mgr.AllocateLane(ch)
	require.NoError(t, err)
	require.EqualValues(t, lane, 1)
}

func TestAllocateLaneWithExistingLaneState(t *testing.T) {
	ctx := context.Background()

	fromKeyPrivate, fromKeyPublic := testGenerateKeyPair(t)

	ch := tutils.NewIDAddr(t, 100)
	from := tutils.NewSECP256K1Addr(t, string(fromKeyPublic))
	to := tutils.NewSECP256K1Addr(t, "secpTo")
	fromAcct := tutils.NewActorAddr(t, "fromAct")
	toAcct := tutils.NewActorAddr(t, "toAct")

	mock := newMockManagerAPI()
	mock.setAccountState(fromAcct, account.State{Address: from})
	mock.setAccountState(toAcct, account.State{Address: to})
	mock.addWalletAddress(to)

	store := NewStore(ds_sync.MutexWrap(ds.NewMapDatastore()))

	// Create a channel that will be retrieved from state
	actorBalance := big.NewInt(10)
	toSend := big.NewInt(1)

	act := &types.Actor{
		Code:    builtin.AccountActorCodeID,
		Head:    cid.Cid{},
		Nonce:   0,
		Balance: actorBalance,
	}

	arr, err := adt.MakeEmptyArray(mock.store).Root()
	require.NoError(t, err)
	mock.setPaychState(ch, act, paych.State{
		From:            fromAcct,
		To:              toAcct,
		ToSend:          toSend,
		SettlingAt:      abi.ChainEpoch(0),
		MinSettleHeight: abi.ChainEpoch(0),
		LaneStates:      arr,
	})

	mgr, err := newManager(store, mock)
	require.NoError(t, err)

	// Create a voucher on lane 2
	// (also reads the channel from state and puts it in the store)
	voucherLane := uint64(2)
	minDelta := big.NewInt(0)
	nonce := uint64(2)
	voucherAmount := big.NewInt(5)
	sv := createTestVoucher(t, ch, voucherLane, nonce, voucherAmount, fromKeyPrivate)
	_, err = mgr.AddVoucherInbound(ctx, ch, sv, nil, minDelta)
	require.NoError(t, err)

	// Allocate lane should return the next lane (lane 3)
	lane, err := mgr.AllocateLane(ch)
	require.NoError(t, err)
	require.EqualValues(t, 3, lane)
}

func TestAddVoucherProof(t *testing.T) {
	ctx := context.Background()

	// Set up a manager with a single payment channel
	mgr, ch, fromKeyPrivate := testSetupMgrWithChannel(ctx, t)

	nonce := uint64(1)
	voucherAmount := big.NewInt(1)
	minDelta := big.NewInt(0)
	voucherAmount = big.NewInt(2)
	voucherLane := uint64(1)

	// Add a voucher with no proof
	var proof []byte
	sv := createTestVoucher(t, ch, voucherLane, nonce, voucherAmount, fromKeyPrivate)
	_, err := mgr.AddVoucherOutbound(ctx, ch, sv, nil, minDelta)
	require.NoError(t, err)

	// Expect one voucher with no proof
	ci, err := mgr.GetChannelInfo(ch)
	require.NoError(t, err)
	require.Len(t, ci.Vouchers, 1)
	require.Len(t, ci.Vouchers[0].Proof, 0)

	// Add same voucher with no proof
	voucherLane = uint64(1)
	_, err = mgr.AddVoucherOutbound(ctx, ch, sv, proof, minDelta)
	require.NoError(t, err)

	// Expect one voucher with no proof
	ci, err = mgr.GetChannelInfo(ch)
	require.NoError(t, err)
	require.Len(t, ci.Vouchers, 1)
	require.Len(t, ci.Vouchers[0].Proof, 0)

	// Add same voucher with proof
	proof = []byte{1}
	_, err = mgr.AddVoucherOutbound(ctx, ch, sv, proof, minDelta)
	require.NoError(t, err)

	// Should add proof to existing voucher
	ci, err = mgr.GetChannelInfo(ch)
	require.NoError(t, err)
	require.Len(t, ci.Vouchers, 1)
	require.Len(t, ci.Vouchers[0].Proof, 1)
}

func TestAddVoucherInboundWalletKey(t *testing.T) {
	ctx := context.Background()

	fromKeyPrivate, fromKeyPublic := testGenerateKeyPair(t)

	ch := tutils.NewIDAddr(t, 100)
	from := tutils.NewSECP256K1Addr(t, string(fromKeyPublic))
	to := tutils.NewSECP256K1Addr(t, "secpTo")
	fromAcct := tutils.NewActorAddr(t, "fromAct")
	toAcct := tutils.NewActorAddr(t, "toAct")

	// Create an actor for the channel in state
	act := &types.Actor{
		Code:    builtin.AccountActorCodeID,
		Head:    cid.Cid{},
		Nonce:   0,
		Balance: types.NewInt(20),
	}

	mock := newMockManagerAPI()
	arr, err := adt.MakeEmptyArray(mock.store).Root()
	require.NoError(t, err)
	mock.setAccountState(fromAcct, account.State{Address: from})
	mock.setAccountState(toAcct, account.State{Address: to})

	mock.setPaychState(ch, act, paych.State{
		From:            fromAcct,
		To:              toAcct,
		ToSend:          types.NewInt(0),
		SettlingAt:      abi.ChainEpoch(0),
		MinSettleHeight: abi.ChainEpoch(0),
		LaneStates:      arr,
	})

	// Create a manager
	store := NewStore(ds_sync.MutexWrap(ds.NewMapDatastore()))
	mgr, err := newManager(store, mock)
	require.NoError(t, err)

	// Add a voucher
	nonce := uint64(1)
	voucherLane := uint64(1)
	minDelta := big.NewInt(0)
	voucherAmount := big.NewInt(2)
	sv := createTestVoucher(t, ch, voucherLane, nonce, voucherAmount, fromKeyPrivate)
	_, err = mgr.AddVoucherInbound(ctx, ch, sv, nil, minDelta)

	// Should fail because there is no wallet key matching the channel To
	// address (ie, the channel is not "owned" by this node)
	require.Error(t, err)

	// Add wallet key for To address
	mock.addWalletAddress(to)

	// Add voucher again
	sv = createTestVoucher(t, ch, voucherLane, nonce, voucherAmount, fromKeyPrivate)
	_, err = mgr.AddVoucherInbound(ctx, ch, sv, nil, minDelta)

	// Should now pass because there is a wallet key matching the channel To
	// address
	require.NoError(t, err)
}

func TestNextNonceForLane(t *testing.T) {
	ctx := context.Background()

	// Set up a manager with a single payment channel
	mgr, ch, key := testSetupMgrWithChannel(ctx, t)

	// Expect next nonce for non-existent lane to be 1
	next, err := mgr.NextNonceForLane(ctx, ch, 1)
	require.NoError(t, err)
	require.EqualValues(t, next, 1)

	voucherAmount := big.NewInt(1)
	minDelta := big.NewInt(0)
	voucherAmount = big.NewInt(2)

	// Add vouchers such that we have
	// lane 1: nonce 2
	// lane 1: nonce 4
	voucherLane := uint64(1)
	for _, nonce := range []uint64{2, 4} {
		voucherAmount = big.Add(voucherAmount, big.NewInt(1))
		sv := createTestVoucher(t, ch, voucherLane, nonce, voucherAmount, key)
		_, err := mgr.AddVoucherOutbound(ctx, ch, sv, nil, minDelta)
		require.NoError(t, err)
	}

	// lane 2: nonce 7
	voucherLane = uint64(2)
	nonce := uint64(7)
	sv := createTestVoucher(t, ch, voucherLane, nonce, voucherAmount, key)
	_, err = mgr.AddVoucherOutbound(ctx, ch, sv, nil, minDelta)
	require.NoError(t, err)

	// Expect next nonce for lane 1 to be 5
	next, err = mgr.NextNonceForLane(ctx, ch, 1)
	require.NoError(t, err)
	require.EqualValues(t, next, 5)

	// Expect next nonce for lane 2 to be 8
	next, err = mgr.NextNonceForLane(ctx, ch, 2)
	require.NoError(t, err)
	require.EqualValues(t, next, 8)
}

func testSetupMgrWithChannel(ctx context.Context, t *testing.T) (*Manager, address.Address, []byte) {
	fromKeyPrivate, fromKeyPublic := testGenerateKeyPair(t)

	ch := tutils.NewIDAddr(t, 100)
	from := tutils.NewSECP256K1Addr(t, string(fromKeyPublic))
	to := tutils.NewSECP256K1Addr(t, "secpTo")
	fromAcct := tutils.NewActorAddr(t, "fromAct")
	toAcct := tutils.NewActorAddr(t, "toAct")

	mock := newMockManagerAPI()
	arr, err := adt.MakeEmptyArray(mock.store).Root()
	require.NoError(t, err)
	mock.setAccountState(fromAcct, account.State{Address: from})
	mock.setAccountState(toAcct, account.State{Address: to})

	// Create channel in state
	act := &types.Actor{
		Code:    builtin.AccountActorCodeID,
		Head:    cid.Cid{},
		Nonce:   0,
		Balance: big.NewInt(20),
	}
	mock.setPaychState(ch, act, paych.State{
		From:            fromAcct,
		To:              toAcct,
		ToSend:          big.NewInt(0),
		SettlingAt:      abi.ChainEpoch(0),
		MinSettleHeight: abi.ChainEpoch(0),
		LaneStates:      arr,
	})

	store := NewStore(ds_sync.MutexWrap(ds.NewMapDatastore()))
	mgr, err := newManager(store, mock)
	require.NoError(t, err)

	// Create the channel in the manager's store
	ci := &ChannelInfo{
		Channel:   &ch,
		Control:   fromAcct,
		Target:    toAcct,
		Direction: DirOutbound,
	}
	err = mgr.store.putChannelInfo(ci)
	require.NoError(t, err)

	return mgr, ch, fromKeyPrivate
}

func testGenerateKeyPair(t *testing.T) ([]byte, []byte) {
	priv, err := sigs.Generate(crypto.SigTypeSecp256k1)
	require.NoError(t, err)
	pub, err := sigs.ToPublic(crypto.SigTypeSecp256k1, priv)
	require.NoError(t, err)
	return priv, pub
}

func createTestVoucher(t *testing.T, ch address.Address, voucherLane uint64, nonce uint64, voucherAmount big.Int, key []byte) *paych.SignedVoucher {
	sv := &paych.SignedVoucher{
		ChannelAddr: ch,
		Lane:        voucherLane,
		Nonce:       nonce,
		Amount:      voucherAmount,
	}

	signingBytes, err := sv.SigningBytes()
	require.NoError(t, err)
	sig, err := sigs.Sign(crypto.SigTypeSecp256k1, key, signingBytes)
	require.NoError(t, err)
	sv.Signature = sig
	return sv
}
