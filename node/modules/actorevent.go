package modules

import (
	"time"

	"go.uber.org/fx"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/events/filter"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/node/config"
	"github.com/filecoin-project/lotus/node/impl/full"
)

type ActorEventHandlerParams struct {
	fx.In

	*filter.EventFilterManager
	*store.ChainStore
}

func ActorEventHandler(cfg config.EventsConfig) func(ActorEventHandlerParams) *full.ActorEventHandler {
	return func(params ActorEventHandlerParams) *full.ActorEventHandler {
		return full.NewActorEventHandler(
			params.ChainStore,
			params.EventFilterManager,
			time.Duration(buildconstants.BlockDelaySecs)*time.Second,
			abi.ChainEpoch(cfg.MaxFilterHeightRange),
		)
	}
}
