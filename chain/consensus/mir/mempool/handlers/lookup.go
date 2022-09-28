package handlers

import (
	"github.com/filecoin-project/lotus/chain/consensus/mir/mempool/types"
	"github.com/filecoin-project/mir/pkg/dsl"
	mpdsl "github.com/filecoin-project/mir/pkg/mempool/dsl"
	"github.com/filecoin-project/mir/pkg/pb/mempoolpb"
	"github.com/filecoin-project/mir/pkg/pb/requestpb"
	t "github.com/filecoin-project/mir/pkg/types"
)

// IncludeTransactionLookupByID registers event handlers for processing RequestTransactions events.
func IncludeTransactionLookupByID(
	m dsl.Module,
	mc *types.ModuleConfig,
	params *types.ModuleParams,
	commonState *types.State,
) {
	mpdsl.UponRequestTransactions(m, func(txIDs []t.TxID, origin *mempoolpb.RequestTransactionsOrigin) error {
		present := make([]bool, len(txIDs))
		txs := make([]*requestpb.Request, len(txIDs))
		for i, txID := range txIDs {
			txs[i], present[i] = commonState.TxByID[txID]
		}

		mpdsl.TransactionsResponse(m, t.ModuleID(origin.Module), present, txs, origin)
		return nil
	})
}
