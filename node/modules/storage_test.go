package modules

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	"github.com/filecoin-project/lotus/node/modules/dtypes"
	"github.com/filecoin-project/lotus/node/modules/helpers"
	"github.com/filecoin-project/lotus/node/repo"
)

func TestF3DatastoreStopReleasesRepo(t *testing.T) {
	ctx := context.Background()

	r, err := repo.NewFS(t.TempDir())
	require.NoError(t, err)
	require.NoError(t, r.Init(repo.FullNode))

	lr, err := r.Lock(repo.FullNode)
	require.NoError(t, err)

	app := fx.New(
		fx.NopLogger,
		fx.Provide(
			func() helpers.MetricsCtx { return ctx },
			LockedRepo(lr),
			F3Datastore,
		),
		fx.Invoke(func(dtypes.F3DS) {}),
	)
	require.NoError(t, app.Start(ctx))
	require.NoError(t, app.Stop(ctx))

	lr, err = r.Lock(repo.FullNode)
	require.NoError(t, err)
	require.NoError(t, lr.Close())
}
