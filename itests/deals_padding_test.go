package itests

import (
	"context"
	"testing"
	"time"

	commcid "github.com/filecoin-project/go-fil-commcid"
	commp "github.com/filecoin-project/go-fil-commp-hashhash"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/chain/actors/policy"
	"github.com/filecoin-project/lotus/itests/kit"
	"github.com/stretchr/testify/require"
)

func TestDealPadding(t *testing.T) {

	kit.QuietMiningLogs()

	var blockTime = 250 * time.Millisecond
	startEpoch := abi.ChainEpoch(2 << 12)
	policy.SetPreCommitChallengeDelay(2)

	client, miner, ens := kit.EnsembleMinimal(t, kit.ThroughRPC(), kit.WithAllSubsystems()) // no mock proofs.
	ens.InterconnectAll().BeginMining(blockTime)
	dh := kit.NewDealHarness(t, client, miner, miner)

	ctx := context.Background()
	client.WaitTillChain(ctx, kit.BlockMinedBy(miner.ActorAddr))

	// Create a random file, would originally be a 256-byte sector
	res, inFile := client.CreateImportFile(ctx, 1, 200)

	// Get the piece size and commP
	pieceInfo, err := client.ClientDealPieceCID(ctx, res.Root)
	require.NoError(t, err)
	t.Log("FILE CID:", res.Root)

	runTest := func(t *testing.T, upscale abi.PaddedPieceSize) {
		// test whether padding works as intended
		newRawCp, err := commp.PadCommP(
			pieceInfo.PieceCID.Hash()[len(pieceInfo.PieceCID.Hash())-32:],
			uint64(pieceInfo.PieceSize),
			uint64(upscale),
		)
		require.NoError(t, err)

		pcid, err := commcid.DataCommitmentV1ToCID(newRawCp)
		require.NoError(t, err)

		dp := dh.DefaultStartDealParams()
		dp.Data.Root = res.Root
		dp.Data.PieceCid = &pcid
		dp.Data.PieceSize = upscale.Unpadded()
		dp.DealStartEpoch = startEpoch
		proposalCid := dh.StartDeal(ctx, dp)

		// TODO: this sleep is only necessary because deals don't immediately get logged in the dealstore, we should fix this
		time.Sleep(time.Second)

		di, err := client.ClientGetDealInfo(ctx, *proposalCid)
		require.NoError(t, err)
		require.True(t, di.PieceCID.Equals(pcid))

		dh.WaitDealSealed(ctx, proposalCid, false, false, nil)

		// Retrieve the deal
		outFile := dh.PerformRetrieval(ctx, proposalCid, res.Root, false)

		kit.AssertFilesEqual(t, inFile, outFile)
	}

	t.Run("padQuarterSector", func(t *testing.T) { runTest(t, 512) })
	t.Run("padHalfSector", func(t *testing.T) { runTest(t, 1024) })
	t.Run("padFullSector", func(t *testing.T) { runTest(t, 2048) })
}
