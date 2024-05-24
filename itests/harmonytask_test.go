package itests

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"testing"
	"time"

	logging "github.com/ipfs/go-log/v2"
	"github.com/stretchr/testify/require"

	harmonytask2 "github.com/filecoin-project/lotus/curiosrc/harmony/harmonytask"
	"github.com/filecoin-project/lotus/curiosrc/harmony/resources"
	"github.com/filecoin-project/lotus/itests/kit"
	"github.com/filecoin-project/lotus/lib/harmony/harmonydb"
	"github.com/filecoin-project/lotus/node/impl"
)

type task1 struct {
	toAdd               []int
	myPersonalTableLock sync.Mutex
	myPersonalTable     map[harmonytask2.TaskID]int // This would typically be a DB table
	WorkCompleted       []string
}

func withDbSetup(t *testing.T, f func(*kit.TestMiner)) {
	_, miner, _ := kit.EnsembleMinimal(t,
		kit.LatestActorsAt(-1),
		kit.MockProofs(),
		kit.WithSectorIndexDB(),
	)
	_ = logging.SetLogLevel("harmonytask", "debug")

	f(miner)
}

func (t *task1) Do(taskID harmonytask2.TaskID, stillOwned func() bool) (done bool, err error) {
	if !stillOwned() {
		return false, errors.New("Why not still owned?")
	}
	t.myPersonalTableLock.Lock()
	defer t.myPersonalTableLock.Unlock()
	t.WorkCompleted = append(t.WorkCompleted, fmt.Sprintf("taskResult%d", t.myPersonalTable[taskID]))
	return true, nil
}
func (t *task1) CanAccept(list []harmonytask2.TaskID, e *harmonytask2.TaskEngine) (*harmonytask2.TaskID, error) {
	return &list[0], nil
}
func (t *task1) TypeDetails() harmonytask2.TaskTypeDetails {
	return harmonytask2.TaskTypeDetails{
		Max:         100,
		Name:        "ThingOne",
		MaxFailures: 1,
		Cost: resources.Resources{
			Cpu: 1,
			Ram: 100 << 10, // at 100kb, it's tiny
		},
	}
}
func (t *task1) Adder(add harmonytask2.AddTaskFunc) {
	for _, vTmp := range t.toAdd {
		v := vTmp
		add(func(tID harmonytask2.TaskID, tx *harmonydb.Tx) (bool, error) {
			t.myPersonalTableLock.Lock()
			defer t.myPersonalTableLock.Unlock()

			t.myPersonalTable[tID] = v
			return true, nil
		})
	}
}

func init() {
	//logging.SetLogLevel("harmonydb", "debug")
	//logging.SetLogLevel("harmonytask", "debug")
}

func TestHarmonyTasks(t *testing.T) {
	//t.Parallel()
	withDbSetup(t, func(m *kit.TestMiner) {
		cdb := m.BaseAPI.(*impl.StorageMinerAPI).HarmonyDB
		t1 := &task1{
			toAdd:           []int{56, 73},
			myPersonalTable: map[harmonytask2.TaskID]int{},
		}
		harmonytask2.POLL_DURATION = time.Millisecond * 100
		e, err := harmonytask2.New(cdb, []harmonytask2.TaskInterface{t1}, "test:1")
		require.NoError(t, err)
		time.Sleep(time.Second) // do the work. FLAKYNESS RISK HERE.
		e.GracefullyTerminate()
		expected := []string{"taskResult56", "taskResult73"}
		sort.Strings(t1.WorkCompleted)
		require.Equal(t, expected, t1.WorkCompleted, "unexpected results")
	})
}

type passthru struct {
	dtl       harmonytask2.TaskTypeDetails
	do        func(tID harmonytask2.TaskID, stillOwned func() bool) (done bool, err error)
	canAccept func(list []harmonytask2.TaskID, e *harmonytask2.TaskEngine) (*harmonytask2.TaskID, error)
	adder     func(add harmonytask2.AddTaskFunc)
}

func (t *passthru) Do(taskID harmonytask2.TaskID, stillOwned func() bool) (done bool, err error) {
	return t.do(taskID, stillOwned)
}
func (t *passthru) CanAccept(list []harmonytask2.TaskID, e *harmonytask2.TaskEngine) (*harmonytask2.TaskID, error) {
	return t.canAccept(list, e)
}
func (t *passthru) TypeDetails() harmonytask2.TaskTypeDetails {
	return t.dtl
}
func (t *passthru) Adder(add harmonytask2.AddTaskFunc) {
	if t.adder != nil {
		t.adder(add)
	}
}

// Common stuff
var dtl = harmonytask2.TaskTypeDetails{Name: "foo", Max: -1, Cost: resources.Resources{}}
var lettersMutex sync.Mutex

func fooLetterAdder(t *testing.T, cdb *harmonydb.DB) *passthru {
	return &passthru{
		dtl: dtl,
		canAccept: func(list []harmonytask2.TaskID, e *harmonytask2.TaskEngine) (*harmonytask2.TaskID, error) {
			return nil, nil
		},
		adder: func(add harmonytask2.AddTaskFunc) {
			for _, vTmp := range []string{"A", "B"} {
				v := vTmp
				add(func(tID harmonytask2.TaskID, tx *harmonydb.Tx) (bool, error) {
					_, err := tx.Exec("INSERT INTO itest_scratch (some_int, content) VALUES ($1,$2)", tID, v)
					require.NoError(t, err)
					return true, nil
				})
			}
		},
	}
}
func fooLetterSaver(t *testing.T, cdb *harmonydb.DB, dest *[]string) *passthru {
	return &passthru{
		dtl: dtl,
		canAccept: func(list []harmonytask2.TaskID, e *harmonytask2.TaskEngine) (*harmonytask2.TaskID, error) {
			return &list[0], nil
		},
		do: func(tID harmonytask2.TaskID, stillOwned func() bool) (done bool, err error) {
			var content string
			err = cdb.QueryRow(context.Background(),
				"SELECT content FROM itest_scratch WHERE some_int=$1", tID).Scan(&content)
			require.NoError(t, err)
			lettersMutex.Lock()
			defer lettersMutex.Unlock()
			*dest = append(*dest, content)
			return true, nil
		},
	}
}

func TestHarmonyTasksWith2PartiesPolling(t *testing.T) {
	//t.Parallel()
	withDbSetup(t, func(m *kit.TestMiner) {
		cdb := m.BaseAPI.(*impl.StorageMinerAPI).HarmonyDB
		senderParty := fooLetterAdder(t, cdb)
		var dest []string
		workerParty := fooLetterSaver(t, cdb, &dest)
		harmonytask2.POLL_DURATION = time.Millisecond * 100
		sender, err := harmonytask2.New(cdb, []harmonytask2.TaskInterface{senderParty}, "test:1")
		require.NoError(t, err)
		worker, err := harmonytask2.New(cdb, []harmonytask2.TaskInterface{workerParty}, "test:2")
		require.NoError(t, err)
		time.Sleep(time.Second) // do the work. FLAKYNESS RISK HERE.
		sender.GracefullyTerminate()
		worker.GracefullyTerminate()
		sort.Strings(dest)
		require.Equal(t, []string{"A", "B"}, dest)
	})
}

func TestWorkStealing(t *testing.T) {
	//t.Parallel()
	withDbSetup(t, func(m *kit.TestMiner) {
		cdb := m.BaseAPI.(*impl.StorageMinerAPI).HarmonyDB
		ctx := context.Background()

		// The dead worker will be played by a few SQL INSERTS.
		_, err := cdb.Exec(ctx, `INSERT INTO harmony_machines
		(id, last_contact,host_and_port, cpu, ram, gpu)
		VALUES (300, DATE '2000-01-01', 'test:1', 4, 400000, 1)`)
		require.ErrorIs(t, err, nil)
		_, err = cdb.Exec(ctx, `INSERT INTO harmony_task 
		(id, name, owner_id, posted_time, added_by) 
		VALUES (1234, 'foo', 300, DATE '2000-01-01', 300)`)
		require.ErrorIs(t, err, nil)
		_, err = cdb.Exec(ctx, "INSERT INTO itest_scratch (some_int, content) VALUES (1234, 'M')")
		require.ErrorIs(t, err, nil)

		harmonytask2.POLL_DURATION = time.Millisecond * 100
		harmonytask2.CLEANUP_FREQUENCY = time.Millisecond * 100
		var dest []string
		worker, err := harmonytask2.New(cdb, []harmonytask2.TaskInterface{fooLetterSaver(t, cdb, &dest)}, "test:2")
		require.ErrorIs(t, err, nil)
		time.Sleep(time.Second) // do the work. FLAKYNESS RISK HERE.
		worker.GracefullyTerminate()
		require.Equal(t, []string{"M"}, dest)
	})
}

func TestTaskRetry(t *testing.T) {
	//t.Parallel()
	withDbSetup(t, func(m *kit.TestMiner) {
		cdb := m.BaseAPI.(*impl.StorageMinerAPI).HarmonyDB
		senderParty := fooLetterAdder(t, cdb)
		harmonytask2.POLL_DURATION = time.Millisecond * 100
		sender, err := harmonytask2.New(cdb, []harmonytask2.TaskInterface{senderParty}, "test:1")
		require.NoError(t, err)

		alreadyFailed := map[string]bool{}
		var dest []string
		fails2xPerMsg := &passthru{
			dtl: dtl,
			canAccept: func(list []harmonytask2.TaskID, e *harmonytask2.TaskEngine) (*harmonytask2.TaskID, error) {
				return &list[0], nil
			},
			do: func(tID harmonytask2.TaskID, stillOwned func() bool) (done bool, err error) {
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
				dest = append(dest, content)
				return true, nil
			},
		}
		rcv, err := harmonytask2.New(cdb, []harmonytask2.TaskInterface{fails2xPerMsg}, "test:2")
		require.NoError(t, err)
		time.Sleep(time.Second)
		sender.GracefullyTerminate()
		rcv.GracefullyTerminate()
		sort.Strings(dest)
		require.Equal(t, []string{"A", "B"}, dest)
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

func TestBoredom(t *testing.T) {
	//t.Parallel()
	withDbSetup(t, func(m *kit.TestMiner) {
		cdb := m.BaseAPI.(*impl.StorageMinerAPI).HarmonyDB
		harmonytask2.POLL_DURATION = time.Millisecond * 100
		var taskID harmonytask2.TaskID
		var ran bool
		boredParty := &passthru{
			dtl: harmonytask2.TaskTypeDetails{
				Name: "boredTest",
				Max:  -1,
				Cost: resources.Resources{},
				IAmBored: func(add harmonytask2.AddTaskFunc) error {
					add(func(tID harmonytask2.TaskID, tx *harmonydb.Tx) (bool, error) {
						taskID = tID
						return true, nil
					})
					return nil
				},
			},
			canAccept: func(list []harmonytask2.TaskID, e *harmonytask2.TaskEngine) (*harmonytask2.TaskID, error) {
				require.Equal(t, harmonytask2.WorkSourceIAmBored, e.WorkOrigin)
				return &list[0], nil
			},
			do: func(tID harmonytask2.TaskID, stillOwned func() bool) (done bool, err error) {
				require.Equal(t, taskID, tID)
				ran = true
				return true, nil
			},
		}
		ht, err := harmonytask2.New(cdb, []harmonytask2.TaskInterface{boredParty}, "test:1")
		require.NoError(t, err)
		require.Eventually(t, func() bool { return ran }, time.Second, time.Millisecond*100)
		ht.GracefullyTerminate()
	})
}
