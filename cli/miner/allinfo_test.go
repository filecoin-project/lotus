package miner

import (
	"flag"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v2"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/itests/kit"
	"github.com/filecoin-project/lotus/node/repo"
)

func TestMinerAllInfo(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}

	_test = true

	kit.QuietMiningLogs()

	client, miner, ens := kit.EnsembleMinimal(t)
	ens.InterconnectAll().BeginMiningMustPost(5 * time.Millisecond)

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

	t.Run("post-info-all", run)
}
