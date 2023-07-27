package harmonytask

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/filecoin-project/lotus/lib/harmony/harmonydb"
	logging "github.com/ipfs/go-log/v2"
)

var logger = logging.Logger("harmonytask")

type taskTypeHandler struct {
	TaskInterface
	TaskTypeDetails
	TaskEngine *TaskEngine
	Count      int /// locked by TaskEngine's mutex

	LastCleanup atomic.Value
}

func (h *taskTypeHandler) AddTask(extra func(TaskID, *harmonydb.Tx) bool) {
	var tID TaskID
	did, err := h.TaskEngine.db.BeginTransaction(h.TaskEngine.ctx, func(tx *harmonydb.Tx) bool {
		// create taskID (from DB)
		_, err := tx.Exec(`INSERT INTO harmony_task (name, added_by, posted_time) 
			VALUES ($1, $2, CURRENT_TIMESTAMP) `, h.Name, h.TaskEngine.ownerID)
		if err != nil {
			logger.Error("Could not insert into harmonyTask", err)
			return false
		}
		err = tx.QueryRow("SELECT id FROM harmony_task ORDER BY update_time DESC LIMIT 1").Scan(&tID)
		if err != nil {
			logger.Error("Could not select ID: ", err)
		}
		return extra(tID, tx)
	})
	if err != nil {
		logger.Error(err)
	}
	if !did {
		return
	}

	if !h.considerWork([]TaskID{tID}) {
		h.TaskEngine.bump(h.Name) // We can't do it. How about someone else.
	}
}

func (h *taskTypeHandler) considerWork(ids []TaskID) (workAccepted bool) {
	if len(ids) == 0 {
		return true // stop looking for takers
	}

	h.TaskEngine.workAdderMutex.Lock()
	defer h.TaskEngine.workAdderMutex.Unlock()

	// 1. Can we do any more of this task type?
	if h.Max > -1 && h.Count == h.Max {
		logger.Info("Did not accept " + h.Name + " task: at max already.")
		return false
	}

	// 2. Can we do any more work?
	err := h.AssertMachineHasCapacity()
	if err != nil {
		logger.Info(err)
		return false
	}

	// 3. What does the impl say?
	tID, err := h.CanAccept(ids)
	if err != nil {
		logger.Error(err)
		return false
	}
	if tID == nil {
		logger.Info("Did not accept task " + strconv.Itoa(int(*tID)) + ": CanAccept() refused")
		return false
	}

	// 4. Can we claim the work for our hostname?
	ct, err := h.TaskEngine.db.Exec(h.TaskEngine.ctx, "UPDATE harmony_task SET owner_id=$1 WHERE id=$2 AND owner_id IS NULL", h.TaskEngine.ownerID, *tID)
	if err != nil {
		logger.Error(err)
		return false
	}
	if ct == 0 {
		logger.Info("Did not accept task " + strconv.Itoa(int(*tID)) + ": Already Taken")
		return false
	}

	go func() {
		h.TaskEngine.workAdderMutex.Lock()
		h.Count++
		h.TaskEngine.workAdderMutex.Unlock()

		var done bool
		var doErr error
		workStart := time.Now()

		defer func() {
			if r := recover(); r != nil {
				logger.Error("Recovered from a serious error "+
					"while processing "+h.Name+" task "+strconv.Itoa(int(*tID))+": ", r)
			}
			h.TaskEngine.workAdderMutex.Lock()
			h.Count--
			h.TaskEngine.workAdderMutex.Unlock()

			h.recordCompletion(*tID, workStart, done, doErr)
			if done {
				h.triggerCompletionListeners(*tID)
			}

			h.TaskEngine.tryAllWork <- true // Activate tasks in this machine
		}()

		done, doErr = h.Do(*tID, func() bool {
			var owner int
			// Background here because we don't want GracefulRestart to block this save.
			err := h.TaskEngine.db.QueryRow(context.Background(),
				`SELECT owner_id FROM harmony_task WHERE id=$1`, *tID).Scan(&owner)
			if err != nil {
				logger.Error("Cannot determine ownership: ", err)
				return false
			}
			return owner == h.TaskEngine.ownerID
		})
		if doErr != nil {
			logger.Error("Do("+h.Name+", taskID="+strconv.Itoa(int(*tID))+") returned error: ", doErr)
		}
	}()
	return true
}

func (h *taskTypeHandler) recordCompletion(tID TaskID, workStart time.Time, done bool, doErr error) {
	workEnd := time.Now()

	cm, err := h.TaskEngine.db.BeginTransaction(h.TaskEngine.ctx, func(tx *harmonydb.Tx) bool {
		var postedTime time.Time
		err := tx.QueryRow(`SELECT posted_time FROM harmony_task WHERE id=$1`, tID).Scan(&postedTime)
		if err != nil {
			logger.Error("Could not log completion: ", err)
			return false
		}
		result := "unspecified error"
		if done {
			_, err = tx.Exec("DELETE FROM harmony_task WHERE id=$1", tID)
			if err != nil {
				logger.Error("Could not log completion: ", err)
				return false
			}
			result = ""
		} else {
			if doErr != nil {
				result = "error: " + doErr.Error()
			}
			if h.MaxFailures > 0 {
				ct := uint(0)
				err = tx.QueryRow(`SELECT count(*) FROM harmony_task_history 
				WHERE task_id=$1 AND result=FALSE`, tID).Scan(&ct)
				if err != nil {
					logger.Error("Could not read task history:", err)
					return false
				}
				if ct > h.MaxFailures {
					_, err = tx.Exec("DELETE FROM harmony_task WHERE id=$1", tID)
					if err != nil {
						logger.Error("Could not delete failed job: ", err)
						return false
					}
					// Note: Extra Info is left laying around for later review & clean-up
				}
			}
		}
		_, err = tx.Exec(`INSERT INTO harmony_task_history 
												(task_id, name, posted,    work_start, work_end, result, err)
			 VALUES ($1, $2, $3, $4, $5, $6, $7)`, tID, h.Name, postedTime, workStart, workEnd, done, result)
		if err != nil {
			logger.Error("Could not write history: ", err)
			return false
		}
		return true
	})
	if err != nil {
		logger.Error("Could not record transaction: ", err)
		return
	}
	if !cm {
		logger.Error("Committing the task records failed")
	}
}

func (h *taskTypeHandler) AssertMachineHasCapacity() error {
	r := h.TaskEngine.resourcesInUse()

	if r.Cpu-h.Cost.Cpu < 0 {
		return errors.New("Did not accept " + h.Name + " task: out of cpu")
	}
	if h.Cost.Ram > r.Ram {
		return errors.New("Did not accept " + h.Name + " task: out of RAM")
	}
	if r.Gpu-h.Cost.Gpu < 0 {
		return errors.New("Did not accept " + h.Name + " task: out of available GPU")
	}
	if h.Cost.Scratch > r.Scratch {
		return errors.New("Did not accept " + h.Name + " task: out of scratch space")
	}
	return nil
}

var hClient = http.Client{}

func init() {
	hClient.Timeout = 3 * time.Second
}

// triggerCompletionListeners does in order:
// 1. Trigger all in-process followers (b/c it's fast).
// 2. Trigger all living processes with followers via DB
// 3. Future followers (think partial upgrade) can read harmony_task_history
// 3a. The Listen() handles slow follows.
func (h *taskTypeHandler) triggerCompletionListeners(tID TaskID) {
	// InProcess (#1 from Description)
	inProcessDefs := h.TaskEngine.follows[h.Name]
	inProcessFollowers := make([]string, len(inProcessDefs))
	for _, fs := range inProcessDefs {
		if fs.f(tID, fs.h.AddTask) {
			inProcessFollowers = append(inProcessFollowers, fs.h.Name)
		}
	}

	// Over HTTP (#2 from Description)
	var hps []struct {
		hostAndPort string
		toType      string
	}
	err := h.TaskEngine.db.Select(h.TaskEngine.ctx, &hps, `SELECT host_and_port, to_type
		FROM harmony_task_follow WHERE from_type=$1 AND to_type NOT IN $2 AND host_and_port != $3`,
		h.Name, inProcessFollowers, h.TaskEngine.hostAndPort)
	if err != nil {
		logger.Warn("Could not fast-trigger partner processes.", err)
		return
	}
	hostsVisited := map[string]bool{}
	tasksVisited := map[string]bool{}
	for _, v := range hps {
		if hostsVisited[v.hostAndPort] || tasksVisited[v.toType] {
			continue
		}
		resp, err := hClient.Get(v.hostAndPort + "/scheduler/follows/" + h.Name)
		if err != nil {
			logger.Warn("Couldn't hit http endpoint: ", err)
			h.TaskEngine.cleanupDepartedMachines()
			continue
		}
		b, err := io.ReadAll(resp.Body)
		if err != nil {
			logger.Warn("Couldn't hit http endpoint: ", err)
			continue
		}
		hostsVisited[v.hostAndPort], tasksVisited[v.toType] = true, true
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
			logger.Error("IO failed for fast nudge: ", string(b))
			continue
		}
	}
}
