package index

import (
	"context"
	"time"

	"github.com/filecoin-project/lotus/chain/actors/policy"
)

// TODO: GC based on policy
// TODO Fix go-routine management in ChainIndexer
func (ci *ChainIndexer) cleanupReorgsLoop(ctx context.Context) error {
	// Initial cleanup before entering the loop
	ci.cleanupRevertedTipsets(ctx)

	// a timer that fires every 8 hours
	cleanupTicker := time.NewTicker(time.Duration(8) * time.Hour)
	defer cleanupTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-cleanupTicker.C:
			ci.cleanupRevertedTipsets(ctx)
			continue
		}
	}
}

func (ci *ChainIndexer) cleanupRevertedTipsets(ctx context.Context) {
	finalityHeight := ci.cs.GetHeaviestTipSet().Height()

	finalEpoch := (finalityHeight - policy.ChainFinality) - 10 // 10 is for some grace period
	if finalEpoch <= 0 {
		return
	}

	// remove all entries from the `tipsets` table where `reverted=true` and height is < finalEpoch
	// cacade delete based on foreign key constraints takes care of cleaning up the other tables
	res, err := ci.stmts.removeRevertedTipsetsBeforeHeight.ExecContext(ctx, finalEpoch)
	if err != nil {
		log.Errorw("failed to remove reverted tipsets before height", "height", finalEpoch, "error", err)
		return
	}

	rows, err := res.RowsAffected()
	if err != nil {
		log.Errorw("failed to get rows affected", "error", err)
		return
	}

	log.Infow("removed reverted tipsets", "height", finalEpoch, "nRows", rows)
}
