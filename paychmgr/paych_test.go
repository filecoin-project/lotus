package paychmgr

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/filecoin-project/specs-actors/actors/builtin"
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
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/types"

	ds "github.com/ipfs/go-datastore"
	ds_sync "github.com/ipfs/go-datastore/sync"
)

type testPchState struct {
	actor *types.Actor
	state paych.State
}

type mockStateManager struct {
	lk           sync.Mutex
	accountState map[address.Address]account.State
	paychState   map[address.Address]testPchState
	response     *api.InvocResult
}

func newMockStateManager() *mockStateManager {
	return &mockStateManager{
		accountState: make(map[address.Address]account.State),
		paychState:   make(map[address.Address]testPchState),
	}
}

func (sm *mockStateManager) setAccountState(a address.Address, state account.State) {
	sm.lk.Lock()
	defer sm.lk.Unlock()
	sm.accountState[a] = state
}

func (sm *mockStateManager) setPaychState(a address.Address, actor *types.Actor, state paych.State) {
	sm.lk.Lock()
	defer sm.lk.Unlock()
	sm.paychState[a] = testPchState{actor, state}
}

func (sm *mockStateManager) LoadActorState(ctx context.Context, a address.Address, out interface{}, ts *types.TipSet) (*types.Actor, error) {
	sm.lk.Lock()
	defer sm.lk.Unlock()

	if outState, ok := out.(*account.State); ok {
		*outState = sm.accountState[a]
		return nil, nil
	}
	if outState, ok := out.(*paych.State); ok {
		info := sm.paychState[a]
		*outState = info.state
		return info.actor, nil
	}
	panic(fmt.Sprintf("unexpected state type %v", out))
}

func (sm *mockStateManager) Call(ctx context.Context, msg *types.Message, ts *types.TipSet) (*api.InvocResult, error) {
	return sm.response, nil
}

func TestPaychOutbound(t *testing.T) {
	ctx := context.Background()
	store := NewStore(ds_sync.MutexWrap(ds.NewMapDatastore()))

	ch := tutils.NewIDAddr(t, 100)
	from := tutils.NewIDAddr(t, 101)
	to := tutils.NewIDAddr(t, 102)
	fromAcct := tutils.NewIDAddr(t, 201)
	toAcct := tutils.NewIDAddr(t, 202)

	sm := newMockStateManager()
	sm.setAccountState(fromAcct, account.State{from})
	sm.setAccountState(toAcct, account.State{to})
	sm.setPaychState(ch, nil, paych.State{
		From:            fromAcct,
		To:              toAcct,
		ToSend:          big.NewInt(0),
		SettlingAt:      abi.ChainEpoch(0),
		MinSettleHeight: abi.ChainEpoch(0),
		LaneStates:      []*paych.LaneState{},
	})

	mgr := newManager(sm, store)
	err := mgr.TrackOutboundChannel(ctx, ch)
	require.NoError(t, err)

	ci, err := mgr.GetChannelInfo(ch)
	require.NoError(t, err)
	require.Equal(t, ci.Channel, ch)
	require.Equal(t, ci.Control, from)
	require.Equal(t, ci.Target, to)
	require.EqualValues(t, ci.Direction, DirOutbound)
	require.EqualValues(t, ci.NextLane, 0)
	require.Len(t, ci.Vouchers, 0)
}

func TestPaychInbound(t *testing.T) {
	ctx := context.Background()
	store := NewStore(ds_sync.MutexWrap(ds.NewMapDatastore()))

	ch := tutils.NewIDAddr(t, 100)
	from := tutils.NewIDAddr(t, 101)
	to := tutils.NewIDAddr(t, 102)
	fromAcct := tutils.NewIDAddr(t, 201)
	toAcct := tutils.NewIDAddr(t, 202)

	sm := newMockStateManager()
	sm.setAccountState(fromAcct, account.State{from})
	sm.setAccountState(toAcct, account.State{to})
	sm.setPaychState(ch, nil, paych.State{
		From:            fromAcct,
		To:              toAcct,
		ToSend:          big.NewInt(0),
		SettlingAt:      abi.ChainEpoch(0),
		MinSettleHeight: abi.ChainEpoch(0),
		LaneStates:      []*paych.LaneState{},
	})

	mgr := newManager(sm, store)
	err := mgr.TrackInboundChannel(ctx, ch)
	require.NoError(t, err)

	ci, err := mgr.GetChannelInfo(ch)
	require.NoError(t, err)
	require.Equal(t, ci.Channel, ch)
	require.Equal(t, ci.Control, to)
	require.Equal(t, ci.Target, from)
	require.EqualValues(t, ci.Direction, DirInbound)
	require.EqualValues(t, ci.NextLane, 0)
	require.Len(t, ci.Vouchers, 0)
}

func TestCheckVoucherValid(t *testing.T) {
	ctx := context.Background()

	fromKeyPrivate, fromKeyPublic := testGenerateKeyPair(t)
	randKeyPrivate, _ := testGenerateKeyPair(t)

	ch := tutils.NewIDAddr(t, 100)
	from := tutils.NewSECP256K1Addr(t, string(fromKeyPublic))
	to := tutils.NewSECP256K1Addr(t, "secpTo")
	fromAcct := tutils.NewActorAddr(t, "fromAct")
	toAcct := tutils.NewActorAddr(t, "toAct")

	sm := newMockStateManager()
	sm.setAccountState(fromAcct, account.State{from})
	sm.setAccountState(toAcct, account.State{to})

	tcases := []struct {
		name          string
		expectError   bool
		key           []byte
		actorBalance  big.Int
		toSend        big.Int
		voucherAmount big.Int
		voucherLane   uint64
		voucherNonce  uint64
		laneStates    []*paych.LaneState
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
		name:          "fails when nonce too low",
		expectError:   true,
		key:           fromKeyPrivate,
		actorBalance:  big.NewInt(10),
		toSend:        big.NewInt(0),
		voucherAmount: big.NewInt(5),
		voucherLane:   1,
		voucherNonce:  2,
		laneStates: []*paych.LaneState{{
			ID:       1,
			Redeemed: big.NewInt(2),
			Nonce:    3,
		}},
	}, {
		name:          "passes when nonce higher",
		key:           fromKeyPrivate,
		actorBalance:  big.NewInt(10),
		toSend:        big.NewInt(0),
		voucherAmount: big.NewInt(5),
		voucherLane:   1,
		voucherNonce:  3,
		laneStates: []*paych.LaneState{{
			ID:       1,
			Redeemed: big.NewInt(2),
			Nonce:    2,
		}},
	}, {
		name:          "passes when nonce for different lane",
		key:           fromKeyPrivate,
		actorBalance:  big.NewInt(10),
		toSend:        big.NewInt(0),
		voucherAmount: big.NewInt(5),
		voucherLane:   2,
		voucherNonce:  2,
		laneStates: []*paych.LaneState{{
			ID:       1,
			Redeemed: big.NewInt(2),
			Nonce:    3,
		}},
	}, {
		name:          "fails when voucher has higher nonce but lower value than lane state",
		expectError:   true,
		key:           fromKeyPrivate,
		actorBalance:  big.NewInt(10),
		toSend:        big.NewInt(0),
		voucherAmount: big.NewInt(5),
		voucherLane:   1,
		voucherNonce:  3,
		laneStates: []*paych.LaneState{{
			ID:       1,
			Redeemed: big.NewInt(6),
			Nonce:    2,
		}},
	}, {
		name:          "fails when voucher + ToSend > balance",
		expectError:   true,
		key:           fromKeyPrivate,
		actorBalance:  big.NewInt(10),
		toSend:        big.NewInt(9),
		voucherAmount: big.NewInt(2),
	}, {
		// required balance = toSend + (voucher - redeemed)
		//                  = 0 + (11 - 2)
		//                  = 9
		// So required balance: 9 < actor balance: 10
		name:          "passes when voucher - redeemed < balance",
		key:           fromKeyPrivate,
		actorBalance:  big.NewInt(10),
		toSend:        big.NewInt(0),
		voucherAmount: big.NewInt(11),
		voucherLane:   1,
		voucherNonce:  3,
		laneStates: []*paych.LaneState{{
			ID:       1,
			Redeemed: big.NewInt(2),
			Nonce:    2,
		}},
	}}

	for _, tcase := range tcases {
		t.Run(tcase.name, func(t *testing.T) {
			store := NewStore(ds_sync.MutexWrap(ds.NewMapDatastore()))

			act := &types.Actor{
				Code:    builtin.AccountActorCodeID,
				Head:    cid.Cid{},
				Nonce:   0,
				Balance: tcase.actorBalance,
			}
			sm.setPaychState(ch, act, paych.State{
				From:            fromAcct,
				To:              toAcct,
				ToSend:          tcase.toSend,
				SettlingAt:      abi.ChainEpoch(0),
				MinSettleHeight: abi.ChainEpoch(0),
				LaneStates:      tcase.laneStates,
			})

			mgr := newManager(sm, store)
			err := mgr.TrackInboundChannel(ctx, ch)
			require.NoError(t, err)

			sv := testCreateVoucher(t, tcase.voucherLane, tcase.voucherNonce, tcase.voucherAmount, tcase.key)

			err = mgr.CheckVoucherValid(ctx, ch, sv)
			if tcase.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestAddVoucherDelta(t *testing.T) {
	ctx := context.Background()

	// Set up a manager with a single payment channel
	mgr, ch, fromKeyPrivate := testSetupMgrWithChannel(t, ctx)

	voucherLane := uint64(1)

	// Expect error when adding a voucher whose amount is less than minDelta
	minDelta := big.NewInt(2)
	nonce := uint64(1)
	voucherAmount := big.NewInt(1)
	sv := testCreateVoucher(t, voucherLane, nonce, voucherAmount, fromKeyPrivate)
	_, err := mgr.AddVoucher(ctx, ch, sv, nil, minDelta)
	require.Error(t, err)

	// Expect success when adding a voucher whose amount is equal to minDelta
	nonce++
	voucherAmount = big.NewInt(2)
	sv = testCreateVoucher(t, voucherLane, nonce, voucherAmount, fromKeyPrivate)
	delta, err := mgr.AddVoucher(ctx, ch, sv, nil, minDelta)
	require.NoError(t, err)
	require.EqualValues(t, delta.Int64(), 2)

	// Check that delta is correct when there's an existing voucher
	nonce++
	voucherAmount = big.NewInt(5)
	sv = testCreateVoucher(t, voucherLane, nonce, voucherAmount, fromKeyPrivate)
	delta, err = mgr.AddVoucher(ctx, ch, sv, nil, minDelta)
	require.NoError(t, err)
	require.EqualValues(t, delta.Int64(), 3)

	// Check that delta is correct when voucher added to a different lane
	nonce = uint64(1)
	voucherAmount = big.NewInt(6)
	voucherLane = uint64(2)
	sv = testCreateVoucher(t, voucherLane, nonce, voucherAmount, fromKeyPrivate)
	delta, err = mgr.AddVoucher(ctx, ch, sv, nil, minDelta)
	require.NoError(t, err)
	require.EqualValues(t, delta.Int64(), 6)
}

func TestAddVoucherNextLane(t *testing.T) {
	ctx := context.Background()

	// Set up a manager with a single payment channel
	mgr, ch, fromKeyPrivate := testSetupMgrWithChannel(t, ctx)

	minDelta := big.NewInt(0)
	voucherAmount := big.NewInt(2)

	// Add a voucher in lane 2
	nonce := uint64(1)
	voucherLane := uint64(2)
	sv := testCreateVoucher(t, voucherLane, nonce, voucherAmount, fromKeyPrivate)
	_, err := mgr.AddVoucher(ctx, ch, sv, nil, minDelta)
	require.NoError(t, err)

	ci, err := mgr.GetChannelInfo(ch)
	require.NoError(t, err)
	require.EqualValues(t, ci.NextLane, 3)

	// Add a voucher in lane 1
	voucherLane = uint64(1)
	sv = testCreateVoucher(t, voucherLane, nonce, voucherAmount, fromKeyPrivate)
	_, err = mgr.AddVoucher(ctx, ch, sv, nil, minDelta)
	require.NoError(t, err)

	ci, err = mgr.GetChannelInfo(ch)
	require.NoError(t, err)
	require.EqualValues(t, ci.NextLane, 3)

	// Add a voucher in lane 5
	voucherLane = uint64(5)
	sv = testCreateVoucher(t, voucherLane, nonce, voucherAmount, fromKeyPrivate)
	_, err = mgr.AddVoucher(ctx, ch, sv, nil, minDelta)
	require.NoError(t, err)

	ci, err = mgr.GetChannelInfo(ch)
	require.NoError(t, err)
	require.EqualValues(t, ci.NextLane, 6)
}

func TestAddVoucherProof(t *testing.T) {
	ctx := context.Background()

	// Set up a manager with a single payment channel
	mgr, ch, fromKeyPrivate := testSetupMgrWithChannel(t, ctx)

	nonce := uint64(1)
	voucherAmount := big.NewInt(1)
	minDelta := big.NewInt(0)
	voucherAmount = big.NewInt(2)
	voucherLane := uint64(1)

	// Add a voucher with no proof
	var proof []byte
	sv := testCreateVoucher(t, voucherLane, nonce, voucherAmount, fromKeyPrivate)
	_, err := mgr.AddVoucher(ctx, ch, sv, nil, minDelta)
	require.NoError(t, err)

	// Expect one voucher with no proof
	ci, err := mgr.GetChannelInfo(ch)
	require.NoError(t, err)
	require.Len(t, ci.Vouchers, 1)
	require.Len(t, ci.Vouchers[0].Proof, 0)

	// Add same voucher with no proof
	voucherLane = uint64(1)
	_, err = mgr.AddVoucher(ctx, ch, sv, proof, minDelta)
	require.NoError(t, err)

	// Expect one voucher with no proof
	ci, err = mgr.GetChannelInfo(ch)
	require.NoError(t, err)
	require.Len(t, ci.Vouchers, 1)
	require.Len(t, ci.Vouchers[0].Proof, 0)

	// Add same voucher with proof
	proof = []byte{1}
	_, err = mgr.AddVoucher(ctx, ch, sv, proof, minDelta)
	require.NoError(t, err)

	// Should add proof to existing voucher
	ci, err = mgr.GetChannelInfo(ch)
	require.NoError(t, err)
	require.Len(t, ci.Vouchers, 1)
	require.Len(t, ci.Vouchers[0].Proof, 1)
}

func TestAllocateLane(t *testing.T) {
	ctx := context.Background()

	// Set up a manager with a single payment channel
	mgr, ch, _ := testSetupMgrWithChannel(t, ctx)

	// First lane should be 0
	lane, err := mgr.AllocateLane(ch)
	require.NoError(t, err)
	require.EqualValues(t, lane, 0)

	// Next lane should be 1
	lane, err = mgr.AllocateLane(ch)
	require.NoError(t, err)
	require.EqualValues(t, lane, 1)
}

func TestNextNonceForLane(t *testing.T) {
	ctx := context.Background()

	// Set up a manager with a single payment channel
	mgr, ch, key := testSetupMgrWithChannel(t, ctx)

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
	// lane 2: nonce 7
	voucherLane := uint64(1)
	for _, nonce := range []uint64{2, 4} {
		voucherAmount = big.Add(voucherAmount, big.NewInt(1))
		sv := testCreateVoucher(t, voucherLane, nonce, voucherAmount, key)
		_, err := mgr.AddVoucher(ctx, ch, sv, nil, minDelta)
		require.NoError(t, err)
	}

	voucherLane = uint64(2)
	nonce := uint64(7)
	sv := testCreateVoucher(t, voucherLane, nonce, voucherAmount, key)
	_, err = mgr.AddVoucher(ctx, ch, sv, nil, minDelta)
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

func testSetupMgrWithChannel(t *testing.T, ctx context.Context) (*Manager, address.Address, []byte) {
	fromKeyPrivate, fromKeyPublic := testGenerateKeyPair(t)

	ch := tutils.NewIDAddr(t, 100)
	from := tutils.NewSECP256K1Addr(t, string(fromKeyPublic))
	to := tutils.NewSECP256K1Addr(t, "secpTo")
	fromAcct := tutils.NewActorAddr(t, "fromAct")
	toAcct := tutils.NewActorAddr(t, "toAct")

	sm := newMockStateManager()
	sm.setAccountState(fromAcct, account.State{from})
	sm.setAccountState(toAcct, account.State{to})

	act := &types.Actor{
		Code:    builtin.AccountActorCodeID,
		Head:    cid.Cid{},
		Nonce:   0,
		Balance: big.NewInt(10),
	}
	sm.setPaychState(ch, act, paych.State{
		From:            fromAcct,
		To:              toAcct,
		ToSend:          big.NewInt(0),
		SettlingAt:      abi.ChainEpoch(0),
		MinSettleHeight: abi.ChainEpoch(0),
		LaneStates:      []*paych.LaneState{},
	})

	store := NewStore(ds_sync.MutexWrap(ds.NewMapDatastore()))
	mgr := newManager(sm, store)
	err := mgr.TrackInboundChannel(ctx, ch)
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

func testCreateVoucher(t *testing.T, voucherLane uint64, nonce uint64, voucherAmount big.Int, key []byte) *paych.SignedVoucher {
	sv := &paych.SignedVoucher{
		Lane:   voucherLane,
		Nonce:  nonce,
		Amount: voucherAmount,
	}

	signingBytes, err := sv.SigningBytes()
	require.NoError(t, err)
	sig, err := sigs.Sign(crypto.SigTypeSecp256k1, key, signingBytes)
	require.NoError(t, err)
	sv.Signature = sig
	return sv
}
