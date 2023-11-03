package lpwindow

import (
	"context"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/lib/harmony/harmonydb"
	"github.com/filecoin-project/lotus/lib/harmony/harmonytask"
	"github.com/filecoin-project/lotus/lib/harmony/resources"
	"github.com/filecoin-project/lotus/lib/promise"
	"github.com/filecoin-project/lotus/provider/chainsched"
	"github.com/filecoin-project/lotus/provider/lpmessage"
)

type WdPostSubmitTask struct {
	sender *lpmessage.Sender
	db     *harmonydb.DB

	submitPoStTF promise.Promise[harmonytask.AddTaskFunc]
}

func (w *WdPostSubmitTask) Do(taskID harmonytask.TaskID, stillOwned func() bool) (done bool, err error) {
	//TODO implement me
	panic("implement me")
}

func (w *WdPostSubmitTask) CanAccept(ids []harmonytask.TaskID, engine *harmonytask.TaskEngine) (*harmonytask.TaskID, error) {
	//TODO implement me
	panic("implement me")
}

func NewWdPostSubmitTask(pcs *chainsched.ProviderChainSched, send *lpmessage.Sender) (*WdPostSubmitTask, error) {
	res := &WdPostSubmitTask{
		sender: send,
	}

	if err := pcs.AddHandler(res.processHeadChange); err != nil {
		return nil, err
	}

	return res, nil
}

func (w *WdPostSubmitTask) TypeDetails() harmonytask.TaskTypeDetails {
	return harmonytask.TaskTypeDetails{
		Max:  128,
		Name: "WdPostSubmit",
		Cost: resources.Resources{
			Cpu: 0,
			Gpu: 0,
			Ram: 10 << 20,
		},
		MaxFailures: 10,
		Follows:     nil, // ??
	}
}

func (w *WdPostSubmitTask) Adder(taskFunc harmonytask.AddTaskFunc) {
	w.submitPoStTF.Set(taskFunc)
}

func (w *WdPostSubmitTask) processHeadChange(ctx context.Context, revert, apply *types.TipSet) error {
	tf := w.submitPoStTF.Val(ctx)

	qry, err := w.db.Query(ctx, `select sp_id, deadline, partition, submit_at_epoch from wdpost_proofs where submit_task_id is null and submit_at_epoch <= $1`, apply.Height())
	if err != nil {
		return err
	}
	defer qry.Close()

	for qry.Next() {
		var spID int64
		var deadline uint64
		var partition uint64
		var submitAtEpoch uint64
		if err := qry.Scan(&spID, &deadline, &partition, &submitAtEpoch); err != nil {
			return err
		}

		tf(func(id harmonytask.TaskID, tx *harmonydb.Tx) (shouldCommit bool, err error) {
			// update in transaction iff submit_task_id is still null
			res, err := tx.Exec(`update wdpost_proofs set submit_task_id = $1 where sp_id = $2 and deadline = $3 and partition = $4 and submit_task_id is null`, id, spID, deadline, partition)
			if err != nil {
				return false, err
			}
			if res != 1 {
				return false, nil
			}

			return true, nil
		})
	}
	if err := qry.Err(); err != nil {
		return err
	}

	return nil
}

var _ harmonytask.TaskInterface = &WdPostSubmitTask{}
