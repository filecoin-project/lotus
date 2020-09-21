package vm

import (
	"bytes"
	"context"
	"fmt"
	"reflect"
	"sync/atomic"
	"time"

	block "github.com/ipfs/go-block-format"
	cid "github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"
	logging "github.com/ipfs/go-log/v2"
	mh "github.com/multiformats/go-multihash"
	cbg "github.com/whyrusleeping/cbor-gen"
	"go.opencensus.io/trace"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/crypto"
	"github.com/filecoin-project/go-state-types/exitcode"
	"github.com/filecoin-project/go-state-types/network"
	"github.com/filecoin-project/specs-actors/actors/builtin"

	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors/adt"
	"github.com/filecoin-project/lotus/chain/actors/aerrors"
	"github.com/filecoin-project/lotus/chain/actors/builtin/account"
	"github.com/filecoin-project/lotus/chain/actors/builtin/reward"
	"github.com/filecoin-project/lotus/chain/state"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/lib/blockstore"
	bstore "github.com/filecoin-project/lotus/lib/blockstore"
	"github.com/filecoin-project/lotus/lib/bufbstore"
)

var log = logging.Logger("vm")
var actorLog = logging.Logger("actors")
var gasOnActorExec = newGasCharge("OnActorExec", 0, 0)

// stat counters
var (
	StatSends   uint64
	StatApplied uint64
)

// ResolveToKeyAddr returns the public key type of address (`BLS`/`SECP256K1`) of an account actor identified by `addr`.
func ResolveToKeyAddr(state types.StateTree, cst cbor.IpldStore, addr address.Address) (address.Address, error) {
	if addr.Protocol() == address.BLS || addr.Protocol() == address.SECP256K1 {
		return addr, nil
	}

	act, err := state.GetActor(addr)
	if err != nil {
		return address.Undef, xerrors.Errorf("failed to find actor: %s", addr)
	}

	aast, err := account.Load(adt.WrapStore(context.TODO(), cst), act)
	if err != nil {
		return address.Undef, xerrors.Errorf("failed to get account actor state for %s: %w", addr, err)
	}

	return aast.PubkeyAddress()
}

var _ cbor.IpldBlockstore = (*gasChargingBlocks)(nil)

type gasChargingBlocks struct {
	chargeGas func(GasCharge)
	pricelist Pricelist
	under     cbor.IpldBlockstore
}

func (bs *gasChargingBlocks) Get(c cid.Cid) (block.Block, error) {
	bs.chargeGas(bs.pricelist.OnIpldGet())
	blk, err := bs.under.Get(c)
	if err != nil {
		return nil, aerrors.Escalate(err, "failed to get block from blockstore")
	}
	bs.chargeGas(newGasCharge("OnIpldGetEnd", 0, 0).WithExtra(len(blk.RawData())))
	bs.chargeGas(gasOnActorExec)

	return blk, nil
}

func (bs *gasChargingBlocks) Put(blk block.Block) error {
	bs.chargeGas(bs.pricelist.OnIpldPut(len(blk.RawData())))

	if err := bs.under.Put(blk); err != nil {
		return aerrors.Escalate(err, "failed to write data to disk")
	}
	bs.chargeGas(gasOnActorExec)
	return nil
}

func (vm *VM) makeRuntime(ctx context.Context, msg *types.Message, origin address.Address, originNonce uint64, usedGas int64, nac uint64) *Runtime {
	rt := &Runtime{
		ctx:         ctx,
		vm:          vm,
		state:       vm.cstate,
		origin:      origin,
		originNonce: originNonce,
		height:      vm.blockHeight,

		gasUsed:          usedGas,
		gasAvailable:     msg.GasLimit,
		numActorsCreated: nac,
		pricelist:        PricelistByEpoch(vm.blockHeight),
		allowInternal:    true,
		callerValidated:  false,
		executionTrace:   types.ExecutionTrace{Msg: msg},
	}

	rt.cst = &cbor.BasicIpldStore{
		Blocks: &gasChargingBlocks{rt.chargeGasFunc(2), rt.pricelist, vm.cst.Blocks},
		Atlas:  vm.cst.Atlas,
	}
	rt.Syscalls = pricedSyscalls{
		under:     vm.Syscalls(ctx, vm.cstate, rt.cst),
		chargeGas: rt.chargeGasFunc(1),
		pl:        rt.pricelist,
	}

	vmm := *msg
	resF, ok := rt.ResolveAddress(msg.From)
	if !ok {
		rt.Abortf(exitcode.SysErrInvalidReceiver, "resolve msg.From address failed")
	}
	vmm.From = resF
	rt.Message = vmm

	return rt
}

type UnsafeVM struct {
	VM *VM
}

func (vm *UnsafeVM) MakeRuntime(ctx context.Context, msg *types.Message, origin address.Address, originNonce uint64, usedGas int64, nac uint64) *Runtime {
	return vm.VM.makeRuntime(ctx, msg, origin, originNonce, usedGas, nac)
}

type CircSupplyCalculator func(context.Context, abi.ChainEpoch, *state.StateTree) (abi.TokenAmount, error)
type NtwkVersionGetter func(context.Context, abi.ChainEpoch) network.Version

type VM struct {
	cstate         *state.StateTree
	base           cid.Cid
	cst            *cbor.BasicIpldStore
	buf            *bufbstore.BufferedBS
	blockHeight    abi.ChainEpoch
	inv            *Invoker
	rand           Rand
	circSupplyCalc CircSupplyCalculator
	ntwkVersion    NtwkVersionGetter
	baseFee        abi.TokenAmount

	Syscalls SyscallBuilder
}

type VMOpts struct {
	StateBase      cid.Cid
	Epoch          abi.ChainEpoch
	Rand           Rand
	Bstore         bstore.Blockstore
	Syscalls       SyscallBuilder
	CircSupplyCalc CircSupplyCalculator
	NtwkVersion    NtwkVersionGetter // TODO: stebalien: In what cases do we actually need this? It seems like even when creating new networks we want to use the 'global'/build-default version getter
	BaseFee        abi.TokenAmount
}

func NewVM(ctx context.Context, opts *VMOpts) (*VM, error) {
	buf := bufbstore.NewBufferedBstore(opts.Bstore)
	cst := cbor.NewCborStore(buf)
	state, err := state.LoadStateTree(cst, opts.StateBase)
	if err != nil {
		return nil, err
	}

	return &VM{
		cstate:         state,
		base:           opts.StateBase,
		cst:            cst,
		buf:            buf,
		blockHeight:    opts.Epoch,
		inv:            NewInvoker(),
		rand:           opts.Rand, // TODO: Probably should be a syscall
		circSupplyCalc: opts.CircSupplyCalc,
		ntwkVersion:    opts.NtwkVersion,
		Syscalls:       opts.Syscalls,
		baseFee:        opts.BaseFee,
	}, nil
}

type Rand interface {
	GetChainRandomness(ctx context.Context, pers crypto.DomainSeparationTag, round abi.ChainEpoch, entropy []byte) ([]byte, error)
	GetBeaconRandomness(ctx context.Context, pers crypto.DomainSeparationTag, round abi.ChainEpoch, entropy []byte) ([]byte, error)
}

type ApplyRet struct {
	types.MessageReceipt
	ActorErr       aerrors.ActorError
	ExecutionTrace types.ExecutionTrace
	Duration       time.Duration
	GasCosts       GasOutputs
}

func (vm *VM) send(ctx context.Context, msg *types.Message, parent *Runtime,
	gasCharge *GasCharge, start time.Time) ([]byte, aerrors.ActorError, *Runtime) {

	defer atomic.AddUint64(&StatSends, 1)

	st := vm.cstate

	origin := msg.From
	on := msg.Nonce
	var nac uint64 = 0
	var gasUsed int64
	if parent != nil {
		gasUsed = parent.gasUsed
		origin = parent.origin
		on = parent.originNonce
		nac = parent.numActorsCreated
	}

	rt := vm.makeRuntime(ctx, msg, origin, on, gasUsed, nac)
	if enableTracing {
		rt.lastGasChargeTime = start
		if parent != nil {
			rt.lastGasChargeTime = parent.lastGasChargeTime
			rt.lastGasCharge = parent.lastGasCharge
			defer func() {
				parent.lastGasChargeTime = rt.lastGasChargeTime
				parent.lastGasCharge = rt.lastGasCharge
			}()
		}
	}

	if parent != nil {
		defer func() {
			parent.gasUsed += rt.gasUsed
		}()
	}
	if gasCharge != nil {
		if err := rt.chargeGasSafe(*gasCharge); err != nil {
			// this should never happen
			return nil, aerrors.Wrap(err, "not enough gas for initial message charge, this should not happen"), rt
		}
	}

	ret, err := func() ([]byte, aerrors.ActorError) {
		_ = rt.chargeGasSafe(newGasCharge("OnGetActor", 0, 0))
		toActor, err := st.GetActor(msg.To)
		if err != nil {
			if xerrors.Is(err, types.ErrActorNotFound) {
				a, err := TryCreateAccountActor(rt, msg.To)
				if err != nil {
					return nil, aerrors.Wrapf(err, "could not create account")
				}
				toActor = a
			} else {
				return nil, aerrors.Escalate(err, "getting actor")
			}
		}

		if aerr := rt.chargeGasSafe(rt.Pricelist().OnMethodInvocation(msg.Value, msg.Method)); aerr != nil {
			return nil, aerrors.Wrap(aerr, "not enough gas for method invocation")
		}

		// not charging any gas, just logging
		//nolint:errcheck
		defer rt.chargeGasSafe(newGasCharge("OnMethodInvocationDone", 0, 0))

		if types.BigCmp(msg.Value, types.NewInt(0)) != 0 {
			if err := vm.transfer(msg.From, msg.To, msg.Value); err != nil {
				return nil, aerrors.Wrap(err, "failed to transfer funds")
			}
		}

		if msg.Method != 0 {
			var ret []byte
			_ = rt.chargeGasSafe(gasOnActorExec)
			ret, err := vm.Invoke(toActor, rt, msg.Method, msg.Params)
			return ret, err
		}
		return nil, nil
	}()

	mr := types.MessageReceipt{
		ExitCode: aerrors.RetCode(err),
		Return:   ret,
		GasUsed:  rt.gasUsed,
	}
	rt.executionTrace.MsgRct = &mr
	rt.executionTrace.Duration = time.Since(start)
	if err != nil {
		rt.executionTrace.Error = err.Error()
	}

	return ret, err, rt
}

func checkMessage(msg *types.Message) error {
	if msg.GasLimit == 0 {
		return xerrors.Errorf("message has no gas limit set")
	}
	if msg.GasLimit < 0 {
		return xerrors.Errorf("message has negative gas limit")
	}

	if msg.GasFeeCap == types.EmptyInt {
		return xerrors.Errorf("message fee cap not set")
	}

	if msg.GasPremium == types.EmptyInt {
		return xerrors.Errorf("message gas premium not set")
	}

	if msg.Value == types.EmptyInt {
		return xerrors.Errorf("message no value set")
	}

	return nil
}

func (vm *VM) ApplyImplicitMessage(ctx context.Context, msg *types.Message) (*ApplyRet, error) {
	start := build.Clock.Now()
	defer atomic.AddUint64(&StatApplied, 1)
	ret, actorErr, rt := vm.send(ctx, msg, nil, nil, start)
	rt.finilizeGasTracing()
	return &ApplyRet{
		MessageReceipt: types.MessageReceipt{
			ExitCode: aerrors.RetCode(actorErr),
			Return:   ret,
			GasUsed:  0,
		},
		ActorErr:       actorErr,
		ExecutionTrace: rt.executionTrace,
		GasCosts:       GasOutputs{},
		Duration:       time.Since(start),
	}, actorErr
}

func (vm *VM) ApplyMessage(ctx context.Context, cmsg types.ChainMsg) (*ApplyRet, error) {
	start := build.Clock.Now()
	ctx, span := trace.StartSpan(ctx, "vm.ApplyMessage")
	defer span.End()
	defer atomic.AddUint64(&StatApplied, 1)
	msg := cmsg.VMMessage()
	if span.IsRecordingEvents() {
		span.AddAttributes(
			trace.StringAttribute("to", msg.To.String()),
			trace.Int64Attribute("method", int64(msg.Method)),
			trace.StringAttribute("value", msg.Value.String()),
		)
	}

	if err := checkMessage(msg); err != nil {
		return nil, err
	}

	pl := PricelistByEpoch(vm.blockHeight)

	msgGas := pl.OnChainMessage(cmsg.ChainLength())
	msgGasCost := msgGas.Total()
	// this should never happen, but is currently still exercised by some tests
	if msgGasCost > msg.GasLimit {
		gasOutputs := ZeroGasOutputs()
		gasOutputs.MinerPenalty = types.BigMul(vm.baseFee, abi.NewTokenAmount(msgGasCost))
		return &ApplyRet{
			MessageReceipt: types.MessageReceipt{
				ExitCode: exitcode.SysErrOutOfGas,
				GasUsed:  0,
			},
			GasCosts: gasOutputs,
			Duration: time.Since(start),
		}, nil
	}

	st := vm.cstate

	minerPenaltyAmount := types.BigMul(vm.baseFee, abi.NewTokenAmount(msg.GasLimit))
	fromActor, err := st.GetActor(msg.From)
	// this should never happen, but is currently still exercised by some tests
	if err != nil {
		if xerrors.Is(err, types.ErrActorNotFound) {
			gasOutputs := ZeroGasOutputs()
			gasOutputs.MinerPenalty = minerPenaltyAmount
			return &ApplyRet{
				MessageReceipt: types.MessageReceipt{
					ExitCode: exitcode.SysErrSenderInvalid,
					GasUsed:  0,
				},
				ActorErr: aerrors.Newf(exitcode.SysErrSenderInvalid, "actor not found: %s", msg.From),
				GasCosts: gasOutputs,
				Duration: time.Since(start),
			}, nil
		}
		return nil, xerrors.Errorf("failed to look up from actor: %w", err)
	}

	// this should never happen, but is currently still exercised by some tests
	if !fromActor.IsAccountActor() {
		gasOutputs := ZeroGasOutputs()
		gasOutputs.MinerPenalty = minerPenaltyAmount
		return &ApplyRet{
			MessageReceipt: types.MessageReceipt{
				ExitCode: exitcode.SysErrSenderInvalid,
				GasUsed:  0,
			},
			ActorErr: aerrors.Newf(exitcode.SysErrSenderInvalid, "send from not account actor: %s", fromActor.Code),
			GasCosts: gasOutputs,
			Duration: time.Since(start),
		}, nil
	}

	if msg.Nonce != fromActor.Nonce {
		gasOutputs := ZeroGasOutputs()
		gasOutputs.MinerPenalty = minerPenaltyAmount
		return &ApplyRet{
			MessageReceipt: types.MessageReceipt{
				ExitCode: exitcode.SysErrSenderStateInvalid,
				GasUsed:  0,
			},
			ActorErr: aerrors.Newf(exitcode.SysErrSenderStateInvalid,
				"actor nonce invalid: msg:%d != state:%d", msg.Nonce, fromActor.Nonce),

			GasCosts: gasOutputs,
			Duration: time.Since(start),
		}, nil
	}

	gascost := types.BigMul(types.NewInt(uint64(msg.GasLimit)), msg.GasFeeCap)
	if fromActor.Balance.LessThan(gascost) {
		gasOutputs := ZeroGasOutputs()
		gasOutputs.MinerPenalty = minerPenaltyAmount
		return &ApplyRet{
			MessageReceipt: types.MessageReceipt{
				ExitCode: exitcode.SysErrSenderStateInvalid,
				GasUsed:  0,
			},
			ActorErr: aerrors.Newf(exitcode.SysErrSenderStateInvalid,
				"actor balance less than needed: %s < %s", types.FIL(fromActor.Balance), types.FIL(gascost)),
			GasCosts: gasOutputs,
			Duration: time.Since(start),
		}, nil
	}

	gasHolder := &types.Actor{Balance: types.NewInt(0)}
	if err := vm.transferToGasHolder(msg.From, gasHolder, gascost); err != nil {
		return nil, xerrors.Errorf("failed to withdraw gas funds: %w", err)
	}

	if err := vm.incrementNonce(msg.From); err != nil {
		return nil, err
	}

	if err := st.Snapshot(ctx); err != nil {
		return nil, xerrors.Errorf("snapshot failed: %w", err)
	}
	defer st.ClearSnapshot()

	ret, actorErr, rt := vm.send(ctx, msg, nil, &msgGas, start)
	if aerrors.IsFatal(actorErr) {
		return nil, xerrors.Errorf("[from=%s,to=%s,n=%d,m=%d,h=%d] fatal error: %w", msg.From, msg.To, msg.Nonce, msg.Method, vm.blockHeight, actorErr)
	}

	if actorErr != nil {
		log.Warnw("Send actor error", "from", msg.From, "to", msg.To, "nonce", msg.Nonce, "method", msg.Method, "height", vm.blockHeight, "error", fmt.Sprintf("%+v", actorErr))
	}

	if actorErr != nil && len(ret) != 0 {
		// This should not happen, something is wonky
		return nil, xerrors.Errorf("message invocation errored, but had a return value anyway: %w", actorErr)
	}

	if rt == nil {
		return nil, xerrors.Errorf("send returned nil runtime, send error was: %s", actorErr)
	}

	if len(ret) != 0 {
		// safely override actorErr since it must be nil
		actorErr = rt.chargeGasSafe(rt.Pricelist().OnChainReturnValue(len(ret)))
		if actorErr != nil {
			ret = nil
		}
	}

	var errcode exitcode.ExitCode
	var gasUsed int64

	if errcode = aerrors.RetCode(actorErr); errcode != 0 {
		// revert all state changes since snapshot
		if err := st.Revert(); err != nil {
			return nil, xerrors.Errorf("revert state failed: %w", err)
		}
	}

	rt.finilizeGasTracing()

	gasUsed = rt.gasUsed
	if gasUsed < 0 {
		gasUsed = 0
	}
	gasOutputs := ComputeGasOutputs(gasUsed, msg.GasLimit, vm.baseFee, msg.GasFeeCap, msg.GasPremium)

	if err := vm.transferFromGasHolder(builtin.BurntFundsActorAddr, gasHolder,
		gasOutputs.BaseFeeBurn); err != nil {
		return nil, xerrors.Errorf("failed to burn base fee: %w", err)
	}

	if err := vm.transferFromGasHolder(reward.Address, gasHolder, gasOutputs.MinerTip); err != nil {
		return nil, xerrors.Errorf("failed to give miner gas reward: %w", err)
	}

	if err := vm.transferFromGasHolder(builtin.BurntFundsActorAddr, gasHolder,
		gasOutputs.OverEstimationBurn); err != nil {
		return nil, xerrors.Errorf("failed to burn overestimation fee: %w", err)
	}

	// refund unused gas
	if err := vm.transferFromGasHolder(msg.From, gasHolder, gasOutputs.Refund); err != nil {
		return nil, xerrors.Errorf("failed to refund gas: %w", err)
	}

	if types.BigCmp(types.NewInt(0), gasHolder.Balance) != 0 {
		return nil, xerrors.Errorf("gas handling math is wrong")
	}

	return &ApplyRet{
		MessageReceipt: types.MessageReceipt{
			ExitCode: errcode,
			Return:   ret,
			GasUsed:  gasUsed,
		},
		ActorErr:       actorErr,
		ExecutionTrace: rt.executionTrace,
		GasCosts:       gasOutputs,
		Duration:       time.Since(start),
	}, nil
}

func (vm *VM) ActorBalance(addr address.Address) (types.BigInt, aerrors.ActorError) {
	act, err := vm.cstate.GetActor(addr)
	if err != nil {
		return types.EmptyInt, aerrors.Absorb(err, 1, "failed to find actor")
	}

	return act.Balance, nil
}

func (vm *VM) Flush(ctx context.Context) (cid.Cid, error) {
	_, span := trace.StartSpan(ctx, "vm.Flush")
	defer span.End()

	from := vm.buf
	to := vm.buf.Read()

	root, err := vm.cstate.Flush(ctx)
	if err != nil {
		return cid.Undef, xerrors.Errorf("flushing vm: %w", err)
	}

	if err := Copy(ctx, from, to, root); err != nil {
		return cid.Undef, xerrors.Errorf("copying tree: %w", err)
	}

	return root, nil
}

// MutateState usage: MutateState(ctx, idAddr, func(cst cbor.IpldStore, st *ActorStateType) error {...})
func (vm *VM) MutateState(ctx context.Context, addr address.Address, fn interface{}) error {
	act, err := vm.cstate.GetActor(addr)
	if err != nil {
		return xerrors.Errorf("actor not found: %w", err)
	}

	st := reflect.New(reflect.TypeOf(fn).In(1).Elem())
	if err := vm.cst.Get(ctx, act.Head, st.Interface()); err != nil {
		return xerrors.Errorf("read actor head: %w", err)
	}

	out := reflect.ValueOf(fn).Call([]reflect.Value{reflect.ValueOf(vm.cst), st})
	if !out[0].IsNil() && out[0].Interface().(error) != nil {
		return out[0].Interface().(error)
	}

	head, err := vm.cst.Put(ctx, st.Interface())
	if err != nil {
		return xerrors.Errorf("put new actor head: %w", err)
	}

	act.Head = head

	if err := vm.cstate.SetActor(addr, act); err != nil {
		return xerrors.Errorf("set actor: %w", err)
	}

	return nil
}

func linksForObj(blk block.Block, cb func(cid.Cid)) error {
	switch blk.Cid().Prefix().Codec {
	case cid.DagCBOR:
		err := cbg.ScanForLinks(bytes.NewReader(blk.RawData()), cb)
		if err != nil {
			return xerrors.Errorf("cbg.ScanForLinks: %w", err)
		}
		return nil
	case cid.Raw:
		// We implicitly have all children of raw blocks.
		return nil
	default:
		return xerrors.Errorf("vm flush copy method only supports dag cbor")
	}
}

func Copy(ctx context.Context, from, to blockstore.Blockstore, root cid.Cid) error {
	ctx, span := trace.StartSpan(ctx, "vm.Copy") // nolint
	defer span.End()

	var numBlocks int
	var totalCopySize int

	var batch []block.Block
	batchCp := func(blk block.Block) error {
		numBlocks++
		totalCopySize += len(blk.RawData())

		batch = append(batch, blk)
		if len(batch) > 100 {
			if err := to.PutMany(batch); err != nil {
				return xerrors.Errorf("batch put in copy: %w", err)
			}
			batch = batch[:0]
		}
		return nil
	}

	if err := copyRec(from, to, root, batchCp); err != nil {
		return xerrors.Errorf("copyRec: %w", err)
	}

	if len(batch) > 0 {
		if err := to.PutMany(batch); err != nil {
			return xerrors.Errorf("batch put in copy: %w", err)
		}
	}

	span.AddAttributes(
		trace.Int64Attribute("numBlocks", int64(numBlocks)),
		trace.Int64Attribute("copySize", int64(totalCopySize)),
	)

	return nil
}

func copyRec(from, to blockstore.Blockstore, root cid.Cid, cp func(block.Block) error) error {
	if root.Prefix().MhType == 0 {
		// identity cid, skip
		return nil
	}

	blk, err := from.Get(root)
	if err != nil {
		return xerrors.Errorf("get %s failed: %w", root, err)
	}

	var lerr error
	err = linksForObj(blk, func(link cid.Cid) {
		if lerr != nil {
			// Theres no erorr return on linksForObj callback :(
			return
		}

		prefix := link.Prefix()
		if prefix.Codec == cid.FilCommitmentSealed || prefix.Codec == cid.FilCommitmentUnsealed {
			return
		}

		// We always have blocks inlined into CIDs, but we may not have their children.
		if prefix.MhType == mh.IDENTITY {
			// Unless the inlined block has no children.
			if prefix.Codec == cid.Raw {
				return
			}
		} else {
			// If we have an object, we already have its children, skip the object.
			has, err := to.Has(link)
			if err != nil {
				lerr = xerrors.Errorf("has: %w", err)
				return
			}
			if has {
				return
			}
		}

		if err := copyRec(from, to, link, cp); err != nil {
			lerr = err
			return
		}
	})
	if err != nil {
		return xerrors.Errorf("linksForObj (%x): %w", blk.RawData(), err)
	}
	if lerr != nil {
		return lerr
	}

	if err := cp(blk); err != nil {
		return xerrors.Errorf("copy: %w", err)
	}
	return nil
}

func (vm *VM) StateTree() types.StateTree {
	return vm.cstate
}

func (vm *VM) SetBlockHeight(h abi.ChainEpoch) {
	vm.blockHeight = h
}

func (vm *VM) Invoke(act *types.Actor, rt *Runtime, method abi.MethodNum, params []byte) ([]byte, aerrors.ActorError) {
	ctx, span := trace.StartSpan(rt.ctx, "vm.Invoke")
	defer span.End()
	if span.IsRecordingEvents() {
		span.AddAttributes(
			trace.StringAttribute("to", rt.Receiver().String()),
			trace.Int64Attribute("method", int64(method)),
			trace.StringAttribute("value", rt.ValueReceived().String()),
		)
	}

	var oldCtx context.Context
	oldCtx, rt.ctx = rt.ctx, ctx
	defer func() {
		rt.ctx = oldCtx
	}()
	ret, err := vm.inv.Invoke(act.Code, rt, method, params)
	if err != nil {
		return nil, err
	}
	return ret, nil
}

func (vm *VM) SetInvoker(i *Invoker) {
	vm.inv = i
}

func (vm *VM) GetNtwkVersion(ctx context.Context, ce abi.ChainEpoch) network.Version {
	return vm.ntwkVersion(ctx, ce)
}

func (vm *VM) GetCircSupply(ctx context.Context) (abi.TokenAmount, error) {
	return vm.circSupplyCalc(ctx, vm.blockHeight, vm.cstate)
}

func (vm *VM) incrementNonce(addr address.Address) error {
	return vm.cstate.MutateActor(addr, func(a *types.Actor) error {
		a.Nonce++
		return nil
	})
}

func (vm *VM) transfer(from, to address.Address, amt types.BigInt) aerrors.ActorError {
	if from == to {
		return nil
	}

	fromID, err := vm.cstate.LookupID(from)
	if err != nil {
		return aerrors.Fatalf("transfer failed when resolving sender address: %s", err)
	}

	toID, err := vm.cstate.LookupID(to)
	if err != nil {
		return aerrors.Fatalf("transfer failed when resolving receiver address: %s", err)
	}

	if fromID == toID {
		return nil
	}

	if amt.LessThan(types.NewInt(0)) {
		return aerrors.Newf(exitcode.SysErrForbidden, "attempted to transfer negative value: %s", amt)
	}

	f, err := vm.cstate.GetActor(fromID)
	if err != nil {
		return aerrors.Fatalf("transfer failed when retrieving sender actor: %s", err)
	}

	t, err := vm.cstate.GetActor(toID)
	if err != nil {
		return aerrors.Fatalf("transfer failed when retrieving receiver actor: %s", err)
	}

	if err := deductFunds(f, amt); err != nil {
		return aerrors.Newf(exitcode.SysErrInsufficientFunds, "transfer failed when deducting funds (%s): %s", types.FIL(amt), err)
	}
	depositFunds(t, amt)

	if err := vm.cstate.SetActor(fromID, f); err != nil {
		return aerrors.Fatalf("transfer failed when setting receiver actor: %s", err)
	}

	if err := vm.cstate.SetActor(toID, t); err != nil {
		return aerrors.Fatalf("transfer failed when setting sender actor: %s", err)
	}

	return nil
}

func (vm *VM) transferToGasHolder(addr address.Address, gasHolder *types.Actor, amt types.BigInt) error {
	if amt.LessThan(types.NewInt(0)) {
		return xerrors.Errorf("attempted to transfer negative value to gas holder")
	}

	return vm.cstate.MutateActor(addr, func(a *types.Actor) error {
		if err := deductFunds(a, amt); err != nil {
			return err
		}
		depositFunds(gasHolder, amt)
		return nil
	})
}

func (vm *VM) transferFromGasHolder(addr address.Address, gasHolder *types.Actor, amt types.BigInt) error {
	if amt.LessThan(types.NewInt(0)) {
		return xerrors.Errorf("attempted to transfer negative value from gas holder")
	}

	if amt.Equals(big.NewInt(0)) {
		return nil
	}

	return vm.cstate.MutateActor(addr, func(a *types.Actor) error {
		if err := deductFunds(gasHolder, amt); err != nil {
			return err
		}
		depositFunds(a, amt)
		return nil
	})
}

func deductFunds(act *types.Actor, amt types.BigInt) error {
	if act.Balance.LessThan(amt) {
		return fmt.Errorf("not enough funds")
	}

	act.Balance = types.BigSub(act.Balance, amt)
	return nil
}

func depositFunds(act *types.Actor, amt types.BigInt) {
	act.Balance = types.BigAdd(act.Balance, amt)
}
