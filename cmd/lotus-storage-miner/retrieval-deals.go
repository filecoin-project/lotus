package main

import (
	"fmt"

	lcli "github.com/filecoin-project/lotus/cli"
	"github.com/urfave/cli/v2"
)

var retrievalDealsCmd = &cli.Command{
	Name:  "retrieval-deals",
	Usage: "Manage retrieval deals and related configuration",
	Subcommands: []*cli.Command{
		retrievalDealSelectionCmd,
	},
}

var retrievalDealSelectionCmd = &cli.Command{
	Name:  "selection",
	Usage: "Configure acceptance criteria for retrieval deal proposals",
	Subcommands: []*cli.Command{
		retrievalDealSelectionShowCmd,
		retrievalDealSelectionResetCmd,
		retrievalDealSelectionRejectCmd,
	},
}

var retrievalDealSelectionShowCmd = &cli.Command{
	Name:  "list",
	Usage: "List retrieval deal proposal selection criteria",
	Action: func(cctx *cli.Context) error {
		smapi, closer, err := lcli.GetStorageMinerAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		onlineOk, err := smapi.DealsConsiderOnlineRetrievalDeals(lcli.DaemonContext(cctx))
		if err != nil {
			return err
		}

		offlineOk, err := smapi.DealsConsiderOfflineRetrievalDeals(lcli.DaemonContext(cctx))
		if err != nil {
			return err
		}

		fmt.Printf("considering online retrieval deals: %t\n", onlineOk)
		fmt.Printf("considering offline retrieval deals: %t\n", offlineOk)

		return nil
	},
}

var retrievalDealSelectionResetCmd = &cli.Command{
	Name:  "reset",
	Usage: "Reset retrieval deal proposal selection criteria to default values",
	Action: func(cctx *cli.Context) error {
		smapi, closer, err := lcli.GetStorageMinerAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		err = smapi.DealsSetConsiderOnlineRetrievalDeals(lcli.DaemonContext(cctx), true)
		if err != nil {
			return err
		}

		err = smapi.DealsSetConsiderOfflineRetrievalDeals(lcli.DaemonContext(cctx), true)
		if err != nil {
			return err
		}

		return nil
	},
}

var retrievalDealSelectionRejectCmd = &cli.Command{
	Name:  "reject",
	Usage: "Configure criteria which necessitate automatic rejection",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name: "online",
		},
		&cli.BoolFlag{
			Name: "offline",
		},
	},
	Action: func(cctx *cli.Context) error {
		smapi, closer, err := lcli.GetStorageMinerAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		if cctx.Bool("online") {
			err = smapi.DealsSetConsiderOnlineRetrievalDeals(lcli.DaemonContext(cctx), false)
			if err != nil {
				return err
			}
		}

		if cctx.Bool("offline") {
			err = smapi.DealsSetConsiderOfflineRetrievalDeals(lcli.DaemonContext(cctx), false)
			if err != nil {
				return err
			}
		}

		return nil
	},
}
