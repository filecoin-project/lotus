package impl

import (
	"context"

	logging "github.com/ipfs/go-log/v2"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/node/impl/client"
	"github.com/filecoin-project/lotus/node/impl/common"
	"github.com/filecoin-project/lotus/node/impl/full"
	"github.com/filecoin-project/lotus/node/impl/market"
	"github.com/filecoin-project/lotus/node/impl/paych"
)

var log = logging.Logger("node")

type FullNodeAPI struct {
	common.CommonAPI
	full.ChainAPI
	client.API
	full.MpoolAPI
	full.GasAPI
	market.MarketAPI
	paych.PaychAPI
	full.StateAPI
	full.MsigAPI
	full.WalletAPI
	full.SyncAPI
}

// MpoolEstimateGasPrice estimates gas price
// Deprecated: used GasEstimateGasPrice instead
func (fa *FullNodeAPI) MpoolEstimateGasPrice(ctx context.Context, nblocksincl uint64, sender address.Address, limit int64, tsk types.TipSetKey) (types.BigInt, error) {
	return fa.GasEstimateGasPrice(ctx, nblocksincl, sender, limit, tsk)
}

var _ api.FullNode = &FullNodeAPI{}
