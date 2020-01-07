package main

import (
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/chain/types"
	lcli "github.com/filecoin-project/lotus/cli"
	"gopkg.in/urfave/cli.v2"
)

var noncefix = &cli.Command{
	Name: "noncefix",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "repo",
			EnvVars: []string{"LOTUS_PATH"},
			Hidden:  true,
			Value:   "~/.lotus", // TODO: Consider XDG_DATA_HOME
		},
		&cli.Uint64Flag{
			Name: "start",
		},
		&cli.Uint64Flag{
			Name: "end",
		},
		&cli.StringFlag{
			Name: "addr",
		},
	},
	Action: func(cctx *cli.Context) error {
		api, closer, err := lcli.GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}

		defer closer()
		ctx := lcli.ReqContext(cctx)

		addr, err := address.NewFromString(cctx.String("addr"))
		if err != nil {
			return err
		}

		for i := cctx.Uint64("start"); i < cctx.Uint64("end"); i++ {
			msg := &types.Message{
				From:     addr,
				To:       addr,
				Value:    types.NewInt(1),
				GasLimit: types.NewInt(1000),
				GasPrice: types.NewInt(1),
				Nonce:    i,
			}
			smsg, err := api.WalletSignMessage(ctx, addr, msg)
			if err != nil {
				return err
			}

			err = api.MpoolPush(ctx, smsg)
			if err != nil {
				return err
			}
		}

		return nil
	},
}
