package market

import (
	"context"

	"go.opencensus.io/tag"
	"go.uber.org/fx"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/chain/market"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/metrics"

	"github.com/ipfs/go-cid"
)

type MarketAPI struct {
	fx.In

	FMgr *market.FundMgr
}

func (a *MarketAPI) MarketEnsureAvailable(ctx context.Context, addr, wallet address.Address, amt types.BigInt) (cid.Cid, error) {
	ctx, _ = tag.New(ctx, tag.Upsert(metrics.Endpoint, "MarketEnsureAvailable"))
	stop := metrics.Timer(ctx, metrics.APIRequestDuration)
	defer stop()

	return a.FMgr.EnsureAvailable(ctx, addr, wallet, amt)
}
