package handlers

import (
	"github.com/filecoin-project/lotus/chain/consensus/mir/mempool/types"
	"github.com/filecoin-project/mir/pkg/dsl"
	mpdsl "github.com/filecoin-project/mir/pkg/mempool/dsl"
	"github.com/filecoin-project/mir/pkg/pb/mempoolpb"
	"github.com/filecoin-project/mir/pkg/pb/requestpb"
	t "github.com/filecoin-project/mir/pkg/types"
)

type State struct {
	*types.State
	NewTxIDs []t.TxID
}

// IncludeBatchCreation registers event handlers for processing NewRequests and RequestBatch events.
func IncludeBatchCreation(
	m dsl.Module,
	mc *types.ModuleConfig,
	params *types.ModuleParams,
	commonState *types.State,
) {
	state := &State{
		State:    commonState,
		NewTxIDs: nil,
	}

	dsl.UponNewRequests(m, func(txs []*requestpb.Request) error {
		mpdsl.RequestTransactionIDs(m, mc.Self, txs, &requestTxIDsContext{txs})
		return nil
	})

	mpdsl.UponTransactionIDsResponse(m, func(txIDs []t.TxID, context *requestTxIDsContext) error {
		for i := range txIDs {
			state.TxByID[txIDs[i]] = context.txs[i]
		}
		state.NewTxIDs = append(state.NewTxIDs, txIDs...)
		return nil
	})

	mpdsl.UponRequestBatch(m, func(origin *mempoolpb.RequestBatchOrigin) error {
		var txs []*requestpb.Request
		var txIDs []t.TxID
		batchSize := 0

		submitChan := make(chan []*requestpb.Request)

		state.DescriptorChan <- types.Descriptor{
			Limit:      0,
			SubmitChan: submitChan,
		}

		receivedRequests := <-submitChan
		mpdsl.RequestTransactionIDs(m, mc.Self, receivedRequests, &requestTxIDsContext{receivedRequests})

		txCount := 0
		for _, txID := range state.NewTxIDs {
			tx := state.TxByID[txID]

			// TODO: add other limitations (if any) here.
			if txCount == params.MaxTransactionsInBatch {
				break
			}

			txIDs = append(txIDs, txID)
			txs = append(txs, tx)
			batchSize += len(tx.Data)
			txCount++
		}

		state.NewTxIDs = state.NewTxIDs[txCount:]

		// Note that a batch may be empty.
		mpdsl.NewBatch(m, t.ModuleID(origin.Module), txIDs, txs, origin)
		return nil
	})
}

// Context data structures

type requestTxIDsContext struct {
	txs []*requestpb.Request
}
