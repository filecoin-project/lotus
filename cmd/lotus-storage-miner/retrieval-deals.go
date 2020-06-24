package main

import (
	lcli "github.com/filecoin-project/lotus/cli"
	"github.com/urfave/cli/v2"
)

var retrievalDealsCmd = &cli.Command{
	Name:  "retrieval-deals",
	Usage: "Manage retrieval deals and related configuration",
	Subcommands: []*cli.Command{
		enableRetrievalCmd,
		disableRetrievalCmd,
	},
}

var enableRetrievalCmd = &cli.Command{
	Name:  "enable",
	Usage: "Configure the miner to consider retrieval deal proposals",
	Flags: []cli.Flag{},
	Action: func(cctx *cli.Context) error {
		api, closer, err := lcli.GetStorageMinerAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		return api.DealsSetAcceptingRetrievalDeals(lcli.DaemonContext(cctx), true)
	},
}

var disableRetrievalCmd = &cli.Command{
	Name:  "disable",
	Usage: "Configure the miner to reject all retrieval deal proposals",
	Flags: []cli.Flag{},
	Action: func(cctx *cli.Context) error {
		api, closer, err := lcli.GetStorageMinerAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		return api.DealsSetAcceptingRetrievalDeals(lcli.DaemonContext(cctx), false)
	},
}
