package main

import (
	"encoding/json"
	"fmt"
	_ "net/http/pprof"
	"os"

	"github.com/urfave/cli/v2"

	lcli "github.com/filecoin-project/lotus/cli"
)

var stopCmd = &cli.Command{
	Name:  "stop",
	Usage: "Stop a running lotus miner",

	Subcommands: []*cli.Command{
		stopCheckCmd,
	},

	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "force",
			Usage: "Stop the miner without running any safety checks first",
		},
	},
	Action: func(cctx *cli.Context) error {
		api, closer, err := lcli.GetAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		if !cctx.Bool("force") {
			fmt.Println("WARNING:\nIn the near future, using 'stop' without the '--force' flag will cancel the stop operation if any safety checks fail. The new 'stop check' subcommand allows these checks to be performed without requesting a stop.")
			isStopSafe, err := CheckIsStopSafe(cctx)
			if err != nil || !isStopSafe.Safe {
				fmt.Println("WARNING: Safety checks performed by 'stop check' failed, but stopping anyway. In the future, this will cancel the stop request unless the '--force' flag is enabled.")
			}
		}

		err = api.Shutdown(lcli.ReqContext(cctx))
		if err != nil {
			return err
		}

		return nil
	},
}

var stopCheckCmd = &cli.Command{
	Name:  "check",
	Usage: "Determine if and when stopping the miner is safe",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "json",
			Usage: "produce machine-readable JSON output",
		},
	},
	Action: func(cctx *cli.Context) error {
		isStopSafe, err := CheckIsStopSafe(cctx)
		if err != nil {
			return err
		}

		if cctx.Bool("json") {
			jsonBytes, err := json.Marshal(isStopSafe)
			if err != nil {
				return err
			}
			fmt.Printf("%s", string(jsonBytes))
		} else {
			fmt.Printf("Miner Address: %s\n", isStopSafe.MinerAddress)
			safeToStopStr := "NO"
			if isStopSafe.Safe {
				safeToStopStr = "proceed with caution"
			}
			fmt.Printf("Safe to stop: %s\n", safeToStopStr)
			fmt.Printf(
				"Deadline fault cutoff: epoch %s\n",
				lcli.EpochTime(isStopSafe.DeadlineFaultCutoff.CurrentEpoch, isStopSafe.DeadlineFaultCutoff.CutoffEpoch),
			)

			if len(isStopSafe.Errors) > 0 {
				fmt.Printf("Errors:\n")
				for _, errMsg := range isStopSafe.Errors {
					fmt.Printf("\t%s\n", errMsg)
				}
			}
		}

		if len(isStopSafe.Errors) > 0 {
			os.Exit(len(isStopSafe.Errors))
		}

		return nil
	},
}
