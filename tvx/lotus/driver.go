package lotus

import (
	"context"
	"fmt"

	"github.com/filecoin-project/lotus/chain/state"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/vm"
	"github.com/filecoin-project/lotus/lib/blockstore"
	"github.com/filecoin-project/sector-storage/ffiwrapper"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/puppet"
	"github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"
)

type Driver struct {
	ctx context.Context
}

func NewDriver(ctx context.Context) *Driver {
	return &Driver{ctx: ctx}
}

func (d *Driver) ExecuteMessage(msg *types.Message, preroot cid.Cid, bs blockstore.Blockstore, epoch abi.ChainEpoch) (*vm.ApplyRet, cid.Cid, error) {
	fmt.Println("execution sanity check")
	cst := cbor.NewCborStore(bs)
	st, err := state.LoadStateTree(cst, preroot)
	if err != nil {
		return nil, cid.Undef, err
	}

	actor, err := st.GetActor(msg.From)
	if err != nil {
		fmt.Println("from actor not found: ", msg.From)
	} else {
		fmt.Println("from actor found: ", actor)
	}

	fmt.Println("creating vm")
	lvm, err := vm.NewVM(preroot, epoch, &vmRand{}, bs, mkFakedSigSyscalls(vm.Syscalls(ffiwrapper.ProofVerifier)), nil)
	if err != nil {
		return nil, cid.Undef, err
	}
	// need to modify the VM invoker to add the puppet actor
	chainValInvoker := vm.NewInvoker()
	chainValInvoker.Register(puppet.PuppetActorCodeID, puppet.Actor{}, puppet.State{})
	lvm.SetInvoker(chainValInvoker)
	if err != nil {
		return nil, cid.Undef, err
	}

	fmt.Println("applying message")
	ret, err := lvm.ApplyMessage(d.ctx, msg)
	if err != nil {
		return nil, cid.Undef, err
	}

	fmt.Printf("applied message: %+v\n", ret)

	fmt.Println("flushing")
	root, err := lvm.Flush(d.ctx)
	return ret, root, err
}
