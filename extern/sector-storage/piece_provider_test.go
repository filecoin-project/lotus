package sectorstorage

import (
	"context"
	"io/ioutil"
	"strings"
	"testing"
	"time"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-statestore"
	specstorage "github.com/filecoin-project/specs-storage/storage"
	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/namespace"
	ds_sync "github.com/ipfs/go-datastore/sync"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/lotus/extern/sector-storage/sealtasks"
	"github.com/filecoin-project/lotus/extern/sector-storage/stores"
	"github.com/filecoin-project/lotus/extern/sector-storage/storiface"
)

// TestPieceProviderReadPiece verifies that the ReadPiece method works correctly
func TestPieceProviderReadPiece(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	runTest := func(t *testing.T, alreadyUnsealed bool) {
		// Set up sector storage manager
		storage := newTestStorage(t)
		index := stores.NewIndex()
		localStore, err := stores.NewLocal(ctx, storage, index, nil)
		require.NoError(t, err)
		remoteStore := stores.NewRemote(localStore, index, nil, 6000)
		dstore := ds_sync.MutexWrap(datastore.NewMapDatastore())
		wsts := statestore.New(namespace.Wrap(dstore, datastore.NewKey("/worker/calls")))
		smsts := statestore.New(namespace.Wrap(dstore, datastore.NewKey("/stmgr/calls")))
		sealerCfg := SealerConfig{
			ParallelFetchLimit: 10,
			AllowAddPiece:      true,
			AllowPreCommit1:    true,
			AllowPreCommit2:    true,
			AllowCommit:        true,
			AllowUnseal:        true,
		}
		mgr, err := New(ctx, localStore, remoteStore, storage, index, sealerCfg, wsts, smsts)
		require.NoError(t, err)

		// Set up worker
		localTasks := []sealtasks.TaskType{
			sealtasks.TTAddPiece, sealtasks.TTPreCommit1, sealtasks.TTCommit1, sealtasks.TTFinalize, sealtasks.TTFetch,
		}

		csts := statestore.New(namespace.Wrap(dstore, datastore.NewKey("/stmgr/calls")))

		// passing a nil executor here mirrors an actual worker setup as it
		// will initialize the worker to use the rust proofs lib under the hood
		worker := newLocalWorker(nil, WorkerConfig{
			TaskTypes: localTasks,
		}, remoteStore, localStore, index, mgr, csts)

		err = mgr.AddWorker(ctx, worker)
		require.NoError(t, err)

		// Create piece provider
		pp := NewPieceProvider(remoteStore, index, mgr)

		// Mock sector
		sector := specstorage.SectorRef{
			ID: abi.SectorID{
				Miner:  1000,
				Number: 1,
			},
			ProofType: abi.RegisteredSealProof_StackedDrg8MiBV1,
		}

		// Create some data that when padded will be 8MB
		pieceData := strings.Repeat("testthis", 127*1024*8)
		size := abi.UnpaddedPieceSize(len(pieceData))
		pieceInfo, err := mgr.AddPiece(ctx, sector, nil, size, strings.NewReader(pieceData))
		require.NoError(t, err)

		// pre-commit 1
		pieces := []abi.PieceInfo{pieceInfo}
		ticket := abi.SealRandomness{9, 9, 9, 9, 9, 9, 9, 9}
		preCommit1, err := mgr.SealPreCommit1(ctx, sector, ticket, pieces)
		require.NoError(t, err)

		// pre-commit 2
		sectorCids, err := mgr.SealPreCommit2(ctx, sector, preCommit1)
		require.NoError(t, err)
		commD := sectorCids.Unsealed

		// If we want to test what happens when the data must be unsealed
		// (ie there is not an unsealed copy already available)
		if !alreadyUnsealed {
			// Remove the unsealed copy from local storage
			err = localStore.Remove(ctx, sector.ID, storiface.FTUnsealed, false)
			require.NoError(t, err)
		}

		// Read the piece
		offset := storiface.UnpaddedByteIndex(0)
		require.NoError(t, err)
		reader, unsealed, err := pp.ReadPiece(ctx, sector, offset, size, ticket, commD)
		require.NoError(t, err)
		requiresUnseal := !alreadyUnsealed
		require.Equal(t, requiresUnseal, unsealed)

		defer func() { _ = reader.Close() }()

		// Make sure the input matches the output
		readData, err := ioutil.ReadAll(reader)
		require.NoError(t, err)
		require.Equal(t, pieceData, string(readData))
	}

	t.Run("already unsealed", func(t *testing.T) {
		runTest(t, true)
	})
	t.Run("requires unseal", func(t *testing.T) {
		runTest(t, false)
	})
}
