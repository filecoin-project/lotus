package actors

import (
	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/chain/actors/aerrors"
	"github.com/filecoin-project/lotus/chain/types"

	cbg "github.com/whyrusleeping/cbor-gen"
)

type MultiSigActor struct{}
type MultiSigActorState struct {
	Signers  []address.Address
	Required uint64
	NextTxID uint64

	InitialBalance types.BigInt
	StartingBlock  uint64
	UnlockDuration uint64

	//TODO: make this map/sharray/whatever
	Transactions []MTransaction
}

func (msas MultiSigActorState) canSpend(act *types.Actor, amnt types.BigInt, height uint64) bool {
	if msas.UnlockDuration == 0 {
		return true
	}

	offset := height - msas.StartingBlock
	if offset > msas.UnlockDuration {
		return true
	}

	minBalance := types.BigDiv(msas.InitialBalance, types.NewInt(msas.UnlockDuration))
	minBalance = types.BigMul(minBalance, types.NewInt(offset))
	return !minBalance.LessThan(types.BigSub(act.Balance, amnt))
}

func (msas MultiSigActorState) isSigner(addr address.Address) bool {
	for _, s := range msas.Signers {
		if s == addr {
			return true
		}
	}
	return false
}

func (msas MultiSigActorState) getTransaction(txid uint64) (*MTransaction, ActorError) {
	if txid >= uint64(len(msas.Transactions)) {
		return nil, aerrors.Newf(1, "could not get transaction (numbers of tx %d,want to get txid %d)", len(msas.Transactions), txid)
	}
	return &msas.Transactions[txid], nil
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
	RetCode  uint64
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
		2: msa.Propose,
		3: msa.Approve,
		4: msa.Cancel,
		//5: msa.ClearCompleted,
		6: msa.AddSigner,
		7: msa.RemoveSigner,
		8: msa.SwapSigner,
		9: msa.ChangeRequirement,
	}
}

type MultiSigConstructorParams struct {
	Signers        []address.Address
	Required       uint64
	UnlockDuration uint64
}

func (MultiSigActor) MultiSigConstructor(act *types.Actor, vmctx types.VMContext,
	params *MultiSigConstructorParams) ([]byte, ActorError) {
	self := &MultiSigActorState{
		Signers:  params.Signers,
		Required: params.Required,
	}

	if params.UnlockDuration != 0 {
		self.InitialBalance = vmctx.Message().Value
		self.UnlockDuration = params.UnlockDuration
		self.StartingBlock = vmctx.BlockHeight()
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

	tx, err := self.getTransaction(txid)
	if err != nil {
		return nil, err
	}

	if self.Required == 1 {
		if !self.canSpend(act, tx.Value, vmctx.BlockHeight()) {
			return nil, aerrors.New(100, "transaction amount exceeds available")
		}
		_, err := vmctx.Send(tx.To, tx.Method, tx.Value, tx.Params)
		if aerrors.IsFatal(err) {
			return nil, err
		}
		tx.RetCode = uint64(aerrors.RetCode(err))
		tx.Complete = true
	}

	err = msa.save(vmctx, head, self)
	if err != nil {
		return nil, aerrors.Wrap(err, "saving state")
	}

	// REVIEW: On one hand, I like being very explicit about how we're doing the serialization
	// on the other, maybe we shouldnt do direct calls to underlying serialization libs?
	return cbg.CborEncodeMajorType(cbg.MajUnsignedInt, tx.TxID), nil
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

	tx, err := self.getTransaction(params.TxID)
	if err != nil {
		return nil, err
	}

	if err := tx.Active(); err != nil {
		return nil, aerrors.Wrap(err, "could not approve")
	}

	for _, signer := range tx.Approved {
		if signer == vmctx.Message().From {
			return nil, aerrors.New(4, "already signed this message")
		}
	}
	tx.Approved = append(tx.Approved, vmctx.Message().From)
	if uint64(len(tx.Approved)) >= self.Required {
		if !self.canSpend(act, tx.Value, vmctx.BlockHeight()) {
			return nil, aerrors.New(100, "transaction amount exceeds available")
		}
		_, err := vmctx.Send(tx.To, tx.Method, tx.Value, tx.Params)
		if aerrors.IsFatal(err) {
			return nil, err
		}
		tx.RetCode = uint64(aerrors.RetCode(err))
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

	tx, err := self.getTransaction(params.TxID)
	if err != nil {
		return nil, err
	}

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

type MultiSigAddSignerParam struct {
	Signer   address.Address
	Increase bool
}

func (msa MultiSigActor) AddSigner(act *types.Actor, vmctx types.VMContext,
	params *MultiSigAddSignerParam) ([]byte, ActorError) {

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
	if params.Increase {
		self.Required = self.Required + 1
	}

	return nil, msa.save(vmctx, head, self)
}

type MultiSigRemoveSignerParam struct {
	Signer   address.Address
	Decrease bool
}

func (msa MultiSigActor) RemoveSigner(act *types.Actor, vmctx types.VMContext,
	params *MultiSigRemoveSignerParam) ([]byte, ActorError) {

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
	if params.Decrease || uint64(len(self.Signers)-1) < self.Required {
		self.Required = self.Required - 1
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
	Req uint64
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

	if params.Req > uint64(len(self.Signers)) {
		return nil, aerrors.New(6, "requirement must be at most the numbers of signers")
	}

	self.Required = params.Req
	return nil, msa.save(vmctx, head, self)
}
