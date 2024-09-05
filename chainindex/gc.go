package chainindex

import (
	"context"
	"strconv"
	"time"

	logging "github.com/ipfs/go-log/v2"

	"github.com/filecoin-project/lotus/chain/actors/policy"
)

var (
	log             = logging.Logger("chainindex")
	cleanupInterval = time.Duration(8) * time.Hour
)

func (si *SqliteIndexer) gcLoop() {
	defer si.wg.Done()

	// Initial cleanup before entering the loop
	si.cleanupRevertedTipsets(si.ctx)
	si.gc(si.ctx)

	cleanupTicker := time.NewTicker(cleanupInterval)
	defer cleanupTicker.Stop()

	for {
		select {
		case <-cleanupTicker.C:
			si.cleanupRevertedTipsets(si.ctx)
			si.gc(si.ctx)
		case <-si.ctx.Done():
			return
		}
	}
}

func (si *SqliteIndexer) gc(ctx context.Context) {
	if si.gcRetentionEpochs <= 0 {
		return
	}

	head := si.cs.GetHeaviestTipSet().Height()
	removeEpoch := int64(head) - si.gcRetentionEpochs

	if removeEpoch <= 0 {
		return
	}

	res, err := si.removeTipsetsBeforeHeightStmt.ExecContext(ctx, removeEpoch)
	if err != nil {
		log.Errorw("failed to remove reverted tipsets before height", "height", removeEpoch, "error", err)
		return
	}

	rows, err := res.RowsAffected()
	if err != nil {
		log.Errorw("failed to get rows affected", "error", err)
		return
	}

	log.Infow("gc'd tipsets", "height", removeEpoch, "nRows", rows)

	// Also GC eth hashes

	// Calculate the number of days
	days := int((si.gcRetentionEpochs * 30) / (24 * 60 * 60))

	// Ensure we have at least 1 day for GC
	if days < 1 {
		return
	}

	res, err = si.removeEthHashesOlderThanStmt.Exec("-" + strconv.Itoa(days) + " day")
	if err != nil {
		log.Errorw("failed to delete eth hashes older than", "error", err)
		return
	}

	rows, err = res.RowsAffected()
	if err != nil {
		log.Errorw("failed to get rows affected", "error", err)
		return
	}

	log.Infow("gc'd eth hashes", "height", removeEpoch, "nRows", rows)
}

func (si *SqliteIndexer) cleanupRevertedTipsets(ctx context.Context) {
	head := si.cs.GetHeaviestTipSet().Height()

	finalEpoch := (head - policy.ChainFinality) - 10 // 10 is for some grace period
	if finalEpoch <= 0 {
		return
	}

	// remove all entries from the `tipsets` table where `reverted=true` and height is < finalEpoch
	// cacade delete based on foreign key constraints takes care of cleaning up the other tables
	res, err := si.removeRevertedTipsetsBeforeHeightStmt.ExecContext(ctx, finalEpoch)
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
