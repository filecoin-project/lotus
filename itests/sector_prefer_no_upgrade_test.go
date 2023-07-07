package itests

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/network"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/itests/kit"
	"github.com/filecoin-project/lotus/node/config"
	sealing "github.com/filecoin-project/lotus/storage/pipeline"
)

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

	CCUpgrade := abi.SectorNumber(kit.DefaultPresealsPerBootstrapMiner)
	Sealed := abi.SectorNumber(kit.DefaultPresealsPerBootstrapMiner + 1)

	{
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

	sl, err := miner.SectorsListNonGenesis(ctx)
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
