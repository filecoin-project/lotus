package main

import (
	"encoding/json"
	"fmt"

	"github.com/filecoin-project/lotus/chain/types"
	lcli "github.com/filecoin-project/lotus/cli"
	"github.com/ipfs/go-cid"
	"github.com/urfave/cli/v2"
)

var enableCmd = &cli.Command{
	Name:  "enable",
	Usage: "Configure the miner to consider storage deal proposals",
	Flags: []cli.Flag{},
	Action: func(cctx *cli.Context) error {
		panic("enable storage deals")
	},
}

var disableCmd = &cli.Command{
	Name:  "disable",
	Usage: "Configure the miner to reject all storage deal proposals",
	Flags: []cli.Flag{},
	Action: func(cctx *cli.Context) error {
		panic("disable storage deals")
	},
}

var setPriceCmd = &cli.Command{
	Name:  "set-price",
	Usage: "Set price that miner will accept storage deals at (FIL / GiB / Epoch)",
	Flags: []cli.Flag{},
	Action: func(cctx *cli.Context) error {
		api, closer, err := lcli.GetStorageMinerAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		ctx := lcli.DaemonContext(cctx)

		if !cctx.Args().Present() {
			return fmt.Errorf("must specify price to set")
		}

		fp, err := types.ParseFIL(cctx.Args().First())
		if err != nil {
			return err
		}

		return api.MarketSetPrice(ctx, types.BigInt(fp))
	},
}

var dealsCmd = &cli.Command{
	Name:  "deals",
	Usage: "interact with your deals",
	Subcommands: []*cli.Command{
		dealsImportDataCmd,
		dealsListCmd,
	},
}

var dealsImportDataCmd = &cli.Command{
	Name:      "import-data",
	Usage:     "Manually import data for a deal",
	ArgsUsage: "<proposal CID> <file>",
	Action: func(cctx *cli.Context) error {
		api, closer, err := lcli.GetStorageMinerAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		ctx := lcli.DaemonContext(cctx)

		if cctx.Args().Len() < 2 {
			return fmt.Errorf("must specify proposal CID and file path")
		}

		propCid, err := cid.Decode(cctx.Args().Get(0))
		if err != nil {
			return err
		}

		fpath := cctx.Args().Get(1)

		return api.DealsImportData(ctx, propCid, fpath)

	},
}

var dealsListCmd = &cli.Command{
	Name:  "list",
	Usage: "List all deals for this miner",
	Action: func(cctx *cli.Context) error {
		api, closer, err := lcli.GetStorageMinerAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		ctx := lcli.DaemonContext(cctx)

		deals, err := api.MarketListIncompleteDeals(ctx)
		if err != nil {
			return err
		}

		data, err := json.MarshalIndent(deals, "", "  ")
		if err != nil {
			return err
		}

		fmt.Println(string(data))
		return nil
	},
}
