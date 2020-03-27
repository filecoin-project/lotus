package stmgr_test

import (
	"context"
	"fmt"
	"io"
	"testing"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/abi/big"
	"github.com/filecoin-project/specs-actors/actors/builtin"
	init_ "github.com/filecoin-project/specs-actors/actors/builtin/init"
	"github.com/filecoin-project/specs-actors/actors/builtin/power"
	"github.com/filecoin-project/specs-actors/actors/runtime"
	"github.com/filecoin-project/specs-actors/actors/util/adt"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/aerrors"
	"github.com/filecoin-project/lotus/chain/gen"
	"github.com/filecoin-project/lotus/chain/state"
	"github.com/filecoin-project/lotus/chain/stmgr"
	. "github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/vm"
	_ "github.com/filecoin-project/lotus/lib/sigs/bls"
	_ "github.com/filecoin-project/lotus/lib/sigs/secp"

	"github.com/ipfs/go-cid"
	blockstore "github.com/ipfs/go-ipfs-blockstore"
	cbor "github.com/ipfs/go-ipld-cbor"
	logging "github.com/ipfs/go-log"
	mh "github.com/multiformats/go-multihash"
	cbg "github.com/whyrusleeping/cbor-gen"
)

func init() {
	build.SectorSizes = []abi.SectorSize{2048}
	power.ConsensusMinerMinPower = big.NewInt(2048)
}

const testForkHeight = 40

type testActor struct {
}

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

func (ta *testActor) Exports() []interface{} {
	return []interface{}{
		1: ta.Constructor,
		2: ta.TestMethod,
	}
}

func (ta *testActor) Constructor(rt runtime.Runtime, params *adt.EmptyValue) *adt.EmptyValue {

	rt.State().Create(&testActorState{11})
	fmt.Println("NEW ACTOR ADDRESS IS: ", rt.Message().Receiver())

	return adt.Empty
}

func (ta *testActor) TestMethod(rt runtime.Runtime, params *adt.EmptyValue) *adt.EmptyValue {
	var st testActorState
	rt.State().Readonly(&st)

	if rt.CurrEpoch() > testForkHeight {
		if st.HasUpgraded != 55 {
			panic(aerrors.Fatal("fork updating applied in wrong order"))
		}
	} else {
		if st.HasUpgraded != 11 {
			panic(aerrors.Fatal("fork updating happened too early"))
		}
	}

	return adt.Empty
}

func TestForkHeightTriggers(t *testing.T) {
	logging.SetAllLoggers(logging.LevelInfo)

	ctx := context.TODO()

	cg, err := gen.NewGenerator()
	if err != nil {
		t.Fatal(err)
	}

	sm := NewStateManager(cg.ChainStore())

	inv := vm.NewInvoker()

	pref := cid.NewPrefixV1(cid.Raw, mh.IDENTITY)
	actcid, err := pref.Sum([]byte("testactor"))
	if err != nil {
		t.Fatal(err)
	}

	// predicting the address here... may break if other assumptions change
	taddr, err := address.NewIDAddress(1002)
	if err != nil {
		t.Fatal(err)
	}

	stmgr.ForksAtHeight[testForkHeight] = func(ctx context.Context, sm *StateManager, pstate cid.Cid) (cid.Cid, error) {
		cst := cbor.NewCborStore(sm.ChainStore().Blockstore())
		st, err := state.LoadStateTree(cst, pstate)
		if err != nil {
			return cid.Undef, err
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
	}

	inv.Register(actcid, &testActor{}, &testActorState{})
	sm.SetVMConstructor(func(c cid.Cid, h abi.ChainEpoch, r vm.Rand, a address.Address, b blockstore.Blockstore, s runtime.Syscalls) (*vm.VM, error) {
		nvm, err := vm.NewVM(c, h, r, a, b, s)
		if err != nil {
			return nil, err
		}
		nvm.SetInvoker(inv)
		return nvm, nil
	})

	cg.SetStateManager(sm)

	var msgs []*types.SignedMessage

	enc, err := actors.SerializeParams(&init_.ExecParams{CodeCID: actcid})
	if err != nil {
		t.Fatal(err)
	}

	m := &types.Message{
		From:     cg.Banker(),
		To:       builtin.InitActorAddr,
		Method:   builtin.MethodsInit.Exec,
		Params:   enc,
		GasLimit: 10000,
		GasPrice: types.NewInt(0),
	}
	sig, err := cg.Wallet().Sign(ctx, cg.Banker(), m.Cid().Bytes())
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
			GasLimit: 10000,
			GasPrice: types.NewInt(0),
		}
		nonce++

		sig, err := cg.Wallet().Sign(ctx, cg.Banker(), m.Cid().Bytes())
		if err != nil {
			return nil, err
		}

		return []*types.SignedMessage{
			&types.SignedMessage{
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
