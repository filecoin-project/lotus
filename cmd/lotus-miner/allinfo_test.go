package main

import (
	"context"
	"flag"
	"testing"
	"time"

	"github.com/filecoin-project/lotus/itests/kit"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v2"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/actors/policy"
	"github.com/filecoin-project/lotus/node/repo"
)

func TestMinerAllInfo(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}

	_test = true

	kit.QuietMiningLogs()

	oldDelay := policy.GetPreCommitChallengeDelay()
	policy.SetPreCommitChallengeDelay(5)
	t.Cleanup(func() {
		policy.SetPreCommitChallengeDelay(oldDelay)
	})

	client, miner, ens := kit.EnsembleMinimal(t)
	ens.InterconnectAll().BeginMining(time.Second)

	run := func(t *testing.T) {
		app := cli.NewApp()
		app.Metadata = map[string]interface{}{
			"repoType":         repo.StorageMiner,
			"testnode-full":    client,
			"testnode-storage": miner,
		}
		api.RunningNodeType = api.NodeMiner

		cctx := cli.NewContext(app, flag.NewFlagSet("", flag.ContinueOnError), nil)

		require.NoError(t, infoAllCmd.Action(cctx))
	}

	t.Run("pre-info-all", run)

	dh := kit.NewDealHarness(t, client, miner, miner)
	deal, res, inPath := dh.MakeOnlineDeal(context.Background(), kit.MakeFullDealParams{Rseed: 6})
	outPath := dh.PerformRetrieval(context.Background(), deal, res.Root, false)
	kit.AssertFilesEqual(t, inPath, outPath)

	t.Run("post-info-all", run)
}
