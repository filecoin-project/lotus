package index

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"time"

	logging "github.com/ipfs/go-log/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/chain/actors/builtin"
)

var (
	log             = logging.Logger("chainindex")
	cleanupInterval = time.Duration(4) * time.Hour

	// gcBatchSize sets the rows removed by each GC batch, rounded up to whole
	// heights for tipset rows. Each batch holds writerLk, so tipset indexing
	// waits behind at most one batch.
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

	rows, blooms, err := si.gcTipsets(ctx, removalEpoch)
	if err != nil {
		log.Errorw("failed to remove tipsets before height", "height", removalEpoch, "removed", rows, "bloomsRemoved", blooms, "error", err)
		return
	}
	log.Infof("gc'd %d entries and %d tipset blooms before epoch %d", rows, blooms, removalEpoch)

	// -------------------------------------------------------------------------------------------------
	// Also GC eth hashes

	// Convert gcRetentionEpochs to number of days
	gcRetentionDays := si.gcRetentionEpochs / (builtin.EpochsInDay)
	if gcRetentionDays < 1 {
		log.Infof("skipping gc of eth hashes as retention days is less than 1")
		return
	}

	log.Infof("gc'ing eth hashes older than %d days", gcRetentionDays)
	rows, err = si.gcEthTxHashes(ctx, "-"+strconv.Itoa(int(gcRetentionDays))+" day")
	if err != nil {
		log.Errorw("failed to gc eth hashes", "days", gcRetentionDays, "removed", rows, "error", err)
		return
	}
	log.Infof("gc'd %d eth hashes older than %d days", rows, gcRetentionDays)
}

// gcTipsets removes tipset rows and their completion markers below
// removalEpoch, one batch at a time under writerLk. Each batch removes whole
// heights in a single transaction, so readers see a tipset either fully
// indexed with its marker or absent.
func (si *SqliteIndexer) gcTipsets(ctx context.Context, removalEpoch int64) (rows, blooms int64, err error) {
	for {
		if err := ctx.Err(); err != nil {
			return rows, blooms, err
		}
		r, b, done, err := si.gcTipsetBatch(ctx, removalEpoch)
		rows += r
		blooms += b
		if err != nil || done {
			return rows, blooms, err
		}
	}
}

// gcTipsetBatch removes every height up to and including the one holding the
// gcBatchSize-th oldest row, so a batch can exceed gcBatchSize by the rest of
// that height. When fewer rows remain, it removes everything below
// removalEpoch and reports done.
func (si *SqliteIndexer) gcTipsetBatch(ctx context.Context, removalEpoch int64) (rows, blooms int64, done bool, err error) {
	si.writerLk.Lock()
	defer si.writerLk.Unlock()

	err = withTx(ctx, si.db, func(tx *sql.Tx) error {
		done = false
		bound := removalEpoch
		var height int64
		err := tx.Stmt(si.stmts.getGCBatchHeightStmt).QueryRowContext(ctx, removalEpoch, gcBatchSize-1).Scan(&height)
		switch {
		case errors.Is(err, sql.ErrNoRows):
			done = true
		case err != nil:
			return xerrors.Errorf("failed to find gc batch height: %w", err)
		default:
			bound = height + 1
		}

		res, err := tx.Stmt(si.stmts.removeTipsetBloomsBeforeHeightStmt).ExecContext(ctx, bound)
		if err != nil {
			return xerrors.Errorf("failed to remove tipset blooms: %w", err)
		}
		if blooms, err = res.RowsAffected(); err != nil {
			return err
		}

		res, err = tx.Stmt(si.stmts.removeTipsetsBeforeHeightStmt).ExecContext(ctx, bound)
		if err != nil {
			return xerrors.Errorf("failed to remove tipsets: %w", err)
		}
		rows, err = res.RowsAffected()
		return err
	})
	if err != nil {
		return 0, 0, false, err
	}
	return rows, blooms, done, nil
}

// gcEthTxHashes removes eth tx hashes inserted before the datetime modifier
// olderThan, gcBatchSize rows at a time under writerLk.
func (si *SqliteIndexer) gcEthTxHashes(ctx context.Context, olderThan string) (int64, error) {
	var total int64
	for {
		if err := ctx.Err(); err != nil {
			return total, err
		}
		n, err := si.gcEthTxHashBatch(ctx, olderThan)
		total += n
		if err != nil || n < int64(gcBatchSize) {
			return total, err
		}
	}
}

func (si *SqliteIndexer) gcEthTxHashBatch(ctx context.Context, olderThan string) (int64, error) {
	si.writerLk.Lock()
	defer si.writerLk.Unlock()

	res, err := si.stmts.removeEthHashesOlderThanStmt.ExecContext(ctx, olderThan, gcBatchSize)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
