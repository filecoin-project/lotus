package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"regexp"
	"strconv"
	"testing"
	"time"

	logging "github.com/ipfs/go-log/v2"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v2"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors/policy"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/itests/kit"
	"github.com/filecoin-project/lotus/node/repo"
)

func TestWorkerKeyChange(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	_ = logging.SetLogLevel("*", "INFO")

	policy.SetConsensusMinerMinPower(abi.NewStoragePower(2048))
	policy.SetSupportedProofTypes(abi.RegisteredSealProof_StackedDrg2KiBV1)
	policy.SetMinVerifiedDealSize(abi.NewStoragePower(256))

	kit.QuietMiningLogs()

	blocktime := 1 * time.Millisecond

	clients, miners := kit.MockMinerBuilder(t,
		[]kit.FullNodeOpts{kit.FullNodeWithLatestActorsAt(-1), kit.FullNodeWithLatestActorsAt(-1)},
		kit.OneMiner)

	client1 := clients[0]
	client2 := clients[1]

	// Connect the nodes.
	addrinfo, err := client1.NetAddrsListen(ctx)
	require.NoError(t, err)
	err = client2.NetConnect(ctx, addrinfo)
	require.NoError(t, err)

	output := bytes.NewBuffer(nil)
	run := func(cmd *cli.Command, args ...string) error {
		app := cli.NewApp()
		app.Metadata = map[string]interface{}{
			"repoType":         repo.StorageMiner,
			"testnode-full":    clients[0],
			"testnode-storage": miners[0],
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

	// start mining
	kit.ConnectAndStartMining(t, blocktime, miners[0], client1, client2)

	newKey, err := client1.WalletNew(ctx, types.KTBLS)
	require.NoError(t, err)

	// Initialize wallet.
	kit.SendFunds(ctx, t, client1, newKey, abi.NewTokenAmount(0))

	require.NoError(t, run(actorProposeChangeWorker, "--really-do-it", newKey.String()))

	result := output.String()
	output.Reset()

	require.Contains(t, result, fmt.Sprintf("Worker key change to %s successfully proposed.", newKey))

	epochRe := regexp.MustCompile("at or after height (?P<epoch>[0-9]+) to complete")
	matches := epochRe.FindStringSubmatch(result)
	require.NotNil(t, matches)
	targetEpoch, err := strconv.Atoi(matches[1])
	require.NoError(t, err)
	require.NotZero(t, targetEpoch)

	// Too early.
	require.Error(t, run(actorConfirmChangeWorker, "--really-do-it", newKey.String()))
	output.Reset()

	for {
		head, err := client1.ChainHead(ctx)
		require.NoError(t, err)
		if head.Height() >= abi.ChainEpoch(targetEpoch) {
			break
		}
		build.Clock.Sleep(10 * blocktime)
	}
	require.NoError(t, run(actorConfirmChangeWorker, "--really-do-it", newKey.String()))
	output.Reset()

	head, err := client1.ChainHead(ctx)
	require.NoError(t, err)

	// Wait for finality (worker key switch).
	targetHeight := head.Height() + policy.ChainFinality
	for {
		head, err := client1.ChainHead(ctx)
		require.NoError(t, err)
		if head.Height() >= targetHeight {
			break
		}
		build.Clock.Sleep(10 * blocktime)
	}

	// Make sure the other node can catch up.
	for i := 0; i < 20; i++ {
		head, err := client2.ChainHead(ctx)
		require.NoError(t, err)
		if head.Height() >= targetHeight {
			return
		}
		build.Clock.Sleep(10 * blocktime)
	}
	t.Fatal("failed to reach target epoch on the second miner")
}
