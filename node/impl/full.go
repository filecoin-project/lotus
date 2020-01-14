package impl

import (
	logging "github.com/ipfs/go-log/v2"

	"github.com/filecoin-project/lotus/node/impl/client"
	"github.com/filecoin-project/lotus/node/impl/market"
	"github.com/filecoin-project/lotus/node/impl/paych"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/node/impl/full"
)

var log = logging.Logger("node")

type FullNodeAPI struct {
	CommonAPI
	full.ChainAPI
	client.API
	full.MpoolAPI
	market.MarketAPI
	paych.PaychAPI
	full.StateAPI
	full.WalletAPI
	full.SyncAPI
}

var _ api.FullNode = &FullNodeAPI{}
