package sealer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/ipfs/go-datastore"
	syncds "github.com/ipfs/go-datastore/sync"
	logging "github.com/ipfs/go-log/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	ffi "github.com/filecoin-project/filecoin-ffi"
	"github.com/filecoin-project/go-paramfetch"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/proof"
	"github.com/filecoin-project/go-statestore"
	proof7 "github.com/filecoin-project/specs-actors/v7/actors/runtime/proof"

	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/storage/paths"
	"github.com/filecoin-project/lotus/storage/sealer/ffiwrapper"
	"github.com/filecoin-project/lotus/storage/sealer/fsutil"
	"github.com/filecoin-project/lotus/storage/sealer/sealtasks"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
)

func init() {
	logging.SetAllLoggers(logging.LevelDebug)
}

type testStorage storiface.StorageConfig

func (t testStorage) DiskUsage(path string) (int64, error) {
	return 1, nil // close enough
}

func newTestStorage(t *testing.T) *testStorage {
	tp, err := os.MkdirTemp(os.TempDir(), "sealer-test-")
	require.NoError(t, err)

	{
		b, err := json.MarshalIndent(&storiface.LocalStorageMeta{
			ID:       storiface.ID(uuid.New().String()),
			Weight:   1,
			CanSeal:  true,
			CanStore: true,
		}, "", "  ")
		require.NoError(t, err)

		err = os.WriteFile(filepath.Join(tp, "sectorstore.json"), b, 0644)
		require.NoError(t, err)
	}

	return &testStorage{
		StoragePaths: []storiface.LocalPath{
			{Path: tp},
		},
	}
}

func (t testStorage) cleanup() {
	noCleanup := os.Getenv("LOTUS_TEST_NO_CLEANUP") != ""
	for _, path := range t.StoragePaths {
		if noCleanup {
			fmt.Printf("Not cleaning up test storage at %s\n", path)
			continue
		}
		if err := os.RemoveAll(path.Path); err != nil {
			fmt.Println("Cleanup error:", err)
		}
	}
}

func (t testStorage) GetStorage() (storiface.StorageConfig, error) {
	return storiface.StorageConfig(t), nil
}

func (t *testStorage) SetStorage(f func(*storiface.StorageConfig)) error {
	f((*storiface.StorageConfig)(t))
	return nil
}

func (t *testStorage) Stat(path string) (fsutil.FsStat, error) {
	return fsutil.Statfs(path)
}

var _ paths.LocalStorage = &testStorage{}

func newTestMgr(ctx context.Context, t *testing.T, ds datastore.Datastore) (*Manager, *paths.Local, *paths.Remote, *paths.MemIndex, func()) {
	st := newTestStorage(t)

	si := paths.NewMemIndex(nil)

	lstor, err := paths.NewLocal(ctx, st, si, nil)
	require.NoError(t, err)

	prover, err := ffiwrapper.New(&readonlyProvider{stor: lstor, index: si})
	require.NoError(t, err)

	stor := paths.NewRemote(lstor, si, nil, 6000, &paths.DefaultPartialFileHandler{})

	sh, err := newScheduler(ctx, "")
	require.NoError(t, err)

	m := &Manager{
		ls:         st,
		storage:    stor,
		localStore: lstor,
		remoteHnd:  &paths.FetchHandler{Local: lstor},
		index:      si,

		sched:            sh,
		windowPoStSched:  newPoStScheduler(sealtasks.TTGenerateWindowPoSt),
		winningPoStSched: newPoStScheduler(sealtasks.TTGenerateWinningPoSt),

		localProver: prover,

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
	m, lstor, _, _, cleanup := newTestMgr(ctx, t, syncds.MutexWrap(datastore.NewMapDatastore()))
	defer cleanup()

	localTasks := []sealtasks.TaskType{
		sealtasks.TTAddPiece, sealtasks.TTPreCommit1, sealtasks.TTCommit1, sealtasks.TTFinalize, sealtasks.TTFinalizeUnsealed, sealtasks.TTFetch,
	}

	err := m.AddWorker(ctx, newTestWorker(WorkerConfig{
		TaskTypes: localTasks,
	}, lstor, m))
	require.NoError(t, err)

	sid := storiface.SectorRef{
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

type Reader struct{}

func (Reader) Read(out []byte) (int, error) {
	for i := range out {
		out[i] = 0
	}
	return len(out), nil
}

type NullReader struct {
	*io.LimitedReader
}

func NewNullReader(size abi.UnpaddedPieceSize) io.Reader {
	return &NullReader{(io.LimitReader(&Reader{}, int64(size))).(*io.LimitedReader)}
}

func (m NullReader) NullBytes() int64 {
	return m.N
}

func TestMain(m *testing.M) {
	err := paramfetch.GetParams(context.TODO(), build.ParametersJSON(), build.SrsJSON(), uint64(2048))
	if err != nil {
		panic(xerrors.Errorf("failed to acquire Groth parameters for 2KiB sectors: %w", err))
	}

	code := m.Run()
	os.Exit(code)
}

func TestSnapDeals(t *testing.T) {
	logging.SetAllLoggers(logging.LevelWarn)
	ctx := context.Background()
	m, lstor, stor, idx, cleanup := newTestMgr(ctx, t, syncds.MutexWrap(datastore.NewMapDatastore()))
	defer cleanup()

	localTasks := []sealtasks.TaskType{
		sealtasks.TTAddPiece, sealtasks.TTPreCommit1, sealtasks.TTPreCommit2, sealtasks.TTCommit1, sealtasks.TTCommit2, sealtasks.TTFinalize,
		sealtasks.TTFetch, sealtasks.TTReplicaUpdate, sealtasks.TTProveReplicaUpdate1, sealtasks.TTProveReplicaUpdate2, sealtasks.TTUnseal,
		sealtasks.TTRegenSectorKey, sealtasks.TTFinalizeUnsealed,
	}
	wds := syncds.MutexWrap(datastore.NewMapDatastore())

	w := NewLocalWorker(WorkerConfig{TaskTypes: localTasks}, stor, lstor, idx, m, statestore.New(wds))
	err := m.AddWorker(ctx, w)
	require.NoError(t, err)

	proofType := abi.RegisteredSealProof_StackedDrg2KiBV1
	ptStr := os.Getenv("LOTUS_TEST_SNAP_DEALS_PROOF_TYPE")
	switch ptStr {
	case "2k":
	case "8M":
		proofType = abi.RegisteredSealProof_StackedDrg8MiBV1
	case "512M":
		proofType = abi.RegisteredSealProof_StackedDrg512MiBV1
	case "32G":
		proofType = abi.RegisteredSealProof_StackedDrg32GiBV1
	case "64G":
		proofType = abi.RegisteredSealProof_StackedDrg64GiBV1
	default:
		log.Warn("Unspecified proof type, make sure to set LOTUS_TEST_SNAP_DEALS_PROOF_TYPE to '2k', '8M', '512M', '32G' or '64G'")
		log.Warn("Continuing test with 2k sectors")
	}

	sid := storiface.SectorRef{
		ID:        abi.SectorID{Miner: 1000, Number: 1},
		ProofType: proofType,
	}
	ss, err := proofType.SectorSize()
	require.NoError(t, err)

	unpaddedSectorSize := abi.PaddedPieceSize(ss).Unpadded()

	// Pack sector with no pieces
	p0, err := m.AddPiece(ctx, sid, nil, unpaddedSectorSize, NewNullReader(unpaddedSectorSize))
	require.NoError(t, err)
	ccPieces := []abi.PieceInfo{p0}

	// Precommit and Seal a CC sector
	fmt.Printf("PC1\n")
	ticket := abi.SealRandomness{9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9}
	pc1Out, err := m.SealPreCommit1(ctx, sid, ticket, ccPieces)
	require.NoError(t, err)
	fmt.Printf("PC2\n")
	pc2Out, err := m.SealPreCommit2(ctx, sid, pc1Out)
	require.NoError(t, err)

	// Now do a snap deals replica update
	sectorKey := pc2Out.Sealed

	// Two pieces each half the size of the sector
	unpaddedPieceSize := unpaddedSectorSize / 2
	p1, err := m.AddPiece(ctx, sid, nil, unpaddedPieceSize, strings.NewReader(strings.Repeat("k", int(unpaddedPieceSize))))
	require.NoError(t, err)
	require.Equal(t, unpaddedPieceSize.Padded(), p1.Size)

	p2, err := m.AddPiece(ctx, sid, []abi.UnpaddedPieceSize{p1.Size.Unpadded()}, unpaddedPieceSize, strings.NewReader(strings.Repeat("j", int(unpaddedPieceSize))))
	require.NoError(t, err)
	require.Equal(t, unpaddedPieceSize.Padded(), p1.Size)

	pieces := []abi.PieceInfo{p1, p2}
	fmt.Printf("RU\n")
	startRU := time.Now()
	out, err := m.ReplicaUpdate(ctx, sid, pieces)
	require.NoError(t, err)
	fmt.Printf("RU duration (%s): %s\n", ss.ShortString(), time.Since(startRU))

	updateProofType, err := sid.ProofType.RegisteredUpdateProof()
	require.NoError(t, err)
	require.NotNil(t, out)
	fmt.Printf("PR1\n")
	startPR1 := time.Now()
	vanillaProofs, err := m.ProveReplicaUpdate1(ctx, sid, sectorKey, out.NewSealed, out.NewUnsealed)
	require.NoError(t, err)
	require.NotNil(t, vanillaProofs)
	fmt.Printf("PR1 duration (%s): %s\n", ss.ShortString(), time.Since(startPR1))
	fmt.Printf("PR2\n")
	startPR2 := time.Now()
	proof, err := m.ProveReplicaUpdate2(ctx, sid, sectorKey, out.NewSealed, out.NewUnsealed, vanillaProofs)
	require.NoError(t, err)
	require.NotNil(t, proof)
	fmt.Printf("PR2 duration (%s): %s\n", ss.ShortString(), time.Since(startPR2))

	vInfo := proof7.ReplicaUpdateInfo{
		Proof:                proof,
		UpdateProofType:      updateProofType,
		OldSealedSectorCID:   sectorKey,
		NewSealedSectorCID:   out.NewSealed,
		NewUnsealedSectorCID: out.NewUnsealed,
	}
	pass, err := ffiwrapper.ProofVerifier.VerifyReplicaUpdate(vInfo)
	require.NoError(t, err)
	assert.True(t, pass)

	fmt.Printf("Decode\n")
	// Remove unsealed data and decode for retrieval
	require.NoError(t, m.ReleaseUnsealed(ctx, sid, nil))
	startDecode := time.Now()
	require.NoError(t, m.SectorsUnsealPiece(ctx, sid, 0, p1.Size.Unpadded(), ticket, &out.NewUnsealed))
	fmt.Printf("Decode duration (%s): %s\n", ss.ShortString(), time.Since(startDecode))

	// Remove just the first piece and decode for retrieval
	require.NoError(t, m.ReleaseUnsealed(ctx, sid, []storiface.Range{{Offset: p1.Size.Unpadded(), Size: p2.Size.Unpadded()}}))
	require.NoError(t, m.SectorsUnsealPiece(ctx, sid, 0, p1.Size.Unpadded(), ticket, &out.NewUnsealed))

	fmt.Printf("GSK\n")
	require.NoError(t, m.ReleaseSectorKey(ctx, sid))
	startGSK := time.Now()
	require.NoError(t, m.GenerateSectorKeyFromData(ctx, sid, out.NewUnsealed))
	fmt.Printf("GSK duration (%s): %s\n", ss.ShortString(), time.Since(startGSK))

	fmt.Printf("Remove data\n")
	require.NoError(t, m.ReleaseUnsealed(ctx, sid, nil))
	fmt.Printf("Release Sector Key\n")
	require.NoError(t, m.ReleaseSectorKey(ctx, sid))
	fmt.Printf("Unseal Replica\n")
	require.NoError(t, m.SectorsUnsealPiece(ctx, sid, 0, p1.Size.Unpadded(), ticket, &out.NewUnsealed))
}

func TestSnarkPackV2(t *testing.T) {
	logging.SetAllLoggers(logging.LevelWarn)
	ctx := context.Background()
	m, lstor, stor, idx, cleanup := newTestMgr(ctx, t, syncds.MutexWrap(datastore.NewMapDatastore()))
	defer cleanup()

	localTasks := []sealtasks.TaskType{
		sealtasks.TTAddPiece, sealtasks.TTPreCommit1, sealtasks.TTPreCommit2, sealtasks.TTCommit1, sealtasks.TTCommit2, sealtasks.TTFinalize,
		sealtasks.TTFetch, sealtasks.TTReplicaUpdate, sealtasks.TTProveReplicaUpdate1, sealtasks.TTProveReplicaUpdate2, sealtasks.TTUnseal,
		sealtasks.TTRegenSectorKey, sealtasks.TTFinalizeUnsealed,
	}
	wds := syncds.MutexWrap(datastore.NewMapDatastore())

	w := NewLocalWorker(WorkerConfig{TaskTypes: localTasks}, stor, lstor, idx, m, statestore.New(wds))
	err := m.AddWorker(ctx, w)
	require.NoError(t, err)

	proofType := abi.RegisteredSealProof_StackedDrg2KiBV1
	ptStr := os.Getenv("LOTUS_TEST_SNAP_DEALS_PROOF_TYPE")
	switch ptStr {
	case "2k":
	case "8M":
		proofType = abi.RegisteredSealProof_StackedDrg8MiBV1
	case "512M":
		proofType = abi.RegisteredSealProof_StackedDrg512MiBV1
	case "32G":
		proofType = abi.RegisteredSealProof_StackedDrg32GiBV1
	case "64G":
		proofType = abi.RegisteredSealProof_StackedDrg64GiBV1
	default:
		log.Warn("Unspecified proof type, make sure to set LOTUS_TEST_SNAP_DEALS_PROOF_TYPE to '2k', '8M', '512M', '32G' or '64G'")
		log.Warn("Continuing test with 2k sectors")
	}

	mid := abi.ActorID(1000)

	sid1 := storiface.SectorRef{
		ID:        abi.SectorID{Miner: mid, Number: 1},
		ProofType: proofType,
	}

	sid2 := storiface.SectorRef{
		ID:        abi.SectorID{Miner: mid, Number: 2},
		ProofType: proofType,
	}

	ss, err := proofType.SectorSize()
	require.NoError(t, err)

	unpaddedSectorSize := abi.PaddedPieceSize(ss).Unpadded()

	// Pack sector with no pieces
	p1, err := m.AddPiece(ctx, sid1, nil, unpaddedSectorSize, NewNullReader(unpaddedSectorSize))
	require.NoError(t, err)
	ccPieces1 := []abi.PieceInfo{p1}

	p2, err := m.AddPiece(ctx, sid2, nil, unpaddedSectorSize, NewNullReader(unpaddedSectorSize))
	require.NoError(t, err)
	ccPieces2 := []abi.PieceInfo{p2}

	// Precommit and Seal 2 CC sectors
	fmt.Printf("PC1\n")

	ticket1 := abi.SealRandomness{9, 9, 9, 9, 9, 9, 9, 9}
	ticket2 := abi.SealRandomness{1, 9, 8, 9, 1, 9, 8, 9}
	interactiveRandomness1 := abi.InteractiveSealRandomness{1, 9, 2, 1, 2, 5, 3, 0}
	interactiveRandomness2 := abi.InteractiveSealRandomness{1, 5, 2, 2, 1, 5, 2, 2}

	pc1Out1, err := m.SealPreCommit1(ctx, sid1, ticket1, ccPieces1)
	require.NoError(t, err)
	pc1Out2, err := m.SealPreCommit1(ctx, sid2, ticket2, ccPieces2)
	require.NoError(t, err)

	fmt.Printf("PC2\n")

	pc2Out1, err := m.SealPreCommit2(ctx, sid1, pc1Out1)
	require.NoError(t, err)
	pc2Out2, err := m.SealPreCommit2(ctx, sid2, pc1Out2)
	require.NoError(t, err)

	// Commit the sector

	fmt.Printf("C1\n")

	c1Out1, err := m.SealCommit1(ctx, sid1, ticket1, interactiveRandomness1, ccPieces1, pc2Out1)
	require.NoError(t, err)
	c1Out2, err := m.SealCommit1(ctx, sid2, ticket2, interactiveRandomness2, ccPieces2, pc2Out2)
	require.NoError(t, err)

	fmt.Printf("C2\n")

	c2Out1, err := m.SealCommit2(ctx, sid1, c1Out1)
	require.NoError(t, err)
	c2Out2, err := m.SealCommit2(ctx, sid2, c1Out2)
	require.NoError(t, err)

	fmt.Println("Aggregate")
	agg, err := ffi.AggregateSealProofs(proof.AggregateSealVerifyProofAndInfos{
		Miner:          mid,
		SealProof:      proofType,
		AggregateProof: abi.RegisteredAggregationProof_SnarkPackV2,
		Infos: []proof.AggregateSealVerifyInfo{{
			Number:                sid1.ID.Number,
			Randomness:            ticket1,
			InteractiveRandomness: interactiveRandomness1,
			SealedCID:             pc2Out1.Sealed,
			UnsealedCID:           pc2Out1.Unsealed,
		}, {
			Number:                sid2.ID.Number,
			Randomness:            ticket2,
			InteractiveRandomness: interactiveRandomness2,
			SealedCID:             pc2Out2.Sealed,
			UnsealedCID:           pc2Out2.Unsealed,
		}},
	}, [][]byte{c2Out1, c2Out2})
	require.NoError(t, err)

	fmt.Println("Verifying aggregate")
	ret, err := ffi.VerifyAggregateSeals(proof.AggregateSealVerifyProofAndInfos{
		Miner:          mid,
		SealProof:      proofType,
		AggregateProof: abi.RegisteredAggregationProof_SnarkPackV2,
		Proof:          agg,
		Infos: []proof.AggregateSealVerifyInfo{{
			Number:                sid1.ID.Number,
			Randomness:            ticket1,
			InteractiveRandomness: interactiveRandomness1,
			SealedCID:             pc2Out1.Sealed,
			UnsealedCID:           pc2Out1.Unsealed,
		}, {
			Number:                sid2.ID.Number,
			Randomness:            ticket2,
			InteractiveRandomness: interactiveRandomness2,
			SealedCID:             pc2Out2.Sealed,
			UnsealedCID:           pc2Out2.Unsealed,
		}},
	})
	require.NoError(t, err)
	require.True(t, ret, "proof should be good")
}

func TestRedoPC1(t *testing.T) {
	logging.SetAllLoggers(logging.LevelDebug)

	ctx := context.Background()
	m, lstor, _, _, cleanup := newTestMgr(ctx, t, syncds.MutexWrap(datastore.NewMapDatastore()))
	defer cleanup()

	localTasks := []sealtasks.TaskType{
		sealtasks.TTAddPiece, sealtasks.TTPreCommit1, sealtasks.TTCommit1, sealtasks.TTFinalize, sealtasks.TTFinalizeUnsealed, sealtasks.TTFetch,
	}

	tw := newTestWorker(WorkerConfig{
		TaskTypes: localTasks,
	}, lstor, m)

	err := m.AddWorker(ctx, tw)
	require.NoError(t, err)

	sid := storiface.SectorRef{
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

			ds := syncds.MutexWrap(datastore.NewMapDatastore())

			m, lstor, _, _, cleanup := newTestMgr(ctx, t, ds)
			defer cleanup()

			localTasks := []sealtasks.TaskType{
				sealtasks.TTAddPiece, sealtasks.TTPreCommit1, sealtasks.TTCommit1, sealtasks.TTFinalize, sealtasks.TTFinalizeUnsealed, sealtasks.TTFetch,
			}

			tw := newTestWorker(WorkerConfig{
				TaskTypes: localTasks,
			}, lstor, m)

			err := m.AddWorker(ctx, tw)
			require.NoError(t, err)

			sid := storiface.SectorRef{
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

	ds := syncds.MutexWrap(datastore.NewMapDatastore())

	m, lstor, stor, idx, cleanup := newTestMgr(ctx, t, ds)
	defer cleanup()

	localTasks := []sealtasks.TaskType{
		sealtasks.TTAddPiece, sealtasks.TTFetch,
	}

	wds := syncds.MutexWrap(datastore.NewMapDatastore())

	arch := make(chan chan apres)
	w := NewLocalWorkerWithExecutor(func(_ *LocalWorker) (storiface.Storage, error) {
		return &testExec{apch: arch}, nil
	}, WorkerConfig{
		TaskTypes: localTasks,
	}, os.LookupEnv, stor, lstor, idx, m, statestore.New(wds))

	err := m.AddWorker(ctx, w)
	require.NoError(t, err)

	sid := storiface.SectorRef{
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
		if len(m.WorkerStats(ctx)) == 0 {
			break
		}

		time.Sleep(time.Millisecond * 3)
	}

	// restart the worker
	w = NewLocalWorkerWithExecutor(func(_ *LocalWorker) (storiface.Storage, error) {
		return &testExec{apch: arch}, nil
	}, WorkerConfig{
		TaskTypes: localTasks,
	}, os.LookupEnv, stor, lstor, idx, m, statestore.New(wds))

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
	paths.HeartbeatInterval = 5 * time.Millisecond

	ctx, done := context.WithCancel(context.Background())
	defer done()

	ds := syncds.MutexWrap(datastore.NewMapDatastore())

	m, lstor, stor, idx, cleanup := newTestMgr(ctx, t, ds)
	defer cleanup()

	localTasks := []sealtasks.TaskType{
		sealtasks.TTAddPiece, sealtasks.TTPreCommit1, sealtasks.TTCommit1, sealtasks.TTFinalize, sealtasks.TTFinalizeUnsealed, sealtasks.TTFetch,
	}

	wds := datastore.NewMapDatastore()

	arch := make(chan chan apres)
	w := NewLocalWorkerWithExecutor(func(_ *LocalWorker) (storiface.Storage, error) {
		return &testExec{apch: arch}, nil
	}, WorkerConfig{
		TaskTypes: localTasks,
	}, os.LookupEnv, stor, lstor, idx, m, statestore.New(wds))

	err := m.AddWorker(ctx, w)
	require.NoError(t, err)

	time.Sleep(time.Millisecond * 100)

	i, _ := m.sched.Info(ctx)
	require.Len(t, i.(SchedDiagInfo).OpenWindows, 2)

	// disable
	atomic.StoreInt64(&w.testDisable, 1)

	for i := 0; i < 100; i++ {
		if !m.WorkerStats(ctx)[w.session].Enabled {
			break
		}

		time.Sleep(time.Millisecond * 3)
	}
	require.False(t, m.WorkerStats(ctx)[w.session].Enabled)

	i, _ = m.sched.Info(ctx)
	require.Len(t, i.(SchedDiagInfo).OpenWindows, 0)

	// reenable
	atomic.StoreInt64(&w.testDisable, 0)

	for i := 0; i < 100; i++ {
		if m.WorkerStats(ctx)[w.session].Enabled {
			break
		}

		time.Sleep(time.Millisecond * 3)
	}
	require.True(t, m.WorkerStats(ctx)[w.session].Enabled)

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

func TestResUse(t *testing.T) {
	logging.SetAllLoggers(logging.LevelDebug)

	ctx, done := context.WithCancel(context.Background())
	defer done()

	ds := syncds.MutexWrap(datastore.NewMapDatastore())

	m, lstor, stor, idx, cleanup := newTestMgr(ctx, t, ds)
	defer cleanup()

	localTasks := []sealtasks.TaskType{
		sealtasks.TTAddPiece, sealtasks.TTFetch,
	}

	wds := syncds.MutexWrap(datastore.NewMapDatastore())

	arch := make(chan chan apres)
	w := NewLocalWorkerWithExecutor(func(_ *LocalWorker) (storiface.Storage, error) {
		return &testExec{apch: arch}, nil
	}, WorkerConfig{
		TaskTypes: localTasks,
	}, func(s string) (string, bool) {
		return "", false
	}, stor, lstor, idx, m, statestore.New(wds))

	err := m.AddWorker(ctx, w)
	require.NoError(t, err)

	sid := storiface.SectorRef{
		ID:        abi.SectorID{Miner: 1000, Number: 1},
		ProofType: abi.RegisteredSealProof_StackedDrg2KiBV1,
	}

	go func() {
		_, err := m.AddPiece(ctx, sid, nil, 1016, strings.NewReader(strings.Repeat("testthis", 127)))
		require.Error(t, err)
	}()

l:
	for {
		st := m.WorkerStats(ctx)
		require.Len(t, st, 1)
		for _, w := range st {
			if w.MemUsedMax > 0 {
				break l
			}
			time.Sleep(time.Millisecond)
		}
	}

	st := m.WorkerStats(ctx)
	require.Len(t, st, 1)
	for _, w := range st {
		require.Equal(t, storiface.ResourceTable[sealtasks.TTAddPiece][abi.RegisteredSealProof_StackedDrg2KiBV1].MaxMemory, w.MemUsedMax)
	}
}

func TestResOverride(t *testing.T) {
	logging.SetAllLoggers(logging.LevelDebug)

	ctx, done := context.WithCancel(context.Background())
	defer done()

	ds := syncds.MutexWrap(datastore.NewMapDatastore())

	m, lstor, stor, idx, cleanup := newTestMgr(ctx, t, ds)
	defer cleanup()

	localTasks := []sealtasks.TaskType{
		sealtasks.TTAddPiece, sealtasks.TTFetch,
	}

	wds := syncds.MutexWrap(datastore.NewMapDatastore())

	arch := make(chan chan apres)
	w := NewLocalWorkerWithExecutor(func(_ *LocalWorker) (storiface.Storage, error) {
		return &testExec{apch: arch}, nil
	}, WorkerConfig{
		TaskTypes: localTasks,
	}, func(s string) (string, bool) {
		if s == "AP_2K_MAX_MEMORY" {
			return "99999", true
		}

		return "", false
	}, stor, lstor, idx, m, statestore.New(wds))

	err := m.AddWorker(ctx, w)
	require.NoError(t, err)

	sid := storiface.SectorRef{
		ID:        abi.SectorID{Miner: 1000, Number: 1},
		ProofType: abi.RegisteredSealProof_StackedDrg2KiBV1,
	}

	go func() {
		_, err := m.AddPiece(ctx, sid, nil, 1016, strings.NewReader(strings.Repeat("testthis", 127)))
		require.Error(t, err)
	}()

l:
	for {
		st := m.WorkerStats(ctx)
		require.Len(t, st, 1)
		for _, w := range st {
			if w.MemUsedMax > 0 {
				break l
			}
			time.Sleep(time.Millisecond)
		}
	}

	st := m.WorkerStats(ctx)
	require.Len(t, st, 1)
	for _, w := range st {
		require.Equal(t, uint64(99999), w.MemUsedMax)
	}
}
