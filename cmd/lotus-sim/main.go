package main

import (
	"fmt"
	"os"

	logging "github.com/ipfs/go-log/v2"
	"github.com/urfave/cli/v2"

	"github.com/filecoin-project/lotus/chain/actors/builtin/power"
	"github.com/filecoin-project/lotus/chain/stmgr"
)

var root []*cli.Command = []*cli.Command{
	createSimCommand,
	deleteSimCommand,
	listSimCommand,
	runSimCommand,
	upgradeCommand,
}

func main() {
	if _, set := os.LookupEnv("GOLOG_LOG_LEVEL"); !set {
		_ = logging.SetLogLevel("simulation", "DEBUG")
	}
	app := &cli.App{
		Name:     "lotus-sim",
		Usage:    "A tool to simulate a network.",
		Commands: root,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "repo",
				EnvVars: []string{"LOTUS_PATH"},
				Hidden:  true,
				Value:   "~/.lotus",
			},
			&cli.StringFlag{
				Name:    "simulation",
				Aliases: []string{"sim"},
				EnvVars: []string{"LOTUS_SIMULATION"},
				Value:   "default",
			},
		},
	}

	if err := app.Run(os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(1)
		return
	}
}

func run(cctx *cli.Context) error {
	ctx := cctx.Context

	node, err := open(cctx)
	if err != nil {
		return err
	}
	defer node.Close()

	if err := node.Chainstore.Load(); err != nil {
		return err
	}

	ts := node.Chainstore.GetHeaviestTipSet()

	st, err := stmgr.NewStateManagerWithUpgradeSchedule(node.Chainstore, nil)
	if err != nil {
		return err
	}

	powerTableActor, err := st.LoadActor(ctx, power.Address, ts)
	if err != nil {
		return err
	}
	powerTable, err := power.Load(node.Chainstore.ActorStore(ctx), powerTableActor)
	if err != nil {
		return err
	}
	allMiners, err := powerTable.ListAllMiners()
	if err != nil {
		return err
	}
	fmt.Printf("miner count: %d\n", len(allMiners))
	return nil
}
