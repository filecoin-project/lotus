package itests

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/filecoin-project/go-fil-markets/storagemarket"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/itests/kit"
	"github.com/stretchr/testify/require"
)

func TestOfflineDealFlow(t *testing.T) {
	blocktime := 10 * time.Millisecond

	// For these tests where the block time is artificially short, just use
	// a deal start epoch that is guaranteed to be far enough in the future
	// so that the deal starts sealing in time
	startEpoch := abi.ChainEpoch(2 << 12)

	runTest := func(t *testing.T, fastRet bool) {
		ctx := context.Background()
		client, miner, ens := kit.EnsembleMinimal(t, kit.MockProofs())
		ens.InterconnectAll().BeginMining(blocktime)

		dh := kit.NewDealHarness(t, client, miner, miner)

		// Create a random file and import on the client.
		res, inFile := client.CreateImportFile(ctx, 1, 200)

		// Get the piece size and commP
		rootCid := res.Root
		pieceInfo, err := client.ClientDealPieceCID(ctx, rootCid)
		require.NoError(t, err)
		t.Log("FILE CID:", rootCid)

		dp := dh.DefaultStartDealParams()
		dp.DealStartEpoch = startEpoch
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

	t.Run("stdretrieval", func(t *testing.T) { runTest(t, false) })
	t.Run("fastretrieval", func(t *testing.T) { runTest(t, true) })
}
