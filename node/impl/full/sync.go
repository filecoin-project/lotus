package full

import (
	"context"

	"github.com/filecoin-project/go-lotus/api"
	"github.com/filecoin-project/go-lotus/chain"
	"go.uber.org/fx"
)

type SyncAPI struct {
	fx.In

	Syncer *chain.Syncer
}

func (a *SyncAPI) SyncState(ctx context.Context) (*api.SyncState, error) {
	ss := a.Syncer.State()
	return &api.SyncState{
		Base:   ss.Base,
		Target: ss.Target,
		Stage:  ss.Stage,
		Height: ss.Height,
	}, nil
}
