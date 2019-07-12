package cli

import (
	"fmt"

	"github.com/pkg/errors"
	"gopkg.in/urfave/cli.v2"

	"github.com/filecoin-project/go-lotus/chain/address"
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
		api, err := getAPI(cctx)
		if err != nil {
			return err
		}

		ctx := reqContext(cctx)

		// TODO: this address needs to be the address of an actual miner
		maddr, err := address.NewIDAddress(523423423)
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
