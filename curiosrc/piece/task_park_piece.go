package piece

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	"github.com/hashicorp/go-multierror"
	logging "github.com/ipfs/go-log/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/curiosrc/ffi"
	"github.com/filecoin-project/lotus/curiosrc/seal"
	"github.com/filecoin-project/lotus/lib/harmony/harmonydb"
	"github.com/filecoin-project/lotus/lib/harmony/harmonytask"
	"github.com/filecoin-project/lotus/lib/harmony/resources"
	"github.com/filecoin-project/lotus/lib/promise"
	"github.com/filecoin-project/lotus/storage/paths"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
)

var log = logging.Logger("cu-piece")
var PieceParkPollInterval = time.Second * 15

// ParkPieceTask gets a piece from some origin, and parks it in storage
// Pieces are always f00, piece ID is mapped to pieceCID in the DB
type ParkPieceTask struct {
	db *harmonydb.DB
	sc *ffi.SealCalls

	TF promise.Promise[harmonytask.AddTaskFunc]

	max int
}

func NewParkPieceTask(db *harmonydb.DB, sc *ffi.SealCalls, max int) (*ParkPieceTask, error) {
	pt := &ParkPieceTask{
		db: db,
		sc: sc,

		max: max,
	}

	ctx := context.Background()

	// We should delete all incomplete pieces before we start
	// as we would have lost reader for these. The RPC caller will get an error
	// when Curio shuts down before parking a piece. They can always retry.
	// Leaving these pieces we utilise unnecessary resources in the form of ParkPieceTask

	_, err := db.Exec(ctx, `DELETE FROM parked_pieces WHERE complete = FALSE AND task_id IS NULL`)
	if err != nil {
		return nil, xerrors.Errorf("failed to delete incomplete parked pieces: %w", err)
	}

	go pt.pollPieceTasks(ctx)
	return pt, nil
}

func (p *ParkPieceTask) pollPieceTasks(ctx context.Context) {
	for {
		// select parked pieces with no task_id
		var pieceIDs []struct {
			ID storiface.PieceNumber `db:"id"`
		}

		err := p.db.Select(ctx, &pieceIDs, `SELECT id FROM parked_pieces WHERE complete = FALSE AND task_id IS NULL`)
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
			p.TF.Val(ctx)(func(id harmonytask.TaskID, tx *harmonydb.Tx) (shouldCommit bool, err error) {
				// update
				n, err := tx.Exec(`UPDATE parked_pieces SET task_id = $1 WHERE id = $2 AND complete = FALSE AND task_id IS NULL`, id, pieceID.ID)
				if err != nil {
					return false, xerrors.Errorf("updating parked piece: %w", err)
				}

				// commit only if we updated the piece
				return n > 0, nil
			})
		}
	}
}

func (p *ParkPieceTask) Do(taskID harmonytask.TaskID, stillOwned func() bool) (done bool, err error) {
	ctx := context.Background()

	// Define a struct to hold piece data.
	var piecesData []struct {
		PieceID         int64     `db:"id"`
		PieceCreatedAt  time.Time `db:"created_at"`
		PieceCID        string    `db:"piece_cid"`
		Complete        bool      `db:"complete"`
		PiecePaddedSize int64     `db:"piece_padded_size"`
		PieceRawSize    string    `db:"piece_raw_size"`
	}

	// Select the piece data using the task ID.
	err = p.db.Select(ctx, &piecesData, `
        SELECT id, created_at, piece_cid, complete, piece_padded_size, piece_raw_size
        FROM parked_pieces
        WHERE task_id = $1
    `, taskID)
	if err != nil {
		return false, xerrors.Errorf("fetching piece data: %w", err)
	}

	if len(piecesData) == 0 {
		return false, xerrors.Errorf("no piece data found for task_id: %d", taskID)
	}

	pieceData := piecesData[0]

	if pieceData.Complete {
		log.Warnw("park piece task already complete", "task_id", taskID, "piece_cid", pieceData.PieceCID)
		return true, nil
	}

	// Define a struct for reference data.
	var refData []struct {
		DataURL     string          `db:"data_url"`
		DataHeaders json.RawMessage `db:"data_headers"`
	}

	// Now, select the first reference data that has a URL.
	err = p.db.Select(ctx, &refData, `
        SELECT data_url, data_headers
        FROM parked_piece_refs
        WHERE piece_id = $1 AND data_url IS NOT NULL`, pieceData.PieceID)
	if err != nil {
		return false, xerrors.Errorf("fetching reference data: %w", err)
	}

	if len(refData) == 0 {
		return false, xerrors.Errorf("no refs found for piece_id: %d", pieceData.PieceID)
	}

	// Convert piece_raw_size from string to int64.
	pieceRawSize, err := strconv.ParseInt(pieceData.PieceRawSize, 10, 64)
	if err != nil {
		return false, xerrors.Errorf("parsing piece raw size: %w", err)
	}

	var merr error

	for i := range refData {
		if refData[i].DataURL != "" {
			upr := &seal.UrlPieceReader{
				Url:     refData[0].DataURL,
				RawSize: pieceRawSize,
			}
			defer func() {
				_ = upr.Close()
			}()

			pnum := storiface.PieceNumber(pieceData.PieceID)

			if err := p.sc.WritePiece(ctx, &taskID, pnum, pieceRawSize, upr); err != nil {
				merr = multierror.Append(merr, xerrors.Errorf("write piece: %w", err))
				continue
			}

			// Update the piece as complete after a successful write.
			_, err = p.db.Exec(ctx, `UPDATE parked_pieces SET complete = TRUE task_id = NULL WHERE id = $1`, pieceData.PieceID)
			if err != nil {
				return false, xerrors.Errorf("marking piece as complete: %w", err)
			}

			return true, nil
		}
		return false, merr
	}

	// If no URL is found, this indicates an issue since at least one URL is expected.
	return false, xerrors.Errorf("no data URL found for piece_id: %d", pieceData.PieceID)
}

func (p *ParkPieceTask) CanAccept(ids []harmonytask.TaskID, engine *harmonytask.TaskEngine) (*harmonytask.TaskID, error) {
	id := ids[0]
	return &id, nil
}

func (p *ParkPieceTask) TypeDetails() harmonytask.TaskTypeDetails {
	const maxSizePiece = 64 << 30

	return harmonytask.TaskTypeDetails{
		Max:  p.max,
		Name: "ParkPiece",
		Cost: resources.Resources{
			Cpu:     1,
			Gpu:     0,
			Ram:     64 << 20,
			Storage: p.sc.Storage(p.taskToRef, storiface.FTPiece, storiface.FTNone, maxSizePiece, storiface.PathSealing, paths.MinFreeStoragePercentage),
		},
		MaxFailures: 10,
	}
}

func (p *ParkPieceTask) taskToRef(id harmonytask.TaskID) (ffi.SectorRef, error) {
	var pieceIDs []struct {
		ID storiface.PieceNumber `db:"id"`
	}

	err := p.db.Select(context.Background(), &pieceIDs, `SELECT id FROM parked_pieces WHERE task_id = $1`, id)
	if err != nil {
		return ffi.SectorRef{}, xerrors.Errorf("getting piece id: %w", err)
	}

	if len(pieceIDs) != 1 {
		return ffi.SectorRef{}, xerrors.Errorf("expected 1 piece id, got %d", len(pieceIDs))
	}

	pref := pieceIDs[0].ID.Ref()

	return ffi.SectorRef{
		SpID:         int64(pref.ID.Miner),
		SectorNumber: int64(pref.ID.Number),
		RegSealProof: pref.ProofType,
	}, nil
}

func (p *ParkPieceTask) Adder(taskFunc harmonytask.AddTaskFunc) {
	p.TF.Set(taskFunc)
}

var _ harmonytask.TaskInterface = &ParkPieceTask{}
