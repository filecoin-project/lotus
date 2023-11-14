package modules

import (
	"go.uber.org/fx"

	"github.com/filecoin-project/lotus/chain/gen/slashfilter/slashsvc"
	"github.com/filecoin-project/lotus/node/config"
	"github.com/filecoin-project/lotus/node/impl/full"
	"github.com/filecoin-project/lotus/node/modules/helpers"
)

type consensusReporterModules struct {
	fx.In

	full.WalletAPI
	full.ChainAPI
	full.MpoolAPI
	full.SyncAPI
}

func RunConsensusFaultReporter(config config.FaultReporterConfig) func(mctx helpers.MetricsCtx, lc fx.Lifecycle, mod consensusReporterModules) error {
	return func(mctx helpers.MetricsCtx, lc fx.Lifecycle, mod consensusReporterModules) error {
		ctx := helpers.LifecycleCtx(mctx, lc)

		return slashsvc.SlashConsensus(ctx, &mod, config.ConsensusFaultReporterDataDir, config.ConsensusFaultReporterAddress)
	}
}
