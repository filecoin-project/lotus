package cli

import (
	"github.com/urfave/cli/v2"
)

var sentinelCmd = &cli.Command{
	Name:  "sentinel",
	Usage: "Interact with the sentinel module",
	Subcommands: []*cli.Command{
		sentinelStartWatchCmd,
	},
}

var sentinelStartWatchCmd = &cli.Command{
	Name:  "watch",
	Usage: "start a watch against the chain",
	Flags: []cli.Flag{
		&cli.Int64Flag{
			Name: "confidence",
		},
	},
	Action: func(cctx *cli.Context) error {
		apic, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := ReqContext(cctx)

		//confidence := abi.ChainEpoch(cctx.Int64("confidence"))

		if err := apic.WatchStart(ctx); err != nil {
			return err
		}

		return nil
	},
}
