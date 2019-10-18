package deals

import (
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/types"
)

func VoucherSpec(blocksDuration uint64, price types.BigInt, start uint64, extra *types.ModVerifyParams) []api.VoucherSpec {
	nVouchers := blocksDuration / build.MinDealVoucherIncrement
	if nVouchers < 1 {
		nVouchers = 1
	}
	if nVouchers > build.MaxVouchersPerDeal {
		nVouchers = build.MaxVouchersPerDeal
	}

	hIncrements := blocksDuration / nVouchers
	vouchers := make([]api.VoucherSpec, nVouchers)

	for i := uint64(0); i < nVouchers; i++ {
		vouchers[i] = api.VoucherSpec{
			Amount:   types.BigDiv(types.BigMul(price, types.NewInt(i+1)), types.NewInt(nVouchers)),
			TimeLock: start + (hIncrements * (i + 1)),
			MinClose: start + (hIncrements * (i + 1)),
			Extra:    extra,
		}
	}

	return vouchers
}
