package modules

import (
	"context"

	"go.uber.org/fx"

	"github.com/filecoin-project/lotus/chain/index"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/node/modules/helpers"
	"github.com/filecoin-project/lotus/node/repo"
)

func MsgIndex(lc fx.Lifecycle, mctx helpers.MetricsCtx, cs *store.ChainStore, r repo.LockedRepo) (index.MsgIndex, error) {
	basePath, err := r.SqlitePath()
	if err != nil {
		return nil, err
	}

	msgIndex, err := index.NewMsgIndex(helpers.LifecycleCtx(mctx, lc), basePath, cs)
	if err != nil {
		return nil, err
	}

	lc.Append(fx.Hook{
		OnStop: func(_ context.Context) error {
			return msgIndex.Close()
		},
	})

	return msgIndex, nil
}

func DummyMsgIndex() index.MsgIndex {
	return index.DummyMsgIndex
}
