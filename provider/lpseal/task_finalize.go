package lpseal

import (
	"context"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/lib/harmony/harmonydb"
	"github.com/filecoin-project/lotus/lib/harmony/harmonytask"
	"github.com/filecoin-project/lotus/lib/harmony/resources"
	"github.com/filecoin-project/lotus/provider/lpffi"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
	"golang.org/x/xerrors"
)

type FinalizeTask struct {
	max int
	sp  *SealPoller
	sc  *lpffi.SealCalls
	db  *harmonydb.DB
}

func NewFinalizeTask(max int, sp *SealPoller, sc *lpffi.SealCalls, db *harmonydb.DB) *FinalizeTask {
	return &FinalizeTask{
		max: max,
		sp:  sp,
		sc:  sc,
		db:  db,
	}
}

func (f *FinalizeTask) Do(taskID harmonytask.TaskID, stillOwned func() bool) (done bool, err error) {
	var task struct {
		SpID         int64 `db:"sp_id"`
		SectorNumber int64 `db:"sector_number"`
		RegSealProof int64 `db:"reg_seal_proof"`
	}

	ctx := context.Background()

	err = f.db.Select(ctx, &task, `
		select sp_id, sector_number, reg_seal_proof from sectors_sdr_pipeline where task_id_finalize=$1`, taskID)
	if err != nil {
		return false, xerrors.Errorf("getting task: %w", err)
	}

	sector := storiface.SectorRef{
		ID: abi.SectorID{
			Miner:  abi.ActorID(task.SpID),
			Number: abi.SectorNumber(task.SectorNumber),
		},
		ProofType: abi.RegisteredSealProof(task.RegSealProof),
	}

	err = f.sc.FinalizeSector(ctx, sector)
	if err != nil {
		return false, xerrors.Errorf("finalizing sector: %w", err)
	}

	// set after_finalize
	_, err = f.db.Exec(ctx, `update sectors_sdr_pipeline set after_finalize=true where task_id_finalize=$1`, taskID)
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

	if 4 != storiface.FTCache {
		panic("storiface.FTCache != 4")
	}

	ctx := context.Background()

	err := f.db.Select(ctx, &tasks, `
		select p.task_id_finalize, p.sp_id, p.sector_number, l.storage_id from sectors_sdr_pipeline p
		    inner join sector_location l on p.sp_id=l.miner_id and p.sector_number=l.sector_num
		    where task_id_finalize in ($1) and l.sector_filetype=4`, ids)
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
