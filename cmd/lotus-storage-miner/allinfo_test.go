package main

import (
	"context"
	"flag"
	"testing"
	"time"

	"github.com/filecoin-project/lotus/itests/kit"
	"github.com/filecoin-project/lotus/node/impl"
	logging "github.com/ipfs/go-log/v2"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v2"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/actors/policy"
	"github.com/filecoin-project/lotus/node/repo"
)

func TestMinerAllInfo(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}

	_ = logging.SetLogLevel("*", "INFO")

	policy.SetConsensusMinerMinPower(abi.NewStoragePower(2048))
	policy.SetSupportedProofTypes(abi.RegisteredSealProof_StackedDrg2KiBV1)
	policy.SetMinVerifiedDealSize(abi.NewStoragePower(256))

	_test = true

	kit.QuietMiningLogs()

	oldDelay := policy.GetPreCommitChallengeDelay()
	policy.SetPreCommitChallengeDelay(5)
	t.Cleanup(func() {
		policy.SetPreCommitChallengeDelay(oldDelay)
	})

	n, sn := kit.Builder(t, kit.OneFull, kit.OneMiner)
	client, miner := n[0].FullNode, sn[0]
	kit.ConnectAndStartMining(t, time.Second, miner, client.(*impl.FullNodeAPI))

	run := func(t *testing.T) {
		app := cli.NewApp()
		app.Metadata = map[string]interface{}{
			"repoType":         repo.StorageMiner,
			"testnode-full":    n[0],
			"testnode-storage": sn[0],
		}
		api.RunningNodeType = api.NodeMiner

		cctx := cli.NewContext(app, flag.NewFlagSet("", flag.ContinueOnError), nil)

		require.NoError(t, infoAllCmd.Action(cctx))
	}

	t.Run("pre-info-all", run)

	dh := kit.NewDealHarness(t, client, miner)
	_, _, _ = dh.MakeFullDeal(kit.MakeFullDealParams{
		Ctx:         context.Background(),
		Rseed:       6,
		CarExport:   false,
		FastRet:     false,
		StartEpoch:  0,
		DoRetrieval: true,
	})

	t.Run("post-info-all", run)
}
