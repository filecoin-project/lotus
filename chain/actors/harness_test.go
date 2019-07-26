package actors_test

import (
	"context"
	"testing"

	. "github.com/filecoin-project/go-lotus/chain/actors"
	"github.com/filecoin-project/go-lotus/chain/gen"
	"github.com/filecoin-project/go-lotus/chain/store"
	"github.com/filecoin-project/go-lotus/chain/vm"

	"github.com/filecoin-project/go-lotus/chain/address"
	"github.com/filecoin-project/go-lotus/chain/state"
	"github.com/filecoin-project/go-lotus/chain/types"

	dstore "github.com/ipfs/go-datastore"
	hamt "github.com/ipfs/go-hamt-ipld"
	bstore "github.com/ipfs/go-ipfs-blockstore"
	cbor "github.com/ipfs/go-ipld-cbor"
)

type Harness struct {
	Steps []Step
	From  address.Address
	Third address.Address

	currStep int

	t      *testing.T
	actors []address.Address
	vm     *vm.VM
	bs     bstore.Blockstore
	cs     *store.ChainStore
}

type Step struct {
	M   types.Message
	Ret func(*testing.T, *types.MessageReceipt)
	Err func(*testing.T, error)
}

func NewHarness(t *testing.T) *Harness {
	h := &Harness{t: t}
	h.bs = bstore.NewBlockstore(dstore.NewMapDatastore())

	from := blsaddr(0)
	maddr := blsaddr(1)
	third := blsaddr(1)

	actors := map[address.Address]types.BigInt{
		from:  types.NewInt(1000000),
		maddr: types.NewInt(0),
		third: types.NewInt(1000),
	}
	st, err := gen.MakeInitialStateTree(h.bs, actors)
	if err != nil {
		t.Fatal(err)
	}

	stateroot, err := st.Flush()
	if err != nil {
		t.Fatal(err)
	}

	h.cs = store.NewChainStore(h.bs, nil)

	h.vm, err = vm.NewVM(stateroot, 1, maddr, h.cs)
	if err != nil {
		t.Fatal(err)
	}
	h.actors = []address.Address{from, maddr, third}
	h.From = from
	h.Third = third
	return h
}

func (h *Harness) Execute() *state.StateTree {
	for i, step := range h.Steps {
		h.currStep = i
		ret, err := h.vm.ApplyMessage(&step.M)
		if step.Err != nil {
			step.Err(h.t, err)
		} else {
			h.NoError(h.t, err)
		}
		step.Ret(h.t, ret)
	}
	stateroot, err := h.vm.Flush(context.TODO())
	if err != nil {
		h.t.Fatalf("%+v", err)
	}
	cst := hamt.CSTFromBstore(h.bs)
	state, err := state.LoadStateTree(cst, stateroot)
	if err != nil {
		h.t.Fatal(err)
	}
	return state
}

func (h *Harness) DumpObject(obj interface{}) []byte {
	enc, err := cbor.DumpObject(obj)
	if err != nil {
		h.t.Fatal(err)
	}
	return enc
}
func (h *Harness) NoError(t *testing.T, err error) {
	if err != nil {
		t.Fatalf("Error in step %d: %+v", h.currStep, err)
	}
}

func TestVMInvokeHarness(t *testing.T) {
	h := NewHarness(t)
	var outaddr address.Address
	h.Steps = []Step{
		{
			M: types.Message{
				To:     InitActorAddress,
				From:   h.From,
				Method: 1,
				Params: h.DumpObject(
					&ExecParams{
						Code: StorageMinerCodeCid,
						Params: h.DumpObject(&StorageMinerConstructorParams{
							Owner: h.From,
						}),
					}),
				GasPrice: types.NewInt(1),
				GasLimit: types.NewInt(1),
				Value:    types.NewInt(0),
			},
			Ret: func(t *testing.T, ret *types.MessageReceipt) {
				if ret.ExitCode != 0 {
					t.Fatal("invocation failed: ", ret.ExitCode)
				}

				var err error
				outaddr, err = address.NewFromBytes(ret.Return)
				if err != nil {
					t.Fatal(err)
				}

				if outaddr.String() != "t0102" {
					t.Fatal("hold up")
				}
			},
			Err: h.NoError,
		},
	}

	state := h.Execute()
	act, err := state.GetActor(outaddr)
	if err != nil {
		t.Fatal(err)
	}
	if act.Code != StorageMinerCodeCid {
		t.Fatalf("Expected correct code, got %s, instead of %s", act.Code, StorageMinerCodeCid)
	}
	hblock, err := h.bs.Get(act.Head)
	if err != nil {
		t.Fatal(err)
	}

	smas := &StorageMinerActorState{}
	err = cbor.DecodeInto(hblock.RawData(), smas)
	if err != nil {
		t.Fatal(err)
	}

	if smas.Owner != h.From {
		t.Fatalf("Owner should be %s, but is %s", h.From, smas.Owner)
	}
}
