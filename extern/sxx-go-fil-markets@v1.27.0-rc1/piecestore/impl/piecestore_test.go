package piecestoreimpl_test

import (
	"context"
	"math/rand"
	"testing"
	"time"

	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/namespace"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-statestore"

	"github.com/filecoin-project/go-fil-markets/piecestore"
	piecestoreimpl "github.com/filecoin-project/go-fil-markets/piecestore/impl"
	"github.com/filecoin-project/go-fil-markets/piecestore/migrations"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	"github.com/filecoin-project/go-fil-markets/shared_testutil"
)

func TestStorePieceInfo(t *testing.T) {
	ctx := context.Background()
	pieceCid := shared_testutil.GenerateCids(1)[0]
	pieceCid2 := shared_testutil.GenerateCids(1)[0]
	payloadCid := shared_testutil.GenerateCids(1)[0]
	initializePieceStore := func(t *testing.T, ctx context.Context) piecestore.PieceStore {
		ps, err := piecestoreimpl.NewPieceStore(datastore.NewMapDatastore())
		require.NoError(t, err)
		shared_testutil.StartAndWaitForReady(ctx, t, ps)
		_, err = ps.GetPieceInfo(pieceCid)
		assert.Error(t, err)
		return ps
	}

	// Add a deal info
	t.Run("can add deals", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		ps := initializePieceStore(t, ctx)
		dealInfo := piecestore.DealInfo{
			DealID:   abi.DealID(rand.Uint64()),
			SectorID: abi.SectorNumber(rand.Uint64()),
			Offset:   abi.PaddedPieceSize(rand.Uint64()),
			Length:   abi.PaddedPieceSize(rand.Uint64()),
		}
		err := ps.AddDealForPiece(pieceCid, payloadCid, dealInfo)
		assert.NoError(t, err)

		pi, err := ps.GetPieceInfo(pieceCid)
		assert.NoError(t, err)
		assert.Len(t, pi.Deals, 1)
		assert.Equal(t, pi.Deals[0], dealInfo)

		// Verify that getting a piece with a non-existent CID returns ErrNotFound
		pi, err = ps.GetPieceInfo(pieceCid2)
		assert.Error(t, err)
		assert.True(t, xerrors.Is(err, retrievalmarket.ErrNotFound))
	})

	t.Run("adding same deal twice does not dup", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		ps := initializePieceStore(t, ctx)
		dealInfo := piecestore.DealInfo{
			DealID:   abi.DealID(rand.Uint64()),
			SectorID: abi.SectorNumber(rand.Uint64()),
			Offset:   abi.PaddedPieceSize(rand.Uint64()),
			Length:   abi.PaddedPieceSize(rand.Uint64()),
		}
		err := ps.AddDealForPiece(pieceCid, payloadCid, dealInfo)
		assert.NoError(t, err)

		pi, err := ps.GetPieceInfo(pieceCid)
		assert.NoError(t, err)
		assert.Len(t, pi.Deals, 1)
		assert.Equal(t, pi.Deals[0], dealInfo)

		err = ps.AddDealForPiece(pieceCid, payloadCid, dealInfo)
		assert.NoError(t, err)

		pi, err = ps.GetPieceInfo(pieceCid)
		assert.NoError(t, err)
		assert.Len(t, pi.Deals, 1)
		assert.Equal(t, pi.Deals[0], dealInfo)
	})
}

func TestStoreCIDInfo(t *testing.T) {
	ctx := context.Background()
	pieceCids := shared_testutil.GenerateCids(2)
	pieceCid1 := pieceCids[0]
	pieceCid2 := pieceCids[1]
	testCIDs := shared_testutil.GenerateCids(4)
	blockLocations := make([]piecestore.BlockLocation, 0, 3)
	for i := 0; i < 3; i++ {
		blockLocations = append(blockLocations, piecestore.BlockLocation{
			RelOffset: rand.Uint64(),
			BlockSize: rand.Uint64(),
		})
	}

	initializePieceStore := func(t *testing.T, ctx context.Context) piecestore.PieceStore {
		ps, err := piecestoreimpl.NewPieceStore(datastore.NewMapDatastore())
		assert.NoError(t, err)
		shared_testutil.StartAndWaitForReady(ctx, t, ps)
		_, err = ps.GetCIDInfo(testCIDs[0])
		assert.Error(t, err)
		return ps
	}

	t.Run("can add piece block locations", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		ps := initializePieceStore(t, ctx)
		err := ps.AddPieceBlockLocations(pieceCid1, map[cid.Cid]piecestore.BlockLocation{
			testCIDs[0]: blockLocations[0],
			testCIDs[1]: blockLocations[1],
			testCIDs[2]: blockLocations[2],
		})
		assert.NoError(t, err)

		ci, err := ps.GetCIDInfo(testCIDs[0])
		assert.NoError(t, err)
		assert.Len(t, ci.PieceBlockLocations, 1)
		assert.Equal(t, ci.PieceBlockLocations[0], piecestore.PieceBlockLocation{BlockLocation: blockLocations[0], PieceCID: pieceCid1})

		ci, err = ps.GetCIDInfo(testCIDs[1])
		assert.NoError(t, err)
		assert.Len(t, ci.PieceBlockLocations, 1)
		assert.Equal(t, ci.PieceBlockLocations[0], piecestore.PieceBlockLocation{BlockLocation: blockLocations[1], PieceCID: pieceCid1})

		ci, err = ps.GetCIDInfo(testCIDs[2])
		assert.NoError(t, err)
		assert.Len(t, ci.PieceBlockLocations, 1)
		assert.Equal(t, ci.PieceBlockLocations[0], piecestore.PieceBlockLocation{BlockLocation: blockLocations[2], PieceCID: pieceCid1})

		// Verify that getting CID info with a non-existent CID returns ErrNotFound
		ci, err = ps.GetCIDInfo(testCIDs[3])
		assert.Error(t, err)
		assert.True(t, xerrors.Is(err, retrievalmarket.ErrNotFound))
	})

	t.Run("overlapping adds", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		ps := initializePieceStore(t, ctx)
		err := ps.AddPieceBlockLocations(pieceCid1, map[cid.Cid]piecestore.BlockLocation{
			testCIDs[0]: blockLocations[0],
			testCIDs[1]: blockLocations[2],
		})
		assert.NoError(t, err)
		err = ps.AddPieceBlockLocations(pieceCid2, map[cid.Cid]piecestore.BlockLocation{
			testCIDs[1]: blockLocations[1],
			testCIDs[2]: blockLocations[2],
		})
		assert.NoError(t, err)

		ci, err := ps.GetCIDInfo(testCIDs[0])
		assert.NoError(t, err)
		assert.Len(t, ci.PieceBlockLocations, 1)
		assert.Equal(t, ci.PieceBlockLocations[0], piecestore.PieceBlockLocation{BlockLocation: blockLocations[0], PieceCID: pieceCid1})

		ci, err = ps.GetCIDInfo(testCIDs[1])
		assert.NoError(t, err)
		assert.Len(t, ci.PieceBlockLocations, 2)
		assert.Equal(t, ci.PieceBlockLocations[0], piecestore.PieceBlockLocation{BlockLocation: blockLocations[2], PieceCID: pieceCid1})
		assert.Equal(t, ci.PieceBlockLocations[1], piecestore.PieceBlockLocation{BlockLocation: blockLocations[1], PieceCID: pieceCid2})

		ci, err = ps.GetCIDInfo(testCIDs[2])
		assert.NoError(t, err)
		assert.Len(t, ci.PieceBlockLocations, 1)
		assert.Equal(t, ci.PieceBlockLocations[0], piecestore.PieceBlockLocation{BlockLocation: blockLocations[2], PieceCID: pieceCid2})
	})

	t.Run("duplicate adds", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		ps := initializePieceStore(t, ctx)
		err := ps.AddPieceBlockLocations(pieceCid1, map[cid.Cid]piecestore.BlockLocation{
			testCIDs[0]: blockLocations[0],
			testCIDs[1]: blockLocations[1],
		})
		assert.NoError(t, err)
		err = ps.AddPieceBlockLocations(pieceCid1, map[cid.Cid]piecestore.BlockLocation{
			testCIDs[1]: blockLocations[1],
			testCIDs[2]: blockLocations[2],
		})
		assert.NoError(t, err)

		ci, err := ps.GetCIDInfo(testCIDs[0])
		assert.NoError(t, err)
		assert.Len(t, ci.PieceBlockLocations, 1)
		assert.Equal(t, ci.PieceBlockLocations[0], piecestore.PieceBlockLocation{BlockLocation: blockLocations[0], PieceCID: pieceCid1})

		ci, err = ps.GetCIDInfo(testCIDs[1])
		assert.NoError(t, err)
		assert.Len(t, ci.PieceBlockLocations, 1)
		assert.Equal(t, ci.PieceBlockLocations[0], piecestore.PieceBlockLocation{BlockLocation: blockLocations[1], PieceCID: pieceCid1})

		ci, err = ps.GetCIDInfo(testCIDs[2])
		assert.NoError(t, err)
		assert.Len(t, ci.PieceBlockLocations, 1)
		assert.Equal(t, ci.PieceBlockLocations[0], piecestore.PieceBlockLocation{BlockLocation: blockLocations[2], PieceCID: pieceCid1})
	})
}

func TestMigrations(t *testing.T) {
	ctx := context.Background()
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	pieceCids := shared_testutil.GenerateCids(1)
	pieceCid1 := pieceCids[0]
	testCIDs := shared_testutil.GenerateCids(3)
	blockLocations := make([]piecestore.BlockLocation, 0, 3)
	for i := 0; i < 3; i++ {
		blockLocations = append(blockLocations, piecestore.BlockLocation{
			RelOffset: rand.Uint64(),
			BlockSize: rand.Uint64(),
		})
	}
	dealInfo := piecestore.DealInfo{
		DealID:   abi.DealID(rand.Uint64()),
		SectorID: abi.SectorNumber(rand.Uint64()),
		Offset:   abi.PaddedPieceSize(rand.Uint64()),
		Length:   abi.PaddedPieceSize(rand.Uint64()),
	}

	ds := datastore.NewMapDatastore()

	oldCidInfos := statestore.New(namespace.Wrap(ds, datastore.NewKey(piecestoreimpl.DSCIDPrefix)))
	err := oldCidInfos.Begin(testCIDs[0], &migrations.CIDInfo0{
		CID: testCIDs[0],
		PieceBlockLocations: []migrations.PieceBlockLocation0{
			{
				BlockLocation0: migrations.BlockLocation0{
					RelOffset: blockLocations[0].RelOffset,
					BlockSize: blockLocations[0].BlockSize,
				},
				PieceCID: pieceCid1,
			},
		},
	})
	require.NoError(t, err)
	err = oldCidInfos.Begin(testCIDs[1], &migrations.CIDInfo0{
		CID: testCIDs[1],
		PieceBlockLocations: []migrations.PieceBlockLocation0{
			{
				BlockLocation0: migrations.BlockLocation0{
					RelOffset: blockLocations[1].RelOffset,
					BlockSize: blockLocations[1].BlockSize,
				},
				PieceCID: pieceCid1,
			},
		},
	})
	require.NoError(t, err)
	err = oldCidInfos.Begin(testCIDs[2], &migrations.CIDInfo0{
		CID: testCIDs[2],
		PieceBlockLocations: []migrations.PieceBlockLocation0{
			{
				BlockLocation0: migrations.BlockLocation0{
					RelOffset: blockLocations[2].RelOffset,
					BlockSize: blockLocations[2].BlockSize,
				},
				PieceCID: pieceCid1,
			},
		},
	})
	require.NoError(t, err)
	oldPieces := statestore.New(namespace.Wrap(ds, datastore.NewKey(piecestoreimpl.DSPiecePrefix)))
	err = oldPieces.Begin(pieceCid1, &migrations.PieceInfo0{
		PieceCID: pieceCid1,
		Deals: []migrations.DealInfo0{
			{
				DealID:   dealInfo.DealID,
				SectorID: dealInfo.SectorID,
				Offset:   dealInfo.Offset,
				Length:   dealInfo.Length,
			},
		},
	})
	require.NoError(t, err)

	ps, err := piecestoreimpl.NewPieceStore(ds)
	require.NoError(t, err)
	shared_testutil.StartAndWaitForReady(ctx, t, ps)

	t.Run("migrates deals", func(t *testing.T) {
		pi, err := ps.GetPieceInfo(pieceCid1)
		assert.NoError(t, err)
		assert.Len(t, pi.Deals, 1)
		assert.Equal(t, pi.Deals[0], dealInfo)
	})

	t.Run("migrates piece block locations", func(t *testing.T) {
		ci, err := ps.GetCIDInfo(testCIDs[0])
		assert.NoError(t, err)
		assert.Len(t, ci.PieceBlockLocations, 1)
		assert.Equal(t, ci.PieceBlockLocations[0], piecestore.PieceBlockLocation{BlockLocation: blockLocations[0], PieceCID: pieceCid1})

		ci, err = ps.GetCIDInfo(testCIDs[1])
		assert.NoError(t, err)
		assert.Len(t, ci.PieceBlockLocations, 1)
		assert.Equal(t, ci.PieceBlockLocations[0], piecestore.PieceBlockLocation{BlockLocation: blockLocations[1], PieceCID: pieceCid1})

		ci, err = ps.GetCIDInfo(testCIDs[2])
		assert.NoError(t, err)
		assert.Len(t, ci.PieceBlockLocations, 1)
		assert.Equal(t, ci.PieceBlockLocations[0], piecestore.PieceBlockLocation{BlockLocation: blockLocations[2], PieceCID: pieceCid1})
	})
}
