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
	dtypes "github.com/filecoin-project/lotus/node/modules/dtypes"
	"github.com/filecoin-project/lotus/storage/paths"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
)

//var log = logging.Logger("provider")

func WindowPostScheduler(ctx context.Context, fc config.CurioFees, pc config.CurioProvingConfig,
	api api.FullNode, verif storiface.Verifier, sender *message.Sender, chainSched *chainsched.CurioChainSched,
	as *multictladdr.MultiAddressSelector, addresses map[dtypes.MinerAddress]bool, db *harmonydb.DB,
	stor paths.Store, idx paths.SectorIndex, max int) (*window.WdPostTask, *window.WdPostSubmitTask, *window.WdPostRecoverDeclareTask, error) {

	// todo config
	ft := window.NewSimpleFaultTracker(stor, idx, pc.ParallelCheckLimit, time.Duration(pc.SingleCheckTimeout), time.Duration(pc.PartitionCheckTimeout))

	computeTask, err := window.NewWdPostTask(db, api, ft, stor, verif, chainSched, addresses, max, pc.ParallelCheckLimit, time.Duration(pc.SingleCheckTimeout))
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

	return computeTask, submitTask, recoverTask, nil
}
