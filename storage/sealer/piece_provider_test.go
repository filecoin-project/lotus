package sealer

import (
	"bytes"
	"context"
	"crypto/rand"
	"io"
	"net"
	"net/http"
	"os"
	"testing"

	"github.com/gorilla/mux"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/namespace"
	ds_sync "github.com/ipfs/go-datastore/sync"
	logging "github.com/ipfs/go-log/v2"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-statestore"

	"github.com/filecoin-project/lotus/node/config"
	"github.com/filecoin-project/lotus/storage/paths"
	"github.com/filecoin-project/lotus/storage/sealer/sealtasks"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
)

// TestPieceProviderSimpleNoRemoteWorker verifies that the ReadPiece method works correctly
// only uses miner and does NOT use any remote worker.
func TestPieceProviderSimpleNoRemoteWorker(t *testing.T) {
	// Set up sector storage manager
	sealerCfg := config.SealerConfig{
		ParallelFetchLimit: 10,
		AllowAddPiece:      true,
		AllowPreCommit1:    true,
		AllowPreCommit2:    true,
		AllowCommit:        true,
		AllowUnseal:        true,
	}

	ppt := newPieceProviderTestHarness(t, sealerCfg, abi.RegisteredSealProof_StackedDrg8MiBV1)
	defer ppt.shutdown(t)

	// Create some padded data that aligns with the piece boundaries.
	pieceData := generatePieceData(8 * 127 * 1024 * 8)
	size := abi.UnpaddedPieceSize(len(pieceData))
	ppt.addPiece(t, pieceData)

	// read piece
	ppt.readPiece(t, storiface.UnpaddedByteIndex(0), size,
		false, pieceData)

	// pre-commit 1
	preCommit1 := ppt.preCommit1(t)

	// check if IsUnsealed -> true
	require.True(t, ppt.isUnsealed(t, storiface.UnpaddedByteIndex(0), size))
	// read piece
	ppt.readPiece(t, storiface.UnpaddedByteIndex(0), size,
		false, pieceData)

	// pre-commit 2
	ppt.preCommit2(t, preCommit1)

	// check if IsUnsealed -> true
	require.True(t, ppt.isUnsealed(t, storiface.UnpaddedByteIndex(0), size))
	// read piece
	ppt.readPiece(t, storiface.UnpaddedByteIndex(0), size,
		false, pieceData)

	// finalize -> nil here will remove unsealed file
	ppt.finalizeSector(t, nil)

	// check if IsUnsealed -> false
	require.False(t, ppt.isUnsealed(t, storiface.UnpaddedByteIndex(0), size))
	// Read the piece -> will have to unseal
	ppt.readPiece(t, storiface.UnpaddedByteIndex(0), size,
		true, pieceData)

	// check if IsUnsealed -> true
	require.True(t, ppt.isUnsealed(t, storiface.UnpaddedByteIndex(0), size))
	// read the piece -> will not have to unseal
	ppt.readPiece(t, storiface.UnpaddedByteIndex(0), size,
		false, pieceData)

}
func TestReadPieceRemoteWorkers(t *testing.T) {
	logging.SetAllLoggers(logging.LevelDebug)

	// miner's worker can only add pieces to an unsealed sector.
	sealerCfg := config.SealerConfig{
		ParallelFetchLimit: 10,
		AllowAddPiece:      true,
		AllowPreCommit1:    false,
		AllowPreCommit2:    false,
		AllowCommit:        false,
		AllowUnseal:        false,
	}

	// test harness for an 8M sector.
	ppt := newPieceProviderTestHarness(t, sealerCfg, abi.RegisteredSealProof_StackedDrg8MiBV1)
	defer ppt.shutdown(t)

	// worker 2 will ONLY help with the sealing by first fetching
	// the unsealed file from the miner.
	ppt.addRemoteWorker(t, []sealtasks.TaskType{
		sealtasks.TTPreCommit1, sealtasks.TTPreCommit2, sealtasks.TTCommit1,
		sealtasks.TTFetch, sealtasks.TTFinalize, sealtasks.TTFinalizeUnsealed,
	})

	// create a worker that can ONLY unseal and fetch
	ppt.addRemoteWorker(t, []sealtasks.TaskType{
		sealtasks.TTUnseal, sealtasks.TTFetch,
	})

	// run the test

	// add one piece that aligns with the padding/piece boundaries.
	pd1 := generatePieceData(8 * 127 * 4 * 1024)
	pi1 := ppt.addPiece(t, pd1)
	pd1size := pi1.Size.Unpadded()

	pd2 := generatePieceData(8 * 127 * 4 * 1024)
	pi2 := ppt.addPiece(t, pd2)
	pd2size := pi2.Size.Unpadded()

	// pre-commit 1
	pC1 := ppt.preCommit1(t)

	// check if IsUnsealed -> true
	require.True(t, ppt.isUnsealed(t, storiface.UnpaddedByteIndex(0), pd1size))
	// Read the piece -> no need to unseal
	ppt.readPiece(t, storiface.UnpaddedByteIndex(0), pd1size,
		false, pd1)

	// pre-commit 2
	ppt.preCommit2(t, pC1)

	// check if IsUnsealed -> true
	require.True(t, ppt.isUnsealed(t, storiface.UnpaddedByteIndex(0), pd1size))
	// Read the piece -> no need to unseal
	ppt.readPiece(t, storiface.UnpaddedByteIndex(0), pd1size,
		false, pd1)

	// finalize the sector so we declare to the index we have the sealed file
	// so the unsealing worker can later look it up and fetch it if needed
	// sending nil here will remove all unsealed files after sector is finalized.
	ppt.finalizeSector(t, nil)

	// check if IsUnsealed -> false
	require.False(t, ppt.isUnsealed(t, storiface.UnpaddedByteIndex(0), pd1size))
	// Read the piece -> have to unseal since we removed the file.
	ppt.readPiece(t, storiface.UnpaddedByteIndex(0), pd1size,
		true, pd1)

	// Read the same piece again -> will NOT have to unseal.
	ppt.readPiece(t, storiface.UnpaddedByteIndex(0), pd1size, false, pd1)

	// remove the unsealed file and read again -> will have to unseal.
	ppt.removeAllUnsealedSectorFiles(t)
	// check if IsUnsealed -> false
	require.False(t, ppt.isUnsealed(t, storiface.UnpaddedByteIndex(0), pd1size))
	ppt.readPiece(t, storiface.UnpaddedByteIndex(0), pd1size,
		true, pd1)

	// check if IsUnsealed -> true
	require.True(t, ppt.isUnsealed(t, storiface.UnpaddedByteIndex(pd1size), pd2size))
	// Read Piece 2 -> no unsealing as it got unsealed above.
	ppt.readPiece(t, storiface.UnpaddedByteIndex(pd1size), pd2size, false, pd2)

	// remove all unseal files -> Read Piece 2 -> will have to Unseal.
	ppt.removeAllUnsealedSectorFiles(t)

	// check if IsUnsealed -> false
	require.False(t, ppt.isUnsealed(t, storiface.UnpaddedByteIndex(pd1size), pd2size))
	ppt.readPiece(t, storiface.UnpaddedByteIndex(pd1size), pd2size, true, pd2)
}

type pieceProviderTestHarness struct {
	ctx         context.Context
	index       *paths.MemIndex
	pp          PieceProvider
	sector      storiface.SectorRef
	mgr         *Manager
	ticket      abi.SealRandomness
	commD       cid.Cid
	localStores []*paths.Local

	servers []*http.Server

	addedPieces []abi.PieceInfo
}

func generatePieceData(size uint64) []byte {
	bz := make([]byte, size)
	_, err := rand.Read(bz)
	if err != nil {
		panic(err)
	}
	return bz
}

func newPieceProviderTestHarness(t *testing.T, mgrConfig config.SealerConfig, sectorProofType abi.RegisteredSealProof) *pieceProviderTestHarness {
	ctx := context.Background()
	// listen on tcp socket to create an http server later
	address := "0.0.0.0:0"
	nl, err := net.Listen("tcp", address)
	require.NoError(t, err)

	// create index, storage, local store & remote store.
	index := paths.NewMemIndex(nil)
	storage := newTestStorage(t)
	localStore, err := paths.NewLocal(ctx, storage, index, []string{"http://" + nl.Addr().String() + "/remote"})
	require.NoError(t, err)
	remoteStore := paths.NewRemote(localStore, index, nil, 6000, &paths.DefaultPartialFileHandler{})

	// data stores for state tracking.
	dstore := ds_sync.MutexWrap(datastore.NewMapDatastore())
	wsts := statestore.New(namespace.Wrap(dstore, datastore.NewKey("/worker/calls")))
	smsts := statestore.New(namespace.Wrap(dstore, datastore.NewKey("/stmgr/calls")))

	mgr, err := New(ctx, localStore, remoteStore, storage, index, mgrConfig, config.ProvingConfig{}, wsts, smsts)
	require.NoError(t, err)

	// start a http server on the manager to serve sector file requests.
	svc := &http.Server{
		Addr:    nl.Addr().String(),
		Handler: mgr,
	}
	go func() {
		_ = svc.Serve(nl)
	}()

	pp := NewPieceProvider(remoteStore, index, mgr)

	sector := storiface.SectorRef{
		ID: abi.SectorID{
			Miner:  100,
			Number: 10,
		},
		ProofType: sectorProofType,
	}

	ticket := abi.SealRandomness{9, 9, 9, 9, 9, 9, 9, 9}

	ppt := &pieceProviderTestHarness{
		ctx:    ctx,
		index:  index,
		pp:     pp,
		sector: sector,
		mgr:    mgr,
		ticket: ticket,
	}
	ppt.servers = append(ppt.servers, svc)
	ppt.localStores = append(ppt.localStores, localStore)
	return ppt
}

func (p *pieceProviderTestHarness) addRemoteWorker(t *testing.T, tasks []sealtasks.TaskType) {
	// start an http Server
	address := "0.0.0.0:0"
	nl, err := net.Listen("tcp", address)
	require.NoError(t, err)

	localStore, err := paths.NewLocal(p.ctx, newTestStorage(t), p.index, []string{"http://" + nl.Addr().String() + "/remote"})
	require.NoError(t, err)

	fh := &paths.FetchHandler{
		Local:     localStore,
		PfHandler: &paths.DefaultPartialFileHandler{},
	}

	mux := mux.NewRouter()
	mux.PathPrefix("/remote").HandlerFunc(fh.ServeHTTP)
	svc := &http.Server{
		Addr:    nl.Addr().String(),
		Handler: mux,
	}

	go func() {
		_ = svc.Serve(nl)
	}()

	remote := paths.NewRemote(localStore, p.index, nil, 1000,
		&paths.DefaultPartialFileHandler{})

	dstore := ds_sync.MutexWrap(datastore.NewMapDatastore())
	csts := statestore.New(namespace.Wrap(dstore, datastore.NewKey("/stmgr/calls")))

	worker := NewLocalWorkerWithExecutor(nil, WorkerConfig{
		TaskTypes: tasks,
	}, os.LookupEnv, remote, localStore, p.index, p.mgr, csts)

	p.servers = append(p.servers, svc)
	p.localStores = append(p.localStores, localStore)

	// register self with manager
	require.NoError(t, p.mgr.AddWorker(p.ctx, worker))
}

func (p *pieceProviderTestHarness) removeAllUnsealedSectorFiles(t *testing.T) {
	for i := range p.localStores {
		ls := p.localStores[i]
		require.NoError(t, ls.Remove(p.ctx, p.sector.ID, storiface.FTUnsealed, false, nil))
	}
}

func (p *pieceProviderTestHarness) addPiece(t *testing.T, pieceData []byte) abi.PieceInfo {
	var existing []abi.UnpaddedPieceSize
	for _, pi := range p.addedPieces {
		existing = append(existing, pi.Size.Unpadded())
	}

	size := abi.UnpaddedPieceSize(len(pieceData))
	pieceInfo, err := p.mgr.AddPiece(p.ctx, p.sector, existing, size, bytes.NewReader(pieceData))
	require.NoError(t, err)

	p.addedPieces = append(p.addedPieces, pieceInfo)
	return pieceInfo
}

func (p *pieceProviderTestHarness) preCommit1(t *testing.T) storiface.PreCommit1Out {
	preCommit1, err := p.mgr.SealPreCommit1(p.ctx, p.sector, p.ticket, p.addedPieces)
	require.NoError(t, err)
	return preCommit1
}

func (p *pieceProviderTestHarness) preCommit2(t *testing.T, pc1 storiface.PreCommit1Out) {
	sectorCids, err := p.mgr.SealPreCommit2(p.ctx, p.sector, pc1)
	require.NoError(t, err)
	commD := sectorCids.Unsealed
	p.commD = commD
}

func (p *pieceProviderTestHarness) isUnsealed(t *testing.T, offset storiface.UnpaddedByteIndex, size abi.UnpaddedPieceSize) bool {
	b, err := p.pp.IsUnsealed(p.ctx, p.sector, offset, size)
	require.NoError(t, err)
	return b
}

func (p *pieceProviderTestHarness) readPiece(t *testing.T, offset storiface.UnpaddedByteIndex, size abi.UnpaddedPieceSize,
	expectedHadToUnseal bool, expectedBytes []byte) {
	rd, isUnsealed, err := p.pp.ReadPiece(p.ctx, p.sector, offset, size, p.ticket, p.commD)
	require.NoError(t, err)
	require.NotNil(t, rd)
	require.Equal(t, expectedHadToUnseal, isUnsealed)
	defer func() { _ = rd.Close() }()

	// Make sure the input matches the output
	readData, err := io.ReadAll(rd)
	require.NoError(t, err)
	require.Equal(t, expectedBytes, readData)
}

func (p *pieceProviderTestHarness) finalizeSector(t *testing.T, keepUnseal []storiface.Range) {
	require.NoError(t, p.mgr.ReleaseUnsealed(p.ctx, p.sector, keepUnseal))
	require.NoError(t, p.mgr.FinalizeSector(p.ctx, p.sector))
}

func (p *pieceProviderTestHarness) shutdown(t *testing.T) {
	for _, svc := range p.servers {
		s := svc
		require.NoError(t, s.Shutdown(p.ctx))
	}
}
