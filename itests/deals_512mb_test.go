// stm: #integration
package itests

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/itests/kit"
)

func TestStorageDealMissingBlock(t *testing.T) {
	//stm: @CHAIN_SYNCER_LOAD_GENESIS_001, @CHAIN_SYNCER_FETCH_TIPSET_001,
	//stm: @CHAIN_SYNCER_START_001, @CHAIN_SYNCER_SYNC_001, @BLOCKCHAIN_BEACON_VALIDATE_BLOCK_VALUES_01
	//stm: @CHAIN_SYNCER_COLLECT_CHAIN_001, @CHAIN_SYNCER_COLLECT_HEADERS_001, @CHAIN_SYNCER_VALIDATE_TIPSET_001
	//stm: @CHAIN_SYNCER_NEW_PEER_HEAD_001, @CHAIN_SYNCER_VALIDATE_MESSAGE_META_001, @CHAIN_SYNCER_STOP_001

	//stm: @CHAIN_INCOMING_HANDLE_INCOMING_BLOCKS_001, @CHAIN_INCOMING_VALIDATE_BLOCK_PUBSUB_001, @CHAIN_INCOMING_VALIDATE_MESSAGE_PUBSUB_001
	//stm: @CLIENT_STORAGE_DEALS_LIST_IMPORTS_001
	ctx := context.Background()

	kit.QuietMiningLogs()

	client, miner, ens := kit.EnsembleMinimal(t,
		kit.MockProofs(),
		kit.SectorSize(512<<20), // 512MiB sectors.
	)
	ens.InterconnectAll().BeginMining(50 * time.Millisecond)

	dh := kit.NewDealHarness(t, client, miner, miner)

	client.WaitTillChain(ctx, kit.HeightAtLeast(5))

	res, _ := client.CreateImportFile(ctx, 0, 64<<20) // 64MiB file.
	list, err := client.ClientListImports(ctx)
	require.NoError(t, err)
	require.Len(t, list, 1)
	require.Equal(t, res.Root, *list[0].Root)

	dp := dh.DefaultStartDealParams()
	dp.Data.Root = res.Root
	dp.FastRetrieval = true
	dp.EpochPrice = abi.NewTokenAmount(62500000) // minimum asking price.
	deal := dh.StartDeal(ctx, dp)

	dh.WaitDealSealed(ctx, deal, false, false, nil)
}
