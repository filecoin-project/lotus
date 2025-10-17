package miner

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"regexp"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v2"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/actors/policy"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/cli/spcli"
	"github.com/filecoin-project/lotus/itests/kit"
	"github.com/filecoin-project/lotus/node/repo"
)

func TestWorkerKeyChange(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	kit.QuietMiningLogs()

	blocktime := 5 * time.Millisecond
	client1, miner, ens := kit.EnsembleMinimal(t, kit.MockProofs())
	ens.InterconnectAll().BeginMining(blocktime)

	output := bytes.NewBuffer(nil)
	run := func(cmd *cli.Command, args ...string) error {
		app := cli.NewApp()
		app.Metadata = map[string]interface{}{
			"repoType":         repo.StorageMiner,
			"testnode-full":    client1,
			"testnode-storage": miner,
		}
		app.Writer = output
		api.RunningNodeType = api.NodeMiner

		fs := flag.NewFlagSet("", flag.ContinueOnError)
		for _, f := range cmd.Flags {
			if err := f.Apply(fs); err != nil {
				return err
			}
		}
		require.NoError(t, fs.Parse(args))

		cctx := cli.NewContext(app, fs, nil)
		return cmd.Action(cctx)
	}

	newKey, err := client1.WalletNew(ctx, types.KTBLS)
	require.NoError(t, err)

	// Initialize wallet.
	kit.SendFunds(ctx, t, client1, newKey, abi.NewTokenAmount(0))

	require.NoError(t, run(spcli.ActorProposeChangeWorkerCmd(LMConfigOrActorGetter), "--really-do-it", newKey.String()))

	result := output.String()
	output.Reset()

	require.Contains(t, result, fmt.Sprintf("Worker key change to %s successfully sent", newKey))

	epochRe := regexp.MustCompile("at or after height (?P<epoch>[0-9]+) to complete")
	matches := epochRe.FindStringSubmatch(result)
	require.NotNil(t, matches)
	targetEpoch, err := strconv.Atoi(matches[1])
	require.NoError(t, err)
	require.NotZero(t, targetEpoch)

	// Too early.
	require.Error(t, run(spcli.ActorConfirmChangeWorkerCmd(LMConfigOrActorGetter), "--really-do-it", newKey.String()))
	output.Reset()

	client1.WaitTillChain(ctx, kit.HeightAtLeast(abi.ChainEpoch(targetEpoch)))

	require.NoError(t, run(spcli.ActorConfirmChangeWorkerCmd(LMConfigOrActorGetter), "--really-do-it", newKey.String()))
	output.Reset()

	head, err := client1.ChainHead(ctx)
	require.NoError(t, err)

	// Wait for finality (worker key switch).
	targetHeight := head.Height() + policy.ChainFinality
	client1.WaitTillChain(ctx, kit.HeightAtLeast(targetHeight))
}
