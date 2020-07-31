package main

import (
	"fmt"

	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/types"
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

		var totalGas int64
		for i, f := range filtered {
			from := f.Message.From.String()
			if len(from) > 8 {
				from = "..." + from[len(from)-8:]
			}

			to := f.Message.To.String()
			if len(to) > 8 {
				to = "..." + to[len(to)-8:]
			}

			fmt.Printf("%d: %s -> %s, method %d, gasPrice %s, gasLimit %d, val %s\n", i, from, to, f.Message.Method, f.Message.GasPrice, f.Message.GasLimit, types.FIL(f.Message.Value))
			totalGas += f.Message.GasLimit
		}

		fmt.Println("mempool input messages: ", len(msgs))
		fmt.Println("filtered messages: ", len(filtered))
		fmt.Printf("total gas limit of selected messages: %d / %d (%0.2f%%)\n", totalGas, build.BlockGasLimit, 100*float64(totalGas)/float64(build.BlockGasLimit))
		return nil
	},
}
