package wdpost

import (
	"context"
	"github.com/filecoin-project/go-state-types/dline"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/lib/harmony/harmonydb"
	"github.com/filecoin-project/lotus/lib/harmony/harmonytask"
	"time"
)

type WdPostTaskDetails struct {
	Ts       *types.TipSet
	Deadline *dline.Info
}

type WdPostTask struct {
	tasks     chan *WdPostTaskDetails
	db        *harmonydb.DB
	scheduler *WindowPoStScheduler
}

func (t *WdPostTask) Do(taskID harmonytask.TaskID, stillOwned func() bool) (done bool, err error) {

	time.Sleep(5 * time.Second)
	log.Errorf("WdPostTask.Do() called with taskID: %v", taskID)

	var tsKeyBytes []byte
	var deadline dline.Info

	err = t.db.QueryRow(context.Background(),
		`Select tskey, 
       				current_epoch, 
       				period_start, 
       				index, 
    				open, 
    				close, 
    				challenge, 
    				fault_cutoff, 
    				wpost_period_deadlines, 
    				wpost_proving_period, 
    				wpost_challenge_window, 
    				wpost_challenge_lookback, 
    				fault_declaration_cutoff 
			from wdpost_tasks 
			where task_id = $1`, taskID).Scan(
		&tsKeyBytes,
		&deadline.CurrentEpoch,
		&deadline.PeriodStart,
		&deadline.Index,
		&deadline.Open,
		&deadline.Close,
		&deadline.Challenge,
		&deadline.FaultCutoff,
		&deadline.WPoStPeriodDeadlines,
		&deadline.WPoStProvingPeriod,
		&deadline.WPoStChallengeWindow,
		&deadline.WPoStChallengeLookback,
		&deadline.FaultDeclarationCutoff,
	)
	if err != nil {
		log.Errorf("WdPostTask.Do() failed to queryRow: %v", err)
		return false, err
	}

	log.Errorf("tskEY: %v", tsKeyBytes)
	tsKey, err := types.TipSetKeyFromBytes(tsKeyBytes)
	ts, err := t.scheduler.api.ChainGetTipSet(context.Background(), tsKey)
	if err != nil {
		log.Errorf("WdPostTask.Do() failed to get tipset: %v", err)
		return false, err
	}
	submitWdPostParams, err := t.scheduler.runPoStCycle(context.Background(), false, deadline, ts)
	if err != nil {
		log.Errorf("WdPostTask.Do() failed to runPoStCycle: %v", err)
		return false, err
	}

	log.Errorf("WdPostTask.Do() called with taskID: %v, submitWdPostParams: %v", taskID, submitWdPostParams)

	return true, nil
}

func (t *WdPostTask) CanAccept(ids []harmonytask.TaskID) (*harmonytask.TaskID, error) {
	return &ids[0], nil
}

func (t *WdPostTask) TypeDetails() harmonytask.TaskTypeDetails {
	return harmonytask.TaskTypeDetails{
		Name: "WdPostCompute",
		Max:  -1,
	}
}

func (t *WdPostTask) Adder(taskFunc harmonytask.AddTaskFunc) {

	log.Errorf("WdPostTask.Adder() called -----------------------------	")

	// wait for any channels on t.tasks and call taskFunc on them
	for taskDetails := range t.tasks {

		log.Errorf("WdPostTask.Adder() received taskDetails: %v", taskDetails)

		taskFunc(func(tID harmonytask.TaskID, tx *harmonydb.Tx) (bool, error) {
			return t.addTaskToDB(taskDetails.Ts, taskDetails.Deadline, tID, tx)
		})
	}
}

func NewWdPostTask(db *harmonydb.DB, scheduler *WindowPoStScheduler) *WdPostTask {
	return &WdPostTask{
		tasks:     make(chan *WdPostTaskDetails, 2),
		db:        db,
		scheduler: scheduler,
	}
}

func (t *WdPostTask) AddTask(ctx context.Context, ts *types.TipSet, deadline *dline.Info) error {

	t.tasks <- &WdPostTaskDetails{
		Ts:       ts,
		Deadline: deadline,
	}

	log.Errorf("WdPostTask.AddTask() called with ts: %v, deadline: %v, taskList: %v", ts, deadline, t.tasks)

	return nil
}

func (t *WdPostTask) addTaskToDB(ts *types.TipSet, deadline *dline.Info, taskId harmonytask.TaskID, tx *harmonydb.Tx) (bool, error) {

	tsKey := ts.Key()

	log.Errorf("WdPostTask.addTaskToDB() called with tsKey: %v, taskId: %v", tsKey, taskId)

	_, err := tx.Exec(
		`INSERT INTO wdpost_tasks (
                         task_id,
                         tskey, 
                         current_epoch,
                         period_start,
                         index,
                         open,
                         close,
                         challenge,
                         fault_cutoff,
                         wpost_period_deadlines,
                         wpost_proving_period,
                         wpost_challenge_window,
                         wpost_challenge_lookback,
                         fault_declaration_cutoff
                        ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10 , $11, $12, $13, $14)`,
		taskId,
		tsKey.Bytes(),
		deadline.CurrentEpoch,
		deadline.PeriodStart,
		deadline.Index,
		deadline.Open,
		deadline.Close,
		deadline.Challenge,
		deadline.FaultCutoff,
		deadline.WPoStPeriodDeadlines,
		deadline.WPoStProvingPeriod,
		deadline.WPoStChallengeWindow,
		deadline.WPoStChallengeLookback,
		deadline.FaultDeclarationCutoff,
	)
	if err != nil {
		return false, err
	}

	return true, nil
}

func (t *WdPostTask) AddTaskOld(ctx context.Context, ts *types.TipSet, deadline *dline.Info, taskId harmonytask.TaskID) error {

	tsKey := ts.Key()
	_, err := t.db.Exec(ctx,
		`INSERT INTO wdpost_tasks (
                         task_id,
                         tskey, 
                         current_epoch,
                         period_start,
                         index,
                         open,
                         close,
                         challenge,
                         fault_cutoff,
                         wpost_period_deadlines,
                         wpost_proving_period,
                         wpost_challenge_window,
                         wpost_challenge_lookback,
                         fault_declaration_cutoff
                        ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10 , $11, $12, $13, $14)`,
		taskId,
		tsKey.Bytes(),
		deadline.CurrentEpoch,
		deadline.PeriodStart,
		deadline.Index,
		deadline.Open,
		deadline.Close,
		deadline.Challenge,
		deadline.FaultCutoff,
		deadline.WPoStPeriodDeadlines,
		deadline.WPoStProvingPeriod,
		deadline.WPoStChallengeWindow,
		deadline.WPoStChallengeLookback,
		deadline.FaultDeclarationCutoff,
	)
	if err != nil {
		return err
	}

	return nil
}

var _ harmonytask.TaskInterface = &WdPostTask{}
