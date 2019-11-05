package cli

import (
	"fmt"

	"github.com/filecoin-project/lotus/chain/address"
	"gopkg.in/urfave/cli.v2"
)

var unregisterMinerCmd = &cli.Command{
	Name:  "unregister-miner",
	Usage: "Manually unregister miner actor",
	Action: func(cctx *cli.Context) error {
		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := ReqContext(cctx)

		if !cctx.Args().Present() {
			return fmt.Errorf("must pass address of miner to unregister")
		}

		maddr, err := address.NewFromString(cctx.Args().First())
		if err != nil {
			return err
		}

		return api.MinerUnregister(ctx, maddr)
	},
}
