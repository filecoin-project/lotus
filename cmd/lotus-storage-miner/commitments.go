package main

import (
	"fmt"
	lcli "github.com/filecoin-project/lotus/cli"

	"gopkg.in/urfave/cli.v2"
)

var commitmentsCmd = &cli.Command{
	Name:  "commitments",
	Usage: "interact with commitment tracker",
	Subcommands: []*cli.Command{
		commitmentsListCmd,
	},
}

var commitmentsListCmd = &cli.Command{
	Name:  "list",
	Usage: "List tracked sector commitments",
	Action: func(cctx *cli.Context) error {
		api, closer, err := lcli.GetStorageMinerAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		ctx := lcli.ReqContext(cctx)

		comms, err := api.CommitmentsList(ctx)
		if err != nil {
			return err
		}

		for _, comm := range comms {
			fmt.Printf("%s:%d msg:%s, deals: %v\n", comm.Miner, comm.SectorID, comm.CommitMsg, comm.DealIDs)
		}

		return nil
	},
}
