package itests

import (
	"bytes"
	"context"
	"crypto/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-state-types/abi"

	minertypes "github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/itests/kit"
	"github.com/filecoin-project/lotus/node/config"
	"github.com/filecoin-project/lotus/storage/pipeline/piece"
)

func TestOnboardRawPiece(t *testing.T) {
	kit.QuietMiningLogs()

	var (
		blocktime = 2 * time.Millisecond
		ctx       = context.Background()
	)

	client, miner, ens := kit.EnsembleMinimal(t, kit.ThroughRPC())
	ens.InterconnectAll().BeginMining(blocktime)

	pieceSize := abi.PaddedPieceSize(2048).Unpadded()
	pieceData := make([]byte, pieceSize)
	_, _ = rand.Read(pieceData)

	dc, err := miner.ComputeDataCid(ctx, pieceSize, bytes.NewReader(pieceData))
	require.NoError(t, err)

	head, err := client.ChainHead(ctx)
	require.NoError(t, err)

	so, err := miner.SectorAddPieceToAny(ctx, pieceSize, bytes.NewReader(pieceData), piece.PieceDealInfo{
		PublishCid:   nil,
		DealID:       0,
		DealProposal: nil,
		DealSchedule: piece.DealSchedule{
			StartEpoch: head.Height() + 2880*2,
			EndEpoch:   head.Height() + 2880*400,
		},
		KeepUnsealed: false,
		PieceActivationManifest: &minertypes.PieceActivationManifest{
			CID:                   dc.PieceCID,
			Size:                  dc.Size,
			VerifiedAllocationKey: nil,
			Notify:                nil,
		},
	})
	require.NoError(t, err)

	// wait for sector to commit

	// wait for sector to commit and enter proving state
	toCheck := map[abi.SectorNumber]struct{}{
		so.Sector: {},
	}

	miner.WaitSectorsProving(ctx, toCheck)
}

// TODO: Test a sector with a market piece + DDO Piece

// TODO: Test a sector with Fil+ DDO piece

// TODO: Test a sector with a (fil+) DDO piece + custom market actor??

// TODO: Test the above with a piece filling half of a sector, and CC padding

// TODO: Test the above with two pieces and inter-piece padding (p1-PP-p2-p2)

// TODO: Test the above with snapdeals sector

func TestOnboardRawPieceSnap(t *testing.T) {
	kit.QuietMiningLogs()

	var (
		blocktime = 2 * time.Millisecond
		ctx       = context.Background()
	)

	client, miner, ens := kit.EnsembleMinimal(t, kit.ThroughRPC(), kit.MutateSealingConfig(func(sc *config.SealingConfig) {
		sc.PreferNewSectorsForDeals = false
		sc.MakeNewSectorForDeals = false
		sc.MakeCCSectorsAvailable = true
		sc.AggregateCommits = false
	}))
	ens.InterconnectAll().BeginMining(blocktime)

	miner.PledgeSectors(ctx, 1, 0, nil)

	pieceSize := abi.PaddedPieceSize(2048).Unpadded()
	pieceData := make([]byte, pieceSize)
	_, _ = rand.Read(pieceData)

	dc, err := miner.ComputeDataCid(ctx, pieceSize, bytes.NewReader(pieceData))
	require.NoError(t, err)

	head, err := client.ChainHead(ctx)
	require.NoError(t, err)

	so, err := miner.SectorAddPieceToAny(ctx, pieceSize, bytes.NewReader(pieceData), piece.PieceDealInfo{
		PublishCid:   nil,
		DealID:       0,
		DealProposal: nil,
		DealSchedule: piece.DealSchedule{
			StartEpoch: head.Height() + 2880*2,
			EndEpoch:   head.Height() + 2880*400, // todo set so that it works with the sector
		},
		KeepUnsealed: false,
		PieceActivationManifest: &minertypes.PieceActivationManifest{
			CID:                   dc.PieceCID,
			Size:                  dc.Size,
			VerifiedAllocationKey: nil,
			Notify:                nil,
		},
	})
	require.NoError(t, err)

	// wait for sector to commit

	// wait for sector to commit and enter proving state
	toCheck := map[abi.SectorNumber]struct{}{
		so.Sector: {},
	}

	miner.WaitSectorsProving(ctx, toCheck)
}
