// stm: #integration
package itests

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	commcid "github.com/filecoin-project/go-fil-commcid"
	commp "github.com/filecoin-project/go-fil-commp-hashhash"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
	"github.com/filecoin-project/go-state-types/abi"

	lapi "github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/itests/kit"
)

func TestOfflineDealFlow(t *testing.T) {
	//stm: @CHAIN_SYNCER_LOAD_GENESIS_001, @CHAIN_SYNCER_FETCH_TIPSET_001,
	//stm: @CHAIN_SYNCER_START_001, @CHAIN_SYNCER_SYNC_001, @BLOCKCHAIN_BEACON_VALIDATE_BLOCK_VALUES_01
	//stm: @CHAIN_SYNCER_COLLECT_CHAIN_001, @CHAIN_SYNCER_COLLECT_HEADERS_001, @CHAIN_SYNCER_VALIDATE_TIPSET_001
	//stm: @CHAIN_SYNCER_NEW_PEER_HEAD_001, @CHAIN_SYNCER_VALIDATE_MESSAGE_META_001, @CHAIN_SYNCER_STOP_001

	//stm: @CHAIN_INCOMING_HANDLE_INCOMING_BLOCKS_001, @CHAIN_INCOMING_VALIDATE_BLOCK_PUBSUB_001, @CHAIN_INCOMING_VALIDATE_MESSAGE_PUBSUB_001
	//stm: @CLIENT_DATA_CALCULATE_COMMP_001, @CLIENT_DATA_GENERATE_CAR_001, @CLIENT_DATA_GET_DEAL_PIECE_CID_001, @CLIENT_DATA_GET_DEAL_PIECE_CID_001
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

		//stm: @CLIENT_STORAGE_DEALS_GET_001
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
		err = client.ClientGenCar(ctx, lapi.FileRef{Path: inFile}, carFilePath)
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
