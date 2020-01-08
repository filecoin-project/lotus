package paych

import (
	"context"
	"fmt"

	"github.com/ipfs/go-cid"
	"go.uber.org/fx"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/types"
	full "github.com/filecoin-project/lotus/node/impl/full"
	"github.com/filecoin-project/lotus/paych"
)

type PaychAPI struct {
	fx.In

	full.MpoolAPI
	full.WalletAPI
	full.ChainAPI

	PaychMgr *paych.Manager
}

func (a *PaychAPI) PaychGet(ctx context.Context, from, to address.Address, ensureFunds types.BigInt) (*api.ChannelInfo, error) {
	ch, mcid, err := a.PaychMgr.GetPaych(ctx, from, to, ensureFunds)
	if err != nil {
		return nil, err
	}

	return &api.ChannelInfo{
		Channel:        ch,
		ChannelMessage: mcid,
	}, nil
}

func (a *PaychAPI) PaychAllocateLane(ctx context.Context, ch address.Address) (uint64, error) {
	return a.PaychMgr.AllocateLane(ch)
}

func (a *PaychAPI) PaychNewPayment(ctx context.Context, from, to address.Address, vouchers []api.VoucherSpec) (*api.PaymentInfo, error) {
	amount := vouchers[len(vouchers)-1].Amount

	// TODO: Fix free fund tracking in PaychGet
	// TODO: validate voucher spec before locking funds
	ch, err := a.PaychGet(ctx, from, to, amount)
	if err != nil {
		return nil, err
	}

	lane, err := a.PaychMgr.AllocateLane(ch.Channel)
	if err != nil {
		return nil, err
	}

	svs := make([]*types.SignedVoucher, len(vouchers))

	for i, v := range vouchers {
		sv, err := a.paychVoucherCreate(ctx, ch.Channel, types.SignedVoucher{
			Amount: v.Amount,
			Lane:   lane,

			Extra:          v.Extra,
			TimeLock:       v.TimeLock,
			MinCloseHeight: v.MinClose,
		})
		if err != nil {
			return nil, err
		}

		svs[i] = sv
	}

	var pchCid *cid.Cid
	if ch.ChannelMessage != cid.Undef {
		pchCid = &ch.ChannelMessage
	}

	return &api.PaymentInfo{
		Channel:        ch.Channel,
		ChannelMessage: pchCid,
		Vouchers:       svs,
	}, nil
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
		ControlAddr: ci.Control,
		Direction:   api.PCHDir(ci.Direction),
	}, nil
}

func (a *PaychAPI) PaychClose(ctx context.Context, addr address.Address) (cid.Cid, error) {
	ci, err := a.PaychMgr.GetChannelInfo(addr)
	if err != nil {
		return cid.Undef, err
	}

	nonce, err := a.MpoolGetNonce(ctx, ci.Control)
	if err != nil {
		return cid.Undef, err
	}

	msg := &types.Message{
		To:     addr,
		From:   ci.Control,
		Value:  types.NewInt(0),
		Method: actors.PCAMethods.Close,
		Nonce:  nonce,

		GasLimit: types.NewInt(500),
		GasPrice: types.NewInt(0),
	}

	smsg, err := a.WalletSignMessage(ctx, ci.Control, msg)
	if err != nil {
		return cid.Undef, err
	}

	if _, err := a.MpoolPush(ctx, smsg); err != nil {
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

func (a *PaychAPI) PaychVoucherAdd(ctx context.Context, ch address.Address, sv *types.SignedVoucher, proof []byte, minDelta types.BigInt) (types.BigInt, error) {
	_ = a.PaychMgr.TrackInboundChannel(ctx, ch) // TODO: expose those calls

	return a.PaychMgr.AddVoucher(ctx, ch, sv, proof, minDelta)
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
		return nil, xerrors.Errorf("get channel info: %w", err)
	}

	nonce, err := a.PaychMgr.NextNonceForLane(ctx, pch, voucher.Lane)
	if err != nil {
		return nil, xerrors.Errorf("getting next nonce for lane: %w", err)
	}

	sv := &voucher
	sv.Nonce = nonce

	vb, err := sv.SigningBytes()
	if err != nil {
		return nil, err
	}

	sig, err := a.WalletSign(ctx, ci.Control, vb)
	if err != nil {
		return nil, err
	}

	sv.Signature = sig

	if _, err := a.PaychMgr.AddVoucher(ctx, pch, sv, nil, types.NewInt(0)); err != nil {
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

	nonce, err := a.MpoolGetNonce(ctx, ci.Control)
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
		From:     ci.Control,
		To:       ch,
		Value:    types.NewInt(0),
		Nonce:    nonce,
		Method:   actors.PCAMethods.UpdateChannelState,
		Params:   enc,
		GasLimit: types.NewInt(100000),
		GasPrice: types.NewInt(0),
	}

	smsg, err := a.WalletSignMessage(ctx, ci.Control, msg)
	if err != nil {
		return cid.Undef, err
	}

	if _, err := a.MpoolPush(ctx, smsg); err != nil {
		return cid.Undef, err
	}

	// TODO: should we wait for it...?
	return smsg.Cid(), nil
}
