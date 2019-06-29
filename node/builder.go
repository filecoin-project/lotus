package node

import (
	"context"

	"github.com/libp2p/go-libp2p-core/peer"
	"go.uber.org/fx"

	"github.com/filecoin-project/go-lotus/api"
	"github.com/filecoin-project/go-lotus/build"
	"github.com/filecoin-project/go-lotus/node/modules"
)

func New(ctx context.Context) (api.API, error) {
	var resApi api.Struct

	app := fx.New(
		fx.Provide(modules.RandomPeerID),

		fx.Invoke(versionApi(&resApi.Internal.Version)),
		fx.Invoke(idApi(&resApi.Internal.ID)),
	)

	if err := app.Start(ctx); err != nil {
		return nil, err
	}

	return &resApi, nil
}

// TODO: figure out a better way, this isn't usable in long term
func idApi(set *func(ctx context.Context) (peer.ID, error)) func(id peer.ID) {
	return func(id peer.ID) {
		*set = func(ctx context.Context) (peer.ID, error) {
			return id, nil
		}
	}
}

func versionApi(set *func(context.Context) (api.Version, error)) func() {
	return func() {
		*set = func(context.Context) (api.Version, error) {
			return api.Version{
				Version: build.Version,
			}, nil
		}
	}
}
