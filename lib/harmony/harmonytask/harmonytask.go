package harmonytask

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/filecoin-project/lotus/lib/harmony/harmonydb"
	"github.com/filecoin-project/lotus/lib/harmony/resources"
)

// Consts (except for unit test)
var POLL_DURATION = time.Minute         // Poll for Work this frequently
var CLEANUP_FREQUENCY = 5 * time.Minute // Check for dead workers this often * everyone

type TaskTypeDetails struct {
	// Max returns how many tasks this machine can run of this type.
	// Negative means unrestricted.
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
	CanAccept([]TaskID) (*TaskID, error)

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
	//	    addTask("baz", func(t harmonytask.TaskID, txn db.Transaction) bool {
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

type AddTaskFunc func(extraInfo func(TaskID, *harmonydb.Tx) (bool, error))

type TaskEngine struct {
	ctx            context.Context
	handlers       []*taskTypeHandler
	db             *harmonydb.DB
	workAdderMutex sync.Mutex
	reg            *resources.Reg
	grace          context.CancelFunc
	taskMap        map[string]*taskTypeHandler
	ownerID        int
	tryAllWork     chan bool // notify if work completed
	follows        map[string][]followStruct
	lastFollowTime time.Time
	lastCleanup    atomic.Value
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
		ctx:        ctx,
		grace:      grace,
		db:         db,
		reg:        reg,
		ownerID:    reg.Resources.MachineID, // The current number representing "hostAndPort"
		taskMap:    make(map[string]*taskTypeHandler, len(impls)),
		tryAllWork: make(chan bool),
		follows:    make(map[string][]followStruct),
	}
	e.lastCleanup.Store(time.Now())
	for _, c := range impls {
		h := taskTypeHandler{
			TaskInterface:   c,
			TaskTypeDetails: c.TypeDetails(),
			TaskEngine:      e,
		}
		e.handlers = append(e.handlers, &h)
		e.taskMap[h.TaskTypeDetails.Name] = &h

		_, err := db.Exec(e.ctx, `INSERT INTO harmony_task_impl (owner_id, name) 
			VALUES ($1,$2)`, e.ownerID, h.Name)
		if err != nil {
			return nil, fmt.Errorf("can't update impl: %w", err)
		}

		for name, fn := range c.TypeDetails().Follows {
			e.follows[name] = append(e.follows[name], followStruct{fn, &h, name})

			// populate harmony_task_follows
			_, err := db.Exec(e.ctx, `INSERT INTO harmony_task_follows (owner_id, from_task, to_task)
				VALUES ($1,$2,$3)`, e.ownerID, name, h.Name)
			if err != nil {
				return nil, fmt.Errorf("can't update harmony_task_follows: %w", err)
			}
		}
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
			if !h.considerWork("recovered", []TaskID{TaskID(w.ID)}) {
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

	ctx := context.TODO()

	// block bumps & follows by unreg from DBs.
	_, err := e.db.Exec(ctx, `DELETE FROM harmony_task_impl WHERE owner_id=$1`, e.ownerID)
	if err != nil {
		log.Warn("Could not clean-up impl table: %w", err)
	}
	_, err = e.db.Exec(ctx, `DELETE FROM harmony_task_follow WHERE owner_id=$1`, e.ownerID)
	if err != nil {
		log.Warn("Could not clean-up impl table: %w", err)
	}
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
		case <-e.tryAllWork: ///////////////////// Find work after some work finished
		case <-time.NewTicker(POLL_DURATION).C: // Find work periodically
		case <-e.ctx.Done(): ///////////////////// Graceful exit
			return
		}
		e.followWorkInDB()   // "Follows" the slow way
		e.pollerTryAllWork() // "Bumps" (round robin tasks) the slow way
	}
}

// followWorkInDB implements "Follows" the slow way
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

// pollerTryAllWork implements "Bumps" (next task) the slow way
func (e *TaskEngine) pollerTryAllWork() {
	if time.Since(e.lastCleanup.Load().(time.Time)) > CLEANUP_FREQUENCY {
		e.lastCleanup.Store(time.Now())
		resources.CleanupMachines(e.ctx, e.db)
	}
	for _, v := range e.handlers {
	rerun:
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
		accepted := v.considerWork("poller", unownedTasks)
		if !accepted {
			log.Warn("Work not accepted")
			continue
		}
		if len(unownedTasks) > 1 {
			e.bump(v.Name) // wait for others before trying again to add work.
			goto rerun
		}
	}
}

// GetHttpHandlers needs to be used by the http server to register routes.
// This implements the receiver-side of "follows" and "bumps" the fast way.
func (e *TaskEngine) ApplyHttpHandlers(root gin.IRouter) {
	s := root.Group("/scheduler")
	f := s.Group("/follows")
	b := s.Group("/bump")
	for name, vs := range e.follows {
		name, vs := name, vs
		f.GET("/"+name+"/:tID", func(c *gin.Context) {
			tIDString := c.Param("tID")
			tID, err := strconv.Atoi(tIDString)
			if err != nil {
				c.JSON(401, map[string]any{"error": err.Error()})
				return
			}
			taskAdded := false
			for _, vTmp := range vs {
				v := vTmp
				b, err := v.f(TaskID(tID), v.h.AddTask)
				if err != nil {
					log.Errorw("Follow attempt failed", "error", err, "from", name, "to", v.name)
				}
				taskAdded = taskAdded || b
			}
			if taskAdded {
				e.tryAllWork <- true
				c.Status(200)
				return
			}
			c.Status(202) // NOTE: 202 for "accepted" but not worked.
		})
	}
	for _, hTmp := range e.handlers {
		h := hTmp
		b.GET("/"+h.Name+"/:tID", func(c *gin.Context) {
			tIDString := c.Param("tID")
			tID, err := strconv.Atoi(tIDString)
			if err != nil {
				c.JSON(401, map[string]any{"error": err.Error()})
				return
			}
			// We NEED to block while trying to deliver
			// this work to ease the network impact.
			if h.considerWork("bump", []TaskID{TaskID(tID)}) {
				c.Status(200)
				return
			}
			c.Status(202) // NOTE: 202 for "accepted" but not worked.
		})
	}
}

func (e *TaskEngine) bump(taskType string) {
	var res []string
	err := e.db.Select(e.ctx, &res, `SELECT host_and_port FROM harmony_machines m
	JOIN harmony_task_impl i ON i.owner_id=m.id
	WHERE i.name=$1`, taskType)
	if err != nil {
		log.Error("Could not read db for bump: ", err)
		return
	}
	for _, url := range res {
		resp, err := hClient.Get(url + "/scheduler/bump/" + taskType)
		if err != nil {
			log.Info("Server unreachable to bump: ", err)
			continue
		}
		if resp.StatusCode == 200 {
			return // just want 1 taker.
		}
	}
}

// resourcesInUse requires workListsMutex to be already locked.
func (e *TaskEngine) resourcesInUse() resources.Resources {
	tmp := e.reg.Resources
	copy(tmp.GpuRam, e.reg.Resources.GpuRam)
	for _, t := range e.handlers {
		ct := t.Count.Load()
		tmp.Cpu -= int(ct) * t.Cost.Cpu
		tmp.Gpu -= float64(ct) * t.Cost.Gpu
		tmp.Ram -= uint64(ct) * t.Cost.Ram
		if len(t.Cost.GpuRam) == 0 {
			continue
		}
		for i := int32(0); i < ct; i++ {
			for grIdx, j := range tmp.GpuRam {
				if j > t.Cost.GpuRam[0] {
					tmp.GpuRam[grIdx] = 0 // Only 1 per GPU. j - t.Cost.GpuRam[0]
					break
				}
			}
			log.Warn("We should never get out of gpuram for what's consumed.")
		}
	}
	return tmp
}
