package lppiece

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/lib/harmony/harmonydb"
	"github.com/filecoin-project/lotus/lib/harmony/harmonytask"
	"github.com/filecoin-project/lotus/lib/harmony/resources"
	"github.com/filecoin-project/lotus/lib/promise"
	"github.com/filecoin-project/lotus/provider/lpffi"
	"github.com/filecoin-project/lotus/provider/lpseal"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
)

var log = logging.Logger("lppiece")

// ParkPieceTask gets a piece from some origin, and parks it in storage
// Pieces are always f00, piece ID is mapped to pieceCID in the DB
type ParkPieceTask struct {
	db *harmonydb.DB
	sc *lpffi.SealCalls

	TF promise.Promise[harmonytask.AddTaskFunc]

	max int
}

func NewParkPieceTask(db *harmonydb.DB, sc *lpffi.SealCalls, max int) *ParkPieceTask {
	return &ParkPieceTask{
		db: db,
		sc: sc,

		max: max,
	}
}

func (p *ParkPieceTask) PullPiece(ctx context.Context, pieceCID cid.Cid, rawSize int64, paddedSize abi.PaddedPieceSize, dataUrl string, headers http.Header) error {
	p.TF.Val(ctx)(func(id harmonytask.TaskID, tx *harmonydb.Tx) (shouldCommit bool, err error) {
		var pieceID int
		err = tx.QueryRow(`INSERT INTO parked_pieces (piece_cid, piece_padded_size, piece_raw_size) VALUES ($1, $2, $3) RETURNING id`, pieceCID.String(), int64(paddedSize), rawSize).Scan(&pieceID)
		if err != nil {
			return false, xerrors.Errorf("inserting parked piece: %w", err)
		}

		var refID int
		err = tx.QueryRow(`INSERT INTO parked_piece_refs (piece_id) VALUES ($1) RETURNING ref_id`, pieceID).Scan(&refID)
		if err != nil {
			return false, xerrors.Errorf("inserting parked piece ref: %w", err)
		}

		headersJson, err := json.Marshal(headers)
		if err != nil {
			return false, xerrors.Errorf("marshaling headers: %w", err)
		}

		_, err = tx.Exec(`INSERT INTO park_piece_tasks (task_id, piece_ref_id, data_url, data_headers, data_raw_size, data_delete_on_finalize)
			VALUES ($1, $2, $3, $4, $5, $6)`, id, refID, dataUrl, headersJson, rawSize, false)
		if err != nil {
			return false, xerrors.Errorf("inserting park piece task: %w", err)
		}

		return true, nil
	})

	return nil
}

func (p *ParkPieceTask) Do(taskID harmonytask.TaskID, stillOwned func() bool) (done bool, err error) {
	ctx := context.Background()

	var taskData []struct {
		PieceID        int       `db:"id"`
		PieceCreatedAt time.Time `db:"created_at"`
		PieceCID       string    `db:"piece_cid"`
		Complete       bool      `db:"complete"`

		DataURL              string `db:"data_url"`
		DataHeaders          string `db:"data_headers"`
		DataRawSize          int64  `db:"data_raw_size"`
		DataDeleteOnFinalize bool   `db:"data_delete_on_finalize"`
	}

	err = p.db.Select(ctx, &taskData, `
		select
			pp.id,
			pp.created_at,
			pp.piece_cid,
			pp.complete,
			ppt.data_url,
			ppt.data_headers,
			ppt.data_raw_size,
			ppt.data_delete_on_finalize
		from park_piece_tasks ppt
		join parked_piece_refs ppr on ppt.piece_ref_id = ppr.ref_id
		join parked_pieces pp on ppr.piece_id = pp.id
		where ppt.task_id = $1
	`, taskID)
	if err != nil {
		return false, err
	}

	if len(taskData) != 1 {
		return false, xerrors.Errorf("expected 1 task, got %d", len(taskData))
	}

	if taskData[0].Complete {
		log.Warnw("park piece task already complete", "task_id", taskID, "piece_cid", taskData[0].PieceCID)
		return true, nil
	}

	upr := &lpseal.UrlPieceReader{
		Url:     taskData[0].DataURL,
		RawSize: taskData[0].DataRawSize,
	}
	defer func() {
		_ = upr.Close()
	}()

	pnum := storiface.PieceNumber(taskData[0].PieceID)

	if err := p.sc.WritePiece(ctx, pnum, taskData[0].DataRawSize, upr); err != nil {
		return false, xerrors.Errorf("write piece: %w", err)
	}

	_, err = p.db.Exec(ctx, `update parked_pieces set complete = true where id = $1`, taskData[0].PieceID)
	if err != nil {
		return false, xerrors.Errorf("marking piece as complete: %w", err)
	}

	return true, nil
}

func (p *ParkPieceTask) CanAccept(ids []harmonytask.TaskID, engine *harmonytask.TaskEngine) (*harmonytask.TaskID, error) {
	id := ids[0]
	return &id, nil
}

func (p *ParkPieceTask) TypeDetails() harmonytask.TaskTypeDetails {
	return harmonytask.TaskTypeDetails{
		Max:  p.max,
		Name: "ParkPiece",
		Cost: resources.Resources{
			Cpu:     1,
			Gpu:     0,
			Ram:     64 << 20,
			Storage: nil, // TODO
		},
		MaxFailures: 10,
	}
}

func (p *ParkPieceTask) Adder(taskFunc harmonytask.AddTaskFunc) {
	p.TF.Set(taskFunc)
}

var _ harmonytask.TaskInterface = &ParkPieceTask{}
