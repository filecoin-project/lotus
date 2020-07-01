package market

import (
	"context"

	"go.uber.org/fx"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/chain/market"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/ipfs/go-cid"
)

type MarketAPI struct {
	fx.In

	FMgr *market.FundMgr
}

func (a *MarketAPI) MarketEnsureAvailable(ctx context.Context, addr, wallet address.Address, amt types.BigInt) (cid.Cid, error) {
	return a.FMgr.EnsureAvailable(ctx, addr, wallet, amt)
}
