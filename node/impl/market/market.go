package market

import (
	"context"

	"go.uber.org/fx"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/chain/market"
	"github.com/filecoin-project/lotus/chain/types"
)

type MarketAPI struct {
	fx.In

	FMgr *market.FundMgr
}

func (a *MarketAPI) MarketEnsureAvailable(ctx context.Context, addr address.Address, amt types.BigInt) error {
	return a.FMgr.EnsureAvailable(ctx, addr, amt)
}
