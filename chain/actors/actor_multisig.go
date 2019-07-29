package actors

import (
	"github.com/filecoin-project/go-lotus/chain/actors/aerrors"
	"github.com/filecoin-project/go-lotus/chain/address"
	"github.com/filecoin-project/go-lotus/chain/types"
)

type MultiSigActor struct{}
type MultiSigActorState struct {
	Signers  []address.Address
	Required uint32
	NextTxId uint64

	//TODO: make this map/sharray/whatever
	Transactions []MTransaction
}

func (msas MultiSigActorState) isSigner(addr address.Address) bool {
	for _, s := range msas.Signers {
		if s == addr {
			return true
		}
	}
	return false
}

func (msas MultiSigActorState) getTransaction(txid uint64) *MTransaction {
	if txid > uint64(len(msas.Transactions)) {
		return nil
	}
	return &msas.Transactions[txid]
}

type MTransaction struct {
	Created uint64 // NOT USED ??
	TxID    uint64

	To     address.Address
	Value  types.BigInt
	Method uint64
	Params []byte

	Approved []address.Address
	Complete bool
	Canceled bool
	RetCode  uint8
}

type musigMethods struct {
	MultiSigConstructor uint64
	Propose             uint64
	Approve             uint64
	Cancel              uint64
	ClearCompleted      uint64
	AddSigner           uint64
	RemoveSigner        uint64
	SwapSigner          uint64
	ChangeRequirement   uint64
}

var MultiSigMethods = musigMethods{1, 2, 3, 4, 5, 6, 7, 8, 9}

func (msa MultiSigActor) Exports() []interface{} {
	return []interface{}{
		1: msa.MultiSigConstructor,
	}
}

type MultiSigConstructorParams struct {
	Signers  []address.Address
	Required uint32
}

func (MultiSigActor) MultiSigConstructor(act *types.Actor, vmctx types.VMContext,
	params *MultiSigConstructorParams) ([]byte, ActorError) {
	self := &MultiSigActorState{
		Signers:  params.Signers,
		Required: params.Required,
	}
	head, err := vmctx.Storage().Put(self)
	if err != nil {
		return nil, aerrors.Wrap(err, "could not put new head")
	}
	err = vmctx.Storage().Commit(EmptyCBOR, head)
	if err != nil {
		return nil, aerrors.Wrap(err, "could not commit new head")
	}
	return nil, nil
}

type MultiSigProposeParams struct {
	To     address.Address
	Value  types.BigInt
	Method uint64
	Params []byte
}

func (MultiSigActor) Propose(act *types.Actor, vmctx types.VMContext,
	params *MultiSigProposeParams) ([]byte, ActorError) {
	var self MultiSigActorState
	head := vmctx.Storage().GetHead()

	err := vmctx.Storage().Get(head, &self)
	if err != nil {
		return nil, aerrors.Wrap(err, "could not get self")
	}

	if !self.isSigner(vmctx.Message().From) {
		return nil, aerrors.New(1, "not authorized")
	}

	txid := self.NextTxId
	self.NextTxId++

	tx := MTransaction{
		TxID:     txid,
		To:       params.To,
		Value:    params.Value,
		Method:   params.Method,
		Params:   params.Params,
		Approved: []address.Address{vmctx.Message().From},
	}

	if self.Required == 1 {
		_, err := vmctx.Send(tx.To, tx.Method, tx.Value, tx.Params)
		if aerrors.IsFatal(err) {
			return nil, err
		}
		tx.RetCode = aerrors.RetCode(err)
		tx.Complete = true
	}

	self.Transactions = append(self.Transactions, tx)

	newHead, err := vmctx.Storage().Put(self)
	if err != nil {
		return nil, aerrors.Wrap(err, "could not put new head")
	}
	err = vmctx.Storage().Commit(head, newHead)
	if err != nil {
		return nil, aerrors.Wrap(err, "could not commit new head")
	}

	return SerializeParams(tx.TxID)
}

type MultiSigApproveParams struct {
	TxID uint64
}

func (MultiSigActor) Approve(act *types.Actor, vmctx types.VMContext,
	params *MultiSigApproveParams) ([]byte, ActorError) {
	var self MultiSigActorState
	head := vmctx.Storage().GetHead()

	err := vmctx.Storage().Get(head, &self)
	if err != nil {
		return nil, aerrors.Wrap(err, "could not get self")
	}

	if !self.isSigner(vmctx.Message().From) {
		return nil, aerrors.New(1, "not authorized")
	}

	tx := self.getTransaction(params.TxID)

	if tx.Complete {
		return nil, aerrors.New(2, "transaction already completed")
	}
	if tx.Canceled {
		return nil, aerrors.New(3, "transaction canceled")
	}

	for _, signer := range tx.Approved {
		if signer == vmctx.Message().From {
			return nil, aerrors.New(4, "already signed this message")
		}
	}
	tx.Approved = append(tx.Approved, vmctx.Message().From)
	if uint32(len(tx.Approved)) >= self.Required {
		_, err := vmctx.Send(tx.To, tx.Method, tx.Value, tx.Params)
		if aerrors.IsFatal(err) {
			return nil, err
		}
		tx.RetCode = aerrors.RetCode(err)
		tx.Complete = true
	}

	newHead, err := vmctx.Storage().Put(self)
	if err != nil {
		return nil, aerrors.Wrap(err, "could not put new head")
	}
	err = vmctx.Storage().Commit(head, newHead)
	if err != nil {
		return nil, aerrors.Wrap(err, "could not commit new head")
	}

	return nil, nil
}
