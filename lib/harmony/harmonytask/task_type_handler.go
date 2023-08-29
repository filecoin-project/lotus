package harmonytask

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync/atomic"
	"time"

	logging "github.com/ipfs/go-log/v2"
	"github.com/samber/lo"

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
	did, err := h.TaskEngine.db.BeginTransaction(h.TaskEngine.ctx, func(tx *harmonydb.Tx) (bool, error) {
		// create taskID (from DB)
		_, err := tx.Exec(`INSERT INTO harmony_task (name, added_by, posted_time) 
			VALUES ($1, $2, CURRENT_TIMESTAMP) `, h.Name, h.TaskEngine.ownerID)
		if err != nil {
			return false, fmt.Errorf("Could not insert into harmonyTask: %w", err)
		}
		err = tx.QueryRow("SELECT id FROM harmony_task ORDER BY update_time DESC LIMIT 1").Scan(&tID)
		if err != nil {
			log.Error("Could not select ID: ", err)
		}
		return extra(tID, tx)
	})
	if err != nil {
		log.Error(err)
	}
	if !did {
		return
	}

	if !h.considerWork("adder", []TaskID{tID}) {
		h.TaskEngine.bump(h.Name) // We can't do it. How about someone else.
	}
}

func (h *taskTypeHandler) considerWork(from string, ids []TaskID) (workAccepted bool) {
top:
	if len(ids) == 0 {
		return true // stop looking for takers
	}

	// 1. Can we do any more of this task type?
	if h.Max > -1 && int(h.Count.Load()) == h.Max {
		log.Infow("did not accept task", "name", h.Name, "reason", "at max already")
		return false
	}

	h.TaskEngine.workAdderMutex.Lock()
	defer h.TaskEngine.workAdderMutex.Unlock()

	// 2. Can we do any more work?
	err := h.AssertMachineHasCapacity()
	if err != nil {
		log.Info(err)
		return false
	}

	// 3. What does the impl say?
	tID, err := h.CanAccept(ids)
	if err != nil {
		log.Error(err)
		return false
	}
	if tID == nil {
		log.Infow("did not accept task", "task_id", ids[0], "reason", "CanAccept() refused")
		return false
	}

	// 4. Can we claim the work for our hostname?
	ct, err := h.TaskEngine.db.Exec(h.TaskEngine.ctx, "UPDATE harmony_task SET owner_id=$1 WHERE id=$2 AND owner_id IS NULL", h.TaskEngine.ownerID, *tID)
	if err != nil {
		log.Error(err)
		return false
	}
	if ct == 0 {
		log.Infow("did not accept task", "task_id", strconv.Itoa(int(*tID)), "reason", "already Taken")
		var tryAgain = make([]TaskID, 0, len(ids)-1)
		for _, id := range ids {
			if id != *tID {
				tryAgain = append(tryAgain, id)
			}
		}
		ids = tryAgain
		goto top
	}

	go func() {
		h.Count.Add(1)
		log.Infow("Beginning work on Task", "id", *tID, "from", from, "type", h.Name)

		var done bool
		var doErr error
		workStart := time.Now()

		defer func() {
			if r := recover(); r != nil {
				log.Error("Recovered from a serious error "+
					"while processing "+h.Name+" task "+strconv.Itoa(int(*tID))+": ", r)
			}
			h.Count.Add(-1)

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
			return false, fmt.Errorf("Could not log completion: %w ", err)
		}
		result := "unspecified error"
		if done {
			_, err = tx.Exec("DELETE FROM harmony_task WHERE id=$1", tID)
			if err != nil {

				return false, fmt.Errorf("Could not log completion: %w", err)
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
					return false, fmt.Errorf("Could not read task history: %w", err)
				}
				if ct >= h.MaxFailures {
					deleteTask = true
				}
			}
			if deleteTask {
				_, err = tx.Exec("DELETE FROM harmony_task WHERE id=$1", tID)
				if err != nil {
					return false, fmt.Errorf("Could not delete failed job: %w", err)
				}
				// Note: Extra Info is left laying around for later review & clean-up
			} else {
				_, err := tx.Exec(`UPDATE harmony_task SET owner_id=NULL WHERE id=$1`, tID)
				if err != nil {
					return false, fmt.Errorf("Could not disown failed task: %v %v", tID, err)
				}
			}
		}
		_, err = tx.Exec(`INSERT INTO harmony_task_history 
												(task_id, name, posted,    work_start, work_end, result, err)
			 VALUES ($1, $2, $3, $4, $5, $6, $7)`, tID, h.Name, postedTime, workStart, workEnd, done, result)
		if err != nil {
			return false, fmt.Errorf("Could not write history: %w", err)
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
	for _, u := range r.GpuRam {
		if u > lo.Sum(h.Cost.GpuRam) {
			goto enoughGpuRam
		}
	}
	return errors.New("Did not accept " + h.Name + " task: out of GPURam")
enoughGpuRam:
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
		b, err := fs.f(tID, fs.h.AddTask)
		if err != nil {
			log.Error("Could not follow", "error", err, "from", h.Name, "to", fs.name)
		}
		if b {
			inProcessFollowers = append(inProcessFollowers, fs.h.Name)
		}
	}

	// Over HTTP (#2 from Description)
	var hps []struct {
		HostAndPort string
		ToType      string
	}
	err := h.TaskEngine.db.Select(h.TaskEngine.ctx, &hps, `SELECT m.host_and_port, to_type
		FROM harmony_task_follow f JOIN harmony_machines m ON m.id=f.owner_id
		WHERE from_type=$1 AND to_type NOT IN $2 AND f.owner_id != $3`,
		h.Name, inProcessFollowers, h.TaskEngine.ownerID)
	if err != nil {
		log.Warn("Could not fast-trigger partner processes.", err)
		return
	}
	hostsVisited := map[string]bool{}
	tasksVisited := map[string]bool{}
	for _, v := range hps {
		if hostsVisited[v.HostAndPort] || tasksVisited[v.ToType] {
			continue
		}
		resp, err := hClient.Get(v.HostAndPort + "/scheduler/follows/" + h.Name)
		if err != nil {
			log.Warn("Couldn't hit http endpoint: ", err)
			continue
		}
		b, err := io.ReadAll(resp.Body)
		if err != nil {
			log.Warn("Couldn't hit http endpoint: ", err)
			continue
		}
		hostsVisited[v.HostAndPort], tasksVisited[v.ToType] = true, true
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
			log.Error("IO failed for fast nudge: ", string(b))
			continue
		}
	}
}
