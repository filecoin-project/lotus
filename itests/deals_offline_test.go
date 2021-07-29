package itests

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	commcid "github.com/filecoin-project/go-fil-commcid"
	commp "github.com/filecoin-project/go-fil-commp-hashhash"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/itests/kit"
	"github.com/stretchr/testify/require"
)

func TestOfflineDealFlow(t *testing.T) {

	runTest := func(t *testing.T, fastRet bool, upscale abi.PaddedPieceSize) {
		ctx := context.Background()
		client, miner, ens := kit.EnsembleMinimal(t, kit.WithAllSubsystems()) // no mock proofs
		ens.InterconnectAll().BeginMining(250 * time.Millisecond)

		dh := kit.NewDealHarness(t, client, miner, miner)

		// Create a random file and import on the client.
		res, inFile := client.CreateImportFile(ctx, 1, 200)

		// Get the piece size and commP
		rootCid := res.Root
		pieceInfo, err := client.ClientDealPieceCID(ctx, rootCid)
		require.NoError(t, err)
		t.Log("FILE CID:", rootCid)

		// test whether padding works as intended
		if upscale > 0 {
			newRawCp, err := commp.PadCommP(
				pieceInfo.PieceCID.Hash()[len(pieceInfo.PieceCID.Hash())-32:],
				uint64(pieceInfo.PieceSize),
				uint64(upscale),
			)
			require.NoError(t, err)

			pieceInfo.PieceSize = upscale
			pieceInfo.PieceCID, err = commcid.DataCommitmentV1ToCID(newRawCp)
			require.NoError(t, err)
		}

		dp := dh.DefaultStartDealParams()
		dp.DealStartEpoch = abi.ChainEpoch(4 << 10)
		dp.FastRetrieval = fastRet
		// Replace with params for manual storage deal (offline deal)
		dp.Data = &storagemarket.DataRef{
			TransferType: storagemarket.TTManual,
			Root:         rootCid,
			PieceCid:     &pieceInfo.PieceCID,
			PieceSize:    pieceInfo.PieceSize.Unpadded(),
		}

		proposalCid := dh.StartDeal(ctx, dp)

		// Wait for the deal to reach StorageDealCheckForAcceptance on the client
		cd, err := client.ClientGetDealInfo(ctx, *proposalCid)
		require.NoError(t, err)
		require.Eventually(t, func() bool {
			cd, _ := client.ClientGetDealInfo(ctx, *proposalCid)
			return cd.State == storagemarket.StorageDealCheckForAcceptance
		}, 30*time.Second, 1*time.Second, "actual deal status is %s", storagemarket.DealStates[cd.State])

		// Create a CAR file from the raw file
		carFileDir := t.TempDir()
		carFilePath := filepath.Join(carFileDir, "out.car")
		err = client.ClientGenCar(ctx, api.FileRef{Path: inFile}, carFilePath)
		require.NoError(t, err)

		// Import the CAR file on the miner - this is the equivalent to
		// transferring the file across the wire in a normal (non-offline) deal
		err = miner.DealsImportData(ctx, *proposalCid, carFilePath)
		require.NoError(t, err)

		// Wait for the deal to be published
		dh.WaitDealPublished(ctx, proposalCid)

		t.Logf("deal published, retrieving")

		// Retrieve the deal
		outFile := dh.PerformRetrieval(ctx, proposalCid, rootCid, false)

		kit.AssertFilesEqual(t, inFile, outFile)

	}

	t.Run("stdretrieval", func(t *testing.T) { runTest(t, false, 0) })
	t.Run("fastretrieval", func(t *testing.T) { runTest(t, true, 0) })
	t.Run("fastretrieval", func(t *testing.T) { runTest(t, true, 1024) })
}
