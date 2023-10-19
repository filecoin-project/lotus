package wdpost

import (
	"bytes"
	"context"
	"sort"
	"time"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/dline"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/lib/harmony/harmonydb"
	"github.com/filecoin-project/lotus/lib/harmony/harmonytask"
	"github.com/filecoin-project/lotus/lib/harmony/resources"
	"github.com/filecoin-project/lotus/lib/harmony/taskhelp"
	"github.com/filecoin-project/lotus/storage/sealer/sealtasks"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
	"github.com/samber/lo"
	cbg "github.com/whyrusleeping/cbor-gen"
)

type WdPostTaskDetails struct {
	Ts       *types.TipSet
	Deadline *dline.Info
}

type WdPostTask struct {
	tasks     chan *WdPostTaskDetails
	db        *harmonydb.DB
	Scheduler *WindowPoStScheduler
	max       int
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
	if err != nil {
		log.Errorf("WdPostTask.Do() failed to get tipset key: %v", err)
		return false, err
	}
	head, err := t.Scheduler.api.ChainHead(context.Background())
	if err != nil {
		log.Errorf("WdPostTask.Do() failed to get chain head: %v", err)
		return false, err
	}

	if deadline.Close > head.Height() {
		log.Errorf("WdPost removed stale task: %v %v", taskID, tsKey)
		return true, nil
	}

	ts, err := t.Scheduler.api.ChainGetTipSet(context.Background(), tsKey)
	if err != nil {
		log.Errorf("WdPostTask.Do() failed to get tipset: %v", err)
		return false, err
	}
	submitWdPostParams, err := t.Scheduler.runPoStCycle(context.Background(), false, deadline, ts)
	if err != nil {
		log.Errorf("WdPostTask.Do() failed to runPoStCycle: %v", err)
		return false, err
	}

	log.Errorf("WdPostTask.Do() called with taskID: %v, submitWdPostParams: %v", taskID, submitWdPostParams)

	// Enter an entry for each wdpost message proof into the wdpost_proofs table
	for _, params := range submitWdPostParams {

		// Convert submitWdPostParams.Partitions to a byte array using CBOR
		buf := new(bytes.Buffer)
		scratch := make([]byte, 9)
		if err := cbg.WriteMajorTypeHeaderBuf(scratch, buf, cbg.MajArray, uint64(len(params.Partitions))); err != nil {
			return false, err
		}
		for _, v := range params.Partitions {
			if err := v.MarshalCBOR(buf); err != nil {
				return false, err
			}
		}

		// Insert into wdpost_proofs table
		_, err = t.db.Exec(context.Background(),
			`INSERT INTO wdpost_proofs (
                           deadline,
                           partitions,
                           proof_type,
                           proof_bytes)
    			 VALUES ($1, $2, $3, $4)`,
			params.Deadline,
			buf.Bytes(),
			params.Proofs[0].PoStProof,
			params.Proofs[0].ProofBytes)
	}

	return true, nil
}

func (t *WdPostTask) CanAccept(ids []harmonytask.TaskID, te *harmonytask.TaskEngine) (*harmonytask.TaskID, error) {
	// GetEpoch
	ts, err := t.Scheduler.api.ChainHead(context.Background())

	if err != nil {
		return nil, err
	}

	// GetData for tasks
	type wdTaskDef struct {
		abi.RegisteredSealProof
		Task_id harmonytask.TaskID
		Tskey   []byte
		Open    abi.ChainEpoch
		Close   abi.ChainEpoch
	}
	var tasks []wdTaskDef
	err = t.db.Select(context.Background(), tasks,
		`Select tskey, 
			task_id,
			period_start,
			open, 
			close	
	from wdpost_tasks 
	where task_id IN $1`, ids)
	if err != nil {
		return nil, err
	}

	// Accept those past deadline, then delete them in Do().
	for _, task := range tasks {
		if task.Close < ts.Height() {
			return &task.Task_id, nil
		}
	}

	// Discard those too big for our free RAM
	freeRAM := te.ResourcesAvailable().Ram
	tasks = lo.Filter(tasks, func(d wdTaskDef, _ int) bool {
		return res[d.RegisteredSealProof].MaxMemory <= freeRAM
	})
	if len(tasks) == 0 {
		log.Infof("RAM too small for any WDPost task")
		return nil, nil
	}

	// Ignore those with too many failures unless they are the only ones left.
	tasks, _ = taskhelp.SliceIfFound(tasks, func(d wdTaskDef) bool {
		var r int
		err := t.db.QueryRow(context.Background(), `SELECT COUNT(*) 
		FROM harmony_task_history 
		WHERE task_id = $1 AND success = false`, d.Task_id).Scan(&r)
		if err != nil {
			log.Errorf("WdPostTask.CanAccept() failed to queryRow: %v", err)
		}
		return r < 2
	})

	// Select the one closest to the deadline
	sort.Slice(tasks, func(i, j int) bool {
		return tasks[i].Close < tasks[j].Close
	})

	return &tasks[0].Task_id, nil
}

var res = storiface.ResourceTable[sealtasks.TTGenerateWindowPoSt]

func (t *WdPostTask) TypeDetails() harmonytask.TaskTypeDetails {
	return harmonytask.TaskTypeDetails{
		Name:        "WdPost",
		Max:         t.max,
		MaxFailures: 3,
		Follows:     nil,
		Cost: resources.Resources{
			Cpu: 1,
			Gpu: 1,
			// RAM of smallest proof's max is listed here
			Ram: lo.Reduce(lo.Keys(res), func(i uint64, k abi.RegisteredSealProof, _ int) uint64 {
				if res[k].MaxMemory < i {
					return res[k].MaxMemory
				}
				return i
			}, 1<<63),
		},
	}
}

func (t *WdPostTask) Adder(taskFunc harmonytask.AddTaskFunc) {

	// wait for any channels on t.tasks and call taskFunc on them
	for taskDetails := range t.tasks {

		//log.Errorf("WdPostTask.Adder() received taskDetails: %v", taskDetails)

		taskFunc(func(tID harmonytask.TaskID, tx *harmonydb.Tx) (bool, error) {
			return t.addTaskToDB(taskDetails.Ts, taskDetails.Deadline, tID, tx)
		})
	}
}

func NewWdPostTask(db *harmonydb.DB, scheduler *WindowPoStScheduler, max int) *WdPostTask {
	return &WdPostTask{
		tasks:     make(chan *WdPostTaskDetails, 2),
		db:        db,
		Scheduler: scheduler,
		max:       max,
	}
}

func (t *WdPostTask) AddTask(ctx context.Context, ts *types.TipSet, deadline *dline.Info) error {

	t.tasks <- &WdPostTaskDetails{
		Ts:       ts,
		Deadline: deadline,
	}

	//log.Errorf("WdPostTask.AddTask() called with ts: %v, deadline: %v, taskList: %v", ts, deadline, t.tasks)

	return nil
}

func (t *WdPostTask) addTaskToDB(ts *types.TipSet, deadline *dline.Info, taskId harmonytask.TaskID, tx *harmonydb.Tx) (bool, error) {

	tsKey := ts.Key()

	//log.Errorf("WdPostTask.addTaskToDB() called with tsKey: %v, taskId: %v", tsKey, taskId)

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
