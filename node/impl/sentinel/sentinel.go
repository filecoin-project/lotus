package sentinel

import (
	"context"

	logging "github.com/ipfs/go-log/v2"
	"go.uber.org/fx"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/events"
	"github.com/filecoin-project/lotus/sentinel"
)

var log = logging.Logger("sentinel-module")

// SentinelAPI is an implementation of the sentinel api that logs each tipset as its applied and reverted.
type SentinelAPI struct {
	fx.In

	Events *events.Events
}

func (m *SentinelAPI) WatchStart(ctx context.Context) error {
	log.Info("starting sentinel watch")
	return m.Events.Observe(&sentinel.LoggingTipSetObserver{})
}

var _ api.Sentinel = &SentinelAPI{}
