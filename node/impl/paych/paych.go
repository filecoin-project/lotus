package paych

import (
	"context"

	"golang.org/x/xerrors"

	"github.com/ipfs/go-cid"
	"go.uber.org/fx"

	"github.com/filecoin-project/go-address"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/actors/builtin/paych"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/paychmgr"
)

type PaychAPI struct {
	fx.In

	PaychMgr *paychmgr.Manager
}

func (a *PaychAPI) PaychGet(ctx context.Context, from, to address.Address, amt types.BigInt) (*api.ChannelInfo, error) {
	ch, mcid, err := a.PaychMgr.GetPaych(ctx, from, to, amt)
	if err != nil {
		return nil, err
	}

	return &api.ChannelInfo{
		Channel:      ch,
		WaitSentinel: mcid,
	}, nil
}

func (a *PaychAPI) PaychAvailableFunds(ctx context.Context, ch address.Address) (*api.ChannelAvailableFunds, error) {
	return a.PaychMgr.AvailableFunds(ch)
}

func (a *PaychAPI) PaychAvailableFundsByFromTo(ctx context.Context, from, to address.Address) (*api.ChannelAvailableFunds, error) {
	return a.PaychMgr.AvailableFundsByFromTo(from, to)
}

func (a *PaychAPI) PaychGetWaitReady(ctx context.Context, sentinel cid.Cid) (address.Address, error) {
	return a.PaychMgr.GetPaychWaitReady(ctx, sentinel)
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

	svs := make([]*paych.SignedVoucher, len(vouchers))

	for i, v := range vouchers {
		sv, err := a.PaychMgr.CreateVoucher(ctx, ch.Channel, paych.SignedVoucher{
			Amount: v.Amount,
			Lane:   lane,

			Extra:           v.Extra,
			TimeLockMin:     v.TimeLockMin,
			TimeLockMax:     v.TimeLockMax,
			MinSettleHeight: v.MinSettle,
		})
		if err != nil {
			return nil, err
		}
		if sv.Voucher == nil {
			return nil, xerrors.Errorf("Could not create voucher - shortfall of %d", sv.Shortfall)
		}

		svs[i] = sv.Voucher
	}

	return &api.PaymentInfo{
		Channel:      ch.Channel,
		WaitSentinel: ch.WaitSentinel,
		Vouchers:     svs,
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

func (a *PaychAPI) PaychSettle(ctx context.Context, addr address.Address) (cid.Cid, error) {
	return a.PaychMgr.Settle(ctx, addr)
}

func (a *PaychAPI) PaychCollect(ctx context.Context, addr address.Address) (cid.Cid, error) {
	return a.PaychMgr.Collect(ctx, addr)
}

func (a *PaychAPI) PaychVoucherCheckValid(ctx context.Context, ch address.Address, sv *paych.SignedVoucher) error {
	return a.PaychMgr.CheckVoucherValid(ctx, ch, sv)
}

func (a *PaychAPI) PaychVoucherCheckSpendable(ctx context.Context, ch address.Address, sv *paych.SignedVoucher, secret []byte, proof []byte) (bool, error) {
	return a.PaychMgr.CheckVoucherSpendable(ctx, ch, sv, secret, proof)
}

func (a *PaychAPI) PaychVoucherAdd(ctx context.Context, ch address.Address, sv *paych.SignedVoucher, proof []byte, minDelta types.BigInt) (types.BigInt, error) {
	return a.PaychMgr.AddVoucherInbound(ctx, ch, sv, proof, minDelta)
}

// PaychVoucherCreate creates a new signed voucher on the given payment channel
// with the given lane and amount.  The value passed in is exactly the value
// that will be used to create the voucher, so if previous vouchers exist, the
// actual additional value of this voucher will only be the difference between
// the two.
// If there are insufficient funds in the channel to create the voucher,
// returns a nil voucher and the shortfall.
func (a *PaychAPI) PaychVoucherCreate(ctx context.Context, pch address.Address, amt types.BigInt, lane uint64) (*api.VoucherCreateResult, error) {
	return a.PaychMgr.CreateVoucher(ctx, pch, paych.SignedVoucher{Amount: amt, Lane: lane})
}

func (a *PaychAPI) PaychVoucherList(ctx context.Context, pch address.Address) ([]*paych.SignedVoucher, error) {
	vi, err := a.PaychMgr.ListVouchers(ctx, pch)
	if err != nil {
		return nil, err
	}

	out := make([]*paych.SignedVoucher, len(vi))
	for k, v := range vi {
		out[k] = v.Voucher
	}

	return out, nil
}

func (a *PaychAPI) PaychVoucherSubmit(ctx context.Context, ch address.Address, sv *paych.SignedVoucher, secret []byte, proof []byte) (cid.Cid, error) {
	return a.PaychMgr.SubmitVoucher(ctx, ch, sv, secret, proof)
}
