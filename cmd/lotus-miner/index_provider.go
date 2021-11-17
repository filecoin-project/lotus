package main

import (
	"fmt"

	"github.com/ipfs/go-cid"

	"github.com/fatih/color"
	"github.com/urfave/cli/v2"

	lcli "github.com/filecoin-project/lotus/cli"
)

var indexProvCmd = &cli.Command{
	Name:  "index",
	Usage: "Manage the index provider on the markets subsystem",
	Subcommands: []*cli.Command{
		indexProvAnnounceCmd,
	},
}

var indexProvAnnounceCmd = &cli.Command{
	Name:      "announce",
	ArgsUsage: "<deal proposal cid>",
	Usage:     "Announce a deal to indexers so they can download its index",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:        "color",
			Usage:       "use color in display output",
			DefaultText: "depends on output being a TTY",
		},
	},
	Action: func(cctx *cli.Context) error {
		if cctx.IsSet("color") {
			color.NoColor = !cctx.Bool("color")
		}

		if cctx.NArg() != 1 {
			return fmt.Errorf("must provide the deal proposal CID")
		}

		proposalCidStr := cctx.Args().First()
		proposalCid, err := cid.Parse(proposalCidStr)
		if err != nil {
			return fmt.Errorf("invalid deal proposal CID: %w", proposalCid)
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
