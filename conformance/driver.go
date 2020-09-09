package conformance

import (
	"context"

	"github.com/filecoin-project/go-state-types/crypto"

	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/vm"
	"github.com/filecoin-project/lotus/extern/sector-storage/ffiwrapper"
	"github.com/filecoin-project/lotus/lib/blockstore"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/test-vectors/chaos"
	"github.com/filecoin-project/test-vectors/schema"

	"github.com/filecoin-project/go-address"

	"github.com/ipfs/go-cid"
	ds "github.com/ipfs/go-datastore"
)

var (
	// BaseFee to use in the VM.
	// TODO make parametrisable through vector.
	BaseFee = abi.NewTokenAmount(100)
)

type Driver struct {
	ctx      context.Context
	selector schema.Selector
}

func NewDriver(ctx context.Context, selector schema.Selector) *Driver {
	return &Driver{ctx: ctx, selector: selector}
}

type ExecuteTipsetResult struct {
	ReceiptsRoot  cid.Cid
	PostStateRoot cid.Cid

	// AppliedMessages stores the messages that were applied, in the order they
	// were applied. It includes implicit messages (cron, rewards).
	AppliedMessages []*types.Message
	// AppliedResults stores the results of AppliedMessages, in the same order.
	AppliedResults []*vm.ApplyRet
}

// ExecuteTipset executes the supplied tipset on top of the state represented
// by the preroot CID.
//
// parentEpoch is the last epoch in which an actual tipset was processed. This
// is used by Lotus for null block counting and cron firing.
//
// This method returns the the receipts root, the poststate root, and the VM
// message results. The latter _include_ implicit messages, such as cron ticks
// and reward withdrawal per miner.
func (d *Driver) ExecuteTipset(bs blockstore.Blockstore, ds ds.Batching, preroot cid.Cid, parentEpoch abi.ChainEpoch, tipset *schema.Tipset) (*ExecuteTipsetResult, error) {
	var (
		syscalls = mkFakedSigSyscalls(vm.Syscalls(ffiwrapper.ProofVerifier))
		vmRand   = new(testRand)

		cs = store.NewChainStore(bs, ds, syscalls)
		sm = stmgr.NewStateManager(cs)
	)

	blocks := make([]store.BlockMessages, 0, len(tipset.Blocks))
	for _, b := range tipset.Blocks {
		sb := store.BlockMessages{
			Miner:    b.MinerAddr,
			WinCount: b.WinCount,
		}
		for _, m := range b.Messages {
			msg, err := types.DecodeMessage(m)
			if err != nil {
				return nil, err
			}
			switch msg.From.Protocol() {
			case address.SECP256K1:
				sb.SecpkMessages = append(sb.SecpkMessages, toChainMsg(msg))
			case address.BLS:
				sb.BlsMessages = append(sb.BlsMessages, toChainMsg(msg))
			default:
				// sneak in messages originating from other addresses as both kinds.
				// these should fail, as they are actually invalid senders.
				sb.SecpkMessages = append(sb.SecpkMessages, msg)
				sb.BlsMessages = append(sb.BlsMessages, msg)
			}
		}
		blocks = append(blocks, sb)
	}

	var (
		messages []*types.Message
		results  []*vm.ApplyRet
	)

	postcid, receiptsroot, err := sm.ApplyBlocks(context.Background(), parentEpoch, preroot, blocks, tipset.Epoch, vmRand, func(_ cid.Cid, msg *types.Message, ret *vm.ApplyRet) error {
		messages = append(messages, msg)
		results = append(results, ret)
		return nil
	}, tipset.BaseFee, nil)

	if err != nil {
		return nil, err
	}

	ret := &ExecuteTipsetResult{
		ReceiptsRoot:    receiptsroot,
		PostStateRoot:   postcid,
		AppliedMessages: messages,
		AppliedResults:  results,
	}
	return ret, nil
}

// ExecuteMessage executes a conformance test vector message in a temporary VM.
func (d *Driver) ExecuteMessage(bs blockstore.Blockstore, preroot cid.Cid, epoch abi.ChainEpoch, msg *types.Message) (*vm.ApplyRet, cid.Cid, error) {
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

	invoker := vm.NewInvoker()

	// register the chaos actor if required by the vector.
	if chaosOn, ok := d.selector["chaos_actor"]; ok && chaosOn == "true" {
		invoker.Register(chaos.ChaosActorCodeCID, chaos.Actor{}, chaos.State{})
	}

	lvm.SetInvoker(invoker)

	ret, err := lvm.ApplyMessage(d.ctx, toChainMsg(msg))
	if err != nil {
		return nil, cid.Undef, err
	}

	root, err := lvm.Flush(d.ctx)
	return ret, root, err
}

// toChainMsg injects a synthetic 0-filled signature of the right length to
// messages that originate from secp256k senders, leaving all
// others untouched.
// TODO: generate a signature in the DSL so that it's encoded in
//  the test vector.
func toChainMsg(msg *types.Message) (ret types.ChainMsg) {
	ret = msg
	if msg.From.Protocol() == address.SECP256K1 {
		ret = &types.SignedMessage{
			Message: *msg,
			Signature: crypto.Signature{
				Type: crypto.SigTypeSecp256k1,
				Data: make([]byte, 65),
			},
		}
	}
	return ret
}
