package itests

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/filecoin-project/lotus/itests/kit2"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/chain/types"
)

func TestCCUpgrade(t *testing.T) {
	kit2.QuietMiningLogs()

	for _, height := range []abi.ChainEpoch{
		-1,   // before
		162,  // while sealing
		530,  // after upgrade deal
		5000, // after
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

	opts := kit2.ConstructorOpts(kit2.LatestActorsAt(upgradeHeight))
	client, miner, ens := kit2.EnsembleMinimal(t, kit2.MockProofs(), opts)
	ens.InterconnectAll().BeginMining(blockTime)

	maddr, err := miner.ActorAddress(ctx)
	if err != nil {
		t.Fatal(err)
	}

	CC := abi.SectorNumber(kit2.DefaultPresealsPerBootstrapMiner + 1)
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

	err = miner.SectorMarkForUpgrade(ctx, sl[0])
	require.NoError(t, err)

	dh := kit2.NewDealHarness(t, client, miner)

	dh.MakeOnlineDeal(context.Background(), 6, false, 0)

	// Validate upgrade

	{
		exp, err := client.StateSectorExpiration(ctx, maddr, CC, types.EmptyTSK)
		require.NoError(t, err)
		require.NotNil(t, exp)
		require.Greater(t, 50000, int(exp.OnTime))
	}
	{
		exp, err := client.StateSectorExpiration(ctx, maddr, Upgraded, types.EmptyTSK)
		require.NoError(t, err)
		require.Less(t, 50000, int(exp.OnTime))
	}

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
