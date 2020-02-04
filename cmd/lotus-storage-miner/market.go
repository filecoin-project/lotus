package main

import (
	"fmt"

	"github.com/filecoin-project/lotus/chain/types"
	lcli "github.com/filecoin-project/lotus/cli"
	"gopkg.in/urfave/cli.v2"
)

var setPriceCmd = &cli.Command{
	Name:  "set-price",
	Usage: "Set price that miner will accept storage deals at",
	Flags: []cli.Flag{},
	Action: func(cctx *cli.Context) error {
		api, closer, err := lcli.GetStorageMinerAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		ctx := lcli.DaemonContext(cctx)

		if !cctx.Args().Present() {
			return fmt.Errorf("must specify price to set")
		}

		fp, err := types.ParseFIL(cctx.Args().First())
		if err != nil {
			return err
		}

		return api.SetPrice(ctx, types.BigInt(fp))
	},
}
