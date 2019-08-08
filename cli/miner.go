package cli

import (
	"fmt"

	"gopkg.in/urfave/cli.v2"
)

var minerCmd = &cli.Command{
	Name:  "miner",
	Usage: "Manage mining",
	Subcommands: []*cli.Command{
		minerStart,
	},
}

var minerStart = &cli.Command{
	Name:  "start",
	Usage: "start mining",
	Action: func(cctx *cli.Context) error {
		api, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}

		ctx := ReqContext(cctx)

		// TODO: this address needs to be the address of an actual miner
		maddr, err := api.WalletDefaultAddress(ctx)
		if err != nil {
			return err
		}

		if err := api.MinerStart(ctx, maddr); err != nil {
			return err
		}

		fmt.Println("started mining")

		return nil
	},
}
