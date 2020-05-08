package sectorstorage

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/filecoin-project/sector-storage/sealtasks"
	logging "github.com/ipfs/go-log"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/specs-actors/actors/abi"

	"github.com/filecoin-project/sector-storage/ffiwrapper"
	"github.com/filecoin-project/sector-storage/stores"
)

type testStorage stores.StorageConfig

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

var _ stores.LocalStorage = &testStorage{}

func newTestMgr(ctx context.Context, t *testing.T) (*Manager, *stores.Local, *stores.Remote, *stores.Index) {
	st := newTestStorage(t)
	defer st.cleanup()

	si := stores.NewIndex()
	cfg := &ffiwrapper.Config{
		SealProofType: abi.RegisteredProof_StackedDRG2KiBSeal,
	}

	lstor, err := stores.NewLocal(ctx, st, si, nil)
	require.NoError(t, err)

	prover, err := ffiwrapper.New(&readonlyProvider{stor: lstor}, cfg)
	require.NoError(t, err)

	stor := stores.NewRemote(lstor, si, nil)

	m := &Manager{
		scfg: cfg,

		ls:         st,
		storage:    stor,
		localStore: lstor,
		remoteHnd:  &stores.FetchHandler{Local: lstor},
		index:      si,

		sched: newScheduler(cfg.SealProofType),

		Prover: prover,
	}

	go m.sched.runSched()

	return m, lstor, stor, si
}

func TestSimple(t *testing.T) {
	logging.SetAllLoggers(logging.LevelDebug)

	ctx := context.Background()
	m, lstor, _, _ := newTestMgr(ctx, t)

	localTasks := []sealtasks.TaskType{
		sealtasks.TTAddPiece, sealtasks.TTPreCommit1, sealtasks.TTCommit1, sealtasks.TTFinalize, sealtasks.TTFetch,
	}

	err := m.AddWorker(ctx, newTestWorker(WorkerConfig{
		SealProof: abi.RegisteredProof_StackedDRG2KiBSeal,
		TaskTypes: localTasks,
	}, lstor))
	require.NoError(t, err)

	sid := abi.SectorID{Miner: 1000, Number: 1}

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
