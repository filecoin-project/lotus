package vm

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	block "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"
	logging "github.com/ipfs/go-log/v2"
	"github.com/multiformats/go-multicodec"
	cbg "github.com/whyrusleeping/cbor-gen"
	"go.opencensus.io/stats"
	"go.opencensus.io/trace"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	builtin_types "github.com/filecoin-project/go-state-types/builtin"
	"github.com/filecoin-project/go-state-types/exitcode"
	"github.com/filecoin-project/go-state-types/network"

	"github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/actors/adt"
	"github.com/filecoin-project/lotus/chain/actors/aerrors"
	"github.com/filecoin-project/lotus/chain/actors/builtin"
	"github.com/filecoin-project/lotus/chain/actors/builtin/account"
	"github.com/filecoin-project/lotus/chain/actors/builtin/reward"
	"github.com/filecoin-project/lotus/chain/rand"
	"github.com/filecoin-project/lotus/chain/state"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/metrics"
)

const MaxCallDepth = 4096

var (
	log            = logging.Logger("vm")
	actorLog       = logging.WithSkip(logging.Logger("actors"), 1)
	gasOnActorExec = newGasCharge("OnActorExec", 0, 0)
)

// ResolveToDeterministicAddr returns the public key type of address
// (`BLS`/`SECP256K1`) of an actor identified by `addr`, or its
// delegated address.
func ResolveToDeterministicAddr(state types.StateTree, cst cbor.IpldStore, addr address.Address) (address.Address, error) {
	if addr.Protocol() == address.BLS || addr.Protocol() == address.SECP256K1 || addr.Protocol() == address.Delegated {
		return addr, nil
	}

	act, err := state.GetActor(addr)
	if err != nil {
		return address.Undef, xerrors.Errorf("failed to find actor: %s", addr)
	}

	if state.Version() >= types.StateTreeVersion5 {
		if act.DelegatedAddress != nil {
			// If there _is_ an f4 address, return it as "key" address
			return *act.DelegatedAddress, nil
		}
	}

	aast, err := account.Load(adt.WrapStore(context.TODO(), cst), act)
	if err != nil {
		return address.Undef, xerrors.Errorf("failed to get account actor state for %s: %w", addr, err)
	}
	return aast.PubkeyAddress()

}

var (
	_ cbor.IpldBlockstore = (*gasChargingBlocks)(nil)
	_ blockstore.Viewer   = (*gasChargingBlocks)(nil)
)

type gasChargingBlocks struct {
	chargeGas func(GasCharge)
	pricelist Pricelist
	under     cbor.IpldBlockstore
}

func (bs *gasChargingBlocks) View(ctx context.Context, c cid.Cid, cb func([]byte) error) error {
	if v, ok := bs.under.(blockstore.Viewer); ok {
		bs.chargeGas(bs.pricelist.OnIpldGet())
		return v.View(ctx, c, func(b []byte) error {
			// we have successfully retrieved the value; charge for it, even if the user-provided function fails.
			bs.chargeGas(newGasCharge("OnIpldViewEnd", 0, 0).WithExtra(len(b)))
			bs.chargeGas(gasOnActorExec)
			return cb(b)
		})
	}
	// the underlying blockstore doesn't implement the viewer interface, fall back to normal Get behavior.
	blk, err := bs.Get(ctx, c)
	if err == nil && blk != nil {
		return cb(blk.RawData())
	}
	return err
}

func (bs *gasChargingBlocks) Get(ctx context.Context, c cid.Cid) (block.Block, error) {
	bs.chargeGas(bs.pricelist.OnIpldGet())
	blk, err := bs.under.Get(ctx, c)
	if err != nil {
		return nil, aerrors.Escalate(err, "failed to get block from blockstore")
	}
	bs.chargeGas(newGasCharge("OnIpldGetEnd", 0, 0).WithExtra(len(blk.RawData())))
	bs.chargeGas(gasOnActorExec)

	return blk, nil
}

func (bs *gasChargingBlocks) Put(ctx context.Context, blk block.Block) error {
	bs.chargeGas(bs.pricelist.OnIpldPut(len(blk.RawData())))

	if err := bs.under.Put(ctx, blk); err != nil {
		return aerrors.Escalate(err, "failed to write data to disk")
	}
	bs.chargeGas(gasOnActorExec)
	return nil
}

func (vm *LegacyVM) makeRuntime(ctx context.Context, msg *types.Message, parent *Runtime) *Runtime {
	paramsCodec := uint64(0)
	if len(msg.Params) > 0 {
		paramsCodec = uint64(multicodec.Cbor)
	}
	rt := &Runtime{
		ctx:         ctx,
		vm:          vm,
		state:       vm.cstate,
		origin:      msg.From,
		originNonce: msg.Nonce,
		height:      vm.blockHeight,

		gasUsed:          0,
		gasAvailable:     msg.GasLimit,
		depth:            0,
		numActorsCreated: 0,
		pricelist:        PricelistByEpoch(vm.blockHeight),
		allowInternal:    true,
		callerValidated:  false,
		executionTrace: types.ExecutionTrace{Msg: types.MessageTrace{
			From:        msg.From,
			To:          msg.To,
			Value:       msg.Value,
			Method:      msg.Method,
			Params:      msg.Params,
			ParamsCodec: paramsCodec,
		}},
	}

	if parent != nil {
		// TODO: The version check here should be unnecessary, but we can wait to take it out
		if !parent.allowInternal && rt.NetworkVersion() >= network.Version7 {
			rt.Abortf(exitcode.SysErrForbidden, "internal calls currently disabled")
		}
		rt.gasUsed = parent.gasUsed
		rt.origin = parent.origin
		rt.originNonce = parent.originNonce
		rt.numActorsCreated = parent.numActorsCreated
		rt.depth = parent.depth + 1
	}

	if rt.depth > MaxCallDepth && rt.NetworkVersion() >= network.Version6 {
		rt.Abortf(exitcode.SysErrForbidden, "message execution exceeds call depth")
	}

	cbb := &gasChargingBlocks{rt.chargeGasFunc(2), rt.pricelist, vm.cst.Blocks}
	cst := cbor.NewCborStore(cbb)
	cst.Atlas = vm.cst.Atlas // associate the atlas.
	rt.cst = cst

	vmm := *msg
	resF, ok := rt.ResolveAddress(msg.From)
	if !ok {
		rt.Abortf(exitcode.SysErrInvalidReceiver, "resolve msg.From address failed")
	}
	vmm.From = resF

	if vm.networkVersion <= network.Version3 {
		rt.Message = &vmm
	} else {
		resT, _ := rt.ResolveAddress(msg.To)
		// may be set to undef if recipient doesn't exist yet
		vmm.To = resT
		rt.Message = &Message{msg: vmm}
	}

	rt.Syscalls = pricedSyscalls{
		under:     vm.Syscalls(ctx, rt),
		chargeGas: rt.chargeGasFunc(1),
		pl:        rt.pricelist,
	}

	return rt
}

type UnsafeVM struct {
	VM *LegacyVM
}

func (vm *UnsafeVM) MakeRuntime(ctx context.Context, msg *types.Message) *Runtime {
	return vm.VM.makeRuntime(ctx, msg, nil)
}

type (
	CircSupplyCalculator func(context.Context, abi.ChainEpoch, *state.StateTree) (abi.TokenAmount, error)
	NtwkVersionGetter    func(context.Context, abi.ChainEpoch) network.Version
	LookbackStateGetter  func(context.Context, abi.ChainEpoch) (*state.StateTree, error)
	TipSetGetter         func(context.Context, abi.ChainEpoch) (types.TipSetKey, error)
)

var _ Interface = (*LegacyVM)(nil)

type LegacyVM struct {
	cstate         *state.StateTree
	cst            *cbor.BasicIpldStore
	buf            *blockstore.BufferedBlockstore
	blockHeight    abi.ChainEpoch
	areg           *ActorRegistry
	rand           rand.Rand
	circSupplyCalc CircSupplyCalculator
	networkVersion network.Version
	baseFee        abi.TokenAmount
	lbStateGet     LookbackStateGetter
	baseCircSupply abi.TokenAmount

	Syscalls SyscallBuilder
}

type VMOpts struct {
	StateBase      cid.Cid
	Epoch          abi.ChainEpoch
	Timestamp      uint64
	Rand           rand.Rand
	Bstore         blockstore.Blockstore
	Actors         *ActorRegistry
	Syscalls       SyscallBuilder
	CircSupplyCalc CircSupplyCalculator
	NetworkVersion network.Version
	BaseFee        abi.TokenAmount
	LookbackState  LookbackStateGetter
	TipSetGetter   TipSetGetter
	Tracing        bool
	// ReturnEvents decodes and returns emitted events.
	ReturnEvents bool
	// ExecutionLane specifies the execution priority of the created vm
	ExecutionLane ExecutionLane
}

func NewLegacyVM(ctx context.Context, opts *VMOpts) (*LegacyVM, error) {
	if opts.NetworkVersion >= network.Version16 {
		return nil, xerrors.Errorf("the legacy VM does not support network versions 16+")
	}

	buf := blockstore.NewBuffered(opts.Bstore)
	cst := cbor.NewCborStore(buf)
	state, err := state.LoadStateTree(cst, opts.StateBase)
	if err != nil {
		return nil, err
	}

	baseCirc, err := opts.CircSupplyCalc(ctx, opts.Epoch, state)
	if err != nil {
		return nil, err
	}

	return &LegacyVM{
		cstate:         state,
		cst:            cst,
		buf:            buf,
		blockHeight:    opts.Epoch,
		areg:           opts.Actors,
		rand:           opts.Rand, // TODO: Probably should be a syscall
		circSupplyCalc: opts.CircSupplyCalc,
		networkVersion: opts.NetworkVersion,
		Syscalls:       opts.Syscalls,
		baseFee:        opts.BaseFee,
		baseCircSupply: baseCirc,
		lbStateGet:     opts.LookbackState,
	}, nil
}

type ApplyRet struct {
	types.MessageReceipt
	ActorErr       aerrors.ActorError
	ExecutionTrace types.ExecutionTrace
	Duration       time.Duration
	GasCosts       *GasOutputs
	Events         []types.Event
}

func (vm *LegacyVM) send(ctx context.Context, msg *types.Message, parent *Runtime,
	gasCharge *GasCharge, start time.Time) ([]byte, aerrors.ActorError, *Runtime) {
	defer atomic.AddUint64(&StatSends, 1)

	st := vm.cstate

	rt := vm.makeRuntime(ctx, msg, parent)
	if EnableDetailedTracing {
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
			parent.gasUsed = rt.gasUsed
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
			if errors.Is(err, types.ErrActorNotFound) {
				a, aid, err := TryCreateAccountActor(rt, msg.To)
				if err != nil {
					return nil, aerrors.Wrapf(err, "could not create account")
				}
				toActor = a
				if vm.networkVersion > network.Version3 {
					nmsg := Message{
						msg: types.Message{
							To:    aid,
							From:  rt.Message.Caller(),
							Value: rt.Message.ValueReceived(),
						},
					}
					rt.Message = &nmsg
				} // else leave the rt.Message as is
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
			if err := vm.transfer(msg.From, msg.To, msg.Value, vm.networkVersion); err != nil {
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

	retCodec := uint64(0)
	if len(ret) > 0 {
		retCodec = uint64(multicodec.Cbor)
	}
	rt.executionTrace.MsgRct = types.ReturnTrace{
		ExitCode:    aerrors.RetCode(err),
		Return:      ret,
		ReturnCodec: retCodec,
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

func (vm *LegacyVM) ApplyImplicitMessage(ctx context.Context, msg *types.Message) (*ApplyRet, error) {
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
		GasCosts:       nil,
		Duration:       time.Since(start),
	}, actorErr
}

func (vm *LegacyVM) ApplyMessage(ctx context.Context, cmsg types.ChainMsg) (*ApplyRet, error) {
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
			GasCosts: &gasOutputs,
			Duration: time.Since(start),
			ActorErr: aerrors.Newf(exitcode.SysErrOutOfGas,
				"message gas limit does not cover on-chain gas costs"),
		}, nil
	}

	st := vm.cstate

	minerPenaltyAmount := types.BigMul(vm.baseFee, abi.NewTokenAmount(msg.GasLimit))
	fromActor, err := st.GetActor(msg.From)
	// this should never happen, but is currently still exercised by some tests
	if err != nil {
		if errors.Is(err, types.ErrActorNotFound) {
			gasOutputs := ZeroGasOutputs()
			gasOutputs.MinerPenalty = minerPenaltyAmount
			return &ApplyRet{
				MessageReceipt: types.MessageReceipt{
					ExitCode: exitcode.SysErrSenderInvalid,
					GasUsed:  0,
				},
				ActorErr: aerrors.Newf(exitcode.SysErrSenderInvalid, "actor not found: %s", msg.From),
				GasCosts: &gasOutputs,
				Duration: time.Since(start),
			}, nil
		}
		return nil, xerrors.Errorf("failed to look up from actor: %w", err)
	}

	// this should never happen, but is currently still exercised by some tests
	if !builtin.IsAccountActor(fromActor.Code) {
		gasOutputs := ZeroGasOutputs()
		gasOutputs.MinerPenalty = minerPenaltyAmount
		return &ApplyRet{
			MessageReceipt: types.MessageReceipt{
				ExitCode: exitcode.SysErrSenderInvalid,
				GasUsed:  0,
			},
			ActorErr: aerrors.Newf(exitcode.SysErrSenderInvalid, "send from not account actor: %s", fromActor.Code),
			GasCosts: &gasOutputs,
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

			GasCosts: &gasOutputs,
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
			GasCosts: &gasOutputs,
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

	burn, err := vm.ShouldBurn(ctx, st, msg, errcode)
	if err != nil {
		return nil, xerrors.Errorf("deciding whether should burn failed: %w", err)
	}

	gasOutputs := ComputeGasOutputs(gasUsed, msg.GasLimit, vm.baseFee, msg.GasFeeCap, msg.GasPremium, burn)

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
		GasCosts:       &gasOutputs,
		Duration:       time.Since(start),
	}, nil
}

func (vm *LegacyVM) ShouldBurn(ctx context.Context, st *state.StateTree, msg *types.Message, errcode exitcode.ExitCode) (bool, error) {
	if vm.networkVersion <= network.Version12 {
		// Check to see if we should burn funds. We avoid burning on successful
		// window post. This won't catch _indirect_ window post calls, but this
		// is the best we can get for now.
		if vm.blockHeight > buildconstants.UpgradeClausHeight && errcode == exitcode.Ok && msg.Method == builtin_types.MethodsMiner.SubmitWindowedPoSt {
			// Ok, we've checked the _method_, but we still need to check
			// the target actor. It would be nice if we could just look at
			// the trace, but I'm not sure if that's safe?
			if toActor, err := st.GetActor(msg.To); err != nil {
				// If the actor wasn't found, we probably deleted it or something. Move on.
				if !errors.Is(err, types.ErrActorNotFound) {
					// Otherwise, this should never fail and something is very wrong.
					return false, xerrors.Errorf("failed to lookup target actor: %w", err)
				}
			} else if builtin.IsStorageMinerActor(toActor.Code) {
				// Ok, this is a storage miner and we've processed a window post. Remove the burn.
				return false, nil
			}
		}

		return true, nil
	}

	// Any "don't burn" rules from Network v13 onwards go here, for now we always return true
	return true, nil
}

type vmFlushKey struct{}

func (vm *LegacyVM) Flush(ctx context.Context) (cid.Cid, error) {
	_, span := trace.StartSpan(ctx, "vm.Flush")
	defer span.End()

	from := vm.buf
	to := vm.buf.Read()

	root, err := vm.cstate.Flush(ctx)
	if err != nil {
		return cid.Undef, xerrors.Errorf("flushing vm: %w", err)
	}

	if err := Copy(context.WithValue(ctx, vmFlushKey{}, true), from, to, root); err != nil {
		return cid.Undef, xerrors.Errorf("copying tree: %w", err)
	}

	return root, nil
}

// ActorStore gets the buffered blockstore associated with the LegacyVM. This includes any temporary
// blocks produced during this LegacyVM's execution.
func (vm *LegacyVM) ActorStore(ctx context.Context) adt.Store {
	return adt.WrapStore(ctx, vm.cst)
}

func linksForObj(blk block.Block, cb func(cid.Cid)) error {
	switch multicodec.Code(blk.Cid().Prefix().Codec) {
	case multicodec.DagCbor:
		err := cbg.ScanForLinks(bytes.NewReader(blk.RawData()), cb)
		if err != nil {
			return xerrors.Errorf("cbg.ScanForLinks: %w", err)
		}
		return nil
	case multicodec.Raw, multicodec.Cbor:
		// We implicitly have all children of raw/cbor blocks.
		return nil
	default:
		return xerrors.Errorf("vm flush copy method only supports dag cbor")
	}
}

func Copy(ctx context.Context, from, to blockstore.Blockstore, root cid.Cid) error {
	ctx, span := trace.StartSpan(ctx, "vm.Copy") // nolint
	defer span.End()
	start := time.Now()

	var numBlocks int
	var totalCopySize int

	const batchSize = 128
	const bufCount = 3
	freeBufs := make(chan []block.Block, bufCount)
	toFlush := make(chan []block.Block, bufCount)
	for i := 0; i < bufCount; i++ {
		freeBufs <- make([]block.Block, 0, batchSize)
	}

	errFlushChan := make(chan error)

	go func() {
		for b := range toFlush {
			if err := to.PutMany(ctx, b); err != nil {
				close(freeBufs)
				errFlushChan <- xerrors.Errorf("batch put in copy: %w", err)
				return
			}
			freeBufs <- b[:0]
		}
		close(errFlushChan)
		close(freeBufs)
	}()

	batch := <-freeBufs
	batchCp := func(blk block.Block) error {
		numBlocks++
		totalCopySize += len(blk.RawData())

		batch = append(batch, blk)

		if len(batch) >= batchSize {
			toFlush <- batch
			var ok bool
			batch, ok = <-freeBufs
			if !ok {
				return <-errFlushChan
			}
		}
		return nil
	}

	if err := copyRec(ctx, from, to, root, batchCp); err != nil {
		return xerrors.Errorf("copyRec: %w", err)
	}

	if len(batch) > 0 {
		toFlush <- batch
	}
	close(toFlush)        // close the toFlush triggering the loop to end
	err := <-errFlushChan // get error out or get nil if it was closed
	if err != nil {
		return err
	}

	span.AddAttributes(
		trace.Int64Attribute("numBlocks", int64(numBlocks)),
		trace.Int64Attribute("copySize", int64(totalCopySize)),
	)
	if yes, ok := ctx.Value(vmFlushKey{}).(bool); yes && ok {
		took := metrics.SinceInMilliseconds(start)
		stats.Record(ctx, metrics.VMFlushCopyCount.M(int64(numBlocks)), metrics.VMFlushCopyDuration.M(took))
	}

	return nil
}

func copyRec(ctx context.Context, from, to blockstore.Blockstore, root cid.Cid, cp func(block.Block) error) error {
	if root.Prefix().MhType == 0 {
		// identity cid, skip
		return nil
	}

	blk, err := from.Get(ctx, root)
	if err != nil {
		return xerrors.Errorf("get %s failed: %w", root, err)
	}

	var lerr error
	err = linksForObj(blk, func(link cid.Cid) {
		if lerr != nil {
			// There's no error return on linksForObj callback :(
			return
		}

		prefix := link.Prefix()
		codec := multicodec.Code(prefix.Codec)
		switch codec {
		case multicodec.FilCommitmentSealed, cid.FilCommitmentUnsealed:
			return
		}

		// We always have blocks inlined into CIDs, but we may not have their children.
		if multicodec.Code(prefix.MhType) == multicodec.Identity {
			// Unless the inlined block has no children.
			switch codec {
			case multicodec.Raw, multicodec.Cbor:
				return
			}
		} else {
			// If we have an object, we already have its children, skip the object.
			has, err := to.Has(ctx, link)
			if err != nil {
				lerr = xerrors.Errorf("has: %w", err)
				return
			}
			if has {
				return
			}
		}

		if err := copyRec(ctx, from, to, link, cp); err != nil {
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

func (vm *LegacyVM) StateTree() types.StateTree {
	return vm.cstate
}

func (vm *LegacyVM) Invoke(act *types.Actor, rt *Runtime, method abi.MethodNum, params []byte) ([]byte, aerrors.ActorError) {
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
	ret, err := vm.areg.Invoke(act.Code, rt, method, params)
	if err != nil {
		return nil, err
	}
	return ret, nil
}

func (vm *LegacyVM) SetInvoker(i *ActorRegistry) {
	vm.areg = i
}

func (vm *LegacyVM) GetCircSupply(ctx context.Context) (abi.TokenAmount, error) {
	// Before v15, this was recalculated on each invocation as the state tree was mutated
	if vm.networkVersion <= network.Version14 {
		return vm.circSupplyCalc(ctx, vm.blockHeight, vm.cstate)
	}

	return vm.baseCircSupply, nil
}

func (vm *LegacyVM) incrementNonce(addr address.Address) error {
	return vm.cstate.MutateActor(addr, func(a *types.Actor) error {
		a.Nonce++
		return nil
	})
}

func (vm *LegacyVM) transfer(from, to address.Address, amt types.BigInt, networkVersion network.Version) aerrors.ActorError {
	var f *types.Actor
	var fromID, toID address.Address
	var err error
	// switching the order around so that transactions for more than the balance sent to self fail
	if networkVersion >= network.Version15 {
		if amt.LessThan(types.NewInt(0)) {
			return aerrors.Newf(exitcode.SysErrForbidden, "attempted to transfer negative value: %s", amt)
		}

		fromID, err = vm.cstate.LookupIDAddress(from)
		if err != nil {
			return aerrors.Fatalf("transfer failed when resolving sender address: %s", err)
		}

		f, err = vm.cstate.GetActor(fromID)
		if err != nil {
			return aerrors.Fatalf("transfer failed when retrieving sender actor: %s", err)
		}

		if f.Balance.LessThan(amt) {
			return aerrors.Newf(exitcode.SysErrInsufficientFunds, "transfer failed, insufficient balance in sender actor: %v", f.Balance)
		}

		if from == to {
			log.Infow("sending to same address: noop", "from/to addr", from)
			return nil
		}

		toID, err = vm.cstate.LookupIDAddress(to)
		if err != nil {
			return aerrors.Fatalf("transfer failed when resolving receiver address: %s", err)
		}

		if fromID == toID {
			log.Infow("sending to same actor ID: noop", "from/to actor", fromID)
			return nil
		}
	} else {
		if from == to {
			return nil
		}

		fromID, err = vm.cstate.LookupIDAddress(from)
		if err != nil {
			return aerrors.Fatalf("transfer failed when resolving sender address: %s", err)
		}

		toID, err = vm.cstate.LookupIDAddress(to)
		if err != nil {
			return aerrors.Fatalf("transfer failed when resolving receiver address: %s", err)
		}

		if fromID == toID {
			return nil
		}

		if amt.LessThan(types.NewInt(0)) {
			return aerrors.Newf(exitcode.SysErrForbidden, "attempted to transfer negative value: %s", amt)
		}

		f, err = vm.cstate.GetActor(fromID)
		if err != nil {
			return aerrors.Fatalf("transfer failed when retrieving sender actor: %s", err)
		}
	}

	t, err := vm.cstate.GetActor(toID)
	if err != nil {
		return aerrors.Fatalf("transfer failed when retrieving receiver actor: %s", err)
	}

	if err = deductFunds(f, amt); err != nil {
		return aerrors.Newf(exitcode.SysErrInsufficientFunds, "transfer failed when deducting funds (%s): %s", types.FIL(amt), err)
	}
	depositFunds(t, amt)

	if err = vm.cstate.SetActor(fromID, f); err != nil {
		return aerrors.Fatalf("transfer failed when setting sender actor: %s", err)
	}

	if err = vm.cstate.SetActor(toID, t); err != nil {
		return aerrors.Fatalf("transfer failed when setting receiver actor: %s", err)
	}

	return nil
}

func (vm *LegacyVM) transferToGasHolder(addr address.Address, gasHolder *types.Actor, amt types.BigInt) error {
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

func (vm *LegacyVM) transferFromGasHolder(addr address.Address, gasHolder *types.Actor, amt types.BigInt) error {
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
