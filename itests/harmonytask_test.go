package itests

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/filecoin-project/lotus/itests/kit"
	"github.com/filecoin-project/lotus/lib/harmony/harmonydb"
	"github.com/filecoin-project/lotus/lib/harmony/harmonytask"
	"github.com/filecoin-project/lotus/lib/harmony/resources"
	"github.com/filecoin-project/lotus/node/impl"
)

type task1 struct {
	toAdd               []int
	myPersonalTableLock sync.Mutex
	myPersonalTable     map[harmonytask.TaskID]int // This would typicallyb be a DB table
	WorkCompleted       []string
}

func (t *task1) Do(tID harmonytask.TaskID, stillOwned func() bool) (done bool, err error) {
	if !stillOwned() {
		return false, errors.New("Why not still owned?")
	}
	t.myPersonalTableLock.Lock()
	defer t.myPersonalTableLock.Unlock()
	t.WorkCompleted = append(t.WorkCompleted, fmt.Sprintf("taskResult%d", t.myPersonalTable[tID]))
	return true, nil
}
func (t *task1) CanAccept(list []harmonytask.TaskID) (*harmonytask.TaskID, error) {
	return &list[0], nil
}
func (t *task1) TypeDetails() harmonytask.TaskTypeDetails {
	return harmonytask.TaskTypeDetails{
		Max:         100,
		Name:        "ThingOne",
		MaxFailures: 1,
		Cost: resources.Resources{
			Cpu: 1,
			Ram: 100 << 10, // at 100kb, it's tiny
		},
	}
}
func (t *task1) Adder(add harmonytask.AddTaskFunc) {
	for _, v := range t.toAdd {
		add(func(tID harmonytask.TaskID, tx *harmonydb.Tx) bool {
			t.myPersonalTableLock.Lock()
			defer t.myPersonalTableLock.Unlock()

			t.myPersonalTable[tID] = v
			return true
		})
	}
}

func TestHarmonyTasks(t *testing.T) {
	withSetup(t, func(m *kit.TestMiner) {
		cdb := m.BaseAPI.(*impl.StorageMinerAPI).HarmonyDB
		t1 := &task1{
			toAdd:           []int{56, 73},
			myPersonalTable: map[harmonytask.TaskID]int{},
		}
		e, err := harmonytask.New(cdb, []harmonytask.TaskInterface{t1}, "test:1", "/tmp")
		if err != nil {
			t.Fatal("Did not initialize:" + err.Error())
		}
		time.Sleep(3 * time.Second) // do the work. FLAKYNESS RISK HERE.
		e.GracefullyTerminate(time.Minute)
		if len(t1.WorkCompleted) != 2 {
			t.Fatal("wrong amount of work complete: expected 2 got: ", t1.WorkCompleted)
		}
		sort.Strings(t1.WorkCompleted)
		got := strings.Join(t1.WorkCompleted, ",")
		expected := "taskResult56,taskResult73"
		if got != expected {
			t.Fatal("Unexpected results! Wanted " + expected + " got " + got)
		}
		// TODO test history table looks right.
	})
}

/* TODO tests must include cases like:
   2 or more parties
   work added before 2nd party joins
   work picked-up after other work completes (in-party and across-parties)
*/
