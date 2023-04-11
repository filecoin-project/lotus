package main

import (
	"fmt"

	"github.com/urfave/cli/v2"

	api2 "github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/types"
	lcli "github.com/filecoin-project/lotus/cli"
)

var mpoolCmd = &cli.Command{
	Name:  "mpool",
	Usage: "Tools for diagnosing mempool issues",
	Flags: []cli.Flag{},
	Subcommands: []*cli.Command{
		minerSelectMsgsCmd,
		mpoolClear,
	},
}

var minerSelectMsgsCmd = &cli.Command{
	Name:    "miner-select-messages",
	Aliases: []string{"miner-select-msgs"},
	Flags: []cli.Flag{
		&cli.Float64Flag{
			Name:  "ticket-quality",
			Value: 1,
		},
	},
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

		msgs, err := api.MpoolSelect(ctx, head.Key(), cctx.Float64("ticket-quality"))
		if err != nil {
			return err
		}

		var totalGas int64
		for i, f := range msgs {
			from := f.Message.From.String()
			if len(from) > 8 {
				from = "..." + from[len(from)-8:]
			}

			to := f.Message.To.String()
			if len(to) > 8 {
				to = "..." + to[len(to)-8:]
			}

			fmt.Printf("%d: %s -> %s, method %d, gasFeecap %s, gasPremium %s, gasLimit %d, val %s\n", i, from, to, f.Message.Method, f.Message.GasFeeCap, f.Message.GasPremium, f.Message.GasLimit, types.FIL(f.Message.Value))
			totalGas += f.Message.GasLimit
		}

		fmt.Println("selected messages: ", len(msgs))
		fmt.Printf("total gas limit of selected messages: %d / %d (%0.2f%%)\n", totalGas, build.BlockGasLimit, 100*float64(totalGas)/float64(build.BlockGasLimit))
		return nil
	},
}

var mpoolClear = &cli.Command{
	Name:  "clear",
	Usage: "Clear all pending messages from the mpool (USE WITH CARE)",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "local",
			Usage: "also clear local messages",
		},
		&cli.Int64Flag{
			Name:  "below-basefee",
			Usage: "only clear messages with a base fee lower than this (aFIL)",
			Value: 100_000_000, // 100 pFIL
		},
		&cli.BoolFlag{
			Name:  "really-do-it",
			Usage: "must be specified for the action to take effect",
		},
	},
	Action: func(cctx *cli.Context) error {
		api, closer, err := lcli.GetFullNodeAPIV1(cctx)
		if err != nil {
			return err
		}
		defer closer()

		really := cctx.Bool("really-do-it")
		if !really {
			//nolint:golint
			return fmt.Errorf("--really-do-it must be specified for this action to have an effect; you have been warned")
		}

		local := cctx.Bool("local")

		ctx := lcli.ReqContext(cctx)
		return api.MpoolCleanup(ctx, api2.MpoolCleanupOpts{
			DropLocal:  local,
			MinBaseFee: types.NewInt(uint64(cctx.Int64("below-basefee"))),
		})
	},
}
