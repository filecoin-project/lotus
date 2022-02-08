package itests

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/network"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/itests/kit"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCCUpgrade(t *testing.T) {
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

	// wait for deadline 0 to pass so that committing starts after post on preseals
	// this gives max time for post to complete minimizing chances of timeout
	// waitForDeadline(ctx, t, 1, client, maddr)
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

func waitForDeadline(ctx context.Context, t *testing.T, waitIdx uint64, node *kit.TestFullNode, maddr address.Address) {
	for {
		ts, err := node.ChainHead(ctx)
		require.NoError(t, err)
		dl, err := node.StateMinerProvingDeadline(ctx, maddr, ts.Key())
		require.NoError(t, err)
		if dl.Index == waitIdx {
			return
		}
	}
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

func waitForSectorStartUpgrade(ctx context.Context, t *testing.T, sn abi.SectorNumber, miner *kit.TestMiner) {
	for {
		si, err := miner.StorageMiner.SectorsStatus(ctx, sn, false)
		require.NoError(t, err)
		if si.State != api.SectorState("Proving") {
			t.Logf("Done proving sector in state: %s", si.State)
			return
		}

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

func TestTooManyMarkedForUpgrade(t *testing.T) {
	kit.QuietMiningLogs()

	ctx := context.Background()
	blockTime := 1 * time.Millisecond

	client, miner, ens := kit.EnsembleMinimal(t, kit.GenesisNetworkVersion(network.Version15))
	ens.InterconnectAll().BeginMiningMustPost(blockTime)

	maddr, err := miner.ActorAddress(ctx)
	if err != nil {
		t.Fatal(err)
	}

	CCUpgrade := abi.SectorNumber(kit.DefaultPresealsPerBootstrapMiner + 1)
	waitForDeadline(ctx, t, 1, client, maddr)
	miner.PledgeSectors(ctx, 3, 0, nil)

	sl, err := miner.SectorsList(ctx)
	require.NoError(t, err)
	require.Len(t, sl, 3, "expected 3 sectors")

	{
		si, err := client.StateSectorGetInfo(ctx, maddr, CCUpgrade, types.EmptyTSK)
		require.NoError(t, err)
		require.Less(t, 50000, int(si.Expiration))
	}

	waitForSectorActive(ctx, t, CCUpgrade, client, maddr)
	waitForSectorActive(ctx, t, CCUpgrade+1, client, maddr)
	waitForSectorActive(ctx, t, CCUpgrade+2, client, maddr)

	err = miner.SectorMarkForUpgrade(ctx, CCUpgrade, true)
	require.NoError(t, err)
	err = miner.SectorMarkForUpgrade(ctx, CCUpgrade+1, true)
	require.NoError(t, err)

	waitForSectorStartUpgrade(ctx, t, CCUpgrade, miner)
	waitForSectorStartUpgrade(ctx, t, CCUpgrade+1, miner)

	err = miner.SectorMarkForUpgrade(ctx, CCUpgrade+2, true)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no free resources to wait for deals")
}
