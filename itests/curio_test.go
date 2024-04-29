package itests

import (
	"context"
	"testing"
	"time"

	"github.com/docker/go-units"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/cli/spcli"
	"github.com/filecoin-project/lotus/cmd/curio/deps"
	"github.com/filecoin-project/lotus/itests/kit"
	"github.com/filecoin-project/lotus/node/config"
	"github.com/filecoin-project/lotus/node/impl"
)

func TestCurioNewActor(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	full, miner, esemble := kit.EnsembleMinimal(t,
		kit.LatestActorsAt(-1),
		kit.MockProofs(),
		kit.WithSectorIndexDB(),
	)

	esemble.Start()
	blockTime := 100 * time.Millisecond
	esemble.BeginMining(blockTime)

	db := miner.BaseAPI.(*impl.StorageMinerAPI).HarmonyDB

	var titles []string
	err := db.Select(ctx, &titles, `SELECT title FROM harmony_config WHERE LENGTH(config) > 0`)
	require.NoError(t, err)
	require.NotEmpty(t, titles)
	require.NotContains(t, titles, "base")

	addr := miner.OwnerKey.Address
	sectorSizeInt, err := units.RAMInBytes("8MiB")
	require.NoError(t, err)

	maddr, err := spcli.CreateStorageMiner(ctx, full, addr, addr, addr, abi.SectorSize(sectorSizeInt), 0)
	require.NoError(t, err)

	err = deps.CreateMinerConfig(ctx, full, db, []string{maddr.String()}, "FULL NODE API STRING")
	require.NoError(t, err)

	err = db.Select(ctx, &titles, `SELECT title FROM harmony_config WHERE LENGTH(config) > 0`)
	require.NoError(t, err)
	require.Contains(t, titles, "base")
	baseCfg := config.DefaultCurioConfig()
	var baseText string

	err = db.QueryRow(ctx, "SELECT config FROM harmony_config WHERE title='base'").Scan(&baseText)
	require.NoError(t, err)
	_, err = deps.LoadConfigWithUpgrades(baseText, baseCfg)
	require.NoError(t, err)

	require.NotNil(t, baseCfg.Addresses)
	require.GreaterOrEqual(t, len(baseCfg.Addresses), 1)

	require.Contains(t, baseCfg.Addresses[0].MinerAddresses, maddr.String())
}
