package seal

import (
	"context"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/curiosrc/ffi"
	"github.com/filecoin-project/lotus/lib/harmony/harmonydb"
	"github.com/filecoin-project/lotus/lib/harmony/harmonytask"
	"github.com/filecoin-project/lotus/lib/harmony/resources"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
)

type FinalizeTask struct {
	max int
	sp  *SealPoller
	sc  *ffi.SealCalls
	db  *harmonydb.DB
}

func NewFinalizeTask(max int, sp *SealPoller, sc *ffi.SealCalls, db *harmonydb.DB) *FinalizeTask {
	return &FinalizeTask{
		max: max,
		sp:  sp,
		sc:  sc,
		db:  db,
	}
}

func (f *FinalizeTask) Do(taskID harmonytask.TaskID, stillOwned func() bool) (done bool, err error) {
	var tasks []struct {
		SpID         int64 `db:"sp_id"`
		SectorNumber int64 `db:"sector_number"`
		RegSealProof int64 `db:"reg_seal_proof"`
	}

	ctx := context.Background()

	err = f.db.Select(ctx, &tasks, `
		SELECT sp_id, sector_number, reg_seal_proof FROM sectors_sdr_pipeline WHERE task_id_finalize = $1`, taskID)
	if err != nil {
		return false, xerrors.Errorf("getting task: %w", err)
	}

	if len(tasks) != 1 {
		return false, xerrors.Errorf("expected one task")
	}
	task := tasks[0]

	var keepUnsealed bool

	if err := f.db.QueryRow(ctx, `SELECT COALESCE(BOOL_OR(NOT data_delete_on_finalize), FALSE) FROM sectors_sdr_initial_pieces WHERE sp_id = $1 AND sector_number = $2`, task.SpID, task.SectorNumber).Scan(&keepUnsealed); err != nil {
		return false, err
	}

	sector := storiface.SectorRef{
		ID: abi.SectorID{
			Miner:  abi.ActorID(task.SpID),
			Number: abi.SectorNumber(task.SectorNumber),
		},
		ProofType: abi.RegisteredSealProof(task.RegSealProof),
	}

	err = f.sc.FinalizeSector(ctx, sector, keepUnsealed)
	if err != nil {
		return false, xerrors.Errorf("finalizing sector: %w", err)
	}

	if err := DropSectorPieceRefs(ctx, f.db, sector.ID); err != nil {
		return false, xerrors.Errorf("dropping sector piece refs: %w", err)
	}

	// set after_finalize
	_, err = f.db.Exec(ctx, `UPDATE sectors_sdr_pipeline SET after_finalize = TRUE, task_id_finalize = NULL WHERE task_id_finalize = $1`, taskID)
	if err != nil {
		return false, xerrors.Errorf("updating task: %w", err)
	}

	return true, nil
}

func (f *FinalizeTask) CanAccept(ids []harmonytask.TaskID, engine *harmonytask.TaskEngine) (*harmonytask.TaskID, error) {
	var tasks []struct {
		TaskID       harmonytask.TaskID `db:"task_id_finalize"`
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

	err := f.db.Select(ctx, &tasks, `
		SELECT p.task_id_finalize, p.sp_id, p.sector_number, l.storage_id FROM sectors_sdr_pipeline p
			INNER JOIN sector_location l ON p.sp_id = l.miner_id AND p.sector_number = l.sector_num
			WHERE task_id_finalize = ANY ($1) AND l.sector_filetype = 4
`, indIDs)
	if err != nil {
		return nil, xerrors.Errorf("getting tasks: %w", err)
	}

	ls, err := f.sc.LocalStorage(ctx)
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

func (f *FinalizeTask) TypeDetails() harmonytask.TaskTypeDetails {
	return harmonytask.TaskTypeDetails{
		Max:  f.max,
		Name: "Finalize",
		Cost: resources.Resources{
			Cpu: 1,
			Gpu: 0,
			Ram: 100 << 20,
		},
		MaxFailures: 10,
	}
}

func (f *FinalizeTask) Adder(taskFunc harmonytask.AddTaskFunc) {
	f.sp.pollers[pollerFinalize].Set(taskFunc)
}

var _ harmonytask.TaskInterface = &FinalizeTask{}
