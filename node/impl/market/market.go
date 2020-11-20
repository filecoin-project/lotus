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

	FMgr *market.FundManager
}

func (a *MarketAPI) MarketReserveFunds(ctx context.Context, wallet address.Address, addr address.Address, amt types.BigInt) (cid.Cid, error) {
	return a.FMgr.Reserve(ctx, wallet, addr, amt)
}

func (a *MarketAPI) MarketReleaseFunds(ctx context.Context, addr address.Address, amt types.BigInt) error {
	return a.FMgr.Release(addr, amt)
}
