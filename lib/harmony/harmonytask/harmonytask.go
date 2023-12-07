package harmonytask

import (
	"context"
	"fmt"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/filecoin-project/lotus/lib/harmony/harmonydb"
	"github.com/filecoin-project/lotus/lib/harmony/resources"
)

// Consts (except for unit test)
var POLL_DURATION = time.Second * 3     // Poll for Work this frequently
var CLEANUP_FREQUENCY = 5 * time.Minute // Check for dead workers this often * everyone
var FOLLOW_FREQUENCY = 1 * time.Minute  // Check for work to follow this often

type TaskTypeDetails struct {
	// Max returns how many tasks this machine can run of this type.
	// Zero (default) or less means unrestricted.
	Max int

	// Name is the task name to be added to the task list.
	Name string

	// Peak costs to Do() the task.
	Cost resources.Resources

	// Max Failure count before the job is dropped.
	// 0 = retry forever
	MaxFailures uint

	// Follow another task's completion via this task's creation.
	// The function should populate extraInfo from data
	// available from the previous task's tables, using the given TaskID.
	// It should also return success if the trigger succeeded.
	// NOTE: if refatoring tasks, see if your task is
	// necessary. Ex: Is the sector state correct for your stage to run?
	Follows map[string]func(TaskID, AddTaskFunc) (bool, error)
}

// TaskInterface must be implemented in order to have a task used by harmonytask.
type TaskInterface interface {
	// Do the task assigned. Call stillOwned before making single-writer-only
	// changes to ensure the work has not been stolen.
	// This is the ONLY function that should attempt to do the work, and must
	// ONLY be called by harmonytask.
	// Indicate if the task no-longer needs scheduling with done=true including
	// cases where it's past the deadline.
	Do(taskID TaskID, stillOwned func() bool) (done bool, err error)

	// CanAccept should return if the task can run on this machine. It should
	// return null if the task type is not allowed on this machine.
	// It should select the task it most wants to accomplish.
	// It is also responsible for determining & reserving disk space (including scratch).
	CanAccept([]TaskID, *TaskEngine) (*TaskID, error)

	// TypeDetails() returns static details about how this task behaves and
	// how this machine will run it. Read once at the beginning.
	TypeDetails() TaskTypeDetails

	// This listener will consume all external sources continuously for work.
	// Do() may also be called from a backlog of work. This must not
	// start doing the work (it still must be scheduled).
	// Note: Task de-duplication should happen in ExtraInfoFunc by
	//  returning false, typically by determining from the tx that the work
	//  exists already. The easy way is to have a unique joint index
	//  across all fields that will be common.
	// Adder should typically only add its own task type, but multiple
	//   is possible for when 1 trigger starts 2 things.
	// Usage Example:
	// func (b *BazType)Adder(addTask AddTaskFunc) {
	//	  for {
	//      bazMaker := <- bazChannel
	//	    addTask("baz", func(t harmonytask.TaskID, txn db.Transaction) (bool, error) {
	//	       _, err := txn.Exec(`INSERT INTO bazInfoTable (taskID, qix, mot)
	//			  VALUES ($1,$2,$3)`, id, bazMaker.qix, bazMaker.mot)
	//         if err != nil {
	//				scream(err)
	//	 		 	return false
	//		   }
	// 		   return true
	//		})
	//	  }
	// }
	Adder(AddTaskFunc)
}

// AddTaskFunc is responsible for adding a task's details "extra info" to the DB.
// It should return true if the task should be added, false if it was already there.
// This is typically accomplished with a "unique" index on your detals table that
// would cause the insert to fail.
// The error indicates that instead of a conflict (which we should ignore) that we
// actually have a serious problem that needs to be logged with context.
type AddTaskFunc func(extraInfo func(TaskID, *harmonydb.Tx) (shouldCommit bool, seriousError error))

type TaskEngine struct {
	ctx            context.Context
	handlers       []*taskTypeHandler
	db             *harmonydb.DB
	reg            *resources.Reg
	grace          context.CancelFunc
	taskMap        map[string]*taskTypeHandler
	ownerID        int
	follows        map[string][]followStruct
	lastFollowTime time.Time
	lastCleanup    atomic.Value
	hostAndPort    string
}
type followStruct struct {
	f    func(TaskID, AddTaskFunc) (bool, error)
	h    *taskTypeHandler
	name string
}

type TaskID int

// New creates all the task definitions. Note that TaskEngine
// knows nothing about the tasks themselves and serves to be a
// generic container for common work
func New(
	db *harmonydb.DB,
	impls []TaskInterface,
	hostnameAndPort string) (*TaskEngine, error) {

	reg, err := resources.Register(db, hostnameAndPort)
	if err != nil {
		return nil, fmt.Errorf("cannot get resources: %w", err)
	}
	ctx, grace := context.WithCancel(context.Background())
	e := &TaskEngine{
		ctx:         ctx,
		grace:       grace,
		db:          db,
		reg:         reg,
		ownerID:     reg.Resources.MachineID, // The current number representing "hostAndPort"
		taskMap:     make(map[string]*taskTypeHandler, len(impls)),
		follows:     make(map[string][]followStruct),
		hostAndPort: hostnameAndPort,
	}
	e.lastCleanup.Store(time.Now())
	for _, c := range impls {
		h := taskTypeHandler{
			TaskInterface:   c,
			TaskTypeDetails: c.TypeDetails(),
			TaskEngine:      e,
		}

		if len(h.Name) > 16 {
			return nil, fmt.Errorf("task name too long: %s, max 16 characters", h.Name)
		}

		e.handlers = append(e.handlers, &h)
		e.taskMap[h.TaskTypeDetails.Name] = &h
	}

	// resurrect old work
	{
		var taskRet []struct {
			ID   int
			Name string
		}

		err := db.Select(e.ctx, &taskRet, `SELECT id, name from harmony_task WHERE owner_id=$1`, e.ownerID)
		if err != nil {
			return nil, err
		}
		for _, w := range taskRet {
			// edge-case: if old assignments are not available tasks, unlock them.
			h := e.taskMap[w.Name]
			if h == nil {
				_, err := db.Exec(e.ctx, `UPDATE harmony_task SET owner=NULL WHERE id=$1`, w.ID)
				if err != nil {
					log.Errorw("Cannot remove self from owner field", "error", err)
					continue // not really fatal, but not great
				}
			}
			if !h.considerWork(workSourceRecover, []TaskID{TaskID(w.ID)}) {
				log.Error("Strange: Unable to accept previously owned task: ", w.ID, w.Name)
			}
		}
	}
	for _, h := range e.handlers {
		go h.Adder(h.AddTask)
	}
	go e.poller()

	return e, nil
}

// GracefullyTerminate hangs until all present tasks have completed.
// Call this to cleanly exit the process. As some processes are long-running,
// passing a deadline will ignore those still running (to be picked-up later).
func (e *TaskEngine) GracefullyTerminate(deadline time.Duration) {
	e.grace()
	e.reg.Shutdown()
	deadlineChan := time.NewTimer(deadline).C
top:
	for _, h := range e.handlers {
		if h.Count.Load() > 0 {
			select {
			case <-deadlineChan:
				return
			default:
				time.Sleep(time.Millisecond)
				goto top
			}
		}
	}
}

func (e *TaskEngine) poller() {
	for {
		select {
		case <-time.NewTicker(POLL_DURATION).C: // Find work periodically
		case <-e.ctx.Done(): ///////////////////// Graceful exit
			return
		}
		e.pollerTryAllWork()
		if time.Since(e.lastFollowTime) > FOLLOW_FREQUENCY {
			e.followWorkInDB()
		}
	}
}

// followWorkInDB implements "Follows"
func (e *TaskEngine) followWorkInDB() {
	// Step 1: What are we following?
	var lastFollowTime time.Time
	lastFollowTime, e.lastFollowTime = e.lastFollowTime, time.Now()

	for fromName, srcs := range e.follows {
		var cList []int // Which work is done (that we follow) since we last checked?
		err := e.db.Select(e.ctx, &cList, `SELECT h.task_id FROM harmony_task_history 
   		WHERE h.work_end>$1 AND h.name=$2`, lastFollowTime, fromName)
		if err != nil {
			log.Error("Could not query DB: ", err)
			return
		}
		for _, src := range srcs {
			for _, workAlreadyDone := range cList { // Were any tasks made to follow these tasks?
				var ct int
				err := e.db.QueryRow(e.ctx, `SELECT COUNT(*) FROM harmony_task 
					WHERE name=$1 AND previous_task=$2`, src.h.Name, workAlreadyDone).Scan(&ct)
				if err != nil {
					log.Error("Could not query harmony_task: ", err)
					return // not recoverable here
				}
				if ct > 0 {
					continue
				}
				// we need to create this task
				b, err := src.h.Follows[fromName](TaskID(workAlreadyDone), src.h.AddTask)
				if err != nil {
					log.Errorw("Could not follow: ", "error", err)
					continue
				}
				if !b {
					// But someone may have beaten us to it.
					log.Debugf("Unable to add task %s following Task(%d, %s)", src.h.Name, workAlreadyDone, fromName)
				}
			}
		}
	}
}

// pollerTryAllWork starts the next 1 task
func (e *TaskEngine) pollerTryAllWork() {
	if time.Since(e.lastCleanup.Load().(time.Time)) > CLEANUP_FREQUENCY {
		e.lastCleanup.Store(time.Now())
		resources.CleanupMachines(e.ctx, e.db)
	}
	for _, v := range e.handlers {
		if v.AssertMachineHasCapacity() != nil {
			continue
		}
		var unownedTasks []TaskID
		err := e.db.Select(e.ctx, &unownedTasks, `SELECT id 
			FROM harmony_task
			WHERE owner_id IS NULL AND name=$1
			ORDER BY update_time`, v.Name)
		if err != nil {
			log.Error("Unable to read work ", err)
			continue
		}
		if len(unownedTasks) > 0 {
			accepted := v.considerWork(workSourcePoller, unownedTasks)
			if accepted {
				return // accept new work slowly and in priority order
			}
			log.Warn("Work not accepted for " + strconv.Itoa(len(unownedTasks)) + " " + v.Name + " task(s)")
		}
	}
}

// ResourcesAvailable determines what resources are still unassigned.
func (e *TaskEngine) ResourcesAvailable() resources.Resources {
	tmp := e.reg.Resources
	for _, t := range e.handlers {
		ct := t.Count.Load()
		tmp.Cpu -= int(ct) * t.Cost.Cpu
		tmp.Gpu -= float64(ct) * t.Cost.Gpu
		tmp.Ram -= uint64(ct) * t.Cost.Ram
	}
	return tmp
}
