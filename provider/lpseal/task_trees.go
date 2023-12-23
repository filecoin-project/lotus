package lpseal

import (
	"context"
	"github.com/filecoin-project/go-commp-utils/zerocomm"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/lib/harmony/harmonydb"
	"github.com/filecoin-project/lotus/lib/harmony/harmonytask"
	"github.com/filecoin-project/lotus/lib/harmony/resources"
	"github.com/filecoin-project/lotus/provider/lpffi"
	"github.com/filecoin-project/lotus/storage/pipeline/lib/nullreader"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
	"golang.org/x/xerrors"
)

type TreesTask struct {
	sp *SealPoller
	db *harmonydb.DB
	sc *lpffi.SealCalls

	max int
}

func NewTreesTask(sp *SealPoller, db *harmonydb.DB, sc *lpffi.SealCalls, maxTrees int) *TreesTask {
	return &TreesTask{
		sp: sp,
		db: db,
		sc: sc,

		max: maxTrees,
	}
}

func (t *TreesTask) Do(taskID harmonytask.TaskID, stillOwned func() bool) (done bool, err error) {
	ctx := context.Background()

	var sectorParamsArr []struct {
		SpID         int64                   `db:"sp_id"`
		SectorNumber int64                   `db:"sector_number"`
		RegSealProof abi.RegisteredSealProof `db:"reg_seal_proof"`
	}

	err = t.db.Select(ctx, &sectorParamsArr, `
		SELECT sp_id, sector_number, reg_seal_proof
		FROM sectors_sdr_pipeline
		WHERE task_id_tree_r = $1 and task_id_tree_c = $1 and task_id_tree_d = $1`, taskID)
	if err != nil {
		return false, xerrors.Errorf("getting sector params: %w", err)
	}

	if len(sectorParamsArr) != 1 {
		return false, xerrors.Errorf("expected 1 sector params, got %d", len(sectorParamsArr))
	}
	sectorParams := sectorParamsArr[0]

	var pieces []struct {
		PieceIndex int64  `db:"piece_index"`
		PieceCID   string `db:"piece_cid"`
		PieceSize  int64  `db:"piece_size"`
	}

	err = t.db.Select(ctx, &pieces, `
		SELECT piece_index, piece_cid, piece_size
		FROM sectors_sdr_initial_pieces
		WHERE sp_id = $1 AND sector_number = $2`, sectorParams.SpID, sectorParams.SectorNumber)
	if err != nil {
		return false, xerrors.Errorf("getting pieces: %w", err)
	}

	if len(pieces) > 0 {
		// todo sectors with data
		return false, xerrors.Errorf("todo sectors with data")
	}

	ssize, err := sectorParams.RegSealProof.SectorSize()
	if err != nil {
		return false, xerrors.Errorf("getting sector size: %w", err)
	}

	commd := zerocomm.ZeroPieceCommitment(abi.PaddedPieceSize(ssize).Unpadded())

	sref := storiface.SectorRef{
		ID: abi.SectorID{
			Miner:  abi.ActorID(sectorParams.SpID),
			Number: abi.SectorNumber(sectorParams.SectorNumber),
		},
		ProofType: sectorParams.RegSealProof,
	}

	// D
	treeUnsealed, err := t.sc.TreeD(ctx, sref, abi.PaddedPieceSize(ssize), nullreader.NewNullReader(abi.PaddedPieceSize(ssize).Unpadded()))
	if err != nil {
		return false, xerrors.Errorf("computing tree d: %w", err)
	}

	// R / C
	sealed, unsealed, err := t.sc.TreeRC(ctx, sref, commd)
	if err != nil {
		return false, xerrors.Errorf("computing tree r and c: %w", err)
	}

	if unsealed != treeUnsealed {
		return false, xerrors.Errorf("tree-d and tree-r/c unsealed CIDs disagree")
	}

	n, err := t.db.Exec(ctx, `UPDATE sectors_sdr_pipeline
		SET after_tree_r = true, after_tree_c = true, after_tree_d = true, tree_r_cid = $1, tree_d_cid = $3
		WHERE sp_id = $1 AND sector_number = $2`,
		sectorParams.SpID, sectorParams.SectorNumber, sealed, unsealed)
	if err != nil {
		return false, xerrors.Errorf("store sdr-trees success: updating pipeline: %w", err)
	}
	if n != 1 {
		return false, xerrors.Errorf("store sdr-trees success: updated %d rows", n)
	}

	return true, nil
}

func (t *TreesTask) CanAccept(ids []harmonytask.TaskID, engine *harmonytask.TaskEngine) (*harmonytask.TaskID, error) {
	// todo reserve storage

	id := ids[0]
	return &id, nil
}

func (t *TreesTask) TypeDetails() harmonytask.TaskTypeDetails {
	return harmonytask.TaskTypeDetails{
		Max:  t.max,
		Name: "SDRTrees",
		Cost: resources.Resources{
			Cpu: 1,
			Gpu: 1,
			Ram: 8000 << 20, // todo
		},
		MaxFailures: 3,
		Follows:     nil,
	}
}

func (t *TreesTask) Adder(taskFunc harmonytask.AddTaskFunc) {
	t.sp.pollers[pollerTrees].Set(taskFunc)
}

var _ harmonytask.TaskInterface = &TreesTask{}
