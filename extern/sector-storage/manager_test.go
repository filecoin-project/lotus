package sectorstorage

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/ipfs/go-datastore"
	logging "github.com/ipfs/go-log/v2"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-statestore"
	"github.com/filecoin-project/specs-storage/storage"

	"github.com/filecoin-project/lotus/extern/sector-storage/ffiwrapper"
	"github.com/filecoin-project/lotus/extern/sector-storage/fsutil"
	"github.com/filecoin-project/lotus/extern/sector-storage/sealtasks"
	"github.com/filecoin-project/lotus/extern/sector-storage/stores"
	"github.com/filecoin-project/lotus/extern/sector-storage/storiface"
)

func init() {
	logging.SetAllLoggers(logging.LevelDebug)
}

type testStorage stores.StorageConfig

func (t testStorage) DiskUsage(path string) (int64, error) {
	return 1, nil // close enough
}

func newTestStorage(t *testing.T) *testStorage {
	tp, err := ioutil.TempDir(os.TempDir(), "sector-storage-test-")
	require.NoError(t, err)

	{
		b, err := json.MarshalIndent(&stores.LocalStorageMeta{
			ID:       stores.ID(uuid.New().String()),
			Weight:   1,
			CanSeal:  true,
			CanStore: true,
		}, "", "  ")
		require.NoError(t, err)

		err = ioutil.WriteFile(filepath.Join(tp, "sectorstore.json"), b, 0644)
		require.NoError(t, err)
	}

	return &testStorage{
		StoragePaths: []stores.LocalPath{
			{Path: tp},
		},
	}
}

func (t testStorage) cleanup() {
	for _, path := range t.StoragePaths {
		if err := os.RemoveAll(path.Path); err != nil {
			fmt.Println("Cleanup error:", err)
		}
	}
}

func (t testStorage) GetStorage() (stores.StorageConfig, error) {
	return stores.StorageConfig(t), nil
}

func (t *testStorage) SetStorage(f func(*stores.StorageConfig)) error {
	f((*stores.StorageConfig)(t))
	return nil
}

func (t *testStorage) Stat(path string) (fsutil.FsStat, error) {
	return fsutil.Statfs(path)
}

var _ stores.LocalStorage = &testStorage{}

func newTestMgr(ctx context.Context, t *testing.T, ds datastore.Datastore) (*Manager, *stores.Local, *stores.Remote, *stores.Index, func()) {
	st := newTestStorage(t)

	si := stores.NewIndex()

	lstor, err := stores.NewLocal(ctx, st, si, nil)
	require.NoError(t, err)

	prover, err := ffiwrapper.New(&readonlyProvider{stor: lstor, index: si})
	require.NoError(t, err)

	stor := stores.NewRemote(lstor, si, nil, 6000, &stores.DefaultPartialFileHandler{})

	m := &Manager{
		ls:         st,
		storage:    stor,
		localStore: lstor,
		remoteHnd:  &stores.FetchHandler{Local: lstor},
		index:      si,

		sched: newScheduler(),

		Prover: prover,

		work:       statestore.New(ds),
		callToWork: map[storiface.CallID]WorkID{},
		callRes:    map[storiface.CallID]chan result{},
		results:    map[WorkID]result{},
		waitRes:    map[WorkID]chan struct{}{},
	}

	m.setupWorkTracker()

	go m.sched.runSched()

	return m, lstor, stor, si, st.cleanup
}

func TestSimple(t *testing.T) {
	logging.SetAllLoggers(logging.LevelDebug)

	ctx := context.Background()
	m, lstor, _, _, cleanup := newTestMgr(ctx, t, datastore.NewMapDatastore())
	defer cleanup()

	localTasks := []sealtasks.TaskType{
		sealtasks.TTAddPiece, sealtasks.TTPreCommit1, sealtasks.TTCommit1, sealtasks.TTFinalize, sealtasks.TTFetch,
	}

	err := m.AddWorker(ctx, newTestWorker(WorkerConfig{
		TaskTypes: localTasks,
	}, lstor, m))
	require.NoError(t, err)

	sid := storage.SectorRef{
		ID:        abi.SectorID{Miner: 1000, Number: 1},
		ProofType: abi.RegisteredSealProof_StackedDrg2KiBV1,
	}

	pi, err := m.AddPiece(ctx, sid, nil, 1016, strings.NewReader(strings.Repeat("testthis", 127)))
	require.NoError(t, err)
	require.Equal(t, abi.PaddedPieceSize(1024), pi.Size)

	piz, err := m.AddPiece(ctx, sid, nil, 1016, bytes.NewReader(make([]byte, 1016)[:]))
	require.NoError(t, err)
	require.Equal(t, abi.PaddedPieceSize(1024), piz.Size)

	pieces := []abi.PieceInfo{pi, piz}

	ticket := abi.SealRandomness{9, 9, 9, 9, 9, 9, 9, 9}

	_, err = m.SealPreCommit1(ctx, sid, ticket, pieces)
	require.NoError(t, err)
}

func TestRedoPC1(t *testing.T) {
	logging.SetAllLoggers(logging.LevelDebug)

	ctx := context.Background()
	m, lstor, _, _, cleanup := newTestMgr(ctx, t, datastore.NewMapDatastore())
	defer cleanup()

	localTasks := []sealtasks.TaskType{
		sealtasks.TTAddPiece, sealtasks.TTPreCommit1, sealtasks.TTCommit1, sealtasks.TTFinalize, sealtasks.TTFetch,
	}

	tw := newTestWorker(WorkerConfig{
		TaskTypes: localTasks,
	}, lstor, m)

	err := m.AddWorker(ctx, tw)
	require.NoError(t, err)

	sid := storage.SectorRef{
		ID:        abi.SectorID{Miner: 1000, Number: 1},
		ProofType: abi.RegisteredSealProof_StackedDrg2KiBV1,
	}

	pi, err := m.AddPiece(ctx, sid, nil, 1016, strings.NewReader(strings.Repeat("testthis", 127)))
	require.NoError(t, err)
	require.Equal(t, abi.PaddedPieceSize(1024), pi.Size)

	piz, err := m.AddPiece(ctx, sid, nil, 1016, bytes.NewReader(make([]byte, 1016)[:]))
	require.NoError(t, err)
	require.Equal(t, abi.PaddedPieceSize(1024), piz.Size)

	pieces := []abi.PieceInfo{pi, piz}

	ticket := abi.SealRandomness{9, 9, 9, 9, 9, 9, 9, 9}

	_, err = m.SealPreCommit1(ctx, sid, ticket, pieces)
	require.NoError(t, err)

	// tell mock ffi that we expect PC1 again
	require.NoError(t, tw.mockSeal.ForceState(sid, 0)) // sectorPacking

	_, err = m.SealPreCommit1(ctx, sid, ticket, pieces)
	require.NoError(t, err)

	require.Equal(t, 2, tw.pc1s)
}

// Manager restarts in the middle of a task, restarts it, it completes
func TestRestartManager(t *testing.T) {
	test := func(returnBeforeCall bool) func(*testing.T) {
		return func(t *testing.T) {
			logging.SetAllLoggers(logging.LevelDebug)

			ctx, done := context.WithCancel(context.Background())
			defer done()

			ds := datastore.NewMapDatastore()

			m, lstor, _, _, cleanup := newTestMgr(ctx, t, ds)
			defer cleanup()

			localTasks := []sealtasks.TaskType{
				sealtasks.TTAddPiece, sealtasks.TTPreCommit1, sealtasks.TTCommit1, sealtasks.TTFinalize, sealtasks.TTFetch,
			}

			tw := newTestWorker(WorkerConfig{
				TaskTypes: localTasks,
			}, lstor, m)

			err := m.AddWorker(ctx, tw)
			require.NoError(t, err)

			sid := storage.SectorRef{
				ID:        abi.SectorID{Miner: 1000, Number: 1},
				ProofType: abi.RegisteredSealProof_StackedDrg2KiBV1,
			}

			pi, err := m.AddPiece(ctx, sid, nil, 1016, strings.NewReader(strings.Repeat("testthis", 127)))
			require.NoError(t, err)
			require.Equal(t, abi.PaddedPieceSize(1024), pi.Size)

			piz, err := m.AddPiece(ctx, sid, nil, 1016, bytes.NewReader(make([]byte, 1016)[:]))
			require.NoError(t, err)
			require.Equal(t, abi.PaddedPieceSize(1024), piz.Size)

			pieces := []abi.PieceInfo{pi, piz}

			ticket := abi.SealRandomness{0, 9, 9, 9, 9, 9, 9, 9}

			tw.pc1lk.Lock()
			tw.pc1wait = &sync.WaitGroup{}
			tw.pc1wait.Add(1)

			var cwg sync.WaitGroup
			cwg.Add(1)

			var perr error
			go func() {
				defer cwg.Done()
				_, perr = m.SealPreCommit1(ctx, sid, ticket, pieces)
			}()

			tw.pc1wait.Wait()

			require.NoError(t, m.Close(ctx))
			tw.ret = nil

			cwg.Wait()
			require.Error(t, perr)

			m, _, _, _, cleanup2 := newTestMgr(ctx, t, ds)
			defer cleanup2()

			tw.ret = m // simulate jsonrpc auto-reconnect
			err = m.AddWorker(ctx, tw)
			require.NoError(t, err)

			if returnBeforeCall {
				tw.pc1lk.Unlock()
				time.Sleep(100 * time.Millisecond)

				_, err = m.SealPreCommit1(ctx, sid, ticket, pieces)
			} else {
				done := make(chan struct{})
				go func() {
					defer close(done)
					_, err = m.SealPreCommit1(ctx, sid, ticket, pieces)
				}()

				time.Sleep(100 * time.Millisecond)
				tw.pc1lk.Unlock()
				<-done
			}

			require.NoError(t, err)

			require.Equal(t, 1, tw.pc1s)

			ws := m.WorkerJobs()
			require.Empty(t, ws)
		}
	}

	t.Run("callThenReturn", test(false))
	t.Run("returnThenCall", test(true))
}

// Worker restarts in the middle of a task, task fails after restart
func TestRestartWorker(t *testing.T) {
	logging.SetAllLoggers(logging.LevelDebug)

	ctx, done := context.WithCancel(context.Background())
	defer done()

	ds := datastore.NewMapDatastore()

	m, lstor, stor, idx, cleanup := newTestMgr(ctx, t, ds)
	defer cleanup()

	localTasks := []sealtasks.TaskType{
		sealtasks.TTAddPiece, sealtasks.TTPreCommit1, sealtasks.TTCommit1, sealtasks.TTFinalize, sealtasks.TTFetch,
	}

	wds := datastore.NewMapDatastore()

	arch := make(chan chan apres)
	w := newLocalWorker(func() (ffiwrapper.Storage, error) {
		return &testExec{apch: arch}, nil
	}, WorkerConfig{
		TaskTypes: localTasks,
	}, stor, lstor, idx, m, statestore.New(wds))

	err := m.AddWorker(ctx, w)
	require.NoError(t, err)

	sid := storage.SectorRef{
		ID:        abi.SectorID{Miner: 1000, Number: 1},
		ProofType: abi.RegisteredSealProof_StackedDrg2KiBV1,
	}

	apDone := make(chan struct{})

	go func() {
		defer close(apDone)

		_, err := m.AddPiece(ctx, sid, nil, 1016, strings.NewReader(strings.Repeat("testthis", 127)))
		require.Error(t, err)
	}()

	// kill the worker
	<-arch
	require.NoError(t, w.Close())

	for {
		if len(m.WorkerStats()) == 0 {
			break
		}

		time.Sleep(time.Millisecond * 3)
	}

	// restart the worker
	w = newLocalWorker(func() (ffiwrapper.Storage, error) {
		return &testExec{apch: arch}, nil
	}, WorkerConfig{
		TaskTypes: localTasks,
	}, stor, lstor, idx, m, statestore.New(wds))

	err = m.AddWorker(ctx, w)
	require.NoError(t, err)

	<-apDone

	time.Sleep(12 * time.Millisecond)
	uf, err := w.ct.unfinished()
	require.NoError(t, err)
	require.Empty(t, uf)
}

func TestReenableWorker(t *testing.T) {
	logging.SetAllLoggers(logging.LevelDebug)
	stores.HeartbeatInterval = 5 * time.Millisecond

	ctx, done := context.WithCancel(context.Background())
	defer done()

	ds := datastore.NewMapDatastore()

	m, lstor, stor, idx, cleanup := newTestMgr(ctx, t, ds)
	defer cleanup()

	localTasks := []sealtasks.TaskType{
		sealtasks.TTAddPiece, sealtasks.TTPreCommit1, sealtasks.TTCommit1, sealtasks.TTFinalize, sealtasks.TTFetch,
	}

	wds := datastore.NewMapDatastore()

	arch := make(chan chan apres)
	w := newLocalWorker(func() (ffiwrapper.Storage, error) {
		return &testExec{apch: arch}, nil
	}, WorkerConfig{
		TaskTypes: localTasks,
	}, stor, lstor, idx, m, statestore.New(wds))

	err := m.AddWorker(ctx, w)
	require.NoError(t, err)

	time.Sleep(time.Millisecond * 100)

	i, _ := m.sched.Info(ctx)
	require.Len(t, i.(SchedDiagInfo).OpenWindows, 2)

	// disable
	atomic.StoreInt64(&w.testDisable, 1)

	for i := 0; i < 100; i++ {
		if !m.WorkerStats()[w.session].Enabled {
			break
		}

		time.Sleep(time.Millisecond * 3)
	}
	require.False(t, m.WorkerStats()[w.session].Enabled)

	i, _ = m.sched.Info(ctx)
	require.Len(t, i.(SchedDiagInfo).OpenWindows, 0)

	// reenable
	atomic.StoreInt64(&w.testDisable, 0)

	for i := 0; i < 100; i++ {
		if m.WorkerStats()[w.session].Enabled {
			break
		}

		time.Sleep(time.Millisecond * 3)
	}
	require.True(t, m.WorkerStats()[w.session].Enabled)

	for i := 0; i < 100; i++ {
		info, _ := m.sched.Info(ctx)
		if len(info.(SchedDiagInfo).OpenWindows) != 0 {
			break
		}

		time.Sleep(time.Millisecond * 3)
	}

	i, _ = m.sched.Info(ctx)
	require.Len(t, i.(SchedDiagInfo).OpenWindows, 2)
}
