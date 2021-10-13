package conformance

import (
	"context"
	gobig "math/big"
	"os"

	"github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/chain/consensus/filcns"
	"github.com/filecoin-project/lotus/chain/state"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/vm"
	"github.com/filecoin-project/lotus/conformance/chaos"
	"github.com/filecoin-project/lotus/extern/sector-storage/ffiwrapper"

	_ "github.com/filecoin-project/lotus/lib/sigs/bls"  // enable bls signatures
	_ "github.com/filecoin-project/lotus/lib/sigs/secp" // enable secp signatures

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/crypto"

	"github.com/filecoin-project/test-vectors/schema"

	"github.com/filecoin-project/go-address"

	"github.com/ipfs/go-cid"
	ds "github.com/ipfs/go-datastore"
)

var (
	// DefaultCirculatingSupply is the fallback circulating supply returned by
	// the driver's CircSupplyCalculator function, used if the vector specifies
	// no circulating supply.
	DefaultCirculatingSupply = types.TotalFilecoinInt

	// DefaultBaseFee to use in the VM, if one is not supplied in the vector.
	DefaultBaseFee = abi.NewTokenAmount(100)
)

type Driver struct {
	ctx      context.Context
	selector schema.Selector
	vmFlush  bool
}

type DriverOpts struct {
	// DisableVMFlush, when true, avoids calling VM.Flush(), forces a blockstore
	// recursive copy, from the temporary buffer blockstore, to the real
	// system's blockstore. Disabling VM flushing is useful when extracting test
	// vectors and trimming state, as we don't want to force an accidental
	// deep copy of the state tree.
	//
	// Disabling VM flushing almost always should go hand-in-hand with
	// LOTUS_DISABLE_VM_BUF=iknowitsabadidea. That way, state tree writes are
	// immediately committed to the blockstore.
	DisableVMFlush bool
}

func NewDriver(ctx context.Context, selector schema.Selector, opts DriverOpts) *Driver {
	return &Driver{ctx: ctx, selector: selector, vmFlush: !opts.DisableVMFlush}
}

type ExecuteTipsetResult struct {
	ReceiptsRoot  cid.Cid
	PostStateRoot cid.Cid

	// AppliedMessages stores the messages that were applied, in the order they
	// were applied. It includes implicit messages (cron, rewards).
	AppliedMessages []*types.Message
	// AppliedResults stores the results of AppliedMessages, in the same order.
	AppliedResults []*vm.ApplyRet

	// PostBaseFee returns the basefee after applying this tipset.
	PostBaseFee abi.TokenAmount
}

type ExecuteTipsetParams struct {
	Preroot cid.Cid
	// ParentEpoch is the last epoch in which an actual tipset was processed. This
	// is used by Lotus for null block counting and cron firing.
	ParentEpoch abi.ChainEpoch
	Tipset      *schema.Tipset
	ExecEpoch   abi.ChainEpoch
	// Rand is an optional vm.Rand implementation to use. If nil, the driver
	// will use a vm.Rand that returns a fixed value for all calls.
	Rand vm.Rand
	// BaseFee if not nil or zero, will override the basefee of the tipset.
	BaseFee abi.TokenAmount
}

// ExecuteTipset executes the supplied tipset on top of the state represented
// by the preroot CID.
//
// This method returns the the receipts root, the poststate root, and the VM
// message results. The latter _include_ implicit messages, such as cron ticks
// and reward withdrawal per miner.
func (d *Driver) ExecuteTipset(bs blockstore.Blockstore, ds ds.Batching, params ExecuteTipsetParams) (*ExecuteTipsetResult, error) {
	var (
		tipset   = params.Tipset
		syscalls = vm.Syscalls(ffiwrapper.ProofVerifier)

		cs      = store.NewChainStore(bs, bs, ds, filcns.Weight, nil)
		tse     = filcns.NewTipSetExecutor()
		sm, err = stmgr.NewStateManager(cs, tse, syscalls, filcns.DefaultUpgradeSchedule(), nil)
	)
	if err != nil {
		return nil, err
	}

	if params.Rand == nil {
		params.Rand = NewFixedRand()
	}

	if params.BaseFee.NilOrZero() {
		params.BaseFee = abi.NewTokenAmount(tipset.BaseFee.Int64())
	}

	defer cs.Close() //nolint:errcheck

	blocks := make([]filcns.FilecoinBlockMessages, 0, len(tipset.Blocks))
	for _, b := range tipset.Blocks {
		sb := store.BlockMessages{
			Miner: b.MinerAddr,
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
		blocks = append(blocks, filcns.FilecoinBlockMessages{
			BlockMessages: sb,
			WinCount:      b.WinCount,
		})
	}

	recordOutputs := &outputRecorder{
		messages: []*types.Message{},
		results:  []*vm.ApplyRet{},
	}

	postcid, receiptsroot, err := tse.ApplyBlocks(context.Background(),
		sm,
		params.ParentEpoch,
		params.Preroot,
		blocks,
		params.ExecEpoch,
		params.Rand,
		recordOutputs,
		params.BaseFee,
		nil,
	)

	if err != nil {
		return nil, err
	}

	ret := &ExecuteTipsetResult{
		ReceiptsRoot:    receiptsroot,
		PostStateRoot:   postcid,
		AppliedMessages: recordOutputs.messages,
		AppliedResults:  recordOutputs.results,
	}
	return ret, nil
}

type ExecuteMessageParams struct {
	Preroot    cid.Cid
	Epoch      abi.ChainEpoch
	Message    *types.Message
	CircSupply abi.TokenAmount
	BaseFee    abi.TokenAmount

	// Rand is an optional vm.Rand implementation to use. If nil, the driver
	// will use a vm.Rand that returns a fixed value for all calls.
	Rand vm.Rand
}

// ExecuteMessage executes a conformance test vector message in a temporary VM.
func (d *Driver) ExecuteMessage(bs blockstore.Blockstore, params ExecuteMessageParams) (*vm.ApplyRet, cid.Cid, error) {
	if !d.vmFlush {
		// do not flush the VM, just the state tree; this should be used with
		// LOTUS_DISABLE_VM_BUF enabled, so writes will anyway be visible.
		_ = os.Setenv("LOTUS_DISABLE_VM_BUF", "iknowitsabadidea")
	}

	if params.Rand == nil {
		params.Rand = NewFixedRand()
	}

	// dummy state manager; only to reference the GetNetworkVersion method,
	// which does not depend on state.
	sm, err := stmgr.NewStateManager(nil, filcns.NewTipSetExecutor(), nil, filcns.DefaultUpgradeSchedule(), nil)
	if err != nil {
		return nil, cid.Cid{}, err
	}

	vmOpts := &vm.VMOpts{
		StateBase: params.Preroot,
		Epoch:     params.Epoch,
		Bstore:    bs,
		Syscalls:  vm.Syscalls(ffiwrapper.ProofVerifier),
		CircSupplyCalc: func(_ context.Context, _ abi.ChainEpoch, _ *state.StateTree) (abi.TokenAmount, error) {
			return params.CircSupply, nil
		},
		Rand:        params.Rand,
		BaseFee:     params.BaseFee,
		NtwkVersion: sm.GetNtwkVersion,
	}

	lvm, err := vm.NewVM(context.TODO(), vmOpts)
	if err != nil {
		return nil, cid.Undef, err
	}

	invoker := filcns.NewActorRegistry()

	// register the chaos actor if required by the vector.
	if chaosOn, ok := d.selector["chaos_actor"]; ok && chaosOn == "true" {
		invoker.Register(nil, chaos.Actor{})
	}

	lvm.SetInvoker(invoker)

	ret, err := lvm.ApplyMessage(d.ctx, toChainMsg(params.Message))
	if err != nil {
		return nil, cid.Undef, err
	}

	var root cid.Cid
	if d.vmFlush {
		// flush the VM, committing the state tree changes and forcing a
		// recursive copoy from the temporary blcokstore to the real blockstore.
		root, err = lvm.Flush(d.ctx)
	} else {
		root, err = lvm.StateTree().(*state.StateTree).Flush(d.ctx)
	}

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

// BaseFeeOrDefault converts a basefee as passed in a test vector (go *big.Int
// type) to an abi.TokenAmount, or if nil it returns the DefaultBaseFee.
func BaseFeeOrDefault(basefee *gobig.Int) abi.TokenAmount {
	if basefee == nil {
		return DefaultBaseFee
	}
	return big.NewFromGo(basefee)
}

// CircSupplyOrDefault converts a circulating supply as passed in a test vector
// (go *big.Int type) to an abi.TokenAmount, or if nil it returns the
// DefaultCirculatingSupply.
func CircSupplyOrDefault(circSupply *gobig.Int) abi.TokenAmount {
	if circSupply == nil {
		return DefaultCirculatingSupply
	}
	return big.NewFromGo(circSupply)
}

type outputRecorder struct {
	messages []*types.Message
	results  []*vm.ApplyRet
}

func (o *outputRecorder) MessageApplied(ctx context.Context, ts *types.TipSet, mcid cid.Cid, msg *types.Message, ret *vm.ApplyRet, implicit bool) error {
	o.messages = append(o.messages, msg)
	o.results = append(o.results, ret)
	return nil
}
