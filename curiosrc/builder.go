package curio

import (
	"context"
	"time"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/curiosrc/chainsched"
	"github.com/filecoin-project/lotus/curiosrc/message"
	"github.com/filecoin-project/lotus/curiosrc/multictladdr"
	"github.com/filecoin-project/lotus/curiosrc/window"
	"github.com/filecoin-project/lotus/lib/harmony/harmonydb"
	"github.com/filecoin-project/lotus/node/config"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
	"github.com/filecoin-project/lotus/storage/paths"
	"github.com/filecoin-project/lotus/storage/sealer"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
)

//var log = logging.Logger("curio")

func WindowPostScheduler(ctx context.Context, fc config.CurioFees, pc config.ProvingConfig,
	api api.FullNode, verif storiface.Verifier, lw *sealer.LocalWorker, sender *message.Sender,
	as *multictladdr.MultiAddressSelector, addresses map[dtypes.MinerAddress]bool, db *harmonydb.DB,
	stor paths.Store, idx paths.SectorIndex, max int) (*window.WdPostTask, *window.WdPostSubmitTask, *window.WdPostRecoverDeclareTask, error) {

	chainSched := chainsched.New(api)

	// todo config
	ft := window.NewSimpleFaultTracker(stor, idx, 32, 5*time.Second, 300*time.Second)

	computeTask, err := window.NewWdPostTask(db, api, ft, lw, verif, chainSched, addresses, max)
	if err != nil {
		return nil, nil, nil, err
	}

	submitTask, err := window.NewWdPostSubmitTask(chainSched, sender, db, api, fc.MaxWindowPoStGasFee, as)
	if err != nil {
		return nil, nil, nil, err
	}

	recoverTask, err := window.NewWdPostRecoverDeclareTask(sender, db, api, ft, as, chainSched, fc.MaxWindowPoStGasFee, addresses)
	if err != nil {
		return nil, nil, nil, err
	}

	go chainSched.Run(ctx)

	return computeTask, submitTask, recoverTask, nil
}
