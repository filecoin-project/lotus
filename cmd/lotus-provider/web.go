package main

import (
	"github.com/filecoin-project/lotus/provider/lpweb"
	"github.com/urfave/cli/v2"
)

var webCmd = &cli.Command{
	Name:        "web",
	Usage:       "Start lotus provider web interface",
	Description: "Start an instance of lotus provider web interface.",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "listen",
			Usage: "Address to listen on",
			Value: "127.0.0.1:4701",
		},
	},
	Action: func(cctx *cli.Context) error {
		db, err := makeDB(cctx)
		if err != nil {
			return err
		}

		return lpweb.ServeWeb(cctx.String("listen"), db)
	},
}
