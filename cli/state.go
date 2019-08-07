package cli

import (
	"fmt"
	"math/big"

	"gopkg.in/urfave/cli.v2"

	"github.com/filecoin-project/go-lotus/chain/actors"
	"github.com/filecoin-project/go-lotus/chain/address"
	types "github.com/filecoin-project/go-lotus/chain/types"
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
		api, err := GetAPI(cctx)
		if err != nil {
			return err
		}

		ctx := ReqContext(cctx)

		var msg *types.Message
		if cctx.Args().Present() {
			maddr, err := address.NewFromString(cctx.Args().First())
			if err != nil {
				return err
			}

			enc, err := actors.SerializeParams(&actors.PowerLookupParams{
				Miner: maddr,
			})
			if err != nil {
				return err
			}

			msg = &types.Message{
				To:     actors.StorageMarketAddress,
				From:   actors.StorageMarketAddress,
				Method: actors.SMAMethods.PowerLookup,
				Params: enc,
			}
		} else {
			msg = &types.Message{
				To:     actors.StorageMarketAddress,
				From:   actors.StorageMarketAddress,
				Method: actors.SMAMethods.GetTotalStorage,
			}
		}
		ret, err := api.ChainCall(ctx, msg, nil)
		if err != nil {
			return err
		}
		if ret.ExitCode != 0 {
			return fmt.Errorf("call to get power failed: %d", ret.ExitCode)
		}

		v := big.NewInt(0).SetBytes(ret.Return)
		fmt.Println(v.String())
		return nil
	},
}

var stateSectorsCmd = &cli.Command{
	Name:  "sectors",
	Usage: "Query the sector set of a miner",
	Action: func(cctx *cli.Context) error {
		api, err := GetAPI(cctx)
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
		api, err := GetAPI(cctx)
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
