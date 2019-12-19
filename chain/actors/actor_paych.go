package actors

import (
	"bytes"
	"fmt"

	"github.com/ipfs/go-cid"
	"github.com/minio/blake2b-simd"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors/aerrors"
	"github.com/filecoin-project/lotus/chain/types"
)

type PaymentChannelActor struct{}

type PaymentInfo struct {
	PayChActor     address.Address
	Payer          address.Address
	ChannelMessage *cid.Cid

	Vouchers []*types.SignedVoucher
}

type LaneState struct {
	Closed   bool
	Redeemed types.BigInt
	Nonce    uint64
}

type PaymentChannelActorState struct {
	From address.Address
	To   address.Address

	ToSend types.BigInt

	ClosingAt      uint64
	MinCloseHeight uint64

	// TODO: needs to be map[uint64]*laneState
	// waiting on refmt#35 to be fixed
	LaneStates map[string]*LaneState
}

func (pca PaymentChannelActor) Exports() []interface{} {
	return []interface{}{
		1: pca.Constructor,
		2: pca.UpdateChannelState,
		3: pca.Close,
		4: pca.Collect,
		5: pca.GetOwner,
		6: pca.GetToSend,
	}
}

type pcaMethods struct {
	Constructor        uint64
	UpdateChannelState uint64
	Close              uint64
	Collect            uint64
	GetOwner           uint64
	GetToSend          uint64
}

var PCAMethods = pcaMethods{1, 2, 3, 4, 5, 6}

type PCAConstructorParams struct {
	To address.Address
}

func (pca PaymentChannelActor) Constructor(act *types.Actor, vmctx types.VMContext, params *PCAConstructorParams) ([]byte, ActorError) {
	var self PaymentChannelActorState
	self.From = vmctx.Origin()
	self.To = params.To
	self.LaneStates = make(map[string]*LaneState)

	storage := vmctx.Storage()
	c, err := storage.Put(&self)
	if err != nil {
		return nil, err
	}

	if err := storage.Commit(EmptyCBOR, c); err != nil {
		return nil, err
	}

	return nil, nil
}

type PCAUpdateChannelStateParams struct {
	Sv     types.SignedVoucher
	Secret []byte
	Proof  []byte
}

func hash(b []byte) []byte {
	s := blake2b.Sum256(b)
	return s[:]
}

type PaymentVerifyParams struct {
	Extra []byte
	Proof []byte
}

func (pca PaymentChannelActor) UpdateChannelState(act *types.Actor, vmctx types.VMContext, params *PCAUpdateChannelStateParams) ([]byte, ActorError) {
	var self PaymentChannelActorState
	oldstate := vmctx.Storage().GetHead()
	storage := vmctx.Storage()
	if err := storage.Get(oldstate, &self); err != nil {
		return nil, err
	}

	sv := params.Sv

	vb, nerr := sv.SigningBytes()
	if nerr != nil {
		return nil, aerrors.Escalate(nerr, "failed to serialize signedvoucher")
	}

	if err := vmctx.VerifySignature(sv.Signature, self.From, vb); err != nil {
		return nil, err
	}

	if vmctx.BlockHeight() < sv.TimeLock {
		return nil, aerrors.New(2, "cannot use this voucher yet!")
	}

	if len(sv.SecretPreimage) > 0 {
		if !bytes.Equal(hash(params.Secret), sv.SecretPreimage) {
			return nil, aerrors.New(3, "incorrect secret!")
		}
	}

	if sv.Extra != nil {
		encoded, err := SerializeParams(&PaymentVerifyParams{sv.Extra.Data, params.Proof})
		if err != nil {
			return nil, err
		}

		_, err = vmctx.Send(sv.Extra.Actor, sv.Extra.Method, types.NewInt(0), encoded)
		if err != nil {
			return nil, aerrors.Newf(4, "spend voucher verification failed: %s", err)
		}
	}

	ls, ok := self.LaneStates[fmt.Sprint(sv.Lane)]
	if !ok {
		ls = new(LaneState)
		ls.Redeemed = types.NewInt(0) // TODO: kinda annoying that this doesnt default to a usable value
		self.LaneStates[fmt.Sprint(sv.Lane)] = ls
	}
	if ls.Closed {
		return nil, aerrors.New(5, "cannot redeem a voucher on a closed lane")
	}

	if ls.Nonce > sv.Nonce {
		return nil, aerrors.New(6, "voucher has an outdated nonce, cannot redeem")
	}

	mergeValue := types.NewInt(0)
	for _, merge := range sv.Merges {
		if merge.Lane == sv.Lane {
			return nil, aerrors.New(7, "voucher cannot merge its own lane")
		}

		ols := self.LaneStates[fmt.Sprint(merge.Lane)]

		if ols.Nonce >= merge.Nonce {
			return nil, aerrors.New(8, "merge in voucher has outdated nonce, cannot redeem")
		}

		mergeValue = types.BigAdd(mergeValue, ols.Redeemed)
		ols.Nonce = merge.Nonce
	}

	ls.Nonce = sv.Nonce
	balanceDelta := types.BigSub(sv.Amount, types.BigAdd(mergeValue, ls.Redeemed))
	ls.Redeemed = sv.Amount

	newSendBalance := types.BigAdd(self.ToSend, balanceDelta)
	if newSendBalance.LessThan(types.NewInt(0)) {
		// TODO: is this impossible?
		return nil, aerrors.New(9, "voucher would leave channel balance negative")
	}

	if newSendBalance.GreaterThan(act.Balance) {
		return nil, aerrors.New(10, "not enough funds in channel to cover voucher")
	}

	log.Info("vals: ", newSendBalance, sv.Amount, balanceDelta, mergeValue, ls.Redeemed)
	self.ToSend = newSendBalance

	if sv.MinCloseHeight != 0 {
		if self.ClosingAt != 0 && self.ClosingAt < sv.MinCloseHeight {
			self.ClosingAt = sv.MinCloseHeight
		}
		if self.MinCloseHeight < sv.MinCloseHeight {
			self.MinCloseHeight = sv.MinCloseHeight
		}
	}

	ncid, err := storage.Put(&self)
	if err != nil {
		return nil, err
	}
	if err := storage.Commit(oldstate, ncid); err != nil {
		return nil, err
	}

	return nil, nil
}

func (pca PaymentChannelActor) Close(act *types.Actor, vmctx types.VMContext, params *struct{}) ([]byte, aerrors.ActorError) {
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

	self.ClosingAt = vmctx.BlockHeight() + build.PaymentChannelClosingDelay
	if self.ClosingAt < self.MinCloseHeight {
		self.ClosingAt = self.MinCloseHeight
	}

	ncid, err := storage.Put(&self)
	if err != nil {
		return nil, err
	}
	if err := storage.Commit(oldstate, ncid); err != nil {
		return nil, err
	}

	return nil, nil
}

func (pca PaymentChannelActor) Collect(act *types.Actor, vmctx types.VMContext, params *struct{}) ([]byte, aerrors.ActorError) {
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
	_, err := vmctx.Send(self.From, 0, types.BigSub(act.Balance, self.ToSend), nil)
	if err != nil {
		return nil, err
	}
	_, err = vmctx.Send(self.To, 0, self.ToSend, nil)
	if err != nil {
		return nil, err
	}

	self.ToSend = types.NewInt(0)

	ncid, err := storage.Put(&self)
	if err != nil {
		return nil, err
	}
	if err := storage.Commit(oldstate, ncid); err != nil {
		return nil, err
	}

	return nil, nil
}

func (pca PaymentChannelActor) GetOwner(act *types.Actor, vmctx types.VMContext, params *struct{}) ([]byte, aerrors.ActorError) {
	var self PaymentChannelActorState
	storage := vmctx.Storage()
	if err := storage.Get(storage.GetHead(), &self); err != nil {
		return nil, err
	}

	return self.From.Bytes(), nil
}

func (pca PaymentChannelActor) GetToSend(act *types.Actor, vmctx types.VMContext, params *struct{}) ([]byte, aerrors.ActorError) {
	var self PaymentChannelActorState
	storage := vmctx.Storage()
	if err := storage.Get(storage.GetHead(), &self); err != nil {
		return nil, err
	}

	return self.ToSend.Bytes(), nil
}
