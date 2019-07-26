package cli

import (
	"fmt"

	"github.com/pkg/errors"
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
		api, err := GetAPI(cctx)
		if err != nil {
			return err
		}

		ctx := ReqContext(cctx)

		// TODO: this address needs to be the address of an actual miner
		maddr, err := api.WalletDefaultAddress(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to create miner address")
		}

		if err := api.MinerStart(ctx, maddr); err != nil {
			return err
		}

		fmt.Println("started mining")

		return nil
	},
}
