package cli

import (
	"fmt"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/chain/types"
	"gopkg.in/urfave/cli.v2"
)

var sendCmd = &cli.Command{
	Name:      "send",
	Usage:     "Send funds between accounts",
	ArgsUsage: "<target> <amount>",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "source",
			Usage: "optinally specifiy the account to send funds from",
		},
	},
	Action: func(cctx *cli.Context) error {
		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		ctx := ReqContext(cctx)

		if cctx.Args().Len() != 2 {
			return fmt.Errorf("'send' expects two arguments, target and amount")
		}

		toAddr, err := address.NewFromString(cctx.Args().Get(0))
		if err != nil {
			return err
		}

		val, err := types.ParseFIL(cctx.Args().Get(1))
		if err != nil {
			return err
		}

		var fromAddr address.Address
		if from := cctx.String("source"); from == "" {
			defaddr, err := api.WalletDefaultAddress(ctx)
			if err != nil {
				return err
			}

			fromAddr = defaddr
		} else {
			addr, err := address.NewFromString(from)
			if err != nil {
				return err
			}

			fromAddr = addr
		}

		msg := &types.Message{
			From:     fromAddr,
			To:       toAddr,
			Value:    types.BigInt(val),
			GasLimit: types.NewInt(1000),
			GasPrice: types.NewInt(0),
		}

		_, err = api.MpoolPushMessage(ctx, msg)
		if err != nil {
			return err
		}

		return nil
	},
}
