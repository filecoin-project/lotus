package impl

import (
	"context"
	"github.com/filecoin-project/lotus/node/impl/market"

	"github.com/filecoin-project/lotus/node/impl/client"
	"github.com/filecoin-project/lotus/node/impl/paych"

	logging "github.com/ipfs/go-log"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/address"
	"github.com/filecoin-project/lotus/miner"
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

	Miner *miner.Miner
}

func (a *FullNodeAPI) MinerAddresses(context.Context) ([]address.Address, error) {
	return a.Miner.Addresses()
}

func (a *FullNodeAPI) MinerRegister(ctx context.Context, addr address.Address) error {
	return a.Miner.Register(addr)
}

func (a *FullNodeAPI) MinerUnregister(ctx context.Context, addr address.Address) error {
	return a.Miner.Unregister(ctx, addr)
}

var _ api.FullNode = &FullNodeAPI{}
