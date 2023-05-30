package main

import (
	"fmt"

	"github.com/urfave/cli/v2"

	lcli "github.com/filecoin-project/lotus/cli"
)

var chainCmd = &cli.Command{
	Name:  "chain",
	Usage: "chain-related utilities",
	Subcommands: []*cli.Command{
		chainNullTsCmd,
		computeStateRangeCmd,
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

var computeStateRangeCmd = &cli.Command{
	Name:      "compute-state-range",
	Usage:     "forces the computation of a range of tipsets",
	ArgsUsage: "[START_TIPSET_REF] [END_TIPSET_REF]",
	Action: func(cctx *cli.Context) error {
		if cctx.NArg() != 2 {
			return lcli.IncorrectNumArgs(cctx)
		}

		api, closer, err := lcli.GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}

		defer closer()
		ctx := lcli.ReqContext(cctx)

		startTs, err := lcli.ParseTipSetRef(ctx, api, cctx.Args().First())
		if err != nil {
			return err
		}

		endTs, err := lcli.ParseTipSetRef(ctx, api, cctx.Args().Get(1))
		if err != nil {
			return err
		}

		fmt.Printf("computing tipset at height %d (start)\n", startTs.Height())
		if _, err := api.StateCompute(ctx, startTs.Height(), nil, startTs.Key()); err != nil {
			return err
		}

		for height := startTs.Height() + 1; height < endTs.Height(); height++ {
			fmt.Printf("computing tipset at height %d\n", height)

			// The fact that the tipset lookup method takes a tipset is rather annoying.
			// This is because we walk back from the supplied tipset (which could be the HEAD)
			// to locate the desired one.
			ts, err := api.ChainGetTipSetByHeight(ctx, height, endTs.Key())
			if err != nil {
				return err
			}

			if _, err := api.StateCompute(ctx, height, nil, ts.Key()); err != nil {
				return err
			}
		}

		fmt.Printf("computing tipset at height %d (end)\n", endTs.Height())
		if _, err := api.StateCompute(ctx, endTs.Height(), nil, endTs.Key()); err != nil {
			return err
		}

		return nil
	},
}
