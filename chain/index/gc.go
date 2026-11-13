package index

import (
	"context"
	"database/sql"
	"strconv"
	"time"

	logging "github.com/ipfs/go-log/v2"

	"github.com/filecoin-project/lotus/chain/actors/builtin"
)

var (
	log             = logging.Logger("chainindex")
	cleanupInterval = time.Duration(4) * time.Hour

	// gcBatchSize caps the rows removed by each GC delete. Each batch holds
	// writerLk, so tipset indexing waits behind at most one batch.
	gcBatchSize = 1000
)

func (si *SqliteIndexer) gcLoop() {
	defer si.wg.Done()

	// Initial cleanup before entering the loop
	si.gc(si.ctx)

	cleanupTicker := time.NewTicker(cleanupInterval)
	defer cleanupTicker.Stop()

	for si.ctx.Err() == nil {
		if si.isClosed() {
			return
		}

		select {
		case <-cleanupTicker.C:
			si.gc(si.ctx)
		case <-si.ctx.Done():
			return
		}
	}
}

func (si *SqliteIndexer) gc(ctx context.Context) {
	if si.gcRetentionEpochs <= 0 {
		log.Info("gc retention epochs is not set, skipping gc")
		return
	}
	log.Info("starting index gc")

	head := si.cs.GetHeaviestTipSet()

	removalEpoch := int64(head.Height()) - si.gcRetentionEpochs - 10 // 10 is for some grace period
	if removalEpoch <= 0 {
		log.Info("no tipsets to gc")
		return
	}

	log.Infof("gc'ing all (reverted and non-reverted) tipsets before epoch %d", removalEpoch)

	// Completion markers go first: a tipset whose rows are only partly removed
	// must read as unindexed, not as a complete but smaller event set.
	rows, err := si.gcDelete(ctx, si.stmts.removeTipsetBloomsBeforeHeightStmt, removalEpoch)
	if err != nil {
		log.Errorw("failed to remove tipset blooms before height", "height", removalEpoch, "removed", rows, "error", err)
		return
	}
	log.Infof("gc'd %d tipset blooms before epoch %d", rows, removalEpoch)

	rows, err = si.gcDelete(ctx, si.stmts.removeTipsetsBeforeHeightStmt, removalEpoch)
	if err != nil {
		log.Errorw("failed to remove tipsets before height", "height", removalEpoch, "removed", rows, "error", err)
		return
	}
	log.Infof("gc'd %d entries before epoch %d", rows, removalEpoch)

	// -------------------------------------------------------------------------------------------------
	// Also GC eth hashes

	// Convert gcRetentionEpochs to number of days
	gcRetentionDays := si.gcRetentionEpochs / (builtin.EpochsInDay)
	if gcRetentionDays < 1 {
		log.Infof("skipping gc of eth hashes as retention days is less than 1")
		return
	}

	log.Infof("gc'ing eth hashes older than %d days", gcRetentionDays)
	rows, err = si.gcDelete(ctx, si.stmts.removeEthHashesOlderThanStmt, "-"+strconv.Itoa(int(gcRetentionDays))+" day")
	if err != nil {
		log.Errorw("failed to gc eth hashes", "days", gcRetentionDays, "removed", rows, "error", err)
		return
	}
	log.Infof("gc'd %d eth hashes older than %d days", rows, gcRetentionDays)
}

// gcDelete runs a row-limited delete until a batch comes back short, taking
// writerLk for each batch and releasing it between them. It returns the
// number of rows removed, including those removed before an error.
func (si *SqliteIndexer) gcDelete(ctx context.Context, stmt *sql.Stmt, cutoff any) (int64, error) {
	var total int64
	for {
		if err := ctx.Err(); err != nil {
			return total, err
		}
		n, err := si.gcDeleteBatch(ctx, stmt, cutoff)
		total += n
		if err != nil || n < int64(gcBatchSize) {
			return total, err
		}
	}
}

func (si *SqliteIndexer) gcDeleteBatch(ctx context.Context, stmt *sql.Stmt, cutoff any) (int64, error) {
	si.writerLk.Lock()
	defer si.writerLk.Unlock()

	res, err := stmt.ExecContext(ctx, cutoff, gcBatchSize)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
