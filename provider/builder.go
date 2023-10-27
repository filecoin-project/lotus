package provider

import (
	"context"
	"github.com/filecoin-project/lotus/storage/paths"
	"github.com/filecoin-project/lotus/storage/sealer"
	"time"

	logging "github.com/ipfs/go-log/v2"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/lib/harmony/harmonydb"
	"github.com/filecoin-project/lotus/node/config"
	dtypes "github.com/filecoin-project/lotus/node/modules/dtypes"
	"github.com/filecoin-project/lotus/provider/chainsched"
	"github.com/filecoin-project/lotus/provider/lpwindow"
	"github.com/filecoin-project/lotus/storage/ctladdr"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
)

var log = logging.Logger("provider")

func WindowPostScheduler(ctx context.Context, fc config.LotusProviderFees, pc config.ProvingConfig,
	api api.FullNode, verif storiface.Verifier, lw *sealer.LocalWorker,
	as *ctladdr.AddressSelector, maddr []dtypes.MinerAddress, db *harmonydb.DB, stor paths.Store, idx paths.SectorIndex) (*lpwindow.WdPostTask, error) {

	chainSched := chainsched.New(api)

	// todo config
	ft := lpwindow.NewSimpleFaultTracker(stor, idx, 32, 5*time.Second, 300*time.Second)

	return lpwindow.NewWdPostTask(db, api, ft, lw, verif, chainSched, maddr)
}
