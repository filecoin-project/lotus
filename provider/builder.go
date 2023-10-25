package provider

import (
	"context"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/journal"
	"github.com/filecoin-project/lotus/lib/harmony/harmonydb"
	"github.com/filecoin-project/lotus/node/config"
	dtypes "github.com/filecoin-project/lotus/node/modules/dtypes"
	"github.com/filecoin-project/lotus/provider/chainsched"
	"github.com/filecoin-project/lotus/provider/lpwindow"
	"github.com/filecoin-project/lotus/storage/ctladdr"
	"github.com/filecoin-project/lotus/storage/sealer"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
	logging "github.com/ipfs/go-log/v2"
)

var log = logging.Logger("provider")

func WindowPostScheduler(ctx context.Context, fc config.LotusProviderFees, pc config.ProvingConfig,
	api api.FullNode, sealer sealer.SectorManager, verif storiface.Verifier, j journal.Journal,
	as *ctladdr.AddressSelector, maddr []dtypes.MinerAddress, db *harmonydb.DB, max int) (*lpwindow.WdPostTask, error) {

	chainSched := chainsched.New(api)

	return lpwindow.NewWdPostTask(db, nil, chainSched, maddr)
}
