package vm

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"time"

	"github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"
	"golang.org/x/xerrors"

	ffi "github.com/filecoin-project/filecoin-ffi"
	ffi_cgo "github.com/filecoin-project/filecoin-ffi/cgo"
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/exitcode"

	"github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/adt"
	"github.com/filecoin-project/lotus/chain/actors/aerrors"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/actors/policy"
	"github.com/filecoin-project/lotus/chain/state"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/lib/sigs"
)

var _ Interface = (*FVM)(nil)
var _ ffi_cgo.Externs = (*FvmExtern)(nil)

type FvmExtern struct {
	Rand
	blockstore.Blockstore
	epoch   abi.ChainEpoch
	lbState LookbackStateGetter
	base    cid.Cid
}

// This may eventually become identical to ExecutionTrace, but we can make incremental progress towards that
type FvmExecutionTrace struct {
	Msg    *types.Message
	MsgRct *types.MessageReceipt
	Error  string

	Subcalls []FvmExecutionTrace
}

func (t *FvmExecutionTrace) ToExecutionTrace() types.ExecutionTrace {
	if t == nil {
		return types.ExecutionTrace{}
	}

	ret := types.ExecutionTrace{
		Msg:      t.Msg,
		MsgRct:   t.MsgRct,
		Error:    t.Error,
		Subcalls: nil, // Should be nil when there are no subcalls for backwards compatibility
	}

	if len(t.Subcalls) > 0 {
		ret.Subcalls = make([]types.ExecutionTrace, len(t.Subcalls))

		for i, v := range t.Subcalls {
			ret.Subcalls[i] = v.ToExecutionTrace()
		}
	}

	return ret
}

// VerifyConsensusFault is similar to the one in syscalls.go used by the Lotus VM, except it never errors
// Errors are logged and "no fault" is returned, which is functionally what go-actors does anyway
func (x *FvmExtern) VerifyConsensusFault(ctx context.Context, a, b, extra []byte) (*ffi_cgo.ConsensusFault, int64) {
	totalGas := int64(0)
	ret := &ffi_cgo.ConsensusFault{
		Type: ffi_cgo.ConsensusFaultNone,
	}

	// Note that block syntax is not validated. Any validly signed block will be accepted pursuant to the below conditions.
	// Whether or not it could ever have been accepted in a chain is not checked/does not matter here.
	// for that reason when checking block parent relationships, rather than instantiating a Tipset to do so
	// (which runs a syntactic check), we do it directly on the CIDs.

	// (0) cheap preliminary checks

	// can blocks be decoded properly?
	var blockA, blockB types.BlockHeader
	if decodeErr := blockA.UnmarshalCBOR(bytes.NewReader(a)); decodeErr != nil {
		log.Info("invalid consensus fault: cannot decode first block header: %w", decodeErr)
		return ret, totalGas
	}

	if decodeErr := blockB.UnmarshalCBOR(bytes.NewReader(b)); decodeErr != nil {
		log.Info("invalid consensus fault: cannot decode second block header: %w", decodeErr)
		return ret, totalGas
	}

	// are blocks the same?
	if blockA.Cid().Equals(blockB.Cid()) {
		log.Info("invalid consensus fault: submitted blocks are the same")
		return ret, totalGas
	}
	// (1) check conditions necessary to any consensus fault

	// were blocks mined by same miner?
	if blockA.Miner != blockB.Miner {
		log.Info("invalid consensus fault: blocks not mined by the same miner")
		return ret, totalGas
	}

	// block a must be earlier or equal to block b, epoch wise (ie at least as early in the chain).
	if blockB.Height < blockA.Height {
		log.Info("invalid consensus fault: first block must not be of higher height than second")
		return ret, totalGas
	}

	ret.Epoch = blockB.Height

	faultType := ffi_cgo.ConsensusFaultNone

	// (2) check for the consensus faults themselves
	// (a) double-fork mining fault
	if blockA.Height == blockB.Height {
		faultType = ffi_cgo.ConsensusFaultDoubleForkMining
	}

	// (b) time-offset mining fault
	// strictly speaking no need to compare heights based on double fork mining check above,
	// but at same height this would be a different fault.
	if types.CidArrsEqual(blockA.Parents, blockB.Parents) && blockA.Height != blockB.Height {
		faultType = ffi_cgo.ConsensusFaultTimeOffsetMining
	}

	// (c) parent-grinding fault
	// Here extra is the "witness", a third block that shows the connection between A and B as
	// A's sibling and B's parent.
	// Specifically, since A is of lower height, it must be that B was mined omitting A from its tipset
	//
	//      B
	//      |
	//  [A, C]
	var blockC types.BlockHeader
	if len(extra) > 0 {
		if decodeErr := blockC.UnmarshalCBOR(bytes.NewReader(extra)); decodeErr != nil {
			log.Info("invalid consensus fault: cannot decode extra: %w", decodeErr)
			return ret, totalGas
		}

		if types.CidArrsEqual(blockA.Parents, blockC.Parents) && blockA.Height == blockC.Height &&
			types.CidArrsContains(blockB.Parents, blockC.Cid()) && !types.CidArrsContains(blockB.Parents, blockA.Cid()) {
			faultType = ffi_cgo.ConsensusFaultParentGrinding
		}
	}

	// (3) return if no consensus fault by now
	if faultType == ffi_cgo.ConsensusFaultNone {
		log.Info("invalid consensus fault: no fault detected")
		return ret, totalGas
	}

	// else
	// (4) expensive final checks

	// check blocks are properly signed by their respective miner
	// note we do not need to check extra's: it is a parent to block b
	// which itself is signed, so it was willingly included by the miner
	gasA, sigErr := x.VerifyBlockSig(ctx, &blockA)
	totalGas += gasA
	if sigErr != nil {
		log.Info("invalid consensus fault: cannot verify first block sig: %w", sigErr)
		return ret, totalGas
	}

	gas2, sigErr := x.VerifyBlockSig(ctx, &blockB)
	totalGas += gas2
	if sigErr != nil {
		log.Info("invalid consensus fault: cannot verify second block sig: %w", sigErr)
		return ret, totalGas
	}

	ret.Type = faultType
	ret.Target = blockA.Miner

	return ret, totalGas
}

func (x *FvmExtern) VerifyBlockSig(ctx context.Context, blk *types.BlockHeader) (int64, error) {
	waddr, gasUsed, err := x.workerKeyAtLookback(ctx, blk.Miner, blk.Height)
	if err != nil {
		return gasUsed, err
	}

	return gasUsed, sigs.CheckBlockSignature(ctx, blk, waddr)
}

func (x *FvmExtern) workerKeyAtLookback(ctx context.Context, minerId address.Address, height abi.ChainEpoch) (address.Address, int64, error) {
	if height < x.epoch-policy.ChainFinality {
		return address.Undef, 0, xerrors.Errorf("cannot get worker key (currEpoch %d, height %d)", x.epoch, height)
	}

	gasUsed := int64(0)
	gasAdder := func(gc GasCharge) {
		// technically not overflow safe, but that's fine
		gasUsed += gc.Total()
	}

	cstWithoutGas := cbor.NewCborStore(x.Blockstore)
	cbb := &gasChargingBlocks{gasAdder, PricelistByEpoch(x.epoch), x.Blockstore}
	cstWithGas := cbor.NewCborStore(cbb)

	lbState, err := x.lbState(ctx, height)
	if err != nil {
		return address.Undef, gasUsed, err
	}
	// get appropriate miner actor
	act, err := lbState.GetActor(minerId)
	if err != nil {
		return address.Undef, gasUsed, err
	}

	// use that to get the miner state
	mas, err := miner.Load(adt.WrapStore(ctx, cstWithGas), act)
	if err != nil {
		return address.Undef, gasUsed, err
	}

	info, err := mas.Info()
	if err != nil {
		return address.Undef, gasUsed, err
	}

	stateTree, err := state.LoadStateTree(cstWithoutGas, x.base)
	if err != nil {
		return address.Undef, gasUsed, err
	}

	raddr, err := ResolveToKeyAddr(stateTree, cstWithGas, info.Worker)
	if err != nil {
		return address.Undef, gasUsed, err
	}

	return raddr, gasUsed, nil
}

type FVM struct {
	fvm *ffi.FVM
}

func NewFVM(ctx context.Context, opts *VMOpts) (*FVM, error) {
	state, err := state.LoadStateTree(cbor.NewCborStore(opts.Bstore), opts.StateBase)
	if err != nil {
		return nil, err
	}

	circToReport, err := opts.CircSupplyCalc(ctx, opts.Epoch, state)
	if err != nil {
		return nil, err
	}

	fvmopts := &ffi.FVMOpts{
		FVMVersion: 0,
		Externs: &FvmExtern{
			Rand:       opts.Rand,
			Blockstore: opts.Bstore,
			lbState:    opts.LookbackState,
			base:       opts.StateBase,
			epoch:      opts.Epoch,
		},
		Epoch:          opts.Epoch,
		BaseFee:        opts.BaseFee,
		BaseCircSupply: circToReport,
		NetworkVersion: opts.NetworkVersion,
		StateBase:      opts.StateBase,
		Tracing:        EnableDetailedTracing,
	}

	if os.Getenv("LOTUS_USE_FVM_CUSTOM_BUNDLE") == "1" {
		av, err := actors.VersionForNetwork(opts.NetworkVersion)
		if err != nil {
			return nil, xerrors.Errorf("mapping network version to actors version: %w", err)
		}

		c, ok := actors.GetManifest(av)
		if !ok {
			return nil, xerrors.Errorf("no manifest for custom bundle (actors version %d)", av)
		}

		fvmopts.Manifest = c
	}

	fvm, err := ffi.CreateFVM(fvmopts)

	if err != nil {
		return nil, err
	}

	return &FVM{
		fvm: fvm,
	}, nil
}

func (vm *FVM) ApplyMessage(ctx context.Context, cmsg types.ChainMsg) (*ApplyRet, error) {
	start := build.Clock.Now()
	vmMsg := cmsg.VMMessage()
	msgBytes, err := vmMsg.Serialize()
	if err != nil {
		return nil, xerrors.Errorf("serializing msg: %w", err)
	}

	ret, err := vm.fvm.ApplyMessage(msgBytes, uint(cmsg.ChainLength()))
	if err != nil {
		return nil, xerrors.Errorf("applying msg: %w", err)
	}

	duration := time.Since(start)
	receipt := types.MessageReceipt{
		Return:   ret.Return,
		ExitCode: exitcode.ExitCode(ret.ExitCode),
		GasUsed:  ret.GasUsed,
	}

	var aerr aerrors.ActorError
	if ret.ExitCode != 0 {
		amsg := ret.FailureInfo
		if amsg == "" {
			amsg = "unknown error"
		}
		aerr = aerrors.New(exitcode.ExitCode(ret.ExitCode), amsg)
	}

	var et types.ExecutionTrace
	if len(ret.ExecTraceBytes) != 0 {
		var fvmEt FvmExecutionTrace
		if err = fvmEt.UnmarshalCBOR(bytes.NewReader(ret.ExecTraceBytes)); err != nil {
			return nil, xerrors.Errorf("failed to unmarshal exectrace: %w", err)
		}
		et = fvmEt.ToExecutionTrace()
	} else {
		et.Msg = vmMsg
		et.MsgRct = &receipt
		et.Duration = duration
		if aerr != nil {
			et.Error = aerr.Error()
		}
	}

	return &ApplyRet{
		MessageReceipt: receipt,
		GasCosts: &GasOutputs{
			BaseFeeBurn:        ret.BaseFeeBurn,
			OverEstimationBurn: ret.OverEstimationBurn,
			MinerPenalty:       ret.MinerPenalty,
			MinerTip:           ret.MinerTip,
			Refund:             ret.Refund,
			GasRefund:          ret.GasRefund,
			GasBurned:          ret.GasBurned,
		},
		ActorErr:       aerr,
		ExecutionTrace: et,
		Duration:       duration,
	}, nil
}

func (vm *FVM) ApplyImplicitMessage(ctx context.Context, cmsg *types.Message) (*ApplyRet, error) {
	start := build.Clock.Now()
	vmMsg := cmsg.VMMessage()
	msgBytes, err := vmMsg.Serialize()
	if err != nil {
		return nil, xerrors.Errorf("serializing msg: %w", err)
	}
	ret, err := vm.fvm.ApplyImplicitMessage(msgBytes)
	if err != nil {
		return nil, xerrors.Errorf("applying msg: %w", err)
	}

	duration := time.Since(start)
	receipt := types.MessageReceipt{
		Return:   ret.Return,
		ExitCode: exitcode.ExitCode(ret.ExitCode),
		GasUsed:  ret.GasUsed,
	}

	var aerr aerrors.ActorError
	if ret.ExitCode != 0 {
		amsg := ret.FailureInfo
		if amsg == "" {
			amsg = "unknown error"
		}
		aerr = aerrors.New(exitcode.ExitCode(ret.ExitCode), amsg)
	}

	var et types.ExecutionTrace
	if len(ret.ExecTraceBytes) != 0 {
		var fvmEt FvmExecutionTrace
		if err = fvmEt.UnmarshalCBOR(bytes.NewReader(ret.ExecTraceBytes)); err != nil {
			return nil, xerrors.Errorf("failed to unmarshal exectrace: %w", err)
		}
		et = fvmEt.ToExecutionTrace()
	} else {
		et.Msg = vmMsg
		et.MsgRct = &receipt
		et.Duration = duration
		if aerr != nil {
			et.Error = aerr.Error()
		}
	}

	applyRet := &ApplyRet{
		MessageReceipt: receipt,
		ActorErr:       aerr,
		ExecutionTrace: et,
		Duration:       duration,
	}

	if ret.ExitCode != 0 {
		return applyRet, fmt.Errorf("implicit message failed with exit code: %d and error: %w", ret.ExitCode, applyRet.ActorErr)
	}

	return applyRet, nil
}

func (vm *FVM) Flush(ctx context.Context) (cid.Cid, error) {
	return vm.fvm.Flush()
}
