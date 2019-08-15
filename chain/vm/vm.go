package vm

import (
	"context"
	"fmt"

	"github.com/filecoin-project/go-lotus/chain/actors"
	"github.com/filecoin-project/go-lotus/chain/actors/aerrors"
	"github.com/filecoin-project/go-lotus/chain/address"
	"github.com/filecoin-project/go-lotus/chain/state"
	"github.com/filecoin-project/go-lotus/chain/store"
	"github.com/filecoin-project/go-lotus/chain/types"
	"github.com/filecoin-project/go-lotus/lib/bufbstore"
	"go.opencensus.io/trace"

	bserv "github.com/ipfs/go-blockservice"
	cid "github.com/ipfs/go-cid"
	hamt "github.com/ipfs/go-hamt-ipld"
	ipld "github.com/ipfs/go-ipld-format"
	logging "github.com/ipfs/go-log"
	dag "github.com/ipfs/go-merkledag"
	"golang.org/x/xerrors"
)

var log = logging.Logger("vm")

const (
	gasFundTransfer = 10
)

type VMContext struct {
	ctx context.Context

	vm     *VM
	state  *state.StateTree
	msg    *types.Message
	height uint64
	cst    *hamt.CborIpldStore

	gasAvaliable types.BigInt
	gasUsed      types.BigInt

	// root cid of the state of the actor this invocation will be on
	sroot cid.Cid

	// address that started invokation chain
	origin address.Address

	storage *storage
}

// Message is the message that kicked off the current invocation
func (vmc *VMContext) Message() *types.Message {
	return vmc.msg
}

type storage struct {
	// would be great to stop depending on this crap everywhere
	// I am my own worst enemy
	cst  *hamt.CborIpldStore
	head cid.Cid
}

func (s *storage) Put(i interface{}) (cid.Cid, aerrors.ActorError) {
	c, err := s.cst.Put(context.TODO(), i)
	if err != nil {
		return cid.Undef, aerrors.Escalate(err, "putting cid")
	}
	return c, nil
}

func (s *storage) Get(c cid.Cid, out interface{}) aerrors.ActorError {
	return aerrors.Escalate(s.cst.Get(context.TODO(), c, out), "getting cid")
}

func (s *storage) GetHead() cid.Cid {
	return s.head
}

func (s *storage) Commit(oldh, newh cid.Cid) aerrors.ActorError {
	if s.head != oldh {
		return aerrors.New(1, "failed to update, inconsistent base reference")
	}

	s.head = newh
	return nil
}

// Storage provides access to the VM storage layer
func (vmc *VMContext) Storage() types.Storage {
	return vmc.storage
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
		GasLimit: vmc.gasAvaliable,
	}

	ret, err, _ := vmc.vm.send(ctx, msg, vmc)
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
	if types.BigCmp(vmc.gasUsed, vmc.gasAvaliable) > 0 {
		return aerrors.Newf(254, "not enough gas: used=%s, avaliable=%s", vmc.gasUsed, vmc.gasAvaliable)
	}
	return nil
}

func (vmc *VMContext) StateTree() (types.StateTree, aerrors.ActorError) {
	if vmc.msg.To != actors.InitActorAddress {
		return nil, aerrors.Escalate(fmt.Errorf("only init actor can access state tree directly"), "invalid use of StateTree")
	}

	return vmc.state, nil
}

func (vmctx *VMContext) VerifySignature(sig *types.Signature, act address.Address, data []byte) aerrors.ActorError {
	if act.Protocol() == address.ID {
		kaddr, err := vmctx.resolveToKeyAddr(act)
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

func (vmctx *VMContext) resolveToKeyAddr(addr address.Address) (address.Address, aerrors.ActorError) {
	if addr.Protocol() == address.BLS || addr.Protocol() == address.SECP256K1 {
		return addr, nil
	}

	act, err := vmctx.state.GetActor(addr)
	if err != nil {
		return address.Undef, aerrors.Newf(1, "failed to find actor: %s", addr)
	}

	if act.Code != actors.AccountActorCodeCid {
		return address.Undef, aerrors.New(1, "address was not for an account actor")
	}

	var aast actors.AccountActorState
	if err := vmctx.cst.Get(context.TODO(), act.Head, &aast); err != nil {
		return address.Undef, aerrors.Escalate(err, fmt.Sprintf("failed to get account actor state for %s", addr))
	}

	return aast.Address, nil
}

func (vm *VM) makeVMContext(ctx context.Context, sroot cid.Cid, msg *types.Message, origin address.Address, usedGas types.BigInt) *VMContext {

	return &VMContext{
		ctx:    ctx,
		vm:     vm,
		state:  vm.cstate,
		sroot:  sroot,
		msg:    msg,
		origin: origin,
		height: vm.blockHeight,
		cst:    vm.cst,
		storage: &storage{
			cst:  vm.cst,
			head: sroot,
		},

		gasUsed:      usedGas,
		gasAvaliable: msg.GasLimit,
	}
}

type VM struct {
	cstate      *state.StateTree
	base        cid.Cid
	cs          *store.ChainStore
	cst         *hamt.CborIpldStore
	buf         *bufbstore.BufferedBS
	blockHeight uint64
	blockMiner  address.Address
	inv         *invoker
}

func NewVM(base cid.Cid, height uint64, maddr address.Address, cs *store.ChainStore) (*VM, error) {
	buf := bufbstore.NewBufferedBstore(cs.Blockstore())
	cst := hamt.CSTFromBstore(buf)
	state, err := state.LoadStateTree(cst, base)
	if err != nil {
		return nil, err
	}

	return &VM{
		cstate:      state,
		base:        base,
		cs:          cs,
		cst:         cst,
		buf:         buf,
		blockHeight: height,
		blockMiner:  maddr,
		inv:         newInvoker(),
	}, nil
}

type ApplyRet struct {
	types.MessageReceipt
	ActorErr aerrors.ActorError
}

func (vm *VM) send(ctx context.Context, msg *types.Message, parent *VMContext) ([]byte, aerrors.ActorError, *VMContext) {

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

	gasUsed := types.NewInt(0)
	origin := msg.From
	if parent != nil {
		gasUsed = parent.gasUsed
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

		if err := DeductFunds(fromActor, msg.Value); err != nil {
			return nil, aerrors.Absorb(err, 1, "failed to deduct funds"), nil
		}
		DepositFunds(toActor, msg.Value)
	}

	if msg.Method != 0 {
		ret, err := vm.Invoke(toActor, vmctx, msg.Method, msg.Params)
		toActor.Head = vmctx.Storage().GetHead()
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

	gascost := types.BigMul(msg.GasLimit, msg.GasPrice)
	totalCost := types.BigAdd(gascost, msg.Value)
	if types.BigCmp(fromActor.Balance, totalCost) < 0 {
		return nil, xerrors.Errorf("not enough funds (%s < %s)", fromActor.Balance, totalCost)
	}
	if err := DeductFunds(fromActor, gascost); err != nil {
		return nil, xerrors.Errorf("failed to deduct funds: %w", err)
	}

	if msg.Nonce != fromActor.Nonce {
		return nil, xerrors.Errorf("invalid nonce (got %d, expected %d)", msg.Nonce, fromActor.Nonce)
	}
	fromActor.Nonce++

	ret, actorErr, vmctx := vm.send(ctx, msg, nil)

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
		DepositFunds(fromActor, refund)
	}

	miner, err := st.GetActor(vm.blockMiner)
	if err != nil {
		return nil, xerrors.Errorf("getting block miner actor (%s) failed: %w", vm.blockMiner, err)
	}

	gasReward := types.BigMul(msg.GasPrice, gasUsed)
	DepositFunds(miner, gasReward)

	return &ApplyRet{
		MessageReceipt: types.MessageReceipt{
			ExitCode: errcode,
			Return:   ret,
			GasUsed:  gasUsed,
		},
		ActorErr: actorErr,
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
	from := dag.NewDAGService(bserv.New(vm.buf, nil))
	to := dag.NewDAGService(bserv.New(vm.buf.Read(), nil))

	root, err := vm.cstate.Flush()
	if err != nil {
		return cid.Undef, xerrors.Errorf("flushing vm: %w", err)
	}

	if err := Copy(ctx, from, to, root); err != nil {
		return cid.Undef, xerrors.Errorf("copying tree: %w", err)
	}

	return root, nil
}

func Copy(ctx context.Context, from, to ipld.DAGService, root cid.Cid) error {
	if root.Prefix().MhType == 0 {
		// identity cid, skip
		return nil
	}
	node, err := from.Get(ctx, root)
	if err != nil {
		return xerrors.Errorf("get %s failed: %w", root, err)
	}
	links := node.Links()
	for _, link := range links {
		if link.Cid.Prefix().MhType == 0 {
			continue
		}
		_, err := to.Get(ctx, link.Cid)
		switch err {
		default:
			return err
		case nil:
			continue
		case ipld.ErrNotFound:
			// continue
		}
		if err := Copy(ctx, from, to, link.Cid); err != nil {
			return err
		}
	}
	err = to.Add(ctx, node)
	if err != nil {
		return err
	}
	return nil
}

func (vm *VM) TransferFunds(from, to address.Address, amt types.BigInt) error {
	if from == to {
		return nil
	}

	fromAct, err := vm.cstate.GetActor(from)
	if err != nil {
		return err
	}

	toAct, err := vm.cstate.GetActor(from)
	if err != nil {
		return err
	}

	if err := DeductFunds(fromAct, amt); err != nil {
		return xerrors.Errorf("failed to deduct funds: %w", err)
	}
	DepositFunds(toAct, amt)

	return nil
}

func (vm *VM) SetBlockHeight(h uint64) {
	vm.blockHeight = h
}

func (vm *VM) Invoke(act *types.Actor, vmctx *VMContext, method uint64, params []byte) ([]byte, aerrors.ActorError) {
	ctx, span := trace.StartSpan(vmctx.ctx, "vm.Invoke")
	defer span.End()
	var oldCtx context.Context
	oldCtx, vmctx.ctx = vmctx.ctx, ctx
	defer func() {
		vmctx.ctx = oldCtx
	}()
	ret, err := vm.inv.Invoke(act, vmctx, method, params)
	if err != nil {
		return nil, err
	}
	return ret, nil
}

func DeductFunds(act *types.Actor, amt types.BigInt) error {
	if types.BigCmp(act.Balance, amt) < 0 {
		return fmt.Errorf("not enough funds")
	}

	act.Balance = types.BigSub(act.Balance, amt)
	return nil
}

func DepositFunds(act *types.Actor, amt types.BigInt) {
	act.Balance = types.BigAdd(act.Balance, amt)
}

func MiningRewardForBlock(base *types.TipSet) types.BigInt {
	return types.NewInt(10000)
}
