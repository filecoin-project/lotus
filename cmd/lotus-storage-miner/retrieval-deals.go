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

		isAcceptingRetrievalDeals, err := smapi.DealsConsiderOnlineRetrievalDeals(lcli.DaemonContext(cctx))
		if err != nil {
			return err
		}

		fmt.Printf("offline retrieval deals: %t\n", isAcceptingRetrievalDeals)
		fmt.Printf("online retrieval deals: %t\n", isAcceptingRetrievalDeals)

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

		return nil
	},
}

var retrievalDealSelectionRejectCmd = &cli.Command{
	Name:  "reject",
	Usage: "Configure criteria which necessitate automatic rejection",
	Action: func(cctx *cli.Context) error {
		smapi, closer, err := lcli.GetStorageMinerAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		err = smapi.DealsSetConsiderOnlineRetrievalDeals(lcli.DaemonContext(cctx), false)
		if err != nil {
			return err
		}

		return nil
	},
}
