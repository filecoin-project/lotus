package main

import (
	"fmt"
	"strconv"

	"gopkg.in/urfave/cli.v2"

	lcli "github.com/filecoin-project/go-lotus/cli"
)

var storeGarbageCmd = &cli.Command{
	Name:  "store-garbage",
	Usage: "store random data in a sector",
	Action: func(cctx *cli.Context) error {
		nodeApi, err := lcli.GetStorageMinerAPI(cctx)
		if err != nil {
			return err
		}
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
	},
}

var sectorsStatusCmd = &cli.Command{
	Name:  "status",
	Usage: "Get the seal status of a sector by its ID",
	Action: func(cctx *cli.Context) error {
		nodeApi, err := lcli.GetStorageMinerAPI(cctx)
		if err != nil {
			return err
		}
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
		fmt.Printf("SealStatusCode:\t%d\n", status.SealStatusCode)
		fmt.Printf("SealErrorMsg:\t%q\n", status.SealErrorMsg)
		fmt.Printf("CommD:\t\t%x\n", status.CommD)
		fmt.Printf("CommR:\t\t%x\n", status.CommR)
		fmt.Printf("CommR*:\t\t%x\n", status.CommRStar)
		fmt.Printf("Proof:\t\t%x\n", status.Proof)
		fmt.Printf("Pieces:\t\t%s\n", status.Pieces)
		return nil
	},
}

var sectorsStagedListCmd = &cli.Command{
	Name:  "list-staged", // TODO: nest this under a 'staged' subcommand? idk
	Usage: "List staged sectors",
	Action: func(cctx *cli.Context) error {
		nodeApi, err := lcli.GetStorageMinerAPI(cctx)
		if err != nil {
			return err
		}
		ctx := lcli.ReqContext(cctx)

		staged, err := nodeApi.SectorsStagedList(ctx)
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
		nodeApi, err := lcli.GetStorageMinerAPI(cctx)
		if err != nil {
			return err
		}
		ctx := lcli.ReqContext(cctx)

		return nodeApi.SectorsStagedSeal(ctx)
	},
}
