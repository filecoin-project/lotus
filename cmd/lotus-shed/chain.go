package main

import (
	"fmt"

	lcli "github.com/filecoin-project/lotus/cli"
	"github.com/urfave/cli/v2"
)

var chainCmd = &cli.Command{
	Name:  "chain",
	Usage: "chain-related utilities",
	Subcommands: []*cli.Command{
		chainNullTsCmd,
	},
}

var chainNullTsCmd = &cli.Command{
	Name:  "latest-null",
	Usage: "finds the most recent null tipset",
	Action: func(cctx *cli.Context) error {
		api, closer, err := lcli.GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}

		defer closer()
		ctx := lcli.ReqContext(cctx)

		ts, err := lcli.LoadTipSet(ctx, cctx, api)
		if err != nil {
			return err
		}

		for {
			pts, err := api.ChainGetTipSet(ctx, ts.Parents())
			if err != nil {
				return err
			}

			if ts.Height() != pts.Height()+1 {
				fmt.Println("null tipset at height ", ts.Height()-1)
				return nil
			}

			ts = pts
		}
	},
}
