package main

import (
	"fmt"

	lcli "github.com/filecoin-project/lotus/cli"
	"github.com/filecoin-project/lotus/miner"
	"github.com/urfave/cli/v2"
)

var mpoolCmd = &cli.Command{
	Name:  "mpool",
	Usage: "Tools for diagnosing mempool issues",
	Flags: []cli.Flag{},
	Subcommands: []*cli.Command{
		minerSelectMsgsCmd,
	},
}

var minerSelectMsgsCmd = &cli.Command{
	Name: "miner-select-msgs",
	Action: func(cctx *cli.Context) error {
		api, closer, err := lcli.GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}

		defer closer()
		ctx := lcli.ReqContext(cctx)

		head, err := api.ChainHead(ctx)
		if err != nil {
			return err
		}

		msgs, err := api.MpoolPending(ctx, head.Key())
		if err != nil {
			return err
		}

		filtered, err := miner.SelectMessages(ctx, api.StateGetActor, head, msgs)
		if err != nil {
			return err
		}

		fmt.Println("mempool input messages: ", len(msgs))
		fmt.Println("filtered messages: ", len(filtered))
		return nil
	},
}
