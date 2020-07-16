package cli

import (
	"fmt"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/urfave/cli/v2"
)

var sendCmd = &cli.Command{
	Name:      "send",
	Usage:     "Send funds between accounts",
	ArgsUsage: "[targetAddress] [amount]",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "from",
			Usage: "optionally specify the account to send funds from",
		},
		&cli.StringFlag{
			Name:  "gas-price",
			Usage: "specify gas price to use in AttoFIL",
			Value: "0",
		},
		&cli.Int64Flag{
			Name:  "nonce",
			Usage: "specify the nonce to use",
			Value: -1,
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
		if from := cctx.String("from"); from == "" {
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

		gp, err := types.BigFromString(cctx.String("gas-price"))
		if err != nil {
			return err
		}

		msg := &types.Message{
			From:     fromAddr,
			To:       toAddr,
			Value:    types.BigInt(val),
			GasLimit: 10000,
			GasPrice: gp,
		}

		if cctx.Int64("nonce") > 0 {
			msg.Nonce = uint64(cctx.Int64("nonce"))
			sm, err := api.WalletSignMessage(ctx, fromAddr, msg)
			if err != nil {
				return err
			}

			_, err = api.MpoolPush(ctx, sm)
			if err != nil {
				return err
			}
			fmt.Println(sm.Cid())
		} else {
			sm, err := api.MpoolPushMessage(ctx, msg)
			if err != nil {
				return err
			}
			fmt.Println(sm.Cid())
		}

		return nil
	},
}
