package cli

import (
	"fmt"
	"time"

	"github.com/fatih/color"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"
)

var LogCmd = &cli.Command{
	Name:  "log",
	Usage: "Manage logging",
	Subcommands: []*cli.Command{
		LogList,
		LogSetLevel,
		LogAlerts,
	},
}

var LogList = &cli.Command{
	Name:  "list",
	Usage: "List log systems",
	Action: func(cctx *cli.Context) error {
		api, closer, err := GetAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		ctx := ReqContext(cctx)

		systems, err := api.LogList(ctx)
		if err != nil {
			return err
		}

		for _, system := range systems {
			fmt.Println(system)
		}

		return nil
	},
}

var LogSetLevel = &cli.Command{
	Name:      "set-level",
	Usage:     "Set log level",
	ArgsUsage: "[level]",
	Description: `Set the log level for logging systems:

   The system flag can be specified multiple times.

   eg) log set-level --system chain --system chainxchg debug

   Available Levels:
   debug
   info
   warn
   error

   Environment Variables:
   GOLOG_LOG_LEVEL - Default log level for all log systems
   GOLOG_LOG_FMT   - Change output log format (json, nocolor)
   GOLOG_FILE      - Write logs to file
   GOLOG_OUTPUT    - Specify whether to output to file, stderr, stdout or a combination, i.e. file+stderr
`,
	Flags: []cli.Flag{
		&cli.StringSliceFlag{
			Name:  "system",
			Usage: "limit to log system",
			Value: &cli.StringSlice{},
		},
	},
	Action: func(cctx *cli.Context) error {
		api, closer, err := GetAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := ReqContext(cctx)

		if !cctx.Args().Present() {
			return fmt.Errorf("level is required")
		}

		systems := cctx.StringSlice("system")
		if len(systems) == 0 {
			var err error
			systems, err = api.LogList(ctx)
			if err != nil {
				return err
			}
		}

		for _, system := range systems {
			if err := api.LogSetLevel(ctx, system, cctx.Args().First()); err != nil {
				return xerrors.Errorf("setting log level on %s: %v", system, err)
			}
		}

		return nil
	},
}

var LogAlerts = &cli.Command{
	Name:  "alerts",
	Usage: "Get alert states",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "all",
			Usage: "get all (active and inactive) alerts",
		},
	},
	Action: func(cctx *cli.Context) error {
		api, closer, err := GetAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		ctx := ReqContext(cctx)

		alerts, err := api.LogAlerts(ctx)
		if err != nil {
			return xerrors.Errorf("getting alerts: %w", err)
		}

		all := cctx.Bool("all")

		for _, alert := range alerts {
			if !all && !alert.Active {
				continue
			}

			active := color.RedString("active  ")
			if !alert.Active {
				active = color.GreenString("inactive")
			}

			fmt.Printf("%s %s:%s\n", active, alert.Type.System, alert.Type.Subsystem)
			if alert.LastResolved != nil {
				fmt.Printf("         last resolved at %s; reason: %s\n", alert.LastResolved.Time.Truncate(time.Millisecond), alert.LastResolved.Message)
			}
			if alert.LastActive != nil {
				fmt.Printf("         %s %s; reason: %s\n", color.YellowString("last raised at"), alert.LastActive.Time.Truncate(time.Millisecond), alert.LastActive.Message)
			}
		}

		return nil
	},
}
