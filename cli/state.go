package cli

import (
	"fmt"

	"github.com/filecoin-project/go-lotus/chain/address"
	"github.com/filecoin-project/go-lotus/chain/types"

	"github.com/ipfs/go-cid"
	"gopkg.in/urfave/cli.v2"
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

		sectors, err := api.StateMinerProvingSet(ctx, maddr)
		if err != nil {
			return err
		}

		for _, s := range sectors {
			fmt.Printf("%d: %x %x\n", s.SectorID, s.CommR, s.CommD)
		}

		return nil
	},
}

var stateReplaySetCmd = &cli.Command{
	Name:  "replay",
	Usage: "Replay a particular message within a tipset",
	Action: func(cctx *cli.Context) error {
		if cctx.Args().Len() < 2 {
			fmt.Println("usage: <tipset> <message cid>")
			fmt.Println("The last cid passed will be used as the message CID")
			fmt.Println("All preceding ones will be used as the tipset")
			return nil
		}

		args := cctx.Args().Slice()
		mcid, err := cid.Decode(args[len(args)-1])
		if err != nil {
			return fmt.Errorf("message cid was invalid: %s", err)
		}

		var tscids []cid.Cid
		for _, s := range args[:len(args)-1] {
			c, err := cid.Decode(s)
			if err != nil {
				return fmt.Errorf("tipset cid was invalid: %s", err)
			}
			tscids = append(tscids, c)
		}

		api, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}

		ctx := ReqContext(cctx)

		var headers []*types.BlockHeader
		for _, c := range tscids {
			h, err := api.ChainGetBlock(ctx, c)
			if err != nil {
				return err
			}

			headers = append(headers, h)
		}

		ts, err := types.NewTipSet(headers)
		if err != nil {
			return err
		}

		api.StateReplay(ctx, ts, mcid)

		return nil
	},
}
