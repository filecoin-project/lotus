package vm

import (
	"bytes"
	"context"
	"fmt"
	"math/big"

	block "github.com/ipfs/go-block-format"
	cid "github.com/ipfs/go-cid"
	hamt "github.com/ipfs/go-hamt-ipld"
	blockstore "github.com/ipfs/go-ipfs-blockstore"
	logging "github.com/ipfs/go-log/v2"
	cbg "github.com/whyrusleeping/cbor-gen"
	"go.opencensus.io/trace"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/aerrors"
	"github.com/filecoin-project/lotus/chain/state"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/lib/bufbstore"
)

var log = logging.Logger("vm")

const (
	gasFundTransfer = 10
	gasInvoke       = 5

	gasGetObj         = 10
	gasGetPerByte     = 1
	gasPutObj         = 20
	gasPutPerByte     = 2
	gasCommit         = 50
	gasPerMessageByte = 2
)

const (
	outOfGasErrCode = 200
)

type VMContext struct {
	ctx context.Context

	vm     *VM
	state  *state.StateTree
	msg    *types.Message
	height uint64
	cst    *hamt.CborIpldStore

	gasAvailable types.BigInt
	gasUsed      types.BigInt

	sys *types.VMSyscalls

	// root cid of the state of the actor this invocation will be on
	sroot cid.Cid

	// address that started invoke chain
	origin address.Address
}

// Message is the message that kicked off the current invocation
func (vmc *VMContext) Message() *types.Message {
	return vmc.msg
}

func (vmc *VMContext) GetRandomness(height uint64) ([]byte, aerrors.ActorError) {

	res, err := vmc.vm.rand.GetRandomness(vmc.ctx, int64(height))
	if err != nil {
		return nil, aerrors.Escalate(err, "could not get randomness")
	}
	return res, nil
}

func (vmc *VMContext) Sys() *types.VMSyscalls {
	return vmc.sys
}

// Storage interface

func (vmc *VMContext) Put(i cbg.CBORMarshaler) (cid.Cid, aerrors.ActorError) {
	c, err := vmc.cst.Put(context.TODO(), i)
	if err != nil {
		return cid.Undef, aerrors.HandleExternalError(err, fmt.Sprintf("putting object %T", i))
	}
	return c, nil
}

func (vmc *VMContext) Get(c cid.Cid, out cbg.CBORUnmarshaler) aerrors.ActorError {
	err := vmc.cst.Get(context.TODO(), c, out)
	if err != nil {
		return aerrors.HandleExternalError(err, "getting cid")
	}
	return nil
}

func (vmc *VMContext) GetHead() cid.Cid {
	return vmc.sroot
}

func (vmc *VMContext) Commit(oldh, newh cid.Cid) aerrors.ActorError {
	if err := vmc.ChargeGas(gasCommit); err != nil {
		return aerrors.Wrap(err, "out of gas")
	}
	if vmc.sroot != oldh {
		return aerrors.New(1, "failed to update, inconsistent base reference")
	}

	vmc.sroot = newh
	return nil
}

// End of storage interface

// Storage provides access to the VM storage layer
func (vmc *VMContext) Storage() types.Storage {
	return vmc
}

func (vmc *VMContext) Ipld() *hamt.CborIpldStore {
	return vmc.cst
}

func (vmc *VMContext) Origin() address.Address {
	return vmc.origin
}

// Send allows the current execution context to invoke methods on other actors in the system
func (vmc *VMContext) Send(to address.Address, method uint64, value types.BigInt, params []byte) ([]byte, aerrors.ActorError) {
	ctx, span := trace.StartSpan(vmc.ctx, "vmc.Send")
	defer span.End()
	if span.IsRecordingEvents() {
		span.AddAttributes(
			trace.StringAttribute("to", to.String()),
			trace.Int64Attribute("method", int64(method)),
			trace.StringAttribute("value", value.String()),
		)
	}

	msg := &types.Message{
		From:     vmc.msg.To,
		To:       to,
		Method:   method,
		Value:    value,
		Params:   params,
		GasLimit: vmc.gasAvailable,
	}

	ret, err, _ := vmc.vm.send(ctx, msg, vmc, 0)
	return ret, err
}

// BlockHeight returns the height of the block this message was added to the chain in
func (vmc *VMContext) BlockHeight() uint64 {
	return vmc.height
}

func (vmc *VMContext) GasUsed() types.BigInt {
	return vmc.gasUsed
}

func (vmc *VMContext) ChargeGas(amount uint64) aerrors.ActorError {
	toUse := types.NewInt(amount)
	vmc.gasUsed = types.BigAdd(vmc.gasUsed, toUse)
	if vmc.gasUsed.GreaterThan(vmc.gasAvailable) {
		return aerrors.Newf(outOfGasErrCode, "not enough gas: used=%s, available=%s", vmc.gasUsed, vmc.gasAvailable)
	}
	return nil
}

func (vmc *VMContext) StateTree() (types.StateTree, aerrors.ActorError) {
	if vmc.msg.To != actors.InitAddress {
		return nil, aerrors.Escalate(fmt.Errorf("only init actor can access state tree directly"), "invalid use of StateTree")
	}

	return vmc.state, nil
}

const GasVerifySignature = 50

func (vmctx *VMContext) VerifySignature(sig *types.Signature, act address.Address, data []byte) aerrors.ActorError {
	if err := vmctx.ChargeGas(GasVerifySignature); err != nil {
		return err
	}

	if act.Protocol() == address.ID {
		kaddr, err := ResolveToKeyAddr(vmctx.state, vmctx.cst, act)
		if err != nil {
			return aerrors.Wrap(err, "failed to resolve address to key address")
		}
		act = kaddr
	}

	if err := sig.Verify(act, data); err != nil {
		return aerrors.New(2, "signature verification failed")
	}

	return nil
}

func ResolveToKeyAddr(state types.StateTree, cst *hamt.CborIpldStore, addr address.Address) (address.Address, aerrors.ActorError) {
	if addr.Protocol() == address.BLS || addr.Protocol() == address.SECP256K1 {
		return addr, nil
	}

	act, err := state.GetActor(addr)
	if err != nil {
		return address.Undef, aerrors.Newf(1, "failed to find actor: %s", addr)
	}

	if act.Code != actors.AccountCodeCid {
		return address.Undef, aerrors.New(1, "address was not for an account actor")
	}

	var aast actors.AccountActorState
	if err := cst.Get(context.TODO(), act.Head, &aast); err != nil {
		return address.Undef, aerrors.Escalate(err, fmt.Sprintf("failed to get account actor state for %s", addr))
	}

	return aast.Address, nil
}

func (vmctx *VMContext) GetBalance(a address.Address) (types.BigInt, aerrors.ActorError) {
	act, err := vmctx.state.GetActor(a)
	switch err {
	default:
		return types.EmptyInt, aerrors.Escalate(err, "failed to look up actor balance")
	case hamt.ErrNotFound:
		return types.NewInt(0), nil
	case nil:
		return act.Balance, nil
	}
}

func (vmctx *VMContext) Context() context.Context {
	return vmctx.ctx
}

type hBlocks interface {
	GetBlock(context.Context, cid.Cid) (block.Block, error)
	AddBlock(block.Block) error
}

var _ hBlocks = (*gasChargingBlocks)(nil)

type gasChargingBlocks struct {
	chargeGas func(uint64) aerrors.ActorError
	under     hBlocks
}

func (bs *gasChargingBlocks) GetBlock(ctx context.Context, c cid.Cid) (block.Block, error) {
	if err := bs.chargeGas(gasGetObj); err != nil {
		return nil, err
	}
	blk, err := bs.under.GetBlock(ctx, c)
	if err != nil {
		return nil, aerrors.Escalate(err, "failed to get block from blockstore")
	}
	if err := bs.chargeGas(uint64(len(blk.RawData())) * gasGetPerByte); err != nil {
		return nil, err
	}

	return blk, nil
}

func (bs *gasChargingBlocks) AddBlock(blk block.Block) error {
	if err := bs.chargeGas(gasPutObj + uint64(len(blk.RawData()))*gasPutPerByte); err != nil {
		return err
	}
	if err := bs.under.AddBlock(blk); err != nil {
		return aerrors.Escalate(err, "failed to write data to disk")
	}
	return nil
}

func (vm *VM) makeVMContext(ctx context.Context, sroot cid.Cid, msg *types.Message, origin address.Address, usedGas types.BigInt) *VMContext {
	vmc := &VMContext{
		ctx:    ctx,
		vm:     vm,
		state:  vm.cstate,
		sroot:  sroot,
		msg:    msg,
		origin: origin,
		height: vm.blockHeight,
		sys:    vm.Syscalls,

		gasUsed:      usedGas,
		gasAvailable: msg.GasLimit,
	}
	vmc.cst = &hamt.CborIpldStore{
		Blocks: &gasChargingBlocks{vmc.ChargeGas, vm.cst.Blocks},
		Atlas:  vm.cst.Atlas,
	}
	return vmc
}

type VM struct {
	cstate      *state.StateTree
	base        cid.Cid
	cst         *hamt.CborIpldStore
	buf         *bufbstore.BufferedBS
	blockHeight uint64
	blockMiner  address.Address
	inv         *invoker
	rand        Rand

	Syscalls *types.VMSyscalls
}

func NewVM(base cid.Cid, height uint64, r Rand, maddr address.Address, cbs blockstore.Blockstore, syscalls *types.VMSyscalls) (*VM, error) {
	buf := bufbstore.NewBufferedBstore(cbs)
	cst := hamt.CSTFromBstore(buf)
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
		blockMiner:  maddr,
		inv:         newInvoker(),
		rand:        r, // TODO: Probably should be a syscall
		Syscalls:    syscalls,
	}, nil
}

type Rand interface {
	GetRandomness(ctx context.Context, h int64) ([]byte, error)
}

type ApplyRet struct {
	types.MessageReceipt
	ActorErr aerrors.ActorError
}

func (vm *VM) send(ctx context.Context, msg *types.Message, parent *VMContext,
	gasCharge uint64) ([]byte, aerrors.ActorError, *VMContext) {

	st := vm.cstate
	fromActor, err := st.GetActor(msg.From)
	if err != nil {
		return nil, aerrors.Absorb(err, 1, "could not find source actor"), nil
	}

	toActor, err := st.GetActor(msg.To)
	if err != nil {
		if xerrors.Is(err, types.ErrActorNotFound) {
			a, err := TryCreateAccountActor(st, msg.To)
			if err != nil {
				return nil, aerrors.Absorb(err, 1, "could not create account"), nil
			}
			toActor = a
		} else {
			return nil, aerrors.Escalate(err, "getting actor"), nil
		}
	}

	gasUsed := types.NewInt(gasCharge)
	origin := msg.From
	if parent != nil {
		gasUsed = types.BigAdd(parent.gasUsed, gasUsed)
		origin = parent.origin
	}
	vmctx := vm.makeVMContext(ctx, toActor.Head, msg, origin, gasUsed)
	if parent != nil {
		defer func() {
			parent.gasUsed = vmctx.gasUsed
		}()
	}

	if types.BigCmp(msg.Value, types.NewInt(0)) != 0 {
		if aerr := vmctx.ChargeGas(gasFundTransfer); aerr != nil {
			return nil, aerrors.Wrap(aerr, "sending funds"), nil
		}

		if err := Transfer(fromActor, toActor, msg.Value); err != nil {
			return nil, aerrors.Absorb(err, 1, "failed to transfer funds"), nil
		}
	}

	if msg.Method != 0 {
		ret, err := vm.Invoke(toActor, vmctx, msg.Method, msg.Params)
		if !aerrors.IsFatal(err) {
			toActor.Head = vmctx.Storage().GetHead()
		}
		return ret, err, vmctx
	}

	return nil, nil, vmctx
}

func checkMessage(msg *types.Message) error {
	if msg.GasLimit == types.EmptyInt {
		return xerrors.Errorf("message gas no gas limit set")
	}

	if msg.GasPrice == types.EmptyInt {
		return xerrors.Errorf("message gas no gas price set")
	}

	if msg.Value == types.EmptyInt {
		return xerrors.Errorf("message no value set")
	}

	return nil
}

func (vm *VM) ApplyMessage(ctx context.Context, msg *types.Message) (*ApplyRet, error) {
	ctx, span := trace.StartSpan(ctx, "vm.ApplyMessage")
	defer span.End()
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

	st := vm.cstate
	if err := st.Snapshot(); err != nil {
		return nil, xerrors.Errorf("snapshot failed: %w", err)
	}

	fromActor, err := st.GetActor(msg.From)
	if err != nil {
		return nil, xerrors.Errorf("from actor not found: %w", err)
	}

	serMsg, err := msg.Serialize()
	if err != nil {
		return nil, xerrors.Errorf("could not serialize message: %w", err)
	}
	msgGasCost := uint64(len(serMsg)) * gasPerMessageByte

	gascost := types.BigMul(msg.GasLimit, msg.GasPrice)
	totalCost := types.BigAdd(gascost, msg.Value)
	if fromActor.Balance.LessThan(totalCost) {
		return nil, xerrors.Errorf("not enough funds (%s < %s)", fromActor.Balance, totalCost)
	}

	gasHolder := &types.Actor{Balance: types.NewInt(0)}
	if err := Transfer(fromActor, gasHolder, gascost); err != nil {
		return nil, xerrors.Errorf("failed to withdraw gas funds: %w", err)
	}

	if msg.Nonce != fromActor.Nonce {
		return nil, xerrors.Errorf("invalid nonce (got %d, expected %d)", msg.Nonce, fromActor.Nonce)
	}
	fromActor.Nonce++

	ret, actorErr, vmctx := vm.send(ctx, msg, nil, msgGasCost)

	if aerrors.IsFatal(actorErr) {
		return nil, xerrors.Errorf("[from=%s,to=%s,n=%d,m=%d,h=%d] fatal error: %w", msg.From, msg.To, msg.Nonce, msg.Method, vm.blockHeight, actorErr)
	}
	if actorErr != nil {
		log.Warnw("Send actor error", "from", msg.From, "to", msg.To, "nonce", msg.Nonce, "method", msg.Method, "height", vm.blockHeight, "error", fmt.Sprintf("%+v", actorErr))
	}

	var errcode uint8
	var gasUsed types.BigInt

	if errcode = aerrors.RetCode(actorErr); errcode != 0 {
		gasUsed = msg.GasLimit
		// revert all state changes since snapshot
		if err := st.Revert(); err != nil {
			return nil, xerrors.Errorf("revert state failed: %w", err)
		}
	} else {
		// refund unused gas
		gasUsed = vmctx.GasUsed()
		refund := types.BigMul(types.BigSub(msg.GasLimit, gasUsed), msg.GasPrice)
		if err := Transfer(gasHolder, fromActor, refund); err != nil {
			return nil, xerrors.Errorf("failed to refund gas")
		}
	}

	miner, err := st.GetActor(vm.blockMiner)
	if err != nil {
		return nil, xerrors.Errorf("getting block miner actor (%s) failed: %w", vm.blockMiner, err)
	}

	// TODO: support multiple blocks in a tipset
	// TODO: actually wire this up (miner is undef for now)
	gasReward := types.BigMul(msg.GasPrice, gasUsed)
	if err := Transfer(gasHolder, miner, gasReward); err != nil {
		return nil, xerrors.Errorf("failed to give miner gas reward: %w", err)
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
		ActorErr: actorErr,
	}, nil
}

func (vm *VM) SetBlockMiner(m address.Address) {
	vm.blockMiner = m
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

	root, err := vm.cstate.Flush()
	if err != nil {
		return cid.Undef, xerrors.Errorf("flushing vm: %w", err)
	}

	if err := Copy(from, to, root); err != nil {
		return cid.Undef, xerrors.Errorf("copying tree: %w", err)
	}

	return root, nil
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
		if link.Prefix().MhType == 0 {
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

func (vm *VM) SetBlockHeight(h uint64) {
	vm.blockHeight = h
}

func (vm *VM) Invoke(act *types.Actor, vmctx *VMContext, method uint64, params []byte) ([]byte, aerrors.ActorError) {
	ctx, span := trace.StartSpan(vmctx.ctx, "vm.Invoke")
	defer span.End()
	if span.IsRecordingEvents() {
		span.AddAttributes(
			trace.StringAttribute("to", vmctx.Message().To.String()),
			trace.Int64Attribute("method", int64(method)),
			trace.StringAttribute("value", vmctx.Message().Value.String()),
		)
	}

	var oldCtx context.Context
	oldCtx, vmctx.ctx = vmctx.ctx, ctx
	defer func() {
		vmctx.ctx = oldCtx
	}()
	if err := vmctx.ChargeGas(gasInvoke); err != nil {
		return nil, aerrors.Wrap(err, "invokeing")
	}
	ret, err := vm.inv.Invoke(act, vmctx, method, params)
	if err != nil {
		return nil, err
	}
	return ret, nil
}

func Transfer(from, to *types.Actor, amt types.BigInt) error {
	if from == to {
		return nil
	}

	if amt.LessThan(types.NewInt(0)) {
		return xerrors.Errorf("attempted to transfer negative value")
	}

	if err := deductFunds(from, amt); err != nil {
		return err
	}
	depositFunds(to, amt)
	return nil
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

var miningRewardTotal = types.FromFil(build.MiningRewardTotal)
var blocksPerEpoch = types.NewInt(build.BlocksPerEpoch)

// MiningReward returns correct mining reward
//   coffer is amount of FIL in NetworkAddress
func MiningReward(remainingReward types.BigInt) types.BigInt {
	ci := big.NewInt(0).Set(remainingReward.Int)
	res := ci.Mul(ci, build.InitialReward)
	res = res.Div(res, miningRewardTotal.Int)
	res = res.Div(res, blocksPerEpoch.Int)
	return types.BigInt{res}
}
