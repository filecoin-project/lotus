package actors

import (
	"bytes"

	"github.com/filecoin-project/go-lotus/chain/actors/aerrors"
	"github.com/filecoin-project/go-lotus/chain/address"
	"github.com/filecoin-project/go-lotus/chain/types"

	cbor "github.com/ipfs/go-ipld-cbor"
)

const ChannelClosingDelay = 6 * 60 * 2 // six hours

func init() {
	cbor.RegisterCborType(PaymentChannelActorState{})
	cbor.RegisterCborType(PCAConstructorParams{})
	cbor.RegisterCborType(SignedVoucher{})
	cbor.RegisterCborType(Merge{})
	cbor.RegisterCborType(LaneState{})
	cbor.RegisterCborType(UpdateChannelState{})
}

type PaymentChannelActor struct{}

type LaneState struct {
	Closed   bool
	Redeemed types.BigInt
	Nonce    uint64
}

type PaymentChannelActorState struct {
	From address.Address
	To   address.Address

	ChannelTotal types.BigInt
	ToSend       types.BigInt

	ClosingAt      uint64
	MinCloseHeight uint64

	LaneStates map[uint64]*LaneState

	VerifActor  address.Address
	VerifMethod uint64
}

func (pca PaymentChannelActor) Exports() []interface{} {
	return []interface{}{
		0: pca.Constructor,
		1: pca.UpdateChannelState,
		2: pca.Close,
		3: pca.Collect,
	}
}

type PCAConstructorParams struct {
	To          address.Address
	VerifActor  address.Address
	VerifMethod uint64
}

func (pca PaymentChannelActor) Constructor(act *types.Actor, vmctx types.VMContext, params *PCAConstructorParams) ([]byte, ActorError) {
	var self PaymentChannelActorState
	self.From = vmctx.Origin()
	self.To = params.To
	self.VerifActor = params.VerifActor
	self.VerifMethod = params.VerifMethod

	storage := vmctx.Storage()
	c, err := storage.Put(self)
	if err != nil {
		return nil, err
	}

	if err := storage.Commit(EmptyCBOR, c); err != nil {
		return nil, err
	}

	return nil, nil
}

type SignedVoucher struct {
	TimeLock       uint64
	SecretPreimage []byte
	Extra          []byte
	Lane           uint64
	Nonce          uint64
	Amount         types.BigInt
	MinCloseHeight uint64

	Merges []Merge

	Signature types.Signature
}

type Merge struct {
	Lane  uint64
	Nonce uint64
}

type UpdateChannelState struct {
	Sv     SignedVoucher
	Secret []byte
	Proof  []byte
}

func hash(b []byte) []byte {
	panic("blake 2b hash pls")
}

func (pca PaymentChannelActor) UpdateChannelState(act *types.Actor, vmctx types.VMContext, params *UpdateChannelState) ([]byte, ActorError) {
	var self PaymentChannelActorState
	oldstate := vmctx.Storage().GetHead()
	storage := vmctx.Storage()
	if err := storage.Get(oldstate, &self); err != nil {
		return nil, err
	}

	sv := params.Sv

	if err := vmctx.VerifySignature(sv.Signature, self.From); err != nil {
		return nil, err
	}

	if vmctx.BlockHeight() < sv.TimeLock {
		return nil, aerrors.New(2, "cannot use this voucher yet!")
	}

	if sv.SecretPreimage != nil {
		if !bytes.Equal(hash(params.Secret), sv.SecretPreimage) {
			return nil, aerrors.New(3, "Incorrect secret!")
		}
	}

	if sv.Extra != nil {
		if self.VerifActor == address.Undef {
			return nil, aerrors.New(4, "no verifActor for extra data")
		}

		encoded, err := SerializeParams([]interface{}{sv.Extra, params.Proof})
		if err != nil {
			return nil, err
		}

		_, err = vmctx.Send(self.VerifActor, self.VerifMethod, types.NewInt(0), encoded)
		if err != nil {
			return nil, aerrors.New(5, "spend voucher verification failed")
		}
	}

	ls := self.LaneStates[sv.Lane]
	if ls.Closed {
		return nil, aerrors.New(6, "cannot redeem a voucher on a closed lane")
	}

	if ls.Nonce > sv.Nonce {
		return nil, aerrors.New(7, "voucher has an outdated nonce, cannot redeem")
	}

	mergeValue := types.NewInt(0)
	for _, merge := range sv.Merges {
		if merge.Lane == sv.Lane {
			return nil, aerrors.New(8, "voucher cannot merge its own lane")
		}

		ols := self.LaneStates[merge.Lane]

		if ols.Nonce >= merge.Nonce {
			return nil, aerrors.New(9, "merge in voucher has outdated nonce, cannot redeem")
		}

		mergeValue = types.BigAdd(mergeValue, ols.Redeemed)
		ols.Nonce = merge.Nonce
	}

	ls.Nonce = sv.Nonce
	balanceDelta := types.BigSub(sv.Amount, types.BigAdd(mergeValue, ls.Redeemed))
	ls.Redeemed = sv.Amount

	newSendBalance := types.BigAdd(self.ToSend, balanceDelta)
	if types.BigCmp(newSendBalance, types.NewInt(0)) < 0 {
		// TODO: is this impossible?
		return nil, aerrors.New(10, "voucher would leave channel balance negative")
	}

	if types.BigCmp(newSendBalance, self.ChannelTotal) > 0 {
		return nil, aerrors.New(11, "not enough funds in channel to cover voucher")
	}

	self.ToSend = newSendBalance

	if sv.MinCloseHeight != 0 {
		if self.ClosingAt < sv.MinCloseHeight {
			self.ClosingAt = sv.MinCloseHeight
		}
		if self.MinCloseHeight < sv.MinCloseHeight {
			self.MinCloseHeight = sv.MinCloseHeight
		}
	}

	ncid, err := storage.Put(self)
	if err != nil {
		return nil, err
	}
	if err := storage.Commit(oldstate, ncid); err != nil {
		return nil, err
	}

	return nil, nil
}

func (pca PaymentChannelActor) Close(act *types.Actor, vmctx types.VMContext, params struct{}) ([]byte, aerrors.ActorError) {
	var self PaymentChannelActorState
	storage := vmctx.Storage()
	oldstate := storage.GetHead()
	if err := storage.Get(oldstate, &self); err != nil {
		return nil, err
	}

	if vmctx.Message().From != self.From && vmctx.Message().From != self.To {
		return nil, aerrors.New(1, "not authorized to close channel")
	}

	if self.ClosingAt != 0 {
		return nil, aerrors.New(2, "channel already closing")
	}

	self.ClosingAt = vmctx.BlockHeight() + ChannelClosingDelay
	if self.ClosingAt < self.MinCloseHeight {
		self.ClosingAt = self.MinCloseHeight
	}

	ncid, err := storage.Put(self)
	if err != nil {
		return nil, err
	}
	if err := storage.Commit(oldstate, ncid); err != nil {
		return nil, err
	}

	return nil, nil
}

func (pca PaymentChannelActor) Collect(act *types.Actor, vmctx types.VMContext, params struct{}) ([]byte, aerrors.ActorError) {
	var self PaymentChannelActorState
	storage := vmctx.Storage()
	oldstate := storage.GetHead()
	if err := storage.Get(oldstate, &self); err != nil {
		return nil, err
	}

	if self.ClosingAt == 0 {
		return nil, aerrors.New(1, "payment channel not closing or closed")
	}

	if vmctx.BlockHeight() < self.ClosingAt {
		return nil, aerrors.New(2, "payment channel not closed yet")
	}
	_, err := vmctx.Send(self.From, 0, types.BigSub(self.ChannelTotal, self.ToSend), nil)
	if err != nil {
		return nil, err
	}
	_, err = vmctx.Send(self.To, 0, self.ToSend, nil)
	if err != nil {
		return nil, err
	}

	self.ChannelTotal = types.NewInt(0)
	self.ToSend = types.NewInt(0)

	ncid, err := storage.Put(self)
	if err != nil {
		return nil, err
	}
	if err := storage.Commit(oldstate, ncid); err != nil {
		return nil, err
	}

	return nil, nil
}
