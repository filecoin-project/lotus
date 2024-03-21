package provider

import (
	"context"
	"time"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/lib/harmony/harmonydb"
	"github.com/filecoin-project/lotus/node/config"
	dtypes "github.com/filecoin-project/lotus/node/modules/dtypes"
	"github.com/filecoin-project/lotus/provider/chainsched"
	"github.com/filecoin-project/lotus/provider/lpmessage"
	"github.com/filecoin-project/lotus/provider/lpwindow"
	"github.com/filecoin-project/lotus/provider/multictladdr"
	"github.com/filecoin-project/lotus/storage/paths"
	"github.com/filecoin-project/lotus/storage/sealer"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
)

//var log = logging.Logger("provider")

func WindowPostScheduler(ctx context.Context, fc config.LotusProviderFees, pc config.ProvingConfig,
	api api.FullNode, verif storiface.Verifier, lw *sealer.LocalWorker, sender *lpmessage.Sender, chainSched *chainsched.ProviderChainSched,
	as *multictladdr.MultiAddressSelector, addresses map[dtypes.MinerAddress]bool, db *harmonydb.DB,
	stor paths.Store, idx paths.SectorIndex, max int) (*lpwindow.WdPostTask, *lpwindow.WdPostSubmitTask, *lpwindow.WdPostRecoverDeclareTask, error) {

	// todo config
	ft := lpwindow.NewSimpleFaultTracker(stor, idx, pc.ParallelCheckLimit, time.Duration(pc.SingleCheckTimeout), time.Duration(pc.PartitionCheckTimeout))

	computeTask, err := lpwindow.NewWdPostTask(db, api, ft, lw, verif, chainSched, addresses, max)
	if err != nil {
		return nil, nil, nil, err
	}

	submitTask, err := lpwindow.NewWdPostSubmitTask(chainSched, sender, db, api, fc.MaxWindowPoStGasFee, as)
	if err != nil {
		return nil, nil, nil, err
	}

	recoverTask, err := lpwindow.NewWdPostRecoverDeclareTask(sender, db, api, ft, as, chainSched, fc.MaxWindowPoStGasFee, addresses)
	if err != nil {
		return nil, nil, nil, err
	}

	return computeTask, submitTask, recoverTask, nil
}
