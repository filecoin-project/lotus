package cli

import (
	"fmt"

	"github.com/filecoin-project/go-lotus/chain/address"
	types "github.com/filecoin-project/go-lotus/chain/types"
	"gopkg.in/urfave/cli.v2"
)

var sendCmd = &cli.Command{
	Name:  "send",
	Usage: "send funds between accounts",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "source",
			Usage: "optinally specifiy the account to send funds from",
		},
	},
	Action: func(cctx *cli.Context) error {
		api, err := GetAPI(cctx)
		if err != nil {
			return err
		}

		ctx := ReqContext(cctx)

		if cctx.Args().Len() != 2 {
			return fmt.Errorf("'send' expects two arguments, target and amount")
		}

		toAddr, err := address.NewFromString(cctx.Args().Get(0))
		if err != nil {
			return err
		}

		val, err := types.BigFromString(cctx.Args().Get(1))
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

		nonce, err := api.MpoolGetNonce(ctx, fromAddr)
		if err != nil {
			return err
		}

		msg := &types.Message{
			From:     fromAddr,
			To:       toAddr,
			Value:    val,
			Nonce:    nonce,
			GasLimit: types.NewInt(10000),
			GasPrice: types.NewInt(1),
		}

		sermsg, err := msg.Serialize()
		if err != nil {
			return err
		}

		sig, err := api.WalletSign(ctx, fromAddr, sermsg)
		if err != nil {
			return err
		}

		smsg := &types.SignedMessage{
			Message:   *msg,
			Signature: *sig,
		}

		if err := api.MpoolPush(ctx, smsg); err != nil {
			return err
		}

		return nil
	},
}
