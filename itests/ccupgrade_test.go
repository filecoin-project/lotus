//stm: #integration
package itests

import (
	"context"
	"fmt"
	"github.com/filecoin-project/lotus/node/config"
	"testing"
	"time"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/network"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/types"
	sealing "github.com/filecoin-project/lotus/extern/storage-sealing"
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

	runTestCCUpgrade(t)
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

	CCUpgrade := abi.SectorNumber(kit.DefaultPresealsPerBootstrapMiner + 1)
	fmt.Printf("CCUpgrade: %d\n", CCUpgrade)

	miner.PledgeSectors(ctx, 1, 0, nil)
	sl, err := miner.SectorsList(ctx)
	require.NoError(t, err)
	require.Len(t, sl, 1, "expected 1 sector")
	require.Equal(t, CCUpgrade, sl[0], "unexpected sector number")
	{
		si, err := client.StateSectorGetInfo(ctx, maddr, CCUpgrade, types.EmptyTSK)
		require.NoError(t, err)
		require.Less(t, 50000, int(si.Expiration))
	}
	waitForSectorActive(ctx, t, CCUpgrade, client, maddr)

	//stm: @SECTOR_CC_UPGRADE_001
	err = miner.SectorMarkForUpgrade(ctx, sl[0], true)
	require.NoError(t, err)

	sl, err = miner.SectorsList(ctx)
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

func waitForSectorActive(ctx context.Context, t *testing.T, sn abi.SectorNumber, node *kit.TestFullNode, maddr address.Address) {
	for {
		active, err := node.StateMinerActiveSectors(ctx, maddr, types.EmptyTSK)
		require.NoError(t, err)
		for _, si := range active {
			if si.SectorNumber == sn {
				fmt.Printf("ACTIVE\n")
				return
			}
		}

		time.Sleep(time.Second)
	}
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

func TestAbortUpgradeAvailable(t *testing.T) {
	kit.QuietMiningLogs()

	ctx := context.Background()
	blockTime := 1 * time.Millisecond

	client, miner, ens := kit.EnsembleMinimal(t, kit.GenesisNetworkVersion(network.Version15), kit.ThroughRPC())
	ens.InterconnectAll().BeginMiningMustPost(blockTime)

	maddr, err := miner.ActorAddress(ctx)
	if err != nil {
		t.Fatal(err)
	}

	CCUpgrade := abi.SectorNumber(kit.DefaultPresealsPerBootstrapMiner + 1)
	fmt.Printf("CCUpgrade: %d\n", CCUpgrade)

	miner.PledgeSectors(ctx, 1, 0, nil)
	sl, err := miner.SectorsList(ctx)
	require.NoError(t, err)
	require.Len(t, sl, 1, "expected 1 sector")
	require.Equal(t, CCUpgrade, sl[0], "unexpected sector number")
	{
		si, err := client.StateSectorGetInfo(ctx, maddr, CCUpgrade, types.EmptyTSK)
		require.NoError(t, err)
		require.Less(t, 50000, int(si.Expiration))
	}
	waitForSectorActive(ctx, t, CCUpgrade, client, maddr)

	err = miner.SectorMarkForUpgrade(ctx, sl[0], true)
	require.NoError(t, err)

	sl, err = miner.SectorsList(ctx)
	require.NoError(t, err)
	require.Len(t, sl, 1, "expected 1 sector")

	ss, err := miner.SectorsStatus(ctx, sl[0], false)
	require.NoError(t, err)

	for i := 0; i < 100; i++ {
		ss, err = miner.SectorsStatus(ctx, sl[0], false)
		require.NoError(t, err)
		if ss.State == api.SectorState(sealing.Proving) {
			time.Sleep(50 * time.Millisecond)
			continue
		}

		require.Equal(t, api.SectorState(sealing.Available), ss.State)
		break
	}

	require.NoError(t, miner.SectorAbortUpgrade(ctx, sl[0]))

	for i := 0; i < 100; i++ {
		ss, err = miner.SectorsStatus(ctx, sl[0], false)
		require.NoError(t, err)
		if ss.State == api.SectorState(sealing.Available) {
			time.Sleep(50 * time.Millisecond)
			continue
		}

		require.Equal(t, api.SectorState(sealing.Proving), ss.State)
		break
	}
}

func TestPreferNoUpgrade(t *testing.T) {
	kit.QuietMiningLogs()

	ctx := context.Background()
	blockTime := 1 * time.Millisecond

	client, miner, ens := kit.EnsembleMinimal(t, kit.GenesisNetworkVersion(network.Version15), kit.ThroughRPC(), kit.MutateSealingConfig(func(sc *config.SealingConfig) {
		sc.PreferNewSectorsForDeals = true
	}))
	ens.InterconnectAll().BeginMiningMustPost(blockTime)

	maddr, err := miner.ActorAddress(ctx)
	if err != nil {
		t.Fatal(err)
	}

	CCUpgrade := abi.SectorNumber(kit.DefaultPresealsPerBootstrapMiner + 1)
	Sealed := abi.SectorNumber(kit.DefaultPresealsPerBootstrapMiner + 2)

	{
		miner.PledgeSectors(ctx, 1, 0, nil)
		sl, err := miner.SectorsList(ctx)
		require.NoError(t, err)
		require.Len(t, sl, 1, "expected 1 sector")
		require.Equal(t, CCUpgrade, sl[0], "unexpected sector number")
		{
			si, err := client.StateSectorGetInfo(ctx, maddr, CCUpgrade, types.EmptyTSK)
			require.NoError(t, err)
			require.Less(t, 50000, int(si.Expiration))
		}
		waitForSectorActive(ctx, t, CCUpgrade, client, maddr)

		err = miner.SectorMarkForUpgrade(ctx, sl[0], true)
		require.NoError(t, err)

		sl, err = miner.SectorsList(ctx)
		require.NoError(t, err)
		require.Len(t, sl, 1, "expected 1 sector")
	}

	{
		dh := kit.NewDealHarness(t, client, miner, miner)
		deal, res, inPath := dh.MakeOnlineDeal(ctx, kit.MakeFullDealParams{
			Rseed:                        6,
			SuspendUntilCryptoeconStable: true,
		})
		outPath := dh.PerformRetrieval(context.Background(), deal, res.Root, false)
		kit.AssertFilesEqual(t, inPath, outPath)
	}

	sl, err := miner.SectorsList(ctx)
	require.NoError(t, err)
	require.Len(t, sl, 2, "expected 2 sectors")

	{
		status, err := miner.SectorsStatus(ctx, CCUpgrade, true)
		require.NoError(t, err)
		assert.Equal(t, api.SectorState(sealing.Available), status.State)
	}

	{
		status, err := miner.SectorsStatus(ctx, Sealed, true)
		require.NoError(t, err)
		assert.Equal(t, 1, len(status.Deals))
		miner.WaitSectorsProving(ctx, map[abi.SectorNumber]struct{}{
			Sealed: {},
		})
	}
}
