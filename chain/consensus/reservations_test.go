package consensus

import (
	"context"
	"errors"
	"testing"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/crypto"
	"github.com/filecoin-project/go-state-types/network"
	cid "github.com/ipfs/go-cid"

	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/vm"
)

type fakeReservationsVM struct {
	startCalled bool
	endCalled   bool

	startPlan map[address.Address]abi.TokenAmount
	startErr  error
	endErr    error
}

func (f *fakeReservationsVM) ApplyMessage(ctx context.Context, cmsg types.ChainMsg) (*vm.ApplyRet, error) {
	return nil, nil
}

func (f *fakeReservationsVM) ApplyImplicitMessage(ctx context.Context, msg *types.Message) (*vm.ApplyRet, error) {
	return nil, nil
}

func (f *fakeReservationsVM) Flush(ctx context.Context) (cid.Cid, error) {
	return cid.Undef, nil
}

func (f *fakeReservationsVM) StartTipsetReservations(ctx context.Context, plan map[address.Address]abi.TokenAmount) error {
	f.startCalled = true
	f.startPlan = plan
	return f.startErr
}

func (f *fakeReservationsVM) EndTipsetReservations(ctx context.Context) error {
	f.endCalled = true
	return f.endErr
}

func withFeatures(t *testing.T, flags ReservationFeatureFlags) func() {
	t.Helper()
	orig := Feature
	SetFeatures(flags)
	return func() {
		SetFeatures(orig)
	}
}

func TestReservationsEnabledFeatureFlags(t *testing.T) {
	nvPre := network.Version(0)
	nvPost := vm.ReservationsActivationNetworkVersion()

	restore := withFeatures(t, ReservationFeatureFlags{
		MultiStageReservations:       false,
		MultiStageReservationsStrict: false,
	})
	defer restore()

	if ReservationsEnabled(nvPre) {
		t.Fatalf("expected reservations to be disabled when feature flag is false pre-activation")
	}

	// At or after activation, reservations are always enabled regardless of
	// the feature flags.
	if !ReservationsEnabled(nvPost) {
		t.Fatalf("expected reservations to be enabled at or after activation network version")
	}

	SetFeatures(ReservationFeatureFlags{
		MultiStageReservations:       true,
		MultiStageReservationsStrict: false,
	})
	if !ReservationsEnabled(nvPre) {
		t.Fatalf("expected reservations to be enabled when MultiStageReservations is true pre-activation")
	}

	SetFeatures(ReservationFeatureFlags{
		MultiStageReservations:       true,
		MultiStageReservationsStrict: true,
	})
	if !ReservationsEnabled(nvPre) {
		t.Fatalf("expected reservations to remain enabled when strict mode is true pre-activation")
	}
}

func TestBuildReservationPlanDedupAcrossBlocks(t *testing.T) {
	addr1, err := address.NewIDAddress(100)
	if err != nil {
		t.Fatalf("creating addr1: %v", err)
	}
	addr2, err := address.NewIDAddress(200)
	if err != nil {
		t.Fatalf("creating addr2: %v", err)
	}

	feeCapA := abi.NewTokenAmount(2)
	feeCapB := abi.NewTokenAmount(3)
	feeCapC := abi.NewTokenAmount(4)

	const gasLimitA = int64(10)
	const gasLimitB = int64(5)
	const gasLimitC = int64(7)

	msgA := &types.Message{
		From:       addr1,
		To:         addr2,
		Nonce:      0,
		Value:      abi.NewTokenAmount(0),
		GasFeeCap:  feeCapA,
		GasPremium: abi.NewTokenAmount(0),
		GasLimit:   gasLimitA,
	}
	msgB := &types.Message{
		From:       addr1,
		To:         addr2,
		Nonce:      1,
		Value:      abi.NewTokenAmount(0),
		GasFeeCap:  feeCapB,
		GasPremium: abi.NewTokenAmount(0),
		GasLimit:   gasLimitB,
	}
	msgC := &types.Message{
		From:       addr2,
		To:         addr1,
		Nonce:      0,
		Value:      abi.NewTokenAmount(0),
		GasFeeCap:  feeCapC,
		GasPremium: abi.NewTokenAmount(0),
		GasLimit:   gasLimitC,
	}

	signedC := &types.SignedMessage{
		Message:   *msgC,
		Signature: crypto.Signature{Type: crypto.SigTypeSecp256k1, Data: []byte{0x01}},
	}

	b1 := FilecoinBlockMessages{
		BlockMessages: store.BlockMessages{
			BlsMessages:   []types.ChainMsg{msgA},
			SecpkMessages: []types.ChainMsg{signedC},
		},
	}
	b2 := FilecoinBlockMessages{
		BlockMessages: store.BlockMessages{
			BlsMessages:   []types.ChainMsg{msgA, msgB},
			SecpkMessages: []types.ChainMsg{signedC},
		},
	}

	plan := buildReservationPlan([]FilecoinBlockMessages{b1, b2})

	if len(plan) != 2 {
		t.Fatalf("expected 2 senders in plan, got %d", len(plan))
	}

	costA := big.Mul(big.NewInt(gasLimitA), feeCapA)
	costB := big.Mul(big.NewInt(gasLimitB), feeCapB)
	costC := big.Mul(big.NewInt(gasLimitC), feeCapC)

	expected1 := big.Add(costA, costB)
	expected2 := costC

	got1, ok := plan[addr1]
	if !ok {
		t.Fatalf("missing sender addr1 in plan")
	}
	if !got1.Equals(expected1) {
		t.Fatalf("addr1 total mismatch: expected %s, got %s", expected1, got1)
	}

	got2, ok := plan[addr2]
	if !ok {
		t.Fatalf("missing sender addr2 in plan")
	}
	if !got2.Equals(expected2) {
		t.Fatalf("addr2 total mismatch: expected %s, got %s", expected2, got2)
	}
}

func TestStartReservationsEmptyPlanSkipsBegin(t *testing.T) {
	restore := withFeatures(t, ReservationFeatureFlags{
		MultiStageReservations:       true,
		MultiStageReservationsStrict: false,
	})
	defer restore()

	vmStub := &fakeReservationsVM{}
	if err := startReservations(context.Background(), vmStub, nil, network.Version(0)); err != nil {
		t.Fatalf("startReservations returned error for empty plan: %v", err)
	}
	if vmStub.startCalled {
		t.Fatalf("expected StartTipsetReservations not to be called for empty plan")
	}
}

func TestStartReservationsErrorPropagation(t *testing.T) {
	restore := withFeatures(t, ReservationFeatureFlags{
		MultiStageReservations:       true,
		MultiStageReservationsStrict: true,
	})
	defer restore()

	addr, err := address.NewIDAddress(100)
	if err != nil {
		t.Fatalf("creating addr: %v", err)
	}

	feeCap := abi.NewTokenAmount(1)
	const gasLimit = int64(10)

	msg := &types.Message{
		From:       addr,
		To:         addr,
		Nonce:      0,
		Value:      abi.NewTokenAmount(0),
		GasFeeCap:  feeCap,
		GasPremium: abi.NewTokenAmount(0),
		GasLimit:   gasLimit,
	}

	b := FilecoinBlockMessages{
		BlockMessages: store.BlockMessages{
			BlsMessages: []types.ChainMsg{msg},
		},
	}

	vmStub := &fakeReservationsVM{
		startErr: vm.ErrReservationsInsufficientFunds,
	}

	err = startReservations(context.Background(), vmStub, []FilecoinBlockMessages{b}, network.Version(0))
	if !errors.Is(err, vm.ErrReservationsInsufficientFunds) {
		t.Fatalf("expected ErrReservationsInsufficientFunds from startReservations in strict mode, got %v", err)
	}
	if !vmStub.startCalled {
		t.Fatalf("expected StartTipsetReservations to be called")
	}
}

func TestStartReservationsStrictPreActivationPlanTooLargeIsError(t *testing.T) {
	restore := withFeatures(t, ReservationFeatureFlags{
		MultiStageReservations:       true,
		MultiStageReservationsStrict: true,
	})
	defer restore()

	addr, err := address.NewIDAddress(100)
	if err != nil {
		t.Fatalf("creating addr: %v", err)
	}

	feeCap := abi.NewTokenAmount(1)
	const gasLimit = int64(10)

	msg := &types.Message{
		From:       addr,
		To:         addr,
		Nonce:      0,
		Value:      abi.NewTokenAmount(0),
		GasFeeCap:  feeCap,
		GasPremium: abi.NewTokenAmount(0),
		GasLimit:   gasLimit,
	}

	b := FilecoinBlockMessages{
		BlockMessages: store.BlockMessages{
			BlsMessages: []types.ChainMsg{msg},
		},
	}

	vmStub := &fakeReservationsVM{
		startErr: vm.ErrReservationsPlanTooLarge,
	}

	err = startReservations(context.Background(), vmStub, []FilecoinBlockMessages{b}, network.Version(0))
	if !errors.Is(err, vm.ErrReservationsPlanTooLarge) {
		t.Fatalf("expected ErrReservationsPlanTooLarge from startReservations in strict mode, got %v", err)
	}
	if !vmStub.startCalled {
		t.Fatalf("expected StartTipsetReservations to be called")
	}
}

func TestStartReservationsNonStrictPreActivationFallsBackOnInsufficientFunds(t *testing.T) {
	restore := withFeatures(t, ReservationFeatureFlags{
		MultiStageReservations:       true,
		MultiStageReservationsStrict: false,
	})
	defer restore()

	addr, err := address.NewIDAddress(100)
	if err != nil {
		t.Fatalf("creating addr: %v", err)
	}

	feeCap := abi.NewTokenAmount(1)
	const gasLimit = int64(10)

	msg := &types.Message{
		From:       addr,
		To:         addr,
		Nonce:      0,
		Value:      abi.NewTokenAmount(0),
		GasFeeCap:  feeCap,
		GasPremium: abi.NewTokenAmount(0),
		GasLimit:   gasLimit,
	}

	b := FilecoinBlockMessages{
		BlockMessages: store.BlockMessages{
			BlsMessages: []types.ChainMsg{msg},
		},
	}

	vmStub := &fakeReservationsVM{
		startErr: vm.ErrReservationsInsufficientFunds,
	}

	err = startReservations(context.Background(), vmStub, []FilecoinBlockMessages{b}, network.Version(0))
	if err != nil {
		t.Fatalf("expected non-strict pre-activation startReservations to fall back on insufficient funds, got %v", err)
	}
	if !vmStub.startCalled {
		t.Fatalf("expected StartTipsetReservations to be called")
	}
}

func TestStartReservationsNonStrictPreActivationFallsBackOnPlanTooLarge(t *testing.T) {
	restore := withFeatures(t, ReservationFeatureFlags{
		MultiStageReservations:       true,
		MultiStageReservationsStrict: false,
	})
	defer restore()

	addr, err := address.NewIDAddress(100)
	if err != nil {
		t.Fatalf("creating addr: %v", err)
	}

	feeCap := abi.NewTokenAmount(1)
	const gasLimit = int64(10)

	msg := &types.Message{
		From:       addr,
		To:         addr,
		Nonce:      0,
		Value:      abi.NewTokenAmount(0),
		GasFeeCap:  feeCap,
		GasPremium: abi.NewTokenAmount(0),
		GasLimit:   gasLimit,
	}

	b := FilecoinBlockMessages{
		BlockMessages: store.BlockMessages{
			BlsMessages: []types.ChainMsg{msg},
		},
	}

	vmStub := &fakeReservationsVM{
		startErr: vm.ErrReservationsPlanTooLarge,
	}

	err = startReservations(context.Background(), vmStub, []FilecoinBlockMessages{b}, network.Version(0))
	if err != nil {
		t.Fatalf("expected non-strict pre-activation startReservations to fall back on plan too large, got %v", err)
	}
	if !vmStub.startCalled {
		t.Fatalf("expected StartTipsetReservations to be called")
	}
}

func TestStartReservationsPreActivationNotImplementedFallsBack(t *testing.T) {
	restore := withFeatures(t, ReservationFeatureFlags{
		MultiStageReservations:       true,
		MultiStageReservationsStrict: true,
	})
	defer restore()

	addr, err := address.NewIDAddress(100)
	if err != nil {
		t.Fatalf("creating addr: %v", err)
	}

	feeCap := abi.NewTokenAmount(1)
	const gasLimit = int64(10)

	msg := &types.Message{
		From:       addr,
		To:         addr,
		Nonce:      0,
		Value:      abi.NewTokenAmount(0),
		GasFeeCap:  feeCap,
		GasPremium: abi.NewTokenAmount(0),
		GasLimit:   gasLimit,
	}

	b := FilecoinBlockMessages{
		BlockMessages: store.BlockMessages{
			BlsMessages: []types.ChainMsg{msg},
		},
	}

	vmStub := &fakeReservationsVM{
		startErr: vm.ErrReservationsNotImplemented,
	}

	err = startReservations(context.Background(), vmStub, []FilecoinBlockMessages{b}, network.Version(0))
	if err != nil {
		t.Fatalf("expected pre-activation NotImplemented to fall back to legacy mode, got %v", err)
	}
	if !vmStub.startCalled {
		t.Fatalf("expected StartTipsetReservations to be called")
	}
}

func TestStartReservationsPostActivationNotImplementedIsError(t *testing.T) {
	restore := withFeatures(t, ReservationFeatureFlags{
		MultiStageReservations:       true,
		MultiStageReservationsStrict: false,
	})
	defer restore()

	addr, err := address.NewIDAddress(100)
	if err != nil {
		t.Fatalf("creating addr: %v", err)
	}

	feeCap := abi.NewTokenAmount(1)
	const gasLimit = int64(10)

	msg := &types.Message{
		From:       addr,
		To:         addr,
		Nonce:      0,
		Value:      abi.NewTokenAmount(0),
		GasFeeCap:  feeCap,
		GasPremium: abi.NewTokenAmount(0),
		GasLimit:   gasLimit,
	}

	b := FilecoinBlockMessages{
		BlockMessages: store.BlockMessages{
			BlsMessages: []types.ChainMsg{msg},
		},
	}

	vmStub := &fakeReservationsVM{
		startErr: vm.ErrReservationsNotImplemented,
	}

	err = startReservations(context.Background(), vmStub, []FilecoinBlockMessages{b}, vm.ReservationsActivationNetworkVersion())
	if !errors.Is(err, vm.ErrReservationsNotImplemented) {
		t.Fatalf("expected ErrReservationsNotImplemented post-activation, got %v", err)
	}
	if !vmStub.startCalled {
		t.Fatalf("expected StartTipsetReservations to be called")
	}
}

func TestEndReservationsErrorPropagation(t *testing.T) {
	restore := withFeatures(t, ReservationFeatureFlags{
		MultiStageReservations:       true,
		MultiStageReservationsStrict: true,
	})
	defer restore()

	vmStub := &fakeReservationsVM{
		endErr: vm.ErrReservationsNonZeroRemainder,
	}

	err := endReservations(context.Background(), vmStub, network.Version(0))
	if !errors.Is(err, vm.ErrReservationsNonZeroRemainder) {
		t.Fatalf("expected ErrReservationsNonZeroRemainder from endReservations in strict mode, got %v", err)
	}
	if !vmStub.endCalled {
		t.Fatalf("expected EndTipsetReservations to be called")
	}
}

func TestEndReservationsNonStrictPreActivationFallsBackOnPlanTooLarge(t *testing.T) {
	restore := withFeatures(t, ReservationFeatureFlags{
		MultiStageReservations:       true,
		MultiStageReservationsStrict: false,
	})
	defer restore()

	vmStub := &fakeReservationsVM{
		endErr: vm.ErrReservationsPlanTooLarge,
	}

	err := endReservations(context.Background(), vmStub, network.Version(0))
	if err != nil {
		t.Fatalf("expected non-strict pre-activation endReservations to fall back on plan too large, got %v", err)
	}
	if !vmStub.endCalled {
		t.Fatalf("expected EndTipsetReservations to be called")
	}
}

func TestEndReservationsStrictPreActivationPlanTooLargeIsError(t *testing.T) {
	restore := withFeatures(t, ReservationFeatureFlags{
		MultiStageReservations:       true,
		MultiStageReservationsStrict: true,
	})
	defer restore()

	vmStub := &fakeReservationsVM{
		endErr: vm.ErrReservationsPlanTooLarge,
	}

	err := endReservations(context.Background(), vmStub, network.Version(0))
	if !errors.Is(err, vm.ErrReservationsPlanTooLarge) {
		t.Fatalf("expected ErrReservationsPlanTooLarge from endReservations in strict mode, got %v", err)
	}
	if !vmStub.endCalled {
		t.Fatalf("expected EndTipsetReservations to be called")
	}
}
