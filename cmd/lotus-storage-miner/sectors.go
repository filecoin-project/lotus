package main

import (
	"fmt"
	"strconv"

	"gopkg.in/urfave/cli.v2"

	lcli "github.com/filecoin-project/lotus/cli"
)

var storeGarbageCmd = &cli.Command{
	Name:  "store-garbage",
	Usage: "store random data in a sector",
	Action: func(cctx *cli.Context) error {
		nodeApi, closer, err := lcli.GetStorageMinerAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := lcli.ReqContext(cctx)

		sectorId, err := nodeApi.StoreGarbageData(ctx)
		if err != nil {
			return err
		}

		fmt.Println(sectorId)
		return nil
	},
}

var sectorsCmd = &cli.Command{
	Name:  "sectors",
	Usage: "interact with sector store",
	Subcommands: []*cli.Command{
		sectorsStatusCmd,
		sectorsStagedListCmd,
		sectorsStagedSealCmd,
		sectorsRefsCmd,
	},
}

var sectorsStatusCmd = &cli.Command{
	Name:  "status",
	Usage: "Get the seal status of a sector by its ID",
	Action: func(cctx *cli.Context) error {
		nodeApi, closer, err := lcli.GetStorageMinerAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := lcli.ReqContext(cctx)

		if !cctx.Args().Present() {
			return fmt.Errorf("must specify sector ID to get status of")
		}

		id, err := strconv.ParseUint(cctx.Args().First(), 10, 64)
		if err != nil {
			return err
		}

		status, err := nodeApi.SectorsStatus(ctx, id)
		if err != nil {
			return err
		}

		fmt.Printf("SectorID:\t%d\n", status.SectorID)
		fmt.Printf("Status:\t%s\n", status.State.String())
		fmt.Printf("SealErrorMsg:\t%q\n", status.SealErrorMsg)
		fmt.Printf("CommD:\t\t%x\n", status.CommD)
		fmt.Printf("CommR:\t\t%x\n", status.CommR)
		fmt.Printf("Ticket:\t\t%x\n", status.Ticket.TicketBytes)
		fmt.Printf("TicketH:\t\t%d\n", status.Ticket.BlockHeight)
		fmt.Printf("Proof:\t\t%x\n", status.Proof)
		fmt.Printf("Pieces:\t\t%v\n", status.Pieces)
		return nil
	},
}

var sectorsStagedListCmd = &cli.Command{
	Name:  "list-staged", // TODO: nest this under a 'staged' subcommand? idk
	Usage: "List staged sectors",
	Action: func(cctx *cli.Context) error {
		nodeApi, closer, err := lcli.GetStorageMinerAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := lcli.ReqContext(cctx)

		staged, err := nodeApi.SectorsList(ctx)
		if err != nil {
			return err
		}

		for _, s := range staged {
			fmt.Println(s)
		}
		return nil
	},
}

var sectorsStagedSealCmd = &cli.Command{
	Name:  "seal-staged", // TODO: nest this under a 'staged' subcommand? idk
	Usage: "Seal staged sectors",
	Action: func(cctx *cli.Context) error {
		nodeApi, closer, err := lcli.GetStorageMinerAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := lcli.ReqContext(cctx)

		return nodeApi.SectorsStagedSeal(ctx)
	},
}

var sectorsRefsCmd = &cli.Command{
	Name:  "refs",
	Usage: "List References to sectors",
	Action: func(cctx *cli.Context) error {
		nodeApi, closer, err := lcli.GetStorageMinerAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := lcli.ReqContext(cctx)

		refs, err := nodeApi.SectorsRefs(ctx)
		if err != nil {
			return err
		}

		for name, refs := range refs {
			fmt.Printf("Block %s:\n", name)
			for _, ref := range refs {
				fmt.Printf("\t%s+%d %d bytes\n", ref.Piece, ref.Offset, ref.Size)
			}
		}
		return nil
	},
}
