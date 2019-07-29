package actors

import (
	"github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"

	"github.com/filecoin-project/go-lotus/chain/actors/aerrors"
	"github.com/filecoin-project/go-lotus/chain/address"
	"github.com/filecoin-project/go-lotus/chain/types"
)

func init() {
	cbor.RegisterCborType(MultiSigActorState{})
	cbor.RegisterCborType(MultiSigConstructorParams{})
	cbor.RegisterCborType(MultiSigProposeParams{})
	cbor.RegisterCborType(MultiSigTxID{})
	cbor.RegisterCborType(MultiSigSigner{})
	cbor.RegisterCborType(MultiSigSwapSignerParams{})
	cbor.RegisterCborType(MultiSigChangeReqParams{})
}

type MultiSigActor struct{}
type MultiSigActorState struct {
	Signers  []address.Address
	Required uint32
	NextTxID uint64

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

func (tx MTransaction) Active() ActorError {
	if tx.Complete {
		return aerrors.New(2, "transaction already completed")
	}
	if tx.Canceled {
		return aerrors.New(3, "transaction canceled")
	}
	return nil
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
		2: msa.Approve,
		3: msa.Propose,
		4: msa.Cancel,
		//5: msa.ClearCompleted,
		6: msa.AddSigner,
		7: msa.RemoveSigner,
		8: msa.SwapSigner,
		9: msa.ChangeRequirement,
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

func (MultiSigActor) load(vmctx types.VMContext) (cid.Cid, *MultiSigActorState, ActorError) {
	var self MultiSigActorState
	head := vmctx.Storage().GetHead()

	err := vmctx.Storage().Get(head, &self)
	if err != nil {
		return cid.Undef, nil, aerrors.Wrap(err, "could not get self")
	}
	return head, &self, nil
}

func (msa MultiSigActor) loadAndVerify(vmctx types.VMContext) (cid.Cid, *MultiSigActorState, ActorError) {
	head, self, err := msa.load(vmctx)
	if err != nil {
		return cid.Undef, nil, err
	}

	if !self.isSigner(vmctx.Message().From) {
		return cid.Undef, nil, aerrors.New(1, "not authorized")
	}
	return head, self, nil
}

func (MultiSigActor) save(vmctx types.VMContext, oldHead cid.Cid, self *MultiSigActorState) ActorError {
	newHead, err := vmctx.Storage().Put(self)
	if err != nil {
		return aerrors.Wrap(err, "could not put new head")
	}
	err = vmctx.Storage().Commit(oldHead, newHead)
	if err != nil {
		return aerrors.Wrap(err, "could not commit new head")
	}
	return nil

}

func (msa MultiSigActor) Propose(act *types.Actor, vmctx types.VMContext,
	params *MultiSigProposeParams) ([]byte, ActorError) {

	head, self, err := msa.loadAndVerify(vmctx)
	if err != nil {
		return nil, err
	}

	txid := self.NextTxID
	self.NextTxID++

	{
		tx := MTransaction{
			TxID:     txid,
			To:       params.To,
			Value:    params.Value,
			Method:   params.Method,
			Params:   params.Params,
			Approved: []address.Address{vmctx.Message().From},
		}
		self.Transactions = append(self.Transactions, tx)
	}
	tx := self.getTransaction(txid)

	if self.Required == 1 {
		_, err := vmctx.Send(tx.To, tx.Method, tx.Value, tx.Params)
		if aerrors.IsFatal(err) {
			return nil, err
		}
		tx.RetCode = aerrors.RetCode(err)
		tx.Complete = true
	}

	err = msa.save(vmctx, head, self)
	if err != nil {
		return nil, aerrors.Wrap(err, "saving state")
	}

	return SerializeParams(tx.TxID)
}

type MultiSigTxID struct {
	TxID uint64
}

func (msa MultiSigActor) Approve(act *types.Actor, vmctx types.VMContext,
	params *MultiSigTxID) ([]byte, ActorError) {

	head, self, err := msa.loadAndVerify(vmctx)
	if err != nil {
		return nil, err
	}

	tx := self.getTransaction(params.TxID)
	if err := tx.Active(); err != nil {
		return nil, aerrors.Wrap(err, "could not approve")
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

	return nil, msa.save(vmctx, head, self)
}

func (msa MultiSigActor) Cancel(act *types.Actor, vmctx types.VMContext,
	params *MultiSigTxID) ([]byte, ActorError) {

	head, self, err := msa.loadAndVerify(vmctx)
	if err != nil {
		return nil, err
	}

	tx := self.getTransaction(params.TxID)
	if err := tx.Active(); err != nil {
		return nil, aerrors.Wrap(err, "could not cancel")
	}

	proposer := tx.Approved[0]
	if proposer != vmctx.Message().From && self.isSigner(proposer) {
		return nil, aerrors.New(4, "cannot cancel another signers transaction")
	}
	tx.Canceled = true

	return nil, msa.save(vmctx, head, self)
}

type MultiSigSigner struct {
	Signer address.Address
}

func (msa MultiSigActor) AddSigner(act *types.Actor, vmctx types.VMContext,
	params *MultiSigSigner) ([]byte, ActorError) {

	head, self, err := msa.load(vmctx)
	if err != nil {
		return nil, err
	}

	msg := vmctx.Message()
	if msg.From != msg.To {
		return nil, aerrors.New(4, "add signer must be called by wallet itself")
	}
	if self.isSigner(params.Signer) {
		return nil, aerrors.New(5, "new address is already a signer")
	}
	self.Signers = append(self.Signers, params.Signer)

	return nil, msa.save(vmctx, head, self)
}

func (msa MultiSigActor) RemoveSigner(act *types.Actor, vmctx types.VMContext,
	params *MultiSigSigner) ([]byte, ActorError) {

	head, self, err := msa.load(vmctx)
	if err != nil {
		return nil, err
	}

	msg := vmctx.Message()
	if msg.From != msg.To {
		return nil, aerrors.New(4, "remove signer must be called by wallet itself")
	}
	if !self.isSigner(params.Signer) {
		return nil, aerrors.New(5, "given address was not a signer")
	}

	newSigners := make([]address.Address, 0, len(self.Signers)-1)
	for _, s := range self.Signers {
		if s != params.Signer {
			newSigners = append(newSigners, s)
		}
	}
	self.Signers = newSigners

	return nil, msa.save(vmctx, head, self)
}

type MultiSigSwapSignerParams struct {
	From address.Address
	To   address.Address
}

func (msa MultiSigActor) SwapSigner(act *types.Actor, vmctx types.VMContext,
	params *MultiSigSwapSignerParams) ([]byte, ActorError) {

	head, self, err := msa.load(vmctx)
	if err != nil {
		return nil, err
	}

	msg := vmctx.Message()
	if msg.From != msg.To {
		return nil, aerrors.New(4, "swap signer must be called by wallet itself")
	}

	if !self.isSigner(params.From) {
		return nil, aerrors.New(5, "given old address was not a signer")
	}
	if self.isSigner(params.To) {
		return nil, aerrors.New(6, "given new address was already a signer")
	}

	newSigners := make([]address.Address, 0, len(self.Signers))
	for _, s := range self.Signers {
		if s != params.From {
			newSigners = append(newSigners, s)
		}
	}
	newSigners = append(newSigners, params.To)
	self.Signers = newSigners

	return nil, msa.save(vmctx, head, self)
}

type MultiSigChangeReqParams struct {
	Req uint32
}

func (msa MultiSigActor) ChangeRequirement(act *types.Actor, vmctx types.VMContext,
	params *MultiSigChangeReqParams) ([]byte, ActorError) {

	head, self, err := msa.load(vmctx)
	if err != nil {
		return nil, err
	}

	msg := vmctx.Message()
	if msg.From != msg.To {
		return nil, aerrors.New(4, "change requirement must be called by wallet itself")
	}

	if params.Req < 1 {
		return nil, aerrors.New(5, "requirement must be at least 1")
	}
	self.Required = params.Req
	return nil, msa.save(vmctx, head, self)
}
