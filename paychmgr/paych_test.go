package paychmgr

import (
	"bytes"
	"context"
	"testing"

	"github.com/ipfs/go-cid"
	ds "github.com/ipfs/go-datastore"
	ds_sync "github.com/ipfs/go-datastore/sync"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	paychtypes "github.com/filecoin-project/go-state-types/builtin/v8/paych"
	"github.com/filecoin-project/go-state-types/crypto"
	"github.com/filecoin-project/specs-actors/v2/actors/builtin"
	tutils "github.com/filecoin-project/specs-actors/v2/support/testing"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/actors/builtin/paych"
	paychmock "github.com/filecoin-project/lotus/chain/actors/builtin/paych/mock"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/lib/sigs"
	_ "github.com/filecoin-project/lotus/lib/sigs/secp"
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
	mock.setAccountAddress(fromAcct, from)
	mock.setAccountAddress(toAcct, to)

	tcases := []struct {
		name          string
		expectError   bool
		key           []byte
		actorBalance  big.Int
		voucherAmount big.Int
		voucherLane   uint64
		voucherNonce  uint64
		laneStates    map[uint64]paych.LaneState
	}{{
		name:          "passes when voucher amount < balance",
		key:           fromKeyPrivate,
		actorBalance:  big.NewInt(10),
		voucherAmount: big.NewInt(5),
	}, {
		name:          "fails when funds too low",
		expectError:   true,
		key:           fromKeyPrivate,
		actorBalance:  big.NewInt(5),
		voucherAmount: big.NewInt(10),
	}, {
		name:          "fails when invalid signature",
		expectError:   true,
		key:           randKeyPrivate,
		actorBalance:  big.NewInt(10),
		voucherAmount: big.NewInt(5),
	}, {
		name:          "fails when signed by channel To account (instead of From account)",
		expectError:   true,
		key:           toKeyPrivate,
		actorBalance:  big.NewInt(10),
		voucherAmount: big.NewInt(5),
	}, {
		name:          "fails when nonce too low",
		expectError:   true,
		key:           fromKeyPrivate,
		actorBalance:  big.NewInt(10),
		voucherAmount: big.NewInt(5),
		voucherLane:   1,
		voucherNonce:  2,
		laneStates: map[uint64]paych.LaneState{
			1: paychmock.NewMockLaneState(big.NewInt(2), 3),
		},
	}, {
		name:          "passes when nonce higher",
		key:           fromKeyPrivate,
		actorBalance:  big.NewInt(10),
		voucherAmount: big.NewInt(5),
		voucherLane:   1,
		voucherNonce:  3,
		laneStates: map[uint64]paych.LaneState{
			1: paychmock.NewMockLaneState(big.NewInt(2), 2),
		},
	}, {
		name:          "passes when nonce for different lane",
		key:           fromKeyPrivate,
		actorBalance:  big.NewInt(10),
		voucherAmount: big.NewInt(5),
		voucherLane:   2,
		voucherNonce:  2,
		laneStates: map[uint64]paych.LaneState{
			1: paychmock.NewMockLaneState(big.NewInt(2), 3),
		},
	}, {
		name:          "fails when voucher has higher nonce but lower value than lane state",
		expectError:   true,
		key:           fromKeyPrivate,
		actorBalance:  big.NewInt(10),
		voucherAmount: big.NewInt(5),
		voucherLane:   1,
		voucherNonce:  3,
		laneStates: map[uint64]paych.LaneState{
			1: paychmock.NewMockLaneState(big.NewInt(6), 2),
		},
	}, {
		// voucher supersedes lane 1 redeemed so
		// lane 1 effective redeemed = voucher amount
		//
		// required balance = voucher amt
		//                  = 7
		// So required balance: 7 < actor balance: 10
		name:          "passes when voucher total redeemed <= balance",
		key:           fromKeyPrivate,
		actorBalance:  big.NewInt(10),
		voucherAmount: big.NewInt(6),
		voucherLane:   1,
		voucherNonce:  2,
		laneStates: map[uint64]paych.LaneState{
			// Lane 1 (same as voucher lane 1)
			1: paychmock.NewMockLaneState(big.NewInt(4), 1),
		},
	}, {
		// required balance = total redeemed
		//                  = 6 (voucher lane 1) + 5 (lane 2)
		//                  = 11
		// So required balance: 11 > actor balance: 10
		name:          "fails when voucher total redeemed > balance",
		expectError:   true,
		key:           fromKeyPrivate,
		actorBalance:  big.NewInt(10),
		voucherAmount: big.NewInt(6),
		voucherLane:   1,
		voucherNonce:  1,
		laneStates: map[uint64]paych.LaneState{
			// Lane 2 (different from voucher lane 1)
			2: paychmock.NewMockLaneState(big.NewInt(5), 1),
		},
	}, {
		// voucher supersedes lane 1 redeemed so
		// lane 1 effective redeemed = voucher amount
		//
		// required balance = total redeemed
		//                  = 6 (new voucher lane 1) + 5 (lane 2)
		//                  = 11
		// So required balance: 11 > actor balance: 10
		name:          "fails when voucher total redeemed > balance",
		expectError:   true,
		key:           fromKeyPrivate,
		actorBalance:  big.NewInt(10),
		voucherAmount: big.NewInt(6),
		voucherLane:   1,
		voucherNonce:  2,
		laneStates: map[uint64]paych.LaneState{
			// Lane 1 (superseded by new voucher in voucher lane 1)
			1: paychmock.NewMockLaneState(big.NewInt(5), 1),
			// Lane 2 (different from voucher lane 1)
			2: paychmock.NewMockLaneState(big.NewInt(5), 1),
		},
	}, {
		// voucher supersedes lane 1 redeemed so
		// lane 1 effective redeemed = voucher amount
		//
		// required balance = total redeemed
		//                  = 5 (new voucher lane 1) + 5 (lane 2)
		//                  = 10
		// So required balance: 10 <= actor balance: 10
		name:          "passes when voucher total redeemed <= balance",
		expectError:   false,
		key:           fromKeyPrivate,
		actorBalance:  big.NewInt(10),
		voucherAmount: big.NewInt(5),
		voucherLane:   1,
		voucherNonce:  2,
		laneStates: map[uint64]paych.LaneState{
			// Lane 1 (superseded by new voucher in voucher lane 1)
			1: paychmock.NewMockLaneState(big.NewInt(4), 1),
			// Lane 2 (different from voucher lane 1)
			2: paychmock.NewMockLaneState(big.NewInt(5), 1),
		},
	}}

	for _, tcase := range tcases {
		tcase := tcase
		t.Run(tcase.name, func(t *testing.T) {
			// Create an actor for the channel with the test case balance
			act := &types.Actor{
				Code:    builtin.AccountActorCodeID,
				Head:    cid.Cid{},
				Nonce:   0,
				Balance: tcase.actorBalance,
			}

			mock.setPaychState(ch, act, paychmock.NewMockPayChState(
				fromAcct, toAcct, abi.ChainEpoch(0), tcase.laneStates))

			// Create a manager
			store := NewStore(ds_sync.MutexWrap(ds.NewMapDatastore()))
			mgr, err := newManager(store, mock)
			require.NoError(t, err)

			// Add channel To address to wallet
			mock.addWalletAddress(to)

			// Create the test case signed voucher
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

func TestCreateVoucher(t *testing.T) {
	ctx := context.Background()

	// Set up a manager with a single payment channel
	s := testSetupMgrWithChannel(t)

	// Create a voucher in lane 1
	voucherLane1Amt := big.NewInt(5)
	voucher := paychtypes.SignedVoucher{
		Lane:   1,
		Amount: voucherLane1Amt,
	}
	res, err := s.mgr.CreateVoucher(ctx, s.ch, voucher)
	require.NoError(t, err)
	require.NotNil(t, res.Voucher)
	require.Equal(t, s.ch, res.Voucher.ChannelAddr)
	require.Equal(t, voucherLane1Amt, res.Voucher.Amount)
	require.EqualValues(t, 0, res.Shortfall.Int64())

	nonce := res.Voucher.Nonce

	// Create a voucher in lane 1 again, with a higher amount
	voucherLane1Amt = big.NewInt(8)
	voucher = paychtypes.SignedVoucher{
		Lane:   1,
		Amount: voucherLane1Amt,
	}
	res, err = s.mgr.CreateVoucher(ctx, s.ch, voucher)
	require.NoError(t, err)
	require.NotNil(t, res.Voucher)
	require.Equal(t, s.ch, res.Voucher.ChannelAddr)
	require.Equal(t, voucherLane1Amt, res.Voucher.Amount)
	require.EqualValues(t, 0, res.Shortfall.Int64())
	require.Equal(t, nonce+1, res.Voucher.Nonce)

	// Create a voucher in lane 2 that covers all the remaining funds
	// in the channel
	voucherLane2Amt := big.Sub(s.amt, voucherLane1Amt)
	voucher = paychtypes.SignedVoucher{
		Lane:   2,
		Amount: voucherLane2Amt,
	}
	res, err = s.mgr.CreateVoucher(ctx, s.ch, voucher)
	require.NoError(t, err)
	require.NotNil(t, res.Voucher)
	require.Equal(t, s.ch, res.Voucher.ChannelAddr)
	require.Equal(t, voucherLane2Amt, res.Voucher.Amount)
	require.EqualValues(t, 0, res.Shortfall.Int64())

	// Create a voucher in lane 2 that exceeds the remaining funds in the
	// channel
	voucherLane2Amt = big.Add(voucherLane2Amt, big.NewInt(1))
	voucher = paychtypes.SignedVoucher{
		Lane:   2,
		Amount: voucherLane2Amt,
	}
	res, err = s.mgr.CreateVoucher(ctx, s.ch, voucher)
	require.NoError(t, err)

	// Expect a shortfall value equal to the amount required to add the voucher
	// to the channel
	require.Nil(t, res.Voucher)
	require.EqualValues(t, 1, res.Shortfall.Int64())
}

func TestAddVoucherDelta(t *testing.T) {
	ctx := context.Background()

	// Set up a manager with a single payment channel
	s := testSetupMgrWithChannel(t)

	voucherLane := uint64(1)

	// Expect error when adding a voucher whose amount is less than minDelta
	minDelta := big.NewInt(2)
	nonce := uint64(1)
	voucherAmount := big.NewInt(1)
	sv := createTestVoucher(t, s.ch, voucherLane, nonce, voucherAmount, s.fromKeyPrivate)
	_, err := s.mgr.AddVoucherOutbound(ctx, s.ch, sv, nil, minDelta)
	require.Error(t, err)

	// Expect success when adding a voucher whose amount is equal to minDelta
	nonce++
	voucherAmount = big.NewInt(2)
	sv = createTestVoucher(t, s.ch, voucherLane, nonce, voucherAmount, s.fromKeyPrivate)
	delta, err := s.mgr.AddVoucherOutbound(ctx, s.ch, sv, nil, minDelta)
	require.NoError(t, err)
	require.EqualValues(t, delta.Int64(), 2)

	// Check that delta is correct when there's an existing voucher
	nonce++
	voucherAmount = big.NewInt(5)
	sv = createTestVoucher(t, s.ch, voucherLane, nonce, voucherAmount, s.fromKeyPrivate)
	delta, err = s.mgr.AddVoucherOutbound(ctx, s.ch, sv, nil, minDelta)
	require.NoError(t, err)
	require.EqualValues(t, delta.Int64(), 3)

	// Check that delta is correct when voucher added to a different lane
	nonce = uint64(1)
	voucherAmount = big.NewInt(6)
	voucherLane = uint64(2)
	sv = createTestVoucher(t, s.ch, voucherLane, nonce, voucherAmount, s.fromKeyPrivate)
	delta, err = s.mgr.AddVoucherOutbound(ctx, s.ch, sv, nil, minDelta)
	require.NoError(t, err)
	require.EqualValues(t, delta.Int64(), 6)
}

func TestAddVoucherNextLane(t *testing.T) {
	ctx := context.Background()

	// Set up a manager with a single payment channel
	s := testSetupMgrWithChannel(t)

	minDelta := big.NewInt(0)
	voucherAmount := big.NewInt(2)

	// Add a voucher in lane 2
	nonce := uint64(1)
	voucherLane := uint64(2)
	sv := createTestVoucher(t, s.ch, voucherLane, nonce, voucherAmount, s.fromKeyPrivate)
	_, err := s.mgr.AddVoucherOutbound(ctx, s.ch, sv, nil, minDelta)
	require.NoError(t, err)

	ci, err := s.mgr.GetChannelInfo(ctx, s.ch)
	require.NoError(t, err)
	require.EqualValues(t, ci.NextLane, 3)

	// Allocate a lane (should be lane 3)
	lane, err := s.mgr.AllocateLane(ctx, s.ch)
	require.NoError(t, err)
	require.EqualValues(t, lane, 3)

	ci, err = s.mgr.GetChannelInfo(ctx, s.ch)
	require.NoError(t, err)
	require.EqualValues(t, ci.NextLane, 4)

	// Add a voucher in lane 1
	voucherLane = uint64(1)
	sv = createTestVoucher(t, s.ch, voucherLane, nonce, voucherAmount, s.fromKeyPrivate)
	_, err = s.mgr.AddVoucherOutbound(ctx, s.ch, sv, nil, minDelta)
	require.NoError(t, err)

	ci, err = s.mgr.GetChannelInfo(ctx, s.ch)
	require.NoError(t, err)
	require.EqualValues(t, ci.NextLane, 4)

	// Add a voucher in lane 7
	voucherLane = uint64(7)
	sv = createTestVoucher(t, s.ch, voucherLane, nonce, voucherAmount, s.fromKeyPrivate)
	_, err = s.mgr.AddVoucherOutbound(ctx, s.ch, sv, nil, minDelta)
	require.NoError(t, err)

	ci, err = s.mgr.GetChannelInfo(ctx, s.ch)
	require.NoError(t, err)
	require.EqualValues(t, ci.NextLane, 8)
}

func TestAllocateLane(t *testing.T) {
	ctx := context.Background()

	// Set up a manager with a single payment channel
	s := testSetupMgrWithChannel(t)

	// First lane should be 0
	lane, err := s.mgr.AllocateLane(ctx, s.ch)
	require.NoError(t, err)
	require.EqualValues(t, lane, 0)

	// Next lane should be 1
	lane, err = s.mgr.AllocateLane(ctx, s.ch)
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
	mock.setAccountAddress(fromAcct, from)
	mock.setAccountAddress(toAcct, to)
	mock.addWalletAddress(to)

	store := NewStore(ds_sync.MutexWrap(ds.NewMapDatastore()))

	// Create a channel that will be retrieved from state
	actorBalance := big.NewInt(10)

	act := &types.Actor{
		Code:    builtin.AccountActorCodeID,
		Head:    cid.Cid{},
		Nonce:   0,
		Balance: actorBalance,
	}

	mock.setPaychState(ch, act, paychmock.NewMockPayChState(fromAcct, toAcct, abi.ChainEpoch(0), make(map[uint64]paych.LaneState)))

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
	lane, err := mgr.AllocateLane(ctx, ch)
	require.NoError(t, err)
	require.EqualValues(t, 3, lane)
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

	mock := newMockManagerAPI()

	mock.setAccountAddress(fromAcct, from)
	mock.setAccountAddress(toAcct, to)

	act := &types.Actor{
		Code:    builtin.AccountActorCodeID,
		Head:    cid.Cid{},
		Nonce:   0,
		Balance: types.NewInt(20),
	}
	mock.setPaychState(ch, act, paychmock.NewMockPayChState(fromAcct, toAcct, abi.ChainEpoch(0), make(map[uint64]paych.LaneState)))

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

func TestBestSpendable(t *testing.T) {
	ctx := context.Background()

	// Set up a manager with a single payment channel
	s := testSetupMgrWithChannel(t)

	// Add vouchers to lane 1 with amounts: [1, 2, 3]
	voucherLane := uint64(1)
	minDelta := big.NewInt(0)
	nonce := uint64(1)
	voucherAmount := big.NewInt(1)
	svL1V1 := createTestVoucher(t, s.ch, voucherLane, nonce, voucherAmount, s.fromKeyPrivate)
	_, err := s.mgr.AddVoucherInbound(ctx, s.ch, svL1V1, nil, minDelta)
	require.NoError(t, err)

	nonce++
	voucherAmount = big.NewInt(2)
	svL1V2 := createTestVoucher(t, s.ch, voucherLane, nonce, voucherAmount, s.fromKeyPrivate)
	_, err = s.mgr.AddVoucherInbound(ctx, s.ch, svL1V2, nil, minDelta)
	require.NoError(t, err)

	nonce++
	voucherAmount = big.NewInt(3)
	svL1V3 := createTestVoucher(t, s.ch, voucherLane, nonce, voucherAmount, s.fromKeyPrivate)
	_, err = s.mgr.AddVoucherInbound(ctx, s.ch, svL1V3, nil, minDelta)
	require.NoError(t, err)

	// Add voucher to lane 2 with amounts: [2]
	voucherLane = uint64(2)
	nonce = uint64(1)
	voucherAmount = big.NewInt(2)
	svL2V1 := createTestVoucher(t, s.ch, voucherLane, nonce, voucherAmount, s.fromKeyPrivate)
	_, err = s.mgr.AddVoucherInbound(ctx, s.ch, svL2V1, nil, minDelta)
	require.NoError(t, err)

	// Return success exit code from calls to check if voucher is spendable
	bsapi := newMockBestSpendableAPI(s.mgr)
	s.mock.setCallResponse(&api.InvocResult{
		MsgRct: &types.MessageReceipt{
			ExitCode: 0,
		},
	})

	// Verify best spendable vouchers on each lane
	vouchers, err := BestSpendableByLane(ctx, bsapi, s.ch)
	require.NoError(t, err)
	require.Len(t, vouchers, 2)

	vchr, ok := vouchers[1]
	require.True(t, ok)
	require.EqualValues(t, 3, vchr.Amount.Int64())

	vchr, ok = vouchers[2]
	require.True(t, ok)
	require.EqualValues(t, 2, vchr.Amount.Int64())

	// Submit voucher from lane 2
	_, err = s.mgr.SubmitVoucher(ctx, s.ch, svL2V1, nil, nil)
	require.NoError(t, err)

	// Best spendable voucher should no longer include lane 2
	// (because voucher has not been submitted)
	vouchers, err = BestSpendableByLane(ctx, bsapi, s.ch)
	require.NoError(t, err)
	require.Len(t, vouchers, 1)

	// Submit first voucher from lane 1
	_, err = s.mgr.SubmitVoucher(ctx, s.ch, svL1V1, nil, nil)
	require.NoError(t, err)

	// Best spendable voucher for lane 1 should still be highest value voucher
	vouchers, err = BestSpendableByLane(ctx, bsapi, s.ch)
	require.NoError(t, err)
	require.Len(t, vouchers, 1)

	vchr, ok = vouchers[1]
	require.True(t, ok)
	require.EqualValues(t, 3, vchr.Amount.Int64())
}

func TestCheckSpendable(t *testing.T) {
	ctx := context.Background()

	// Set up a manager with a single payment channel
	s := testSetupMgrWithChannel(t)

	// Create voucher with Extra
	voucherLane := uint64(1)
	nonce := uint64(1)
	voucherAmount := big.NewInt(1)
	voucher := createTestVoucher(t, s.ch, voucherLane, nonce, voucherAmount, s.fromKeyPrivate)

	// Add voucher
	minDelta := big.NewInt(0)
	_, err := s.mgr.AddVoucherInbound(ctx, s.ch, voucher, nil, minDelta)
	require.NoError(t, err)

	// Return success exit code from VM call, which indicates that voucher is
	// spendable
	successResponse := &api.InvocResult{
		MsgRct: &types.MessageReceipt{
			ExitCode: 0,
		},
	}
	s.mock.setCallResponse(successResponse)

	// Check that spendable is true
	secret := []byte("secret")
	spendable, err := s.mgr.CheckVoucherSpendable(ctx, s.ch, voucher, secret, nil)
	require.NoError(t, err)
	require.True(t, spendable)

	// Check that the secret was passed through correctly
	lastCall := s.mock.getLastCall()
	var p paychtypes.UpdateChannelStateParams
	err = p.UnmarshalCBOR(bytes.NewReader(lastCall.Params))
	require.NoError(t, err)
	require.Equal(t, secret, p.Secret)

	// Check that if VM call returns non-success exit code, spendable is false
	s.mock.setCallResponse(&api.InvocResult{
		MsgRct: &types.MessageReceipt{
			ExitCode: 1,
		},
	})
	spendable, err = s.mgr.CheckVoucherSpendable(ctx, s.ch, voucher, secret, nil)
	require.NoError(t, err)
	require.False(t, spendable)

	// Return success exit code (indicating voucher is spendable)
	s.mock.setCallResponse(successResponse)
	spendable, err = s.mgr.CheckVoucherSpendable(ctx, s.ch, voucher, secret, nil)
	require.NoError(t, err)
	require.True(t, spendable)

	// Check that voucher is no longer spendable once it has been submitted
	_, err = s.mgr.SubmitVoucher(ctx, s.ch, voucher, nil, nil)
	require.NoError(t, err)

	spendable, err = s.mgr.CheckVoucherSpendable(ctx, s.ch, voucher, secret, nil)
	require.NoError(t, err)
	require.False(t, spendable)
}

func TestSubmitVoucher(t *testing.T) {
	ctx := context.Background()

	// Set up a manager with a single payment channel
	s := testSetupMgrWithChannel(t)

	// Create voucher with Extra
	voucherLane := uint64(1)
	nonce := uint64(1)
	voucherAmount := big.NewInt(1)
	voucher := createTestVoucher(t, s.ch, voucherLane, nonce, voucherAmount, s.fromKeyPrivate)

	// Add voucher
	minDelta := big.NewInt(0)
	_, err := s.mgr.AddVoucherInbound(ctx, s.ch, voucher, nil, minDelta)
	require.NoError(t, err)

	// Submit voucher
	submitCid, err := s.mgr.SubmitVoucher(ctx, s.ch, voucher, nil, nil)
	require.NoError(t, err)

	// Check that the secret was passed through correctly
	msg := s.mock.pushedMessages(submitCid)
	var p paychtypes.UpdateChannelStateParams
	err = p.UnmarshalCBOR(bytes.NewReader(msg.Message.Params))
	require.NoError(t, err)

	// Submit a voucher without first adding it
	nonce++
	voucherAmount = big.NewInt(3)
	voucher = createTestVoucher(t, s.ch, voucherLane, nonce, voucherAmount, s.fromKeyPrivate)
	submitCid, err = s.mgr.SubmitVoucher(ctx, s.ch, voucher, nil, nil)
	require.NoError(t, err)

	msg = s.mock.pushedMessages(submitCid)
	var p3 paychtypes.UpdateChannelStateParams
	err = p3.UnmarshalCBOR(bytes.NewReader(msg.Message.Params))
	require.NoError(t, err)

	// Verify that vouchers are marked as submitted
	vis, err := s.mgr.ListVouchers(ctx, s.ch)
	require.NoError(t, err)
	require.Len(t, vis, 2)

	for _, vi := range vis {
		require.True(t, vi.Submitted)
	}

	// Attempting to submit the same voucher again should fail
	_, err = s.mgr.SubmitVoucher(ctx, s.ch, voucher, nil, nil)
	require.Error(t, err)
}

type testScaffold struct {
	mgr            *Manager
	mock           *mockManagerAPI
	ch             address.Address
	amt            big.Int
	fromAcct       address.Address
	fromKeyPrivate []byte
}

func testSetupMgrWithChannel(t *testing.T) *testScaffold {
	fromKeyPrivate, fromKeyPublic := testGenerateKeyPair(t)

	ch := tutils.NewIDAddr(t, 100)
	from := tutils.NewSECP256K1Addr(t, string(fromKeyPublic))
	to := tutils.NewSECP256K1Addr(t, "secpTo")
	fromAcct := tutils.NewActorAddr(t, "fromAct")
	toAcct := tutils.NewActorAddr(t, "toAct")

	mock := newMockManagerAPI()
	mock.setAccountAddress(fromAcct, from)
	mock.setAccountAddress(toAcct, to)

	// Create channel in state
	balance := big.NewInt(20)
	act := &types.Actor{
		Code:    builtin.AccountActorCodeID,
		Head:    cid.Cid{},
		Nonce:   0,
		Balance: balance,
	}
	mock.setPaychState(ch, act, paychmock.NewMockPayChState(fromAcct, toAcct, abi.ChainEpoch(0), make(map[uint64]paych.LaneState)))

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
	err = mgr.store.putChannelInfo(context.Background(), ci)
	require.NoError(t, err)

	// Add the from signing key to the wallet
	mock.addSigningKey(fromKeyPrivate)

	return &testScaffold{
		mgr:            mgr,
		mock:           mock,
		ch:             ch,
		amt:            balance,
		fromAcct:       fromAcct,
		fromKeyPrivate: fromKeyPrivate,
	}
}

func testGenerateKeyPair(t *testing.T) ([]byte, []byte) {
	priv, err := sigs.Generate(crypto.SigTypeSecp256k1)
	require.NoError(t, err)
	pub, err := sigs.ToPublic(crypto.SigTypeSecp256k1, priv)
	require.NoError(t, err)
	return priv, pub
}

func createTestVoucher(t *testing.T, ch address.Address, voucherLane uint64, nonce uint64, voucherAmount big.Int, key []byte) *paychtypes.SignedVoucher {
	sv := &paychtypes.SignedVoucher{
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

type mockBestSpendableAPI struct {
	mgr *Manager
}

func (m *mockBestSpendableAPI) PaychVoucherList(ctx context.Context, ch address.Address) ([]*paychtypes.SignedVoucher, error) {
	vi, err := m.mgr.ListVouchers(ctx, ch)
	if err != nil {
		return nil, err
	}

	out := make([]*paychtypes.SignedVoucher, len(vi))
	for k, v := range vi {
		out[k] = v.Voucher
	}

	return out, nil
}

func (m *mockBestSpendableAPI) PaychVoucherCheckSpendable(ctx context.Context, ch address.Address, voucher *paychtypes.SignedVoucher, secret []byte, proof []byte) (bool, error) {
	return m.mgr.CheckVoucherSpendable(ctx, ch, voucher, secret, proof)
}

func newMockBestSpendableAPI(mgr *Manager) BestSpendableAPI {
	return &mockBestSpendableAPI{mgr: mgr}
}
