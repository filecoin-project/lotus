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

	"github.com/filecoin-project/lotus/itests/kit"
	"github.com/filecoin-project/lotus/lib/harmony/harmonydb"
	"github.com/filecoin-project/lotus/lib/harmony/harmonytask"
	"github.com/filecoin-project/lotus/lib/harmony/resources"
	"github.com/filecoin-project/lotus/node/impl"
)

type task1 struct {
	toAdd               []int
	myPersonalTableLock sync.Mutex
	myPersonalTable     map[harmonytask.TaskID]int // This would typically be a DB table
	WorkCompleted       []string
}

func withDbSetup(t *testing.T, f func(*kit.TestMiner)) {
	_, miner, _ := kit.EnsembleMinimal(t,
		kit.LatestActorsAt(-1),
		kit.MockProofs(),
		kit.WithSectorIndexDB(),
	)
	logging.SetLogLevel("harmonytask", "debug")

	f(miner)
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
func (t *task1) CanAccept(list []harmonytask.TaskID, e *harmonytask.TaskEngine) (*harmonytask.TaskID, error) {
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
	for _, vTmp := range t.toAdd {
		v := vTmp
		add(func(tID harmonytask.TaskID, tx *harmonydb.Tx) (bool, error) {
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
			myPersonalTable: map[harmonytask.TaskID]int{},
		}
		harmonytask.POLL_DURATION = time.Millisecond * 100
		e, err := harmonytask.New(cdb, []harmonytask.TaskInterface{t1}, "test:1")
		require.NoError(t, err)
		time.Sleep(time.Second) // do the work. FLAKYNESS RISK HERE.
		e.GracefullyTerminate(time.Minute)
		expected := []string{"taskResult56", "taskResult73"}
		sort.Strings(t1.WorkCompleted)
		require.Equal(t, expected, t1.WorkCompleted, "unexpected results")
	})
}

type passthru struct {
	dtl       harmonytask.TaskTypeDetails
	do        func(tID harmonytask.TaskID, stillOwned func() bool) (done bool, err error)
	canAccept func(list []harmonytask.TaskID, e *harmonytask.TaskEngine) (*harmonytask.TaskID, error)
	adder     func(add harmonytask.AddTaskFunc)
}

func (t *passthru) Do(tID harmonytask.TaskID, stillOwned func() bool) (done bool, err error) {
	return t.do(tID, stillOwned)
}
func (t *passthru) CanAccept(list []harmonytask.TaskID, e *harmonytask.TaskEngine) (*harmonytask.TaskID, error) {
	return t.canAccept(list, e)
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
var lettersMutex sync.Mutex

func fooLetterAdder(t *testing.T, cdb *harmonydb.DB) *passthru {
	return &passthru{
		dtl: dtl,
		canAccept: func(list []harmonytask.TaskID, e *harmonytask.TaskEngine) (*harmonytask.TaskID, error) {
			return nil, nil
		},
		adder: func(add harmonytask.AddTaskFunc) {
			for _, vTmp := range []string{"A", "B"} {
				v := vTmp
				add(func(tID harmonytask.TaskID, tx *harmonydb.Tx) (bool, error) {
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
		canAccept: func(list []harmonytask.TaskID, e *harmonytask.TaskEngine) (*harmonytask.TaskID, error) {
			return &list[0], nil
		},
		do: func(tID harmonytask.TaskID, stillOwned func() bool) (done bool, err error) {
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
		harmonytask.POLL_DURATION = time.Millisecond * 100
		sender, err := harmonytask.New(cdb, []harmonytask.TaskInterface{senderParty}, "test:1")
		require.NoError(t, err)
		worker, err := harmonytask.New(cdb, []harmonytask.TaskInterface{workerParty}, "test:2")
		require.NoError(t, err)
		time.Sleep(time.Second) // do the work. FLAKYNESS RISK HERE.
		sender.GracefullyTerminate(time.Second * 5)
		worker.GracefullyTerminate(time.Second * 5)
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

		harmonytask.POLL_DURATION = time.Millisecond * 100
		harmonytask.CLEANUP_FREQUENCY = time.Millisecond * 100
		var dest []string
		worker, err := harmonytask.New(cdb, []harmonytask.TaskInterface{fooLetterSaver(t, cdb, &dest)}, "test:2")
		require.ErrorIs(t, err, nil)
		time.Sleep(time.Second) // do the work. FLAKYNESS RISK HERE.
		worker.GracefullyTerminate(time.Second * 5)
		require.Equal(t, []string{"M"}, dest)
	})
}

func TestTaskRetry(t *testing.T) {
	//t.Parallel()
	withDbSetup(t, func(m *kit.TestMiner) {
		cdb := m.BaseAPI.(*impl.StorageMinerAPI).HarmonyDB
		senderParty := fooLetterAdder(t, cdb)
		harmonytask.POLL_DURATION = time.Millisecond * 100
		sender, err := harmonytask.New(cdb, []harmonytask.TaskInterface{senderParty}, "test:1")
		require.NoError(t, err)

		alreadyFailed := map[string]bool{}
		var dest []string
		fails2xPerMsg := &passthru{
			dtl: dtl,
			canAccept: func(list []harmonytask.TaskID, e *harmonytask.TaskEngine) (*harmonytask.TaskID, error) {
				return &list[0], nil
			},
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
				dest = append(dest, content)
				return true, nil
			},
		}
		rcv, err := harmonytask.New(cdb, []harmonytask.TaskInterface{fails2xPerMsg}, "test:2")
		require.NoError(t, err)
		time.Sleep(time.Second)
		sender.GracefullyTerminate(time.Hour)
		rcv.GracefullyTerminate(time.Hour)
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
