package paych

import (
	"context"

	"github.com/ipfs/go-cid"
	"go.uber.org/fx"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	paychtypes "github.com/filecoin-project/go-state-types/builtin/v8/paych"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/paychmgr"
)

// PaychAPI is the external interface that Lotus provides for interacting with payment channels.
type PaychAPI interface {
	PaychGet(ctx context.Context, from, to address.Address, amt types.BigInt, opts api.PaychGetOpts) (*api.ChannelInfo, error)
	PaychFund(ctx context.Context, from, to address.Address, amt types.BigInt) (*api.ChannelInfo, error)
	PaychAvailableFunds(ctx context.Context, ch address.Address) (*api.ChannelAvailableFunds, error)
	PaychAvailableFundsByFromTo(ctx context.Context, from, to address.Address) (*api.ChannelAvailableFunds, error)
	PaychGetWaitReady(ctx context.Context, sentinel cid.Cid) (address.Address, error)
	PaychAllocateLane(ctx context.Context, ch address.Address) (uint64, error)
	PaychNewPayment(ctx context.Context, from, to address.Address, vouchers []api.VoucherSpec) (*api.PaymentInfo, error)
	PaychList(ctx context.Context) ([]address.Address, error)
	PaychStatus(ctx context.Context, pch address.Address) (*api.PaychStatus, error)
	PaychSettle(ctx context.Context, addr address.Address) (cid.Cid, error)
	PaychCollect(ctx context.Context, addr address.Address) (cid.Cid, error)
	PaychVoucherCheckValid(ctx context.Context, ch address.Address, sv *paychtypes.SignedVoucher) error
	PaychVoucherCheckSpendable(ctx context.Context, ch address.Address, sv *paychtypes.SignedVoucher, secret []byte, proof []byte) (bool, error)
	PaychVoucherAdd(ctx context.Context, ch address.Address, sv *paychtypes.SignedVoucher, proof []byte, minDelta types.BigInt) (types.BigInt, error)
	PaychVoucherCreate(ctx context.Context, pch address.Address, amt types.BigInt, lane uint64) (*api.VoucherCreateResult, error)
	PaychVoucherList(ctx context.Context, pch address.Address) ([]*paychtypes.SignedVoucher, error)
	PaychVoucherSubmit(ctx context.Context, ch address.Address, sv *paychtypes.SignedVoucher, secret []byte, proof []byte) (cid.Cid, error)
}

// PaychImpl is the implementation of the PaychAPI interface that uses the paychmgr.Manager
// to manage payment channels. It is designed to be used with dependency injection via fx.
// It provides methods for creating, funding, and managing payment channels, as well as handling
// vouchers and their associated operations.
type PaychImpl struct {
	fx.In

	PaychMgr *paychmgr.Manager
}

var _ PaychAPI = &PaychImpl{}

func (a *PaychImpl) PaychGet(ctx context.Context, from, to address.Address, amt types.BigInt, opts api.PaychGetOpts) (*api.ChannelInfo, error) {
	ch, mcid, err := a.PaychMgr.GetPaych(ctx, from, to, amt, paychmgr.GetOpts{
		Reserve:  true,
		OffChain: opts.OffChain,
	})
	if err != nil {
		return nil, err
	}

	return &api.ChannelInfo{
		Channel:      ch,
		WaitSentinel: mcid,
	}, nil
}

func (a *PaychImpl) PaychFund(ctx context.Context, from, to address.Address, amt types.BigInt) (*api.ChannelInfo, error) {
	ch, mcid, err := a.PaychMgr.GetPaych(ctx, from, to, amt, paychmgr.GetOpts{
		Reserve:  false,
		OffChain: false,
	})
	if err != nil {
		return nil, err
	}

	return &api.ChannelInfo{
		Channel:      ch,
		WaitSentinel: mcid,
	}, nil
}

func (a *PaychImpl) PaychAvailableFunds(ctx context.Context, ch address.Address) (*api.ChannelAvailableFunds, error) {
	return a.PaychMgr.AvailableFunds(ctx, ch)
}

func (a *PaychImpl) PaychAvailableFundsByFromTo(ctx context.Context, from, to address.Address) (*api.ChannelAvailableFunds, error) {
	return a.PaychMgr.AvailableFundsByFromTo(ctx, from, to)
}

func (a *PaychImpl) PaychGetWaitReady(ctx context.Context, sentinel cid.Cid) (address.Address, error) {
	return a.PaychMgr.GetPaychWaitReady(ctx, sentinel)
}

func (a *PaychImpl) PaychAllocateLane(ctx context.Context, ch address.Address) (uint64, error) {
	return a.PaychMgr.AllocateLane(ctx, ch)
}

func (a *PaychImpl) PaychNewPayment(ctx context.Context, from, to address.Address, vouchers []api.VoucherSpec) (*api.PaymentInfo, error) {
	amount := vouchers[len(vouchers)-1].Amount

	// TODO: Fix free fund tracking in PaychGet
	// TODO: validate voucher spec before locking funds
	ch, err := a.PaychGet(ctx, from, to, amount, api.PaychGetOpts{OffChain: false})
	if err != nil {
		return nil, err
	}

	lane, err := a.PaychMgr.AllocateLane(ctx, ch.Channel)
	if err != nil {
		return nil, err
	}

	svs := make([]*paychtypes.SignedVoucher, len(vouchers))

	for i, v := range vouchers {
		sv, err := a.PaychMgr.CreateVoucher(ctx, ch.Channel, paychtypes.SignedVoucher{
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

func (a *PaychImpl) PaychList(ctx context.Context) ([]address.Address, error) {
	return a.PaychMgr.ListChannels(ctx)
}

func (a *PaychImpl) PaychStatus(ctx context.Context, pch address.Address) (*api.PaychStatus, error) {
	ci, err := a.PaychMgr.GetChannelInfo(ctx, pch)
	if err != nil {
		return nil, err
	}
	return &api.PaychStatus{
		ControlAddr: ci.Control,
		Direction:   api.PCHDir(ci.Direction),
	}, nil
}

func (a *PaychImpl) PaychSettle(ctx context.Context, addr address.Address) (cid.Cid, error) {
	return a.PaychMgr.Settle(ctx, addr)
}

func (a *PaychImpl) PaychCollect(ctx context.Context, addr address.Address) (cid.Cid, error) {
	return a.PaychMgr.Collect(ctx, addr)
}

func (a *PaychImpl) PaychVoucherCheckValid(ctx context.Context, ch address.Address, sv *paychtypes.SignedVoucher) error {
	return a.PaychMgr.CheckVoucherValid(ctx, ch, sv)
}

func (a *PaychImpl) PaychVoucherCheckSpendable(ctx context.Context, ch address.Address, sv *paychtypes.SignedVoucher, secret []byte, proof []byte) (bool, error) {
	return a.PaychMgr.CheckVoucherSpendable(ctx, ch, sv, secret, proof)
}

func (a *PaychImpl) PaychVoucherAdd(ctx context.Context, ch address.Address, sv *paychtypes.SignedVoucher, proof []byte, minDelta types.BigInt) (types.BigInt, error) {
	return a.PaychMgr.AddVoucherInbound(ctx, ch, sv, proof, minDelta)
}

// PaychVoucherCreate creates a new signed voucher on the given payment channel
// with the given lane and amount.  The value passed in is exactly the value
// that will be used to create the voucher, so if previous vouchers exist, the
// actual additional value of this voucher will only be the difference between
// the two.
// If there are insufficient funds in the channel to create the voucher,
// returns a nil voucher and the shortfall.
func (a *PaychImpl) PaychVoucherCreate(ctx context.Context, pch address.Address, amt types.BigInt, lane uint64) (*api.VoucherCreateResult, error) {
	return a.PaychMgr.CreateVoucher(ctx, pch, paychtypes.SignedVoucher{Amount: amt, Lane: lane})
}

func (a *PaychImpl) PaychVoucherList(ctx context.Context, pch address.Address) ([]*paychtypes.SignedVoucher, error) {
	vi, err := a.PaychMgr.ListVouchers(ctx, pch)
	if err != nil {
		return nil, err
	}

	out := make([]*paychtypes.SignedVoucher, len(vi))
	for k, v := range vi {
		out[k] = v.Voucher
	}

	return out, nil
}

func (a *PaychImpl) PaychVoucherSubmit(ctx context.Context, ch address.Address, sv *paychtypes.SignedVoucher, secret []byte, proof []byte) (cid.Cid, error) {
	return a.PaychMgr.SubmitVoucher(ctx, ch, sv, secret, proof)
}

// DisabledPaych is a stub implementation of the PaychAPI interface that returns errors for all
// methods. It is used when the payment channel manager is disabled (e.g., when
// EnablePaymentChannelManager is set to false in the configuration).
type DisabledPaych struct{}

var _ PaychAPI = &DisabledPaych{}

func (a *DisabledPaych) PaychGet(ctx context.Context, from, to address.Address, amt types.BigInt, opts api.PaychGetOpts) (*api.ChannelInfo, error) {
	return nil, api.ErrPaymentChannelDisabled
}

func (a *DisabledPaych) PaychFund(ctx context.Context, from, to address.Address, amt types.BigInt) (*api.ChannelInfo, error) {
	return nil, api.ErrPaymentChannelDisabled
}

func (a *DisabledPaych) PaychAvailableFunds(ctx context.Context, ch address.Address) (*api.ChannelAvailableFunds, error) {
	return nil, api.ErrPaymentChannelDisabled
}

func (a *DisabledPaych) PaychAvailableFundsByFromTo(ctx context.Context, from, to address.Address) (*api.ChannelAvailableFunds, error) {
	return nil, api.ErrPaymentChannelDisabled
}

func (a *DisabledPaych) PaychGetWaitReady(ctx context.Context, sentinel cid.Cid) (address.Address, error) {
	return address.Undef, api.ErrPaymentChannelDisabled
}

func (a *DisabledPaych) PaychAllocateLane(ctx context.Context, ch address.Address) (uint64, error) {
	return 0, api.ErrPaymentChannelDisabled
}

func (a *DisabledPaych) PaychNewPayment(ctx context.Context, from, to address.Address, vouchers []api.VoucherSpec) (*api.PaymentInfo, error) {
	return nil, api.ErrPaymentChannelDisabled
}

func (a *DisabledPaych) PaychList(ctx context.Context) ([]address.Address, error) {
	return nil, api.ErrPaymentChannelDisabled
}

func (a *DisabledPaych) PaychStatus(ctx context.Context, pch address.Address) (*api.PaychStatus, error) {
	return nil, api.ErrPaymentChannelDisabled
}

func (a *DisabledPaych) PaychSettle(ctx context.Context, addr address.Address) (cid.Cid, error) {
	return cid.Undef, api.ErrPaymentChannelDisabled
}

func (a *DisabledPaych) PaychCollect(ctx context.Context, addr address.Address) (cid.Cid, error) {
	return cid.Undef, api.ErrPaymentChannelDisabled
}

func (a *DisabledPaych) PaychVoucherCheckValid(ctx context.Context, ch address.Address, sv *paychtypes.SignedVoucher) error {
	return api.ErrPaymentChannelDisabled
}

func (a *DisabledPaych) PaychVoucherCheckSpendable(ctx context.Context, ch address.Address, sv *paychtypes.SignedVoucher, secret []byte, proof []byte) (bool, error) {
	return false, api.ErrPaymentChannelDisabled
}

func (a *DisabledPaych) PaychVoucherAdd(ctx context.Context, ch address.Address, sv *paychtypes.SignedVoucher, proof []byte, minDelta types.BigInt) (types.BigInt, error) {
	return types.BigInt{}, api.ErrPaymentChannelDisabled
}

func (a *DisabledPaych) PaychVoucherCreate(ctx context.Context, pch address.Address, amt types.BigInt, lane uint64) (*api.VoucherCreateResult, error) {
	return nil, api.ErrPaymentChannelDisabled
}

func (a *DisabledPaych) PaychVoucherList(ctx context.Context, pch address.Address) ([]*paychtypes.SignedVoucher, error) {
	return nil, api.ErrPaymentChannelDisabled
}

func (a *DisabledPaych) PaychVoucherSubmit(ctx context.Context, ch address.Address, sv *paychtypes.SignedVoucher, secret []byte, proof []byte) (cid.Cid, error) {
	return cid.Undef, api.ErrPaymentChannelDisabled
}
