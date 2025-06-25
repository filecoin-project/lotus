package modules

import (
	"context"

	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/namespace"
	"go.uber.org/fx"

	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/metrics"
	"github.com/filecoin-project/lotus/node/impl/full"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
	"github.com/filecoin-project/lotus/node/modules/helpers"
	"github.com/filecoin-project/lotus/paychmgr"
)

func NewManager(mctx helpers.MetricsCtx, lc fx.Lifecycle, sm stmgr.StateManagerAPI, pchstore *paychmgr.Store, api paychmgr.ManagerNodeAPI) *paychmgr.Manager {
	ctx := helpers.LifecycleCtx(mctx, lc)
	ctx, shutdown := context.WithCancel(ctx)
	ctx = metrics.AddNetworkTag(ctx)
	return paychmgr.NewManager(ctx, shutdown, sm, pchstore, api)
}

func NewPaychStore(ds dtypes.MetadataDS) *paychmgr.Store {
	ds = namespace.Wrap(ds, datastore.NewKey("/paych/"))
	return paychmgr.NewStore(ds)
}

type PaychManagerNodeAPI struct {
	fx.In

	full.MpoolAPI
	full.StateAPI
}

var _ paychmgr.ManagerNodeAPI = &PaychManagerNodeAPI{}

// HandlePaychManager is called by dependency injection to set up hooks
func HandlePaychManager(lc fx.Lifecycle, pm *paychmgr.Manager) {
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			return pm.Start()
		},
		OnStop: func(context.Context) error {
			return pm.Stop()
		},
	})
}
