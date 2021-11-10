package dagstore

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/dagstore"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket/impl/testnodes"
	tut "github.com/filecoin-project/go-fil-markets/shared_testutil"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
	"github.com/filecoin-project/go-state-types/abi"
	mock_provider "github.com/filecoin-project/indexer-reference-provider/mock"
	"github.com/filecoin-project/specs-actors/v2/actors/builtin/market"

	"github.com/filecoin-project/lotus/node/config"
)

func TestShardRegistration(t *testing.T) {
	controller := gomock.NewController(t)
	ps := tut.NewTestPieceStore()
	sa := testnodes.NewTestSectorAccessor()

	ctx := context.Background()
	cids := tut.GenerateCids(4)
	pieceCidUnsealed := cids[0]
	pieceCidSealed := cids[1]
	pieceCidUnsealed2 := cids[2]
	pieceCidUnsealed3 := cids[3]

	sealedSector := abi.SectorNumber(1)
	unsealedSector1 := abi.SectorNumber(2)
	unsealedSector2 := abi.SectorNumber(3)
	unsealedSector3 := abi.SectorNumber(4)

	// ps.ExpectPiece(pieceCidUnsealed, piecestore.PieceInfo{
	// 	PieceCID: pieceCidUnsealed,
	// 	Deals: []piecestore.DealInfo{
	// 		{
	// 			SectorID: unsealedSector1,
	// 		},
	// 	},
	// })
	//
	// ps.ExpectPiece(pieceCidSealed, piecestore.PieceInfo{
	// 	PieceCID: pieceCidSealed,
	// 	Deals: []piecestore.DealInfo{
	// 		{
	// 			SectorID: sealedSector,
	// 		},
	// 	},
	// })

	deals := []storagemarket.MinerDeal{{
		// Should be registered
		State:        storagemarket.StorageDealSealing,
		SectorNumber: unsealedSector1,
		ClientDealProposal: market.ClientDealProposal{
			Proposal: market.DealProposal{
				PieceCID: pieceCidUnsealed,
			},
		},
	}, {
		// Should be registered with lazy registration (because sector is sealed)
		State:        storagemarket.StorageDealSealing,
		SectorNumber: sealedSector,
		ClientDealProposal: market.ClientDealProposal{
			Proposal: market.DealProposal{
				PieceCID: pieceCidSealed,
			},
		},
	}, {
		// Should be ignored because deal is no longer active
		State:        storagemarket.StorageDealError,
		SectorNumber: unsealedSector2,
		ClientDealProposal: market.ClientDealProposal{
			Proposal: market.DealProposal{
				PieceCID: pieceCidUnsealed2,
			},
		},
	}, {
		// Should be ignored because deal is not yet sealing
		State:        storagemarket.StorageDealFundsReserved,
		SectorNumber: unsealedSector3,
		ClientDealProposal: market.ClientDealProposal{
			Proposal: market.DealProposal{
				PieceCID: pieceCidUnsealed3,
			},
		},
	}}

	cfg := config.DefaultStorageMiner().DAGStore
	cfg.RootDir = t.TempDir()

	mapi := NewMinerAPI(ps, sa, 10)
	idxprov := mock_provider.NewMockInterface(controller)
	dagst, w, err := NewDAGStore(cfg, mapi, idxprov)
	require.NoError(t, err)
	require.NotNil(t, dagst)
	require.NotNil(t, w)

	err = dagst.Start(context.Background())
	require.NoError(t, err)

	migrated, err := w.MigrateDeals(ctx, deals)
	require.True(t, migrated)
	require.NoError(t, err)

	info := dagst.AllShardsInfo()
	require.Len(t, info, 2)
	for _, i := range info {
		require.Equal(t, dagstore.ShardStateNew, i.ShardState)
	}

	// Run register shard migration again
	migrated, err = w.MigrateDeals(ctx, deals)
	require.False(t, migrated)
	require.NoError(t, err)

	// ps.VerifyExpectations(t)
}
