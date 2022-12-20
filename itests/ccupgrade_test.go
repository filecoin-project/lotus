// stm: #integration
package itests

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/network"

	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/itests/kit"
)

func TestCCUpgrade(t *testing.T) {
	//stm: @CHAIN_SYNCER_LOAD_GENESIS_001, @CHAIN_SYNCER_FETCH_TIPSET_001,
	//stm: @CHAIN_SYNCER_START_001, @CHAIN_SYNCER_SYNC_001, @BLOCKCHAIN_BEACON_VALIDATE_BLOCK_VALUES_01
	//stm: @CHAIN_SYNCER_COLLECT_CHAIN_001, @CHAIN_SYNCER_COLLECT_HEADERS_001, @CHAIN_SYNCER_VALIDATE_TIPSET_001
	//stm: @CHAIN_SYNCER_NEW_PEER_HEAD_001, @CHAIN_SYNCER_VALIDATE_MESSAGE_META_001, @CHAIN_SYNCER_STOP_001

	//stm: @CHAIN_STATE_MINER_GET_INFO_001
	//stm: @CHAIN_INCOMING_HANDLE_INCOMING_BLOCKS_001, @CHAIN_INCOMING_VALIDATE_BLOCK_PUBSUB_001, @CHAIN_INCOMING_VALIDATE_MESSAGE_PUBSUB_001

	//stm: @MINER_SECTOR_LIST_001
	kit.QuietMiningLogs()

	n := runTestCCUpgrade(t)

	t.Run("post", func(t *testing.T) {
		ctx := context.Background()
		ts, err := n.ChainHead(ctx)
		require.NoError(t, err)
		start := ts.Height()
		// wait for a full proving period
		t.Log("waiting for chain")

		n.WaitTillChain(ctx, func(ts *types.TipSet) bool {
			if ts.Height() > start+abi.ChainEpoch(2880) {
				return true
			}
			return false
		})
	})
}

func runTestCCUpgrade(t *testing.T) *kit.TestFullNode {
	ctx := context.Background()
	blockTime := 1 * time.Millisecond

	client, miner, ens := kit.EnsembleMinimal(t, kit.GenesisNetworkVersion(network.Version15), kit.ThroughRPC())
	ens.InterconnectAll().BeginMiningMustPost(blockTime)

	maddr, err := miner.ActorAddress(ctx)
	if err != nil {
		t.Fatal(err)
	}

	CCUpgrade := abi.SectorNumber(kit.DefaultPresealsPerBootstrapMiner)
	fmt.Printf("CCUpgrade: %d\n", CCUpgrade)

	miner.PledgeSectors(ctx, 1, 0, nil)
	sl, err := miner.SectorsListNonGenesis(ctx)
	require.NoError(t, err)
	require.Len(t, sl, 1, "expected 1 sector")
	require.Equal(t, CCUpgrade, sl[0], "unexpected sector number")
	{
		si, err := client.StateSectorGetInfo(ctx, maddr, CCUpgrade, types.EmptyTSK)
		require.NoError(t, err)
		require.NotNil(t, si)
		require.Less(t, 50000, int(si.Expiration))
	}
	client.WaitForSectorActive(ctx, t, CCUpgrade, maddr)

	//stm: @SECTOR_CC_UPGRADE_001
	err = miner.SectorMarkForUpgrade(ctx, sl[0], true)
	require.NoError(t, err)

	sl, err = miner.SectorsListNonGenesis(ctx)
	require.NoError(t, err)
	require.Len(t, sl, 1, "expected 1 sector")

	dh := kit.NewDealHarness(t, client, miner, miner)
	deal, res, inPath := dh.MakeOnlineDeal(ctx, kit.MakeFullDealParams{
		Rseed:                        6,
		SuspendUntilCryptoeconStable: true,
	})
	outPath := dh.PerformRetrieval(context.Background(), deal, res.Root, false)
	kit.AssertFilesEqual(t, inPath, outPath)

	status, err := miner.SectorsStatus(ctx, CCUpgrade, true)
	require.NoError(t, err)
	assert.Equal(t, 1, len(status.Deals))

	miner.WaitSectorsProving(ctx, map[abi.SectorNumber]struct{}{
		CCUpgrade: {},
	})

	return client
}

func TestCCUpgradeAndPoSt(t *testing.T) {
	kit.QuietMiningLogs()
	t.Run("upgrade and then post", func(t *testing.T) {
		ctx := context.Background()
		n := runTestCCUpgrade(t)
		ts, err := n.ChainHead(ctx)
		require.NoError(t, err)
		start := ts.Height()
		// wait for a full proving period
		t.Log("waiting for chain")

		n.WaitTillChain(ctx, func(ts *types.TipSet) bool {
			if ts.Height() > start+abi.ChainEpoch(2880) {
				return true
			}
			return false
		})
	})
}
