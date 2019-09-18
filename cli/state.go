package cli

import (
	"fmt"

	"gopkg.in/urfave/cli.v2"

	"github.com/filecoin-project/go-lotus/chain/address"
)

var stateCmd = &cli.Command{
	Name:  "state",
	Usage: "Interact with and query filecoin chain state",
	Subcommands: []*cli.Command{
		statePowerCmd,
		stateSectorsCmd,
		stateProvingSetCmd,
	},
}

var statePowerCmd = &cli.Command{
	Name:  "power",
	Usage: "Query network or miner power",
	Action: func(cctx *cli.Context) error {
		api, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}

		ctx := ReqContext(cctx)

		var maddr address.Address
		if cctx.Args().Present() {
			maddr, err = address.NewFromString(cctx.Args().First())
			if err != nil {
				return err
			}
		}
		power, err := api.StateMinerPower(ctx, maddr, nil)
		if err != nil {
			return err
		}

		res := power.TotalPower
		if cctx.Args().Present() {
			res = power.MinerPower
		}

		fmt.Println(res.String())
		return nil
	},
}

var stateSectorsCmd = &cli.Command{
	Name:  "sectors",
	Usage: "Query the sector set of a miner",
	Action: func(cctx *cli.Context) error {
		api, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}

		ctx := ReqContext(cctx)

		if !cctx.Args().Present() {
			return fmt.Errorf("must specify miner to list sectors for")
		}

		maddr, err := address.NewFromString(cctx.Args().First())
		if err != nil {
			return err
		}

		sectors, err := api.StateMinerSectors(ctx, maddr)
		if err != nil {
			return err
		}

		for _, s := range sectors {
			fmt.Printf("%d: %x %x\n", s.SectorID, s.CommR, s.CommD)
		}

		return nil
	},
}

var stateProvingSetCmd = &cli.Command{
	Name:  "proving",
	Usage: "Query the proving set of a miner",
	Action: func(cctx *cli.Context) error {
		api, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}

		ctx := ReqContext(cctx)

		if !cctx.Args().Present() {
			return fmt.Errorf("must specify miner to list sectors for")
		}

		maddr, err := address.NewFromString(cctx.Args().First())
		if err != nil {
			return err
		}

		sectors, err := api.StateMinerProvingSet(ctx, maddr, nil)
		if err != nil {
			return err
		}

		for _, s := range sectors {
			fmt.Printf("%d: %x %x\n", s.SectorID, s.CommR, s.CommD)
		}

		return nil
	},
}
