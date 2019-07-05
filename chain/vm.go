package chain

import (
	"context"
	"fmt"
	"github.com/filecoin-project/go-lotus/chain/address"
	"github.com/pkg/errors"
	"math/big"

	bserv "github.com/ipfs/go-blockservice"
	cid "github.com/ipfs/go-cid"
	hamt "github.com/ipfs/go-hamt-ipld"
	ipld "github.com/ipfs/go-ipld-format"
	dag "github.com/ipfs/go-merkledag"
)

type VMContext struct {
	state  *StateTree
	msg    *Message
	height uint64
	cst    *hamt.CborIpldStore
}

// Message is the message that kicked off the current invocation
func (vmc *VMContext) Message() *Message {
	return vmc.msg
}

/*
// Storage provides access to the VM storage layer
func (vmc *VMContext) Storage() Storage {
	panic("nyi")
}
*/

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

func makeVMContext(state *StateTree, msg *Message, height uint64) *VMContext {
	return &VMContext{
		state:  state,
		msg:    msg,
		height: height,
	}
}

type VM struct {
	cstate      *StateTree
	base        cid.Cid
	cs          *ChainStore
	buf         *BufferedBS
	blockHeight uint64
	blockMiner  address.Address
}

func NewVM(base cid.Cid, height uint64, maddr address.Address, cs *ChainStore) (*VM, error) {
	buf := NewBufferedBstore(cs.bs)
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

	vmctx := makeVMContext(st, msg, vm.blockHeight)

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
	to := dag.NewDAGService(bserv.New(vm.buf.read, nil))

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
	panic("Implement me")
}