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

	"github.com/stretchr/testify/require"

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
		e, err := harmonytask.New(cdb, []harmonytask.TaskInterface{t1}, "test:1")
		require.NoError(t, err)
		time.Sleep(3 * time.Second) // do the work. FLAKYNESS RISK HERE.
		e.GracefullyTerminate(time.Minute)
		require.Equal(t, t1.WorkCompleted, 2, "wrong amount of work complete: expected 2 got:")
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

// Common stuff
var dtl = harmonytask.TaskTypeDetails{Name: "foo", Max: -1, Cost: resources.Resources{}}
var letters []string
var lettersMutex sync.Mutex

func fooLetterAdder(t *testing.T, cdb *harmonydb.DB) *passthru {
	return &passthru{
		dtl:       dtl,
		canAccept: func(list []harmonytask.TaskID) (*harmonytask.TaskID, error) { return nil, nil },
		adder: func(add harmonytask.AddTaskFunc) {
			for _, v := range []string{"A", "B"} {
				add(func(tID harmonytask.TaskID, tx *harmonydb.Tx) bool {
					_, err := tx.Exec("INSERT INTO itest_scratch (some_int, content) VALUES ($1,$2)", tID, v)
					require.NoError(t, err)
					return true
				})
			}
		},
	}
}
func fooLetterSaver(t *testing.T, cdb *harmonydb.DB) *passthru {
	return &passthru{
		dtl:       dtl,
		canAccept: func(list []harmonytask.TaskID) (*harmonytask.TaskID, error) { return &list[0], nil },
		do: func(tID harmonytask.TaskID, stillOwned func() bool) (done bool, err error) {
			var content string
			err = cdb.QueryRow(context.Background(),
				"SELECT content FROM itest_scratch WHERE some_int=$1", tID).Scan(&content)
			require.NoError(t, err)
			lettersMutex.Lock()
			defer lettersMutex.Unlock()
			letters = append(letters, content)
			return true, nil
		},
	}
}

func TestHarmonyTasksWith2PartiesPolling(t *testing.T) {
	withSetup(t, func(m *kit.TestMiner) {
		cdb := m.BaseAPI.(*impl.StorageMinerAPI).HarmonyDB
		senderParty := fooLetterAdder(t, cdb)
		workerParty := fooLetterSaver(t, cdb)
		harmonytask.POLL_DURATION = time.Millisecond * 100
		sender, err := harmonytask.New(cdb, []harmonytask.TaskInterface{senderParty}, "test:1")
		require.NoError(t, err)
		worker, err := harmonytask.New(cdb, []harmonytask.TaskInterface{workerParty}, "test:2")
		require.NoError(t, err)
		time.Sleep(3 * time.Second) // do the work. FLAKYNESS RISK HERE.
		sender.GracefullyTerminate(time.Second * 5)
		worker.GracefullyTerminate(time.Second * 5)
		sort.Strings(letters)
		require.Equal(t, letters, []string{"A", "B"})
	})
}

func TestWorkStealing(t *testing.T) {
	withSetup(t, func(m *kit.TestMiner) {
		cdb := m.BaseAPI.(*impl.StorageMinerAPI).HarmonyDB
		ctx := context.Background()

		// The dead worker will be played by a few SQL INSERTS.
		_, err := cdb.Exec(ctx, `INSERT INTO harmony_machines
		(id, last_contact,host_and_port, cpu, ram, gpu, gpuram)
		VALUES (300, DATE '2000-01-01', 'test:1', 4, 400000, 1, 1000000)`)
		require.ErrorIs(t, err, nil)
		_, err = cdb.Exec(ctx, `INSERT INTO harmony_task 
		(id, name, owner_id, posted_time, added_by) 
		VALUES (1234, 'foo', 300, DATE '2000-01-01', 300)`)
		require.ErrorIs(t, err, nil)
		_, err = cdb.Exec(ctx, "INSERT INTO itest_scratch (some_int, content) VALUES (1234, 'M')")
		require.ErrorIs(t, err, nil)

		harmonytask.POLL_DURATION = time.Millisecond * 100
		harmonytask.CLEANUP_FREQUENCY = time.Millisecond * 100
		worker, err := harmonytask.New(cdb, []harmonytask.TaskInterface{fooLetterSaver(t, cdb)}, "test:2")
		require.ErrorIs(t, err, nil)
		time.Sleep(3 * time.Second) // do the work. FLAKYNESS RISK HERE.
		worker.GracefullyTerminate(time.Second * 5)
		require.Equal(t, []string{"M"}, letters)
	})
}

func TestTaskRetry(t *testing.T) {
	withSetup(t, func(m *kit.TestMiner) {
		cdb := m.BaseAPI.(*impl.StorageMinerAPI).HarmonyDB
		senderParty := fooLetterAdder(t, cdb)
		harmonytask.POLL_DURATION = time.Millisecond * 100
		sender, err := harmonytask.New(cdb, []harmonytask.TaskInterface{senderParty}, "test:1")
		require.NoError(t, err)

		alreadyFailed := map[string]bool{}
		fails2xPerMsg := &passthru{
			dtl:       dtl,
			canAccept: func(list []harmonytask.TaskID) (*harmonytask.TaskID, error) { return &list[0], nil },
			do: func(tID harmonytask.TaskID, stillOwned func() bool) (done bool, err error) {
				var content string
				err = cdb.QueryRow(context.Background(),
					"SELECT content FROM itest_scratch WHERE some_int=$1", tID).Scan(&content)
				require.NoError(t, err)
				lettersMutex.Lock()
				defer lettersMutex.Unlock()
				if !alreadyFailed[content] {
					alreadyFailed[content] = true
					return false, errors.New("intentional 'error'")
				}
				letters = append(letters, content)
				return true, nil
			},
		}
		rcv, err := harmonytask.New(cdb, []harmonytask.TaskInterface{fails2xPerMsg}, "test:2")
		require.NoError(t, err)
		time.Sleep(3 * time.Second)
		sender.GracefullyTerminate(time.Hour)
		rcv.GracefullyTerminate(time.Hour)
		sort.Strings(letters)
		require.Equal(t, []string{"A", "B"}, letters)
		type hist struct {
			TaskID int
			Result bool
			Err    string
		}
		var res []hist
		require.NoError(t, cdb.Select(context.Background(), &res,
			`SELECT task_id, result, err FROM harmony_task_history
			 ORDER BY result DESC, task_id`))

		require.Equal(t, []hist{
			{1, true, ""},
			{2, true, ""},
			{1, false, "error: intentional 'error'"},
			{2, false, "error: intentional 'error'"}}, res)
	})
}

/*
FUTURE test fast-pass round-robin via http calls (3party) once the API for that is set
It's necessary for WinningPoSt.

FUTURE test follows.
It's necessary for sealing work.
*/
