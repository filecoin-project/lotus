package itests

import (
	"context"
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
	"github.com/stretchr/testify/require"
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
		e, err := harmonytask.New(cdb, []harmonytask.TaskInterface{t1}, "test:1")
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

type passthru struct {
	dtl       harmonytask.TaskTypeDetails
	do        func(tID harmonytask.TaskID, stillOwned func() bool) (done bool, err error)
	canAccept func(list []harmonytask.TaskID) (*harmonytask.TaskID, error)
	adder     func(add harmonytask.AddTaskFunc)
}

func (t *passthru) Do(tID harmonytask.TaskID, stillOwned func() bool) (done bool, err error) {
	return t.do(tID, stillOwned)
}
func (t *passthru) CanAccept(list []harmonytask.TaskID) (*harmonytask.TaskID, error) {
	return t.canAccept(list)
}
func (t *passthru) TypeDetails() harmonytask.TaskTypeDetails {
	return t.dtl
}
func (t *passthru) Adder(add harmonytask.AddTaskFunc) {
	if t.adder != nil {
		t.adder(add)
	}
}

func TestHarmonyTasksWith2PartiesPolling(t *testing.T) {
	withSetup(t, func(m *kit.TestMiner) {
		cdb := m.BaseAPI.(*impl.StorageMinerAPI).HarmonyDB
		dtl := harmonytask.TaskTypeDetails{Name: "foo", Max: -1, Cost: resources.Resources{}}
		senderParty := &passthru{
			dtl:       dtl,
			canAccept: func(list []harmonytask.TaskID) (*harmonytask.TaskID, error) { return nil, nil },
			adder: func(add harmonytask.AddTaskFunc) {
				for _, v := range []string{"A", "B"} {
					add(func(tID harmonytask.TaskID, tx *harmonydb.Tx) bool {
						_, err := tx.Exec("INSERT INTO itest_scratch (some_int, content) VALUES ($1,$2)", tID, v)
						if err != nil {
							t.Fatal("Err inserting: ", err)
						}
						return true
					})
				}
			},
		}
		letters := []string{}
		var lettersMutex sync.Mutex
		workerParty := &passthru{
			dtl:       dtl,
			canAccept: func(list []harmonytask.TaskID) (*harmonytask.TaskID, error) { return &list[0], nil },
			do: func(tID harmonytask.TaskID, stillOwned func() bool) (done bool, err error) {
				var content string
				err = cdb.QueryRow(context.Background(),
					"SELECT content FROM itest_scratch WHERE some_int=$1", tID).Scan(&content)
				if err != nil {
					t.Fatal("Error reading content:", err)
				}
				lettersMutex.Lock()
				defer lettersMutex.Unlock()
				letters = append(letters, content)
				return true, nil
			},
		}
		harmonytask.POLL_DURATION = time.Millisecond * 100
		sender, err := harmonytask.New(cdb, []harmonytask.TaskInterface{senderParty}, "test:1")
		if err != nil {
			t.Fatal("Did not initialize:" + err.Error())
		}
		worker, err := harmonytask.New(cdb, []harmonytask.TaskInterface{workerParty}, "test:2")
		if err != nil {
			t.Fatal("Did not initialize:" + err.Error())
		}
		time.Sleep(3 * time.Second) // do the work. FLAKYNESS RISK HERE.
		sender.GracefullyTerminate(time.Second * 5)
		worker.GracefullyTerminate(time.Second * 5)
		sort.Strings(letters)
		require.Equal(t, letters, []string{"A", "B"})
	})
}

/*
TODO BEFORE PR DONE tests to write: !!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!
	worker stealing (when worker) died
	    ---> push db entry with owner
	retry flow once do() fails
*/

/*
FUTURE test fast-pass round-robin via http calls once the API for that is set
It's necessary for WinningPoSt.

FUTURE test follows. It's needed for sealing work.
*/
