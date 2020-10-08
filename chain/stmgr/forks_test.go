package stmgr_test

import (
	"context"
	"fmt"
	"io"
	"testing"

	"github.com/ipfs/go-cid"
	ipldcbor "github.com/ipfs/go-ipld-cbor"
	logging "github.com/ipfs/go-log"
	"github.com/stretchr/testify/require"
	cbg "github.com/whyrusleeping/cbor-gen"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/cbor"
	"github.com/filecoin-project/specs-actors/actors/builtin"
	init0 "github.com/filecoin-project/specs-actors/actors/builtin/init"
	"github.com/filecoin-project/specs-actors/actors/runtime"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/aerrors"
	lotusinit "github.com/filecoin-project/lotus/chain/actors/builtin/init"
	"github.com/filecoin-project/lotus/chain/actors/policy"
	"github.com/filecoin-project/lotus/chain/gen"
	. "github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/vm"
	_ "github.com/filecoin-project/lotus/lib/sigs/bls"
	_ "github.com/filecoin-project/lotus/lib/sigs/secp"
)

func init() {
	policy.SetSupportedProofTypes(abi.RegisteredSealProof_StackedDrg2KiBV1)
	policy.SetConsensusMinerMinPower(abi.NewStoragePower(2048))
	policy.SetMinVerifiedDealSize(abi.NewStoragePower(256))
}

const testForkHeight = 40

type testActor struct {
}

// must use existing actor that an account is allowed to exec.
func (testActor) Code() cid.Cid  { return builtin.PaymentChannelActorCodeID }
func (testActor) State() cbor.Er { return new(testActorState) }

type testActorState struct {
	HasUpgraded uint64
}

func (tas *testActorState) MarshalCBOR(w io.Writer) error {
	return cbg.CborWriteHeader(w, cbg.MajUnsignedInt, tas.HasUpgraded)
}

func (tas *testActorState) UnmarshalCBOR(r io.Reader) error {
	t, v, err := cbg.CborReadHeader(r)
	if err != nil {
		return err
	}
	if t != cbg.MajUnsignedInt {
		return fmt.Errorf("wrong type in test actor state (got %d)", t)
	}
	tas.HasUpgraded = v
	return nil
}

func (ta testActor) Exports() []interface{} {
	return []interface{}{
		1: ta.Constructor,
		2: ta.TestMethod,
	}
}

func (ta *testActor) Constructor(rt runtime.Runtime, params *abi.EmptyValue) *abi.EmptyValue {
	rt.ValidateImmediateCallerAcceptAny()
	rt.StateCreate(&testActorState{11})
	//fmt.Println("NEW ACTOR ADDRESS IS: ", rt.Receiver())

	return abi.Empty
}

func (ta *testActor) TestMethod(rt runtime.Runtime, params *abi.EmptyValue) *abi.EmptyValue {
	rt.ValidateImmediateCallerAcceptAny()
	var st testActorState
	rt.StateReadonly(&st)

	if rt.CurrEpoch() > testForkHeight {
		if st.HasUpgraded != 55 {
			panic(aerrors.Fatal("fork updating applied in wrong order"))
		}
	} else {
		if st.HasUpgraded != 11 {
			panic(aerrors.Fatal("fork updating happened too early"))
		}
	}

	return abi.Empty
}

func TestForkHeightTriggers(t *testing.T) {
	logging.SetAllLoggers(logging.LevelInfo)

	ctx := context.TODO()

	cg, err := gen.NewGenerator()
	if err != nil {
		t.Fatal(err)
	}

	// predicting the address here... may break if other assumptions change
	taddr, err := address.NewIDAddress(1002)
	if err != nil {
		t.Fatal(err)
	}

	sm, err := NewStateManagerWithUpgradeSchedule(
		cg.ChainStore(), UpgradeSchedule{{
			Network: 1,
			Height:  testForkHeight,
			Migration: func(ctx context.Context, sm *StateManager, cb ExecCallback,
				root cid.Cid, height abi.ChainEpoch, ts *types.TipSet) (cid.Cid, error) {
				cst := ipldcbor.NewCborStore(sm.ChainStore().Blockstore())

				st, err := sm.StateTree(root)
				if err != nil {
					return cid.Undef, xerrors.Errorf("getting state tree: %w", err)
				}

				act, err := st.GetActor(taddr)
				if err != nil {
					return cid.Undef, err
				}

				var tas testActorState
				if err := cst.Get(ctx, act.Head, &tas); err != nil {
					return cid.Undef, xerrors.Errorf("in fork handler, failed to run get: %w", err)
				}

				tas.HasUpgraded = 55

				ns, err := cst.Put(ctx, &tas)
				if err != nil {
					return cid.Undef, err
				}

				act.Head = ns

				if err := st.SetActor(taddr, act); err != nil {
					return cid.Undef, err
				}

				return st.Flush(ctx)
			}}})
	if err != nil {
		t.Fatal(err)
	}

	inv := vm.NewActorRegistry()
	inv.Register(nil, testActor{})

	sm.SetVMConstructor(func(ctx context.Context, vmopt *vm.VMOpts) (*vm.VM, error) {
		nvm, err := vm.NewVM(ctx, vmopt)
		if err != nil {
			return nil, err
		}
		nvm.SetInvoker(inv)
		return nvm, nil
	})

	cg.SetStateManager(sm)

	var msgs []*types.SignedMessage

	enc, err := actors.SerializeParams(&init0.ExecParams{CodeCID: (testActor{}).Code()})
	if err != nil {
		t.Fatal(err)
	}

	m := &types.Message{
		From:     cg.Banker(),
		To:       lotusinit.Address,
		Method:   builtin.MethodsInit.Exec,
		Params:   enc,
		GasLimit: types.TestGasLimit,
	}
	sig, err := cg.Wallet().WalletSign(ctx, cg.Banker(), m.Cid().Bytes(), api.MsgMeta{})
	if err != nil {
		t.Fatal(err)
	}
	msgs = append(msgs, &types.SignedMessage{
		Signature: *sig,
		Message:   *m,
	})

	nonce := uint64(1)
	cg.GetMessages = func(cg *gen.ChainGen) ([]*types.SignedMessage, error) {
		if len(msgs) > 0 {
			fmt.Println("added construct method")
			m := msgs
			msgs = nil
			return m, nil
		}

		m := &types.Message{
			From:     cg.Banker(),
			To:       taddr,
			Method:   2,
			Params:   nil,
			Nonce:    nonce,
			GasLimit: types.TestGasLimit,
		}
		nonce++

		sig, err := cg.Wallet().WalletSign(ctx, cg.Banker(), m.Cid().Bytes(), api.MsgMeta{})
		if err != nil {
			return nil, err
		}

		return []*types.SignedMessage{
			{
				Signature: *sig,
				Message:   *m,
			},
		}, nil
	}

	for i := 0; i < 50; i++ {
		_, err = cg.NextTipSet()
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestForkRefuseCall(t *testing.T) {
	logging.SetAllLoggers(logging.LevelInfo)

	ctx := context.TODO()

	cg, err := gen.NewGenerator()
	if err != nil {
		t.Fatal(err)
	}

	sm, err := NewStateManagerWithUpgradeSchedule(
		cg.ChainStore(), UpgradeSchedule{{
			Network:   1,
			Expensive: true,
			Height:    testForkHeight,
			Migration: func(ctx context.Context, sm *StateManager, cb ExecCallback,
				root cid.Cid, height abi.ChainEpoch, ts *types.TipSet) (cid.Cid, error) {
				return root, nil
			}}})
	if err != nil {
		t.Fatal(err)
	}

	inv := vm.NewActorRegistry()
	inv.Register(nil, testActor{})

	sm.SetVMConstructor(func(ctx context.Context, vmopt *vm.VMOpts) (*vm.VM, error) {
		nvm, err := vm.NewVM(ctx, vmopt)
		if err != nil {
			return nil, err
		}
		nvm.SetInvoker(inv)
		return nvm, nil
	})

	cg.SetStateManager(sm)

	enc, err := actors.SerializeParams(&init0.ExecParams{CodeCID: (testActor{}).Code()})
	if err != nil {
		t.Fatal(err)
	}

	m := &types.Message{
		From:       cg.Banker(),
		To:         lotusinit.Address,
		Method:     builtin.MethodsInit.Exec,
		Params:     enc,
		GasLimit:   types.TestGasLimit,
		Value:      types.NewInt(0),
		GasPremium: types.NewInt(0),
		GasFeeCap:  types.NewInt(0),
	}

	for i := 0; i < 50; i++ {
		ts, err := cg.NextTipSet()
		if err != nil {
			t.Fatal(err)
		}

		ret, err := sm.CallWithGas(ctx, m, nil, ts.TipSet.TipSet())
		switch ts.TipSet.TipSet().Height() {
		case testForkHeight, testForkHeight + 1:
			// If I had a fork, or I _will_ have a fork, it should fail.
			require.Equal(t, ErrExpensiveFork, err)
		default:
			require.NoError(t, err)
			require.True(t, ret.MsgRct.ExitCode.IsSuccess())
		}
		// Call just runs on the parent state for a tipset, so we only
		// expect an error at the fork height.
		ret, err = sm.Call(ctx, m, ts.TipSet.TipSet())
		switch ts.TipSet.TipSet().Height() {
		case testForkHeight + 1:
			require.Equal(t, ErrExpensiveFork, err)
		default:
			require.NoError(t, err)
			require.True(t, ret.MsgRct.ExitCode.IsSuccess())
		}
	}
}
