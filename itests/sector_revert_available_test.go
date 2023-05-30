package itests

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/network"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/itests/kit"
	sealing "github.com/filecoin-project/lotus/storage/pipeline"
)

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

	err = miner.SectorMarkForUpgrade(ctx, sl[0], true)
	require.NoError(t, err)

	sl, err = miner.SectorsListNonGenesis(ctx)
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
