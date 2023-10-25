package provider

import (
	"context"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/journal"
	"github.com/filecoin-project/lotus/lib/harmony/harmonydb"
	"github.com/filecoin-project/lotus/node/config"
	dtypes "github.com/filecoin-project/lotus/node/modules/dtypes"
	"github.com/filecoin-project/lotus/provider/lpwindow"
	"github.com/filecoin-project/lotus/storage/ctladdr"
	"github.com/filecoin-project/lotus/storage/sealer"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
)

func WindowPostScheduler(ctx context.Context, fc config.LotusProviderFees, pc config.ProvingConfig,
	api api.FullNode, sealer sealer.SectorManager, verif storiface.Verifier, j journal.Journal,
	as *ctladdr.AddressSelector, maddr []dtypes.MinerAddress, db *harmonydb.DB, max int) (*lpwindow.WdPostTask, error) {

	/*fc2 := config.MinerFeeConfig{
		MaxPreCommitGasFee: fc.MaxPreCommitGasFee,
		MaxCommitGasFee:    fc.MaxCommitGasFee,
		MaxTerminateGasFee: fc.MaxTerminateGasFee,
		MaxPublishDealsFee: fc.MaxPublishDealsFee,
	}*/
	ts := lpwindow.NewWdPostTask(db, nil, max)

	panic("change handler thing")

	/*go fps.RunV2(ctx, func(api wdpost.WdPoStCommands, actor address.Address) wdpost.ChangeHandlerIface {
		return wdpost.NewChangeHandler2(api, actor, ts)
	})*/
	return ts, nil

}
