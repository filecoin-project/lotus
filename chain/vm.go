package chain

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"math/big"

	"github.com/filecoin-project/go-lotus/chain/address"
	"github.com/filecoin-project/go-lotus/lib/bufbstore"

	bserv "github.com/ipfs/go-blockservice"
	cid "github.com/ipfs/go-cid"
	hamt "github.com/ipfs/go-hamt-ipld"
	bstore "github.com/ipfs/go-ipfs-blockstore"
	ipld "github.com/ipfs/go-ipld-format"
	dag "github.com/ipfs/go-merkledag"
	"github.com/pkg/errors"
)

type VMContext struct {
	state  *StateTree
	msg    *Message
	height uint64
	cst    *hamt.CborIpldStore

	// root cid of the state of the actor this invocation will be on
	sroot cid.Cid

	storage Storage
}

// Message is the message that kicked off the current invocation
func (vmc *VMContext) Message() *Message {
	return vmc.msg
}

type Storage interface {
	Put(interface{}) (cid.Cid, error)
	Get(cid.Cid, interface{}) error

	GetHead() cid.Cid

	// Commit sets the new head of the actors state as long as the current
	// state matches 'oldh'
	Commit(oldh cid.Cid, newh cid.Cid) error
}

type storage struct {
	// would be great to stop depending on this crap everywhere
	// I am my own worst enemy
	cst  *hamt.CborIpldStore
	head cid.Cid
}

func (s *storage) Put(i interface{}) (cid.Cid, error) {
	return s.cst.Put(context.TODO(), i)
}

func (s *storage) Get(c cid.Cid, out interface{}) error {
	return s.cst.Get(context.TODO(), c, out)
}

func (s *storage) GetHead() cid.Cid {
	return s.head
}

func (s *storage) Commit(oldh, newh cid.Cid) error {
	if s.head != oldh {
		return fmt.Errorf("failed to update, inconsistent base reference")
	}

	s.head = newh
	return nil
}

// Storage provides access to the VM storage layer
func (vmc *VMContext) Storage() Storage {
	return vmc.storage
}

func (vmc *VMContext) Ipld() *hamt.CborIpldStore {
	return vmc.cst
}

// Send allows the current execution context to invoke methods on other actors in the system
func (vmc *VMContext) Send(to address.Address, method string, value *big.Int, params []interface{}) ([][]byte, uint8, error) {
	panic("nyi")
}

// BlockHeight returns the height of the block this message was added to the chain in
func (vmc *VMContext) BlockHeight() uint64 {
	return vmc.height
}

func (vmc *VMContext) GasUsed() BigInt {
	return NewInt(0)
}

func makeVMContext(state *StateTree, bs bstore.Blockstore, sroot cid.Cid, msg *Message, height uint64) *VMContext {
	cst := hamt.CSTFromBstore(bs)
	return &VMContext{
		state:  state,
		sroot:  sroot,
		msg:    msg,
		height: height,
		cst:    cst,
		storage: &storage{
			cst:  cst,
			head: sroot,
		},
	}
}

type VM struct {
	cstate      *StateTree
	base        cid.Cid
	cs          *ChainStore
	buf         *bufbstore.BufferedBS
	blockHeight uint64
	blockMiner  address.Address
	inv         *invoker
}

func NewVM(base cid.Cid, height uint64, maddr address.Address, cs *ChainStore) (*VM, error) {
	buf := bufbstore.NewBufferedBstore(cs.bs)
	cst := hamt.CSTFromBstore(buf)
	state, err := LoadStateTree(cst, base)
	if err != nil {
		return nil, err
	}

	return &VM{
		cstate:      state,
		base:        base,
		cs:          cs,
		buf:         buf,
		blockHeight: height,
		blockMiner:  maddr,
		inv:         newInvoker(),
	}, nil
}

func (vm *VM) ApplyMessage(msg *Message) (*MessageReceipt, error) {
	st := vm.cstate
	st.Snapshot()
	fromActor, err := st.GetActor(msg.From)
	if err != nil {
		return nil, errors.Wrap(err, "from actor not found")
	}

	gascost := BigMul(msg.GasLimit, msg.GasPrice)
	totalCost := BigAdd(gascost, msg.Value)
	if BigCmp(fromActor.Balance, totalCost) < 0 {
		return nil, fmt.Errorf("not enough funds")
	}

	if msg.Nonce != fromActor.Nonce {
		return nil, fmt.Errorf("invalid nonce")
	}
	fromActor.Nonce++

	toActor, err := st.GetActor(msg.To)
	if err != nil {
		if err == ErrActorNotFound {
			a, err := TryCreateAccountActor(st, msg.To)
			if err != nil {
				return nil, err
			}
			toActor = a
		} else {
			return nil, err
		}
	}

	if err := DeductFunds(fromActor, totalCost); err != nil {
		return nil, errors.Wrap(err, "failed to deduct funds")
	}
	DepositFunds(toActor, msg.Value)

	vmctx := makeVMContext(st, vm.cs.bs, toActor.Head, msg, vm.blockHeight)

	var errcode byte
	var ret []byte
	if msg.Method != 0 {
		ret, errcode, err = vm.Invoke(toActor, vmctx, msg.Method, msg.Params)
		if err != nil {
			return nil, err
		}

		if errcode != 0 {
			// revert all state changes since snapshot
			st.Revert()
			gascost := BigMul(vmctx.GasUsed(), msg.GasPrice)
			if err := DeductFunds(fromActor, gascost); err != nil {
				panic("invariant violated: " + err.Error())
			}
		} else {
			// refund unused gas
			refund := BigMul(BigSub(msg.GasLimit, vmctx.GasUsed()), msg.GasPrice)
			DepositFunds(fromActor, refund)
		}
	}

	// reward miner gas fees
	miner, err := st.GetActor(vm.blockMiner)
	if err != nil {
		return nil, errors.Wrap(err, "getting block miner actor failed")
	}

	gasReward := BigMul(msg.GasPrice, vmctx.GasUsed())
	DepositFunds(miner, gasReward)

	return &MessageReceipt{
		ExitCode: errcode,
		Return:   ret,
		GasUsed:  vmctx.GasUsed(),
	}, nil
}

func (vm *VM) Flush(ctx context.Context) (cid.Cid, error) {
	from := dag.NewDAGService(bserv.New(vm.buf, nil))
	to := dag.NewDAGService(bserv.New(vm.buf.Read(), nil))

	root, err := vm.cstate.Flush()
	if err != nil {
		return cid.Undef, err
	}

	if err := ipld.Copy(ctx, from, to, root); err != nil {
		return cid.Undef, err
	}

	return root, nil
}

func (vm *VM) TransferFunds(from, to address.Address, amt BigInt) error {
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
		return errors.Wrap(err, "failed to deduct funds")
	}
	DepositFunds(toAct, amt)

	return nil
}

func (vm *VM) Invoke(act *Actor, vmctx *VMContext, method uint64, params []byte) ([]byte, byte, error) {
	ret, err := vm.inv.Invoke(act, vmctx, method, params)
	if err != nil {
		return nil, 0, err
	}
	return ret.result, ret.returnCode, nil
}

func ComputeActorAddress(creator address.Address, nonce uint64) (address.Address, error) {
	buf := new(bytes.Buffer)
	buf.Write(creator.Bytes())

	binary.Write(buf, binary.BigEndian, nonce)

	return address.NewActorAddress(buf.Bytes())
}
