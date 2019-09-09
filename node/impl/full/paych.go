package full

import (
	"context"
	"fmt"

	"github.com/filecoin-project/go-lotus/api"
	"github.com/filecoin-project/go-lotus/chain/actors"
	"github.com/filecoin-project/go-lotus/chain/address"
	"github.com/filecoin-project/go-lotus/chain/types"
	"github.com/filecoin-project/go-lotus/paych"

	"github.com/ipfs/go-cid"
	"go.uber.org/fx"
	"golang.org/x/xerrors"
)

type PaychAPI struct {
	fx.In

	MpoolAPI
	WalletAPI
	ChainAPI

	PaychMgr *paych.Manager
}

func (a *PaychAPI) PaychCreate(ctx context.Context, from, to address.Address, amt types.BigInt) (address.Address, error) {
	act, _, err := a.paychCreate(ctx, from, to, amt)
	return act, err
}

func (a *PaychAPI) paychCreate(ctx context.Context, from, to address.Address, amt types.BigInt) (address.Address, cid.Cid, error) {
	params, aerr := actors.SerializeParams(&actors.PCAConstructorParams{To: to})
	if aerr != nil {
		return address.Undef, cid.Undef, aerr
	}

	nonce, err := a.MpoolGetNonce(ctx, from)
	if err != nil {
		return address.Undef, cid.Undef, err
	}

	enc, err := actors.SerializeParams(&actors.ExecParams{
		Params: params,
		Code:   actors.PaymentChannelActorCodeCid,
	})

	msg := &types.Message{
		To:       actors.InitActorAddress,
		From:     from,
		Value:    amt,
		Nonce:    nonce,
		Method:   actors.IAMethods.Exec,
		Params:   enc,
		GasLimit: types.NewInt(1000000),
		GasPrice: types.NewInt(0),
	}

	ser, err := msg.Serialize()
	if err != nil {
		return address.Undef, cid.Undef, err
	}

	sig, err := a.WalletSign(ctx, from, ser)
	if err != nil {
		return address.Undef, cid.Undef, err
	}

	smsg := &types.SignedMessage{
		Message:   *msg,
		Signature: *sig,
	}

	if err := a.MpoolPush(ctx, smsg); err != nil {
		return address.Undef, cid.Undef, err
	}

	mwait, err := a.ChainWaitMsg(ctx, smsg.Cid())
	if err != nil {
		return address.Undef, cid.Undef, err
	}

	if mwait.Receipt.ExitCode != 0 {
		return address.Undef, cid.Undef, fmt.Errorf("payment channel creation failed (exit code %d)", mwait.Receipt.ExitCode)
	}

	paychaddr, err := address.NewFromBytes(mwait.Receipt.Return)
	if err != nil {
		return address.Undef, cid.Undef, err
	}

	if err := a.PaychMgr.TrackOutboundChannel(ctx, paychaddr); err != nil {
		return address.Undef, cid.Undef, err
	}

	return paychaddr, msg.Cid(), nil
}

func (a *PaychAPI) PaychList(ctx context.Context) ([]address.Address, error) {
	return a.PaychMgr.ListChannels()
}

func (a *PaychAPI) PaychStatus(ctx context.Context, pch address.Address) (*api.PaychStatus, error) {
	ci, err := a.PaychMgr.GetChannelInfo(pch)
	if err != nil {
		return nil, err
	}
	return &api.PaychStatus{
		ControlAddr: ci.ControlAddr,
		Direction:   api.PCHDir(ci.Direction),
	}, nil
}

func (a *PaychAPI) PaychClose(ctx context.Context, addr address.Address) (cid.Cid, error) {
	ci, err := a.PaychMgr.GetChannelInfo(addr)
	if err != nil {
		return cid.Undef, err
	}

	nonce, err := a.MpoolGetNonce(ctx, ci.ControlAddr)
	if err != nil {
		return cid.Undef, err
	}

	msg := &types.Message{
		To:     addr,
		From:   ci.ControlAddr,
		Value:  types.NewInt(0),
		Method: actors.PCAMethods.Close,
		Nonce:  nonce,

		GasLimit: types.NewInt(500),
		GasPrice: types.NewInt(0),
	}

	smsg, err := a.WalletSignMessage(ctx, ci.ControlAddr, msg)
	if err != nil {
		return cid.Undef, err
	}

	if err := a.MpoolPush(ctx, smsg); err != nil {
		return cid.Undef, err
	}

	return smsg.Cid(), nil
}

func (a *PaychAPI) PaychVoucherCheckValid(ctx context.Context, ch address.Address, sv *types.SignedVoucher) error {
	return a.PaychMgr.CheckVoucherValid(ctx, ch, sv)
}

func (a *PaychAPI) PaychVoucherCheckSpendable(ctx context.Context, ch address.Address, sv *types.SignedVoucher, secret []byte, proof []byte) (bool, error) {
	return a.PaychMgr.CheckVoucherSpendable(ctx, ch, sv, secret, proof)
}

func (a *PaychAPI) PaychVoucherAdd(ctx context.Context, ch address.Address, sv *types.SignedVoucher, proof []byte) error {
	_ = a.PaychMgr.TrackInboundChannel(ctx, ch) // TODO: expose those calls

	if err := a.PaychVoucherCheckValid(ctx, ch, sv); err != nil {
		return err
	}

	return a.PaychMgr.AddVoucher(ctx, ch, sv, proof)
}

// PaychVoucherCreate creates a new signed voucher on the given payment channel
// with the given lane and amount.  The value passed in is exactly the value
// that will be used to create the voucher, so if previous vouchers exist, the
// actual additional value of this voucher will only be the difference between
// the two.
func (a *PaychAPI) PaychVoucherCreate(ctx context.Context, pch address.Address, amt types.BigInt, lane uint64) (*types.SignedVoucher, error) {
	return a.paychVoucherCreate(ctx, pch, types.SignedVoucher{Amount: amt, Lane: lane})
}

func (a *PaychAPI) paychVoucherCreate(ctx context.Context, pch address.Address, voucher types.SignedVoucher) (*types.SignedVoucher, error) {
	ci, err := a.PaychMgr.GetChannelInfo(pch)
	if err != nil {
		return nil, err
	}

	nonce, err := a.PaychMgr.NextNonceForLane(ctx, pch, voucher.Lane)
	if err != nil {
		return nil, err
	}

	sv := &voucher
	sv.Nonce = nonce

	vb, err := sv.SigningBytes()
	if err != nil {
		return nil, err
	}

	sig, err := a.WalletSign(ctx, ci.ControlAddr, vb)
	if err != nil {
		return nil, err
	}

	sv.Signature = sig

	if err := a.PaychMgr.AddVoucher(ctx, pch, sv, nil); err != nil {
		return nil, xerrors.Errorf("failed to persist voucher: %w", err)
	}

	return sv, nil
}

func (a *PaychAPI) PaychVoucherList(ctx context.Context, pch address.Address) ([]*types.SignedVoucher, error) {
	vi, err := a.PaychMgr.ListVouchers(ctx, pch)
	if err != nil {
		return nil, err
	}

	out := make([]*types.SignedVoucher, len(vi))
	for k, v := range vi {
		out[k] = v.Voucher
	}

	return out, nil
}

func (a *PaychAPI) PaychVoucherSubmit(ctx context.Context, ch address.Address, sv *types.SignedVoucher) (cid.Cid, error) {
	ci, err := a.PaychMgr.GetChannelInfo(ch)
	if err != nil {
		return cid.Undef, err
	}

	nonce, err := a.MpoolGetNonce(ctx, ci.ControlAddr)
	if err != nil {
		return cid.Undef, err
	}

	if sv.Extra != nil || len(sv.SecretPreimage) > 0 {
		return cid.Undef, fmt.Errorf("cant handle more advanced payment channel stuff yet")
	}

	enc, err := actors.SerializeParams(&actors.PCAUpdateChannelStateParams{
		Sv: *sv,
	})
	if err != nil {
		return cid.Undef, err
	}

	msg := &types.Message{
		From:     ci.ControlAddr,
		To:       ch,
		Value:    types.NewInt(0),
		Nonce:    nonce,
		Method:   actors.PCAMethods.UpdateChannelState,
		Params:   enc,
		GasLimit: types.NewInt(100000),
		GasPrice: types.NewInt(0),
	}

	smsg, err := a.WalletSignMessage(ctx, ci.ControlAddr, msg)
	if err != nil {
		return cid.Undef, err
	}

	if err := a.MpoolPush(ctx, smsg); err != nil {
		return cid.Undef, err
	}

	// TODO: should we wait for it...?
	return smsg.Cid(), nil
}
