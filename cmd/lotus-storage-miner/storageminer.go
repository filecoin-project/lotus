package main

import (
	"gopkg.in/urfave/cli.v2"

	lcli "github.com/filecoin-project/go-lotus/cli"
)

var RunCmd = &cli.Command{
	Name:  "run",
	Usage: "Start a lotus storage miner process",
	Action: func(cctx *cli.Context) error {
		api, err := lcli.GetAPI(cctx)
		if err != nil {
			return err
		}
		ctx := lcli.ReqContext(cctx)

		v, err := api.Version(ctx)

		// TODO: libp2p node

		log.Infof("Remote version %s", v)
		return nil
	},
}
