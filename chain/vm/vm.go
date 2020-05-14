package vm

import (
	"bytes"
	"context"
	"fmt"
	"reflect"
	"time"

	"github.com/filecoin-project/specs-actors/actors/builtin"

	block "github.com/ipfs/go-block-format"
	cid "github.com/ipfs/go-cid"
	blockstore "github.com/ipfs/go-ipfs-blockstore"
	cbor "github.com/ipfs/go-ipld-cbor"
	logging "github.com/ipfs/go-log/v2"
	mh "github.com/multiformats/go-multihash"
	cbg "github.com/whyrusleeping/cbor-gen"
	"go.opencensus.io/trace"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	commcid "github.com/filecoin-project/go-fil-commcid"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/builtin/account"
	init_ "github.com/filecoin-project/specs-actors/actors/builtin/init"
	"github.com/filecoin-project/specs-actors/actors/crypto"
	"github.com/filecoin-project/specs-actors/actors/runtime"
	"github.com/filecoin-project/specs-actors/actors/runtime/exitcode"

	"github.com/filecoin-project/lotus/chain/actors/aerrors"
	"github.com/filecoin-project/lotus/chain/state"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/lib/bufbstore"
)

var log = logging.Logger("vm")

// ResolveToKeyAddr returns the public key type of address (`BLS`/`SECP256K1`) of an account actor identified by `addr`.
func ResolveToKeyAddr(state types.StateTree, cst cbor.IpldStore, addr address.Address) (address.Address, aerrors.ActorError) {
	if addr.Protocol() == address.BLS || addr.Protocol() == address.SECP256K1 {
		return addr, nil
	}

	act, err := state.GetActor(addr)
	if err != nil {
		return address.Undef, aerrors.Newf(exitcode.SysErrInvalidParameters, "failed to find actor: %s", addr)
	}

	if act.Code != builtin.AccountActorCodeID {
		return address.Undef, aerrors.Newf(exitcode.SysErrInvalidParameters, "address %s was not for an account actor", addr)
	}

	var aast account.State
	if err := cst.Get(context.TODO(), act.Head, &aast); err != nil {
		return address.Undef, aerrors.Absorb(err, exitcode.SysErrInvalidParameters, fmt.Sprintf("failed to get account actor state for %s", addr))
	}

	return aast.Address, nil
}

var _ cbor.IpldBlockstore = (*gasChargingBlocks)(nil)

type gasChargingBlocks struct {
	chargeGas func(int64)
	pricelist Pricelist
	under     cbor.IpldBlockstore
}

func (bs *gasChargingBlocks) Get(c cid.Cid) (block.Block, error) {
	blk, err := bs.under.Get(c)
	if err != nil {
		return nil, aerrors.Escalate(err, "failed to get block from blockstore")
	}
	bs.chargeGas(bs.pricelist.OnIpldGet(len(blk.RawData())))

	return blk, nil
}

func (bs *gasChargingBlocks) Put(blk block.Block) error {
	bs.chargeGas(bs.pricelist.OnIpldPut(len(blk.RawData())))

	if err := bs.under.Put(blk); err != nil {
		return aerrors.Escalate(err, "failed to write data to disk")
	}
	return nil
}

func (vm *VM) makeRuntime(ctx context.Context, msg *types.Message, origin address.Address, originNonce uint64, usedGas int64, nac uint64) *Runtime {
	rt := &Runtime{
		ctx:         ctx,
		vm:          vm,
		state:       vm.cstate,
		msg:         msg,
		origin:      origin,
		originNonce: originNonce,
		height:      vm.blockHeight,

		gasUsed:          usedGas,
		gasAvailable:     msg.GasLimit,
		numActorsCreated: nac,
		pricelist:        PricelistByEpoch(vm.blockHeight),
		allowInternal:    true,
		callerValidated:  false,
	}

	rt.cst = &cbor.BasicIpldStore{
		Blocks: &gasChargingBlocks{rt.ChargeGas, rt.pricelist, vm.cst.Blocks},
		Atlas:  vm.cst.Atlas,
	}
	rt.sys = pricedSyscalls{
		under:     vm.Syscalls,
		chargeGas: rt.ChargeGas,
		pl:        rt.pricelist,
	}

	vmm := *msg
	resF, ok := rt.ResolveAddress(msg.From)
	if !ok {
		rt.Abortf(exitcode.SysErrInvalidReceiver, "resolve msg.From address failed")
	}
	vmm.From = resF
	rt.vmsg = &vmm

	return rt
}

type VM struct {
	cstate      *state.StateTree
	base        cid.Cid
	cst         *cbor.BasicIpldStore
	buf         *bufbstore.BufferedBS
	blockHeight abi.ChainEpoch
	inv         *invoker
	rand        Rand

	Syscalls runtime.Syscalls
}

func NewVM(base cid.Cid, height abi.ChainEpoch, r Rand, cbs blockstore.Blockstore, syscalls runtime.Syscalls) (*VM, error) {
	buf := bufbstore.NewBufferedBstore(cbs)
	cst := cbor.NewCborStore(buf)
	state, err := state.LoadStateTree(cst, base)
	if err != nil {
		return nil, err
	}

	return &VM{
		cstate:      state,
		base:        base,
		cst:         cst,
		buf:         buf,
		blockHeight: height,
		inv:         NewInvoker(),
		rand:        r, // TODO: Probably should be a syscall
		Syscalls:    syscalls,
	}, nil
}

type Rand interface {
	GetRandomness(ctx context.Context, pers crypto.DomainSeparationTag, round abi.ChainEpoch, entropy []byte) ([]byte, error)
}

type ApplyRet struct {
	types.MessageReceipt
	ActorErr           aerrors.ActorError
	Penalty            types.BigInt
	InternalExecutions []*types.ExecutionResult
	Duration           time.Duration
}

func (vm *VM) send(ctx context.Context, msg *types.Message, parent *Runtime,
	gasCharge int64) ([]byte, aerrors.ActorError, *Runtime) {
	st := vm.cstate
	gasUsed := gasCharge

	origin := msg.From
	on := msg.Nonce
	var nac uint64 = 0
	if parent != nil {
		gasUsed = parent.gasUsed + gasUsed
		origin = parent.origin
		on = parent.originNonce
		nac = parent.numActorsCreated
	}

	rt := vm.makeRuntime(ctx, msg, origin, on, gasUsed, nac)
	if parent != nil {
		defer func() {
			parent.gasUsed = rt.gasUsed
		}()
	}

	if aerr := rt.chargeGasSafe(rt.Pricelist().OnMethodInvocation(msg.Value, msg.Method)); aerr != nil {
		return nil, aerrors.Wrap(aerr, "not enough gas for method invocation"), rt
	}

	toActor, err := st.GetActor(msg.To)
	if err != nil {
		if xerrors.Is(err, init_.ErrAddressNotFound) {
			a, err := TryCreateAccountActor(rt, msg.To)
			if err != nil {
				return nil, aerrors.Wrapf(err, "could not create account"), rt
			}
			toActor = a
		} else {
			return nil, aerrors.Escalate(err, "getting actor"), rt
		}
	}

	if types.BigCmp(msg.Value, types.NewInt(0)) != 0 {
		if err := vm.transfer(msg.From, msg.To, msg.Value); err != nil {
			return nil, aerrors.Wrap(err, "failed to transfer funds"), nil
		}
	}

	if msg.Method != 0 {
		ret, err := vm.Invoke(toActor, rt, msg.Method, msg.Params)
		return ret, err, rt
	}

	return nil, nil, rt
}

func checkMessage(msg *types.Message) error {
	if msg.GasLimit == 0 {
		return xerrors.Errorf("message has no gas limit set")
	}
	if msg.GasLimit < 0 {
		return xerrors.Errorf("message has negative gas limit")
	}

	if msg.GasPrice == types.EmptyInt {
		return xerrors.Errorf("message gas no gas price set")
	}

	if msg.Value == types.EmptyInt {
		return xerrors.Errorf("message no value set")
	}

	return nil
}

func (vm *VM) ApplyImplicitMessage(ctx context.Context, msg *types.Message) (*ApplyRet, error) {
	start := time.Now()
	ret, actorErr, rt := vm.send(ctx, msg, nil, 0)
	return &ApplyRet{
		MessageReceipt: types.MessageReceipt{
			ExitCode: exitcode.ExitCode(aerrors.RetCode(actorErr)),
			Return:   ret,
			GasUsed:  0,
		},
		ActorErr:           actorErr,
		InternalExecutions: rt.internalExecutions,
		Penalty:            types.NewInt(0),
		Duration:           time.Since(start),
	}, actorErr
}

func (vm *VM) ApplyMessage(ctx context.Context, cmsg types.ChainMsg) (*ApplyRet, error) {
	start := time.Now()
	ctx, span := trace.StartSpan(ctx, "vm.ApplyMessage")
	defer span.End()
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

	msgGasCost := pl.OnChainMessage(cmsg.ChainLength())
	// this should never happen, but is currently still exercised by some tests
	if msgGasCost > msg.GasLimit {
		return &ApplyRet{
			MessageReceipt: types.MessageReceipt{
				ExitCode: exitcode.SysErrOutOfGas,
				GasUsed:  0,
			},
			Penalty:  types.BigMul(msg.GasPrice, types.NewInt(uint64(msgGasCost))),
			Duration: time.Since(start),
		}, nil
	}

	st := vm.cstate

	minerPenaltyAmount := types.BigMul(msg.GasPrice, types.NewInt(uint64(msgGasCost)))
	fromActor, err := st.GetActor(msg.From)
	// this should never happen, but is currently still exercised by some tests
	if err != nil {
		if xerrors.Is(err, types.ErrActorNotFound) {
			return &ApplyRet{
				MessageReceipt: types.MessageReceipt{
					ExitCode: exitcode.SysErrSenderInvalid,
					GasUsed:  0,
				},
				Penalty:  minerPenaltyAmount,
				Duration: time.Since(start),
			}, nil
		}
		return nil, xerrors.Errorf("failed to look up from actor: %w", err)
	}

	// this should never happen, but is currently still exercised by some tests
	if !fromActor.Code.Equals(builtin.AccountActorCodeID) {
		return &ApplyRet{
			MessageReceipt: types.MessageReceipt{
				ExitCode: exitcode.SysErrSenderInvalid,
				GasUsed:  0,
			},
			Penalty:  minerPenaltyAmount,
			Duration: time.Since(start),
		}, nil
	}

	// TODO: We should remove this, we might punish miners for no fault of their own
	if msg.Nonce != fromActor.Nonce {
		return &ApplyRet{
			MessageReceipt: types.MessageReceipt{
				ExitCode: exitcode.SysErrSenderStateInvalid,
				GasUsed:  0,
			},
			Penalty:  minerPenaltyAmount,
			Duration: time.Since(start),
		}, nil
	}

	gascost := types.BigMul(types.NewInt(uint64(msg.GasLimit)), msg.GasPrice)
	totalCost := types.BigAdd(gascost, msg.Value)
	if fromActor.Balance.LessThan(totalCost) {
		return &ApplyRet{
			MessageReceipt: types.MessageReceipt{
				ExitCode: exitcode.SysErrSenderStateInvalid,
				GasUsed:  0,
			},
			Penalty:  minerPenaltyAmount,
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

	ret, actorErr, rt := vm.send(ctx, msg, nil, msgGasCost)
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
	gasUsed = rt.gasUsed
	if gasUsed < 0 {
		gasUsed = 0
	}
	// refund unused gas
	refund := types.BigMul(types.NewInt(uint64(msg.GasLimit-gasUsed)), msg.GasPrice)
	if err := vm.transferFromGasHolder(msg.From, gasHolder, refund); err != nil {
		return nil, xerrors.Errorf("failed to refund gas")
	}

	gasReward := types.BigMul(msg.GasPrice, types.NewInt(uint64(gasUsed)))
	if err := vm.transferFromGasHolder(builtin.RewardActorAddr, gasHolder, gasReward); err != nil {
		return nil, xerrors.Errorf("failed to give miner gas reward: %w", err)
	}

	if types.BigCmp(types.NewInt(0), gasHolder.Balance) != 0 {
		return nil, xerrors.Errorf("gas handling math is wrong")
	}

	return &ApplyRet{
		MessageReceipt: types.MessageReceipt{
			ExitCode: exitcode.ExitCode(errcode),
			Return:   ret,
			GasUsed:  gasUsed,
		},
		ActorErr:           actorErr,
		InternalExecutions: rt.internalExecutions,
		Penalty:            types.NewInt(0),
		Duration:           time.Since(start),
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

	if err := Copy(from, to, root); err != nil {
		return cid.Undef, xerrors.Errorf("copying tree: %w", err)
	}

	return root, nil
}

// vm.MutateState(idAddr, func(cst cbor.IpldStore, st *ActorStateType) error {...})
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

func linksForObj(blk block.Block) ([]cid.Cid, error) {
	switch blk.Cid().Prefix().Codec {
	case cid.DagCBOR:
		return cbg.ScanForLinks(bytes.NewReader(blk.RawData()))
	default:
		return nil, xerrors.Errorf("vm flush copy method only supports dag cbor")
	}
}

func Copy(from, to blockstore.Blockstore, root cid.Cid) error {
	var batch []block.Block
	batchCp := func(blk block.Block) error {
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
		return err
	}

	if len(batch) > 0 {
		if err := to.PutMany(batch); err != nil {
			return xerrors.Errorf("batch put in copy: %w", err)
		}
	}

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

	links, err := linksForObj(blk)
	if err != nil {
		return err
	}

	for _, link := range links {
		if link.Prefix().MhType == mh.IDENTITY || link.Prefix().MhType == uint64(commcid.FC_SEALED_V1) || link.Prefix().MhType == uint64(commcid.FC_UNSEALED_V1) {
			continue
		}

		has, err := to.Has(link)
		if err != nil {
			return err
		}
		if has {
			continue
		}

		if err := copyRec(from, to, link, cp); err != nil {
			return err
		}
	}

	if err := cp(blk); err != nil {
		return err
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
			trace.StringAttribute("to", rt.Message().Receiver().String()),
			trace.Int64Attribute("method", int64(method)),
			trace.StringAttribute("value", rt.Message().ValueReceived().String()),
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

func (vm *VM) SetInvoker(i *invoker) {
	vm.inv = i
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

	if amt.LessThan(types.NewInt(0)) {
		return aerrors.Newf(exitcode.SysErrForbidden, "attempted to transfer negative value: %s", amt)
	}

	f, err := vm.cstate.GetActor(from)
	if err != nil {
		return aerrors.Fatalf("transfer failed when retrieving sender actor: %s", err)
	}

	t, err := vm.cstate.GetActor(to)
	if err != nil {
		return aerrors.Fatalf("transfer failed when retrieving receiver actor: %s", err)
	}

	if err := deductFunds(f, amt); err != nil {
		return aerrors.Newf(exitcode.SysErrInsufficientFunds, "transfer failed when deducting funds: %s", err)
	}
	depositFunds(t, amt)

	if err := vm.cstate.SetActor(from, f); err != nil {
		return aerrors.Fatalf("transfer failed when setting receiver actor: %s", err)
	}

	if err := vm.cstate.SetActor(to, t); err != nil {
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
