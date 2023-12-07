package harmonytask

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"strconv"
	"sync/atomic"
	"time"

	logging "github.com/ipfs/go-log/v2"

	"github.com/filecoin-project/lotus/lib/harmony/harmonydb"
)

var log = logging.Logger("harmonytask")

type taskTypeHandler struct {
	TaskInterface
	TaskTypeDetails
	TaskEngine *TaskEngine
	Count      atomic.Int32
}

func (h *taskTypeHandler) AddTask(extra func(TaskID, *harmonydb.Tx) (bool, error)) {
	var tID TaskID
	_, err := h.TaskEngine.db.BeginTransaction(h.TaskEngine.ctx, func(tx *harmonydb.Tx) (bool, error) {
		// create taskID (from DB)
		_, err := tx.Exec(`INSERT INTO harmony_task (name, added_by, posted_time) 
			VALUES ($1, $2, CURRENT_TIMESTAMP) `, h.Name, h.TaskEngine.ownerID)
		if err != nil {
			return false, fmt.Errorf("could not insert into harmonyTask: %w", err)
		}
		err = tx.QueryRow("SELECT id FROM harmony_task ORDER BY update_time DESC LIMIT 1").Scan(&tID)
		if err != nil {
			return false, fmt.Errorf("Could not select ID: %v", err)
		}
		return extra(tID, tx)
	})

	if err != nil {
		if harmonydb.IsErrUniqueContraint(err) {
			log.Debugf("addtask(%s) saw unique constraint, so it's added already.", h.Name)
			return
		}
		log.Error("Could not add task. AddTasFunc failed: %v", err)
		return
	}
}

const (
	workSourcePoller  = "poller"
	workSourceRecover = "recovered"
)

// considerWork is called to attempt to start work on a task-id of this task type.
// It presumes single-threaded calling, so there should not be a multi-threaded re-entry.
// The only caller should be the one work poller thread. This does spin off other threads,
// but those should not considerWork. Work completing may lower the resource numbers
// unexpectedly, but that will not invalidate work being already able to fit.
func (h *taskTypeHandler) considerWork(from string, ids []TaskID) (workAccepted bool) {
top:
	if len(ids) == 0 {
		return true // stop looking for takers
	}

	// 1. Can we do any more of this task type?
	// NOTE: 0 is the default value, so this way people don't need to worry about
	// this setting unless they want to limit the number of tasks of this type.
	if h.Max > 0 && int(h.Count.Load()) >= h.Max {
		log.Debugw("did not accept task", "name", h.Name, "reason", "at max already")
		return false
	}

	// 2. Can we do any more work? From here onward, we presume the resource
	// story will not change, so single-threaded calling is best.
	err := h.AssertMachineHasCapacity()
	if err != nil {
		log.Debugw("did not accept task", "name", h.Name, "reason", "at capacity already: "+err.Error())
		return false
	}

	// 3. What does the impl say?
	tID, err := h.CanAccept(ids, h.TaskEngine)
	if err != nil {
		log.Error(err)
		return false
	}
	if tID == nil {
		log.Infow("did not accept task", "task_id", ids[0], "reason", "CanAccept() refused", "name", h.Name)
		return false
	}

	// if recovering we don't need to try to claim anything because those tasks are already claimed by us
	if from != workSourceRecover {
		// 4. Can we claim the work for our hostname?
		ct, err := h.TaskEngine.db.Exec(h.TaskEngine.ctx, "UPDATE harmony_task SET owner_id=$1 WHERE id=$2 AND owner_id IS NULL", h.TaskEngine.ownerID, *tID)
		if err != nil {
			log.Error(err)
			return false
		}
		if ct == 0 {
			log.Infow("did not accept task", "task_id", strconv.Itoa(int(*tID)), "reason", "already Taken", "name", h.Name)
			var tryAgain = make([]TaskID, 0, len(ids)-1)
			for _, id := range ids {
				if id != *tID {
					tryAgain = append(tryAgain, id)
				}
			}
			ids = tryAgain
			goto top
		}
	}

	h.Count.Add(1)
	go func() {
		log.Infow("Beginning work on Task", "id", *tID, "from", from, "name", h.Name)

		var done bool
		var doErr error
		workStart := time.Now()

		defer func() {
			if r := recover(); r != nil {
				stackSlice := make([]byte, 4092)
				sz := runtime.Stack(stackSlice, false)
				log.Error("Recovered from a serious error "+
					"while processing "+h.Name+" task "+strconv.Itoa(int(*tID))+": ", r,
					" Stack: ", string(stackSlice[:sz]))
			}
			h.Count.Add(-1)

			h.recordCompletion(*tID, workStart, done, doErr)
			if done {
				for _, fs := range h.TaskEngine.follows[h.Name] { // Do we know of any follows for this task type?
					if _, err := fs.f(*tID, fs.h.AddTask); err != nil {
						log.Error("Could not follow", "error", err, "from", h.Name, "to", fs.name)
					}
				}
			}
		}()

		done, doErr = h.Do(*tID, func() bool {
			var owner int
			// Background here because we don't want GracefulRestart to block this save.
			err := h.TaskEngine.db.QueryRow(context.Background(),
				`SELECT owner_id FROM harmony_task WHERE id=$1`, *tID).Scan(&owner)
			if err != nil {
				log.Error("Cannot determine ownership: ", err)
				return false
			}
			return owner == h.TaskEngine.ownerID
		})
		if doErr != nil {
			log.Errorw("Do() returned error", "type", h.Name, "id", strconv.Itoa(int(*tID)), "error", doErr)
		}
	}()
	return true
}

func (h *taskTypeHandler) recordCompletion(tID TaskID, workStart time.Time, done bool, doErr error) {
	workEnd := time.Now()

	cm, err := h.TaskEngine.db.BeginTransaction(h.TaskEngine.ctx, func(tx *harmonydb.Tx) (bool, error) {
		var postedTime time.Time
		err := tx.QueryRow(`SELECT posted_time FROM harmony_task WHERE id=$1`, tID).Scan(&postedTime)
		if err != nil {
			return false, fmt.Errorf("could not log completion: %w ", err)
		}
		result := "unspecified error"
		if done {
			_, err = tx.Exec("DELETE FROM harmony_task WHERE id=$1", tID)
			if err != nil {

				return false, fmt.Errorf("could not log completion: %w", err)
			}
			result = ""
		} else {
			if doErr != nil {
				result = "error: " + doErr.Error()
			}
			var deleteTask bool
			if h.MaxFailures > 0 {
				ct := uint(0)
				err = tx.QueryRow(`SELECT count(*) FROM harmony_task_history 
				WHERE task_id=$1 AND result=FALSE`, tID).Scan(&ct)
				if err != nil {
					return false, fmt.Errorf("could not read task history: %w", err)
				}
				if ct >= h.MaxFailures {
					deleteTask = true
				}
			}
			if deleteTask {
				_, err = tx.Exec("DELETE FROM harmony_task WHERE id=$1", tID)
				if err != nil {
					return false, fmt.Errorf("could not delete failed job: %w", err)
				}
				// Note: Extra Info is left laying around for later review & clean-up
			} else {
				_, err := tx.Exec(`UPDATE harmony_task SET owner_id=NULL WHERE id=$1`, tID)
				if err != nil {
					return false, fmt.Errorf("could not disown failed task: %v %v", tID, err)
				}
			}
		}
		_, err = tx.Exec(`INSERT INTO harmony_task_history 
									 (task_id,   name, posted,    work_start, work_end, result, completed_by_host_and_port,      err)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`, tID, h.Name, postedTime, workStart, workEnd, done, h.TaskEngine.hostAndPort, result)
		if err != nil {
			return false, fmt.Errorf("could not write history: %w", err)
		}
		return true, nil
	})
	if err != nil {
		log.Error("Could not record transaction: ", err)
		return
	}
	if !cm {
		log.Error("Committing the task records failed")
	}
}

func (h *taskTypeHandler) AssertMachineHasCapacity() error {
	r := h.TaskEngine.ResourcesAvailable()

	if r.Cpu-h.Cost.Cpu < 0 {
		return errors.New("Did not accept " + h.Name + " task: out of cpu")
	}
	if h.Cost.Ram > r.Ram {
		return errors.New("Did not accept " + h.Name + " task: out of RAM")
	}
	if r.Gpu-h.Cost.Gpu < 0 {
		return errors.New("Did not accept " + h.Name + " task: out of available GPU")
	}
	return nil
}
