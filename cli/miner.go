package cli

import (
	"fmt"

	"github.com/filecoin-project/go-lotus/chain/address"
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

		if !cctx.Args().Present() {
			return fmt.Errorf("must specify miner actor address to mine for")
		}

		// TODO: need to pull this from disk or something
		maddr, err := address.NewFromString(cctx.Args().First())
		if err != nil {
			return err
		}

		if err := api.MinerRegister(ctx, maddr); err != nil {
			return err
		}

		fmt.Println("started mining")

		return nil
	},
}
