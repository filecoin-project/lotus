package conformance

import (
	"context"

	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/puppet"
	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/vm"
	"github.com/filecoin-project/lotus/lib/blockstore"
	"github.com/filecoin-project/sector-storage/ffiwrapper"
)

var (
	// BaseFee to use in the VM.
	// TODO make parametrisable through vector.
	BaseFee = abi.NewTokenAmount(100)
)

type Driver struct {
	ctx context.Context
}

func NewDriver(ctx context.Context) *Driver {
	return &Driver{ctx: ctx}
}

// ExecuteMessage executes a conformance test vector message in a temporary VM.
func (d *Driver) ExecuteMessage(msg *types.Message, preroot cid.Cid, bs blockstore.Blockstore, epoch abi.ChainEpoch) (*vm.ApplyRet, cid.Cid, error) {
	vmOpts := &vm.VMOpts{
		StateBase:      preroot,
		Epoch:          epoch,
		Rand:           &testRand{}, // TODO always succeeds; need more flexibility.
		Bstore:         bs,
		Syscalls:       mkFakedSigSyscalls(vm.Syscalls(ffiwrapper.ProofVerifier)), // TODO always succeeds; need more flexibility.
		CircSupplyCalc: nil,
		BaseFee:        BaseFee,
	}

	lvm, err := vm.NewVM(vmOpts)
	if err != nil {
		return nil, cid.Undef, err
	}

	// add support for the puppet actor.
	invoker := vm.NewInvoker()
	invoker.Register(puppet.PuppetActorCodeID, puppet.Actor{}, puppet.State{})
	lvm.SetInvoker(invoker)

	ret, err := lvm.ApplyMessage(d.ctx, msg)
	if err != nil {
		return nil, cid.Undef, err
	}

	root, err := lvm.Flush(d.ctx)
	return ret, root, err
}
