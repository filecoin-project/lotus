package piece

import (
	"context"
	"time"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/curiosrc/ffi"
	"github.com/filecoin-project/lotus/lib/harmony/harmonydb"
	"github.com/filecoin-project/lotus/lib/harmony/harmonytask"
	"github.com/filecoin-project/lotus/lib/harmony/resources"
	"github.com/filecoin-project/lotus/lib/promise"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
)

type CleanupPieceTask struct {
	max int
	db  *harmonydb.DB
	sc  *ffi.SealCalls

	TF promise.Promise[harmonytask.AddTaskFunc]
}

func NewCleanupPieceTask(db *harmonydb.DB, sc *ffi.SealCalls, max int) *CleanupPieceTask {
	pt := &CleanupPieceTask{
		db: db,
		sc: sc,

		max: max,
	}
	go pt.pollCleanupTasks(context.Background())
	return pt
}

func (c *CleanupPieceTask) pollCleanupTasks(ctx context.Context) {
	for {
		// select pieces with no refs and null cleanup_task_id
		var pieceIDs []struct {
			ID storiface.PieceNumber `db:"id"`
		}

		err := c.db.Select(ctx, &pieceIDs, `SELECT id FROM parked_pieces WHERE cleanup_task_id IS NULL AND (SELECT count(*) FROM parked_piece_refs WHERE piece_id = parked_pieces.id) = 0`)
		if err != nil {
			log.Errorf("failed to get parked pieces: %s", err)
			time.Sleep(PieceParkPollInterval)
			continue
		}

		if len(pieceIDs) == 0 {
			time.Sleep(PieceParkPollInterval)
			continue
		}

		for _, pieceID := range pieceIDs {
			pieceID := pieceID

			// create a task for each piece
			c.TF.Val(ctx)(func(id harmonytask.TaskID, tx *harmonydb.Tx) (shouldCommit bool, err error) {
				// update
				n, err := tx.Exec(`UPDATE parked_pieces SET cleanup_task_id = $1 WHERE id = $2 AND (SELECT count(*) FROM parked_piece_refs WHERE piece_id = parked_pieces.id) = 0`, id, pieceID.ID)
				if err != nil {
					return false, xerrors.Errorf("updating parked piece: %w", err)
				}

				// commit only if we updated the piece
				return n > 0, nil
			})
		}
	}
}

func (c *CleanupPieceTask) Do(taskID harmonytask.TaskID, stillOwned func() bool) (done bool, err error) {
	ctx := context.Background()

	// select by cleanup_task_id
	var pieceID int64

	err = c.db.QueryRow(ctx, "SELECT id FROM parked_pieces WHERE cleanup_task_id = $1", taskID).Scan(&pieceID)
	if err != nil {
		return false, xerrors.Errorf("query parked_piece: %w", err)
	}

	// delete from parked_pieces where id = $1 where ref count = 0
	// note: we delete from the db first because that guarantees that the piece is no longer in use
	// if storage delete fails, it will be retried later is other cleanup tasks
	n, err := c.db.Exec(ctx, "DELETE FROM parked_pieces WHERE id = $1 AND (SELECT count(*) FROM parked_piece_refs WHERE piece_id = $1) = 0", pieceID)
	if err != nil {
		return false, xerrors.Errorf("delete parked_piece: %w", err)
	}

	if n == 0 {
		_, err = c.db.Exec(ctx, `UPDATE parked_pieces SET cleanup_task_id = NULL WHERE id = $1`, pieceID)
		if err != nil {
			return false, xerrors.Errorf("marking piece as complete: %w", err)
		}

		return true, nil
	}

	// remove from storage
	err = c.sc.RemovePiece(ctx, storiface.PieceNumber(pieceID))
	if err != nil {
		log.Errorw("remove piece", "piece_id", pieceID, "error", err)
	}

	return true, nil
}

func (c *CleanupPieceTask) CanAccept(ids []harmonytask.TaskID, engine *harmonytask.TaskEngine) (*harmonytask.TaskID, error) {
	// the remove call runs on paths.Remote storage, so it doesn't really matter where it runs

	id := ids[0]
	return &id, nil
}

func (c *CleanupPieceTask) TypeDetails() harmonytask.TaskTypeDetails {
	return harmonytask.TaskTypeDetails{
		Max:  c.max,
		Name: "DropPiece",
		Cost: resources.Resources{
			Cpu:     1,
			Gpu:     0,
			Ram:     64 << 20,
			Storage: nil,
		},
		MaxFailures: 10,
	}
}

func (c *CleanupPieceTask) Adder(taskFunc harmonytask.AddTaskFunc) {
	c.TF.Set(taskFunc)
}

var _ harmonytask.TaskInterface = &CleanupPieceTask{}
