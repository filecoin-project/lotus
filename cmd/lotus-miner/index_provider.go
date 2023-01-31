package main

import (
	"fmt"

	"github.com/ipfs/go-cid"
	"github.com/urfave/cli/v2"

	lcli "github.com/filecoin-project/lotus/cli"
)

var indexProvCmd = &cli.Command{
	Name:  "index",
	Usage: "Manage the index provider on the markets subsystem",
	Subcommands: []*cli.Command{
		indexProvAnnounceCmd,
		indexProvAnnounceAllCmd,
	},
}

var indexProvAnnounceCmd = &cli.Command{
	Name:      "announce",
	ArgsUsage: "<deal proposal cid>",
	Usage:     "Announce a deal to indexers so they can download its index",
	Action: func(cctx *cli.Context) error {
		if cctx.NArg() != 1 {
			return lcli.IncorrectNumArgs(cctx)
		}

		proposalCidStr := cctx.Args().First()
		proposalCid, err := cid.Parse(proposalCidStr)
		if err != nil {
			return fmt.Errorf("invalid deal proposal CID: %w", err)
		}

		marketsApi, closer, err := lcli.GetMarketsAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		ctx := lcli.ReqContext(cctx)

		return marketsApi.IndexerAnnounceDeal(ctx, proposalCid)
	},
}

var indexProvAnnounceAllCmd = &cli.Command{
	Name:  "announce-all",
	Usage: "Announce all active deals to indexers so they can download the indices",
	Action: func(cctx *cli.Context) error {
		marketsApi, closer, err := lcli.GetMarketsAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		ctx := lcli.ReqContext(cctx)

		return marketsApi.IndexerAnnounceAllDeals(ctx)
	},
}
