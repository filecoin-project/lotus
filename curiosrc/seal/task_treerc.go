package seal

import (
	"context"

	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/curiosrc/ffi"
	"github.com/filecoin-project/lotus/lib/harmony/harmonydb"
	"github.com/filecoin-project/lotus/lib/harmony/harmonytask"
	"github.com/filecoin-project/lotus/lib/harmony/resources"
	"github.com/filecoin-project/lotus/storage/paths"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
)

type TreeRCTask struct {
	sp *SealPoller
	db *harmonydb.DB
	sc *ffi.SealCalls

	max int
}

func NewTreeRCTask(sp *SealPoller, db *harmonydb.DB, sc *ffi.SealCalls, maxTrees int) *TreeRCTask {
	return &TreeRCTask{
		sp: sp,
		db: db,
		sc: sc,

		max: maxTrees,
	}
}

func (t *TreeRCTask) Do(taskID harmonytask.TaskID, stillOwned func() bool) (done bool, err error) {
	ctx := context.Background()

	var sectorParamsArr []struct {
		SpID         int64                   `db:"sp_id"`
		SectorNumber int64                   `db:"sector_number"`
		RegSealProof abi.RegisteredSealProof `db:"reg_seal_proof"`
		CommD        string                  `db:"tree_d_cid"`
	}

	err = t.db.Select(ctx, &sectorParamsArr, `
		SELECT sp_id, sector_number, reg_seal_proof, tree_d_cid
		FROM sectors_sdr_pipeline
		WHERE task_id_tree_c = $1 AND task_id_tree_r = $1`, taskID)
	if err != nil {
		return false, xerrors.Errorf("getting sector params: %w", err)
	}

	if len(sectorParamsArr) != 1 {
		return false, xerrors.Errorf("expected 1 sector params, got %d", len(sectorParamsArr))
	}
	sectorParams := sectorParamsArr[0]

	commd, err := cid.Parse(sectorParams.CommD)
	if err != nil {
		return false, xerrors.Errorf("parsing unsealed CID: %w", err)
	}

	sref := storiface.SectorRef{
		ID: abi.SectorID{
			Miner:  abi.ActorID(sectorParams.SpID),
			Number: abi.SectorNumber(sectorParams.SectorNumber),
		},
		ProofType: sectorParams.RegSealProof,
	}

	// R / C
	sealed, unsealed, err := t.sc.TreeRC(ctx, &taskID, sref, commd)
	if err != nil {
		return false, xerrors.Errorf("computing tree r and c: %w", err)
	}

	if unsealed != commd {
		return false, xerrors.Errorf("commd %s does match unsealed %s", commd.String(), unsealed.String())
	}

	// todo synth porep

	// todo porep challenge check

	n, err := t.db.Exec(ctx, `UPDATE sectors_sdr_pipeline
		SET after_tree_r = true, after_tree_c = true, tree_r_cid = $3, task_id_tree_r = NULL, task_id_tree_c = NULL
		WHERE sp_id = $1 AND sector_number = $2`,
		sectorParams.SpID, sectorParams.SectorNumber, sealed)
	if err != nil {
		return false, xerrors.Errorf("store sdr-trees success: updating pipeline: %w", err)
	}
	if n != 1 {
		return false, xerrors.Errorf("store sdr-trees success: updated %d rows", n)
	}

	return true, nil
}

func (t *TreeRCTask) CanAccept(ids []harmonytask.TaskID, engine *harmonytask.TaskEngine) (*harmonytask.TaskID, error) {
	var tasks []struct {
		TaskID       harmonytask.TaskID `db:"task_id_tree_c"`
		SpID         int64              `db:"sp_id"`
		SectorNumber int64              `db:"sector_number"`
		StorageID    string             `db:"storage_id"`
	}

	if storiface.FTCache != 4 {
		panic("storiface.FTCache != 4")
	}

	ctx := context.Background()

	indIDs := make([]int64, len(ids))
	for i, id := range ids {
		indIDs[i] = int64(id)
	}

	err := t.db.Select(ctx, &tasks, `
		SELECT p.task_id_tree_c, p.sp_id, p.sector_number, l.storage_id FROM sectors_sdr_pipeline p
			INNER JOIN sector_location l ON p.sp_id = l.miner_id AND p.sector_number = l.sector_num
			WHERE task_id_tree_r = ANY ($1) AND l.sector_filetype = 4
`, indIDs)
	if err != nil {
		return nil, xerrors.Errorf("getting tasks: %w", err)
	}

	ls, err := t.sc.LocalStorage(ctx)
	if err != nil {
		return nil, xerrors.Errorf("getting local storage: %w", err)
	}

	acceptables := map[harmonytask.TaskID]bool{}

	for _, t := range ids {
		acceptables[t] = true
	}

	for _, t := range tasks {
		if _, ok := acceptables[t.TaskID]; !ok {
			continue
		}

		for _, l := range ls {
			if string(l.ID) == t.StorageID {
				return &t.TaskID, nil
			}
		}
	}

	return nil, nil
}

func (t *TreeRCTask) TypeDetails() harmonytask.TaskTypeDetails {
	ssize := abi.SectorSize(32 << 30) // todo task details needs taskID to get correct sector size
	if isDevnet {
		ssize = abi.SectorSize(2 << 20)
	}
	gpu := 1.0
	if isDevnet {
		gpu = 0
	}

	return harmonytask.TaskTypeDetails{
		Max:  t.max,
		Name: "TreeRC",
		Cost: resources.Resources{
			Cpu:     1,
			Gpu:     gpu,
			Ram:     8 << 30,
			Storage: t.sc.Storage(t.taskToSector, storiface.FTSealed, storiface.FTCache, ssize, storiface.PathSealing, paths.MinFreeStoragePercentage),
		},
		MaxFailures: 3,
		Follows:     nil,
	}
}

func (t *TreeRCTask) Adder(taskFunc harmonytask.AddTaskFunc) {
	t.sp.pollers[pollerTreeRC].Set(taskFunc)
}

func (t *TreeRCTask) taskToSector(id harmonytask.TaskID) (ffi.SectorRef, error) {
	var refs []ffi.SectorRef

	err := t.db.Select(context.Background(), &refs, `SELECT sp_id, sector_number, reg_seal_proof FROM sectors_sdr_pipeline WHERE task_id_tree_r = $1`, id)
	if err != nil {
		return ffi.SectorRef{}, xerrors.Errorf("getting sector ref: %w", err)
	}

	if len(refs) != 1 {
		return ffi.SectorRef{}, xerrors.Errorf("expected 1 sector ref, got %d", len(refs))
	}

	return refs[0], nil
}

var _ harmonytask.TaskInterface = &TreeRCTask{}
