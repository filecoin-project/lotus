//stm: #integration
package itests

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/itests/kit"

	"github.com/stretchr/testify/require"
)

// TODO: This needs to be repurposed into a SnapDeals test suite
func TestCCUpgrade(t *testing.T) {
	//stm: @CHAIN_SYNCER_LOAD_GENESIS_001, @CHAIN_SYNCER_FETCH_TIPSET_001,
	//stm: @CHAIN_SYNCER_START_001, @CHAIN_SYNCER_SYNC_001, @BLOCKCHAIN_BEACON_VALIDATE_BLOCK_VALUES_01
	//stm: @CHAIN_SYNCER_COLLECT_CHAIN_001, @CHAIN_SYNCER_COLLECT_HEADERS_001, @CHAIN_SYNCER_VALIDATE_TIPSET_001
	//stm: @CHAIN_SYNCER_NEW_PEER_HEAD_001, @CHAIN_SYNCER_VALIDATE_MESSAGE_META_001, @CHAIN_SYNCER_STOP_001

	//stm: @CHAIN_STATE_MINER_GET_INFO_001
	//stm: @CHAIN_INCOMING_HANDLE_INCOMING_BLOCKS_001, @CHAIN_INCOMING_VALIDATE_BLOCK_PUBSUB_001, @CHAIN_INCOMING_VALIDATE_MESSAGE_PUBSUB_001

	//stm: @MINER_SECTOR_LIST_001
	kit.QuietMiningLogs()

	for _, height := range []abi.ChainEpoch{
		-1,  // before
		162, // while sealing
		560, // after upgrade deal
	} {
		height := height // make linters happy by copying
		t.Run(fmt.Sprintf("upgrade-%d", height), func(t *testing.T) {
			runTestCCUpgrade(t, height)
		})
	}
}

func runTestCCUpgrade(t *testing.T, upgradeHeight abi.ChainEpoch) {
	ctx := context.Background()
	blockTime := 5 * time.Millisecond

	client, miner, ens := kit.EnsembleMinimal(t, kit.MockProofs(), kit.TurboUpgradeAt(upgradeHeight))
	ens.InterconnectAll().BeginMining(blockTime)

	maddr, err := miner.ActorAddress(ctx)
	if err != nil {
		t.Fatal(err)
	}

	CC := abi.SectorNumber(kit.DefaultPresealsPerBootstrapMiner + 1)
	Upgraded := CC + 1

	miner.PledgeSectors(ctx, 1, 0, nil)

	sl, err := miner.SectorsList(ctx)
	require.NoError(t, err)
	require.Len(t, sl, 1, "expected 1 sector")
	require.Equal(t, CC, sl[0], "unexpected sector number")

	{
		si, err := client.StateSectorGetInfo(ctx, maddr, CC, types.EmptyTSK)
		require.NoError(t, err)
		require.Less(t, 50000, int(si.Expiration))
	}

	//stm: @SECTOR_CC_UPGRADE_001
	err = miner.SectorMarkForUpgrade(ctx, sl[0])
	require.NoError(t, err)

	dh := kit.NewDealHarness(t, client, miner, miner)
	deal, res, inPath := dh.MakeOnlineDeal(ctx, kit.MakeFullDealParams{
		Rseed:                        6,
		SuspendUntilCryptoeconStable: true,
	})
	outPath := dh.PerformRetrieval(context.Background(), deal, res.Root, false)
	kit.AssertFilesEqual(t, inPath, outPath)

	// Validate upgrade

	{
		exp, err := client.StateSectorExpiration(ctx, maddr, CC, types.EmptyTSK)
		if err != nil {
			require.Contains(t, err.Error(), "failed to find sector 3") // already cleaned up
		} else {
			require.NoError(t, err)
			require.NotNil(t, exp)
			require.Greater(t, 50000, int(exp.OnTime))
		}
	}
	{
		exp, err := client.StateSectorExpiration(ctx, maddr, Upgraded, types.EmptyTSK)
		require.NoError(t, err)
		require.Less(t, 50000, int(exp.OnTime))
	}

	//stm: @CHAIN_STATE_MINER_CALCULATE_DEADLINE_001
	dlInfo, err := client.StateMinerProvingDeadline(ctx, maddr, types.EmptyTSK)
	require.NoError(t, err)

	// Sector should expire.
	for {
		// Wait for the sector to expire.
		status, err := miner.SectorsStatus(ctx, CC, true)
		require.NoError(t, err)
		if status.OnTime == 0 && status.Early == 0 {
			break
		}
		t.Log("waiting for sector to expire")
		// wait one deadline per loop.
		time.Sleep(time.Duration(dlInfo.WPoStChallengeWindow) * blockTime)
	}
}
