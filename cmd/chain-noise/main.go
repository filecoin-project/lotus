package main

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"time"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/types"
	lcli "github.com/filecoin-project/lotus/cli"

	"gopkg.in/urfave/cli.v2"
)

func main() {
	app := &cli.App{
		Name:  "chain-noise",
		Usage: "Generate some spam transactions in the network",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "repo",
				EnvVars: []string{"LOTUS_PATH"},
				Hidden:  true,
				Value:   "~/.lotus", // TODO: Consider XDG_DATA_HOME
			},
		},
		Commands: []*cli.Command{runCmd},
	}

	if err := app.Run(os.Args); err != nil {
		fmt.Println("Error: ", err)
		os.Exit(1)
	}
}

var runCmd = &cli.Command{
	Name: "run",
	Action: func(cctx *cli.Context) error {
		addr, err := address.NewFromString(cctx.Args().First())
		if err != nil {
			return err
		}

		api, closer, err := lcli.GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := lcli.ReqContext(cctx)

		sendSmallFundsTxs(ctx, api, addr, 5)
		return nil
	},
}

func sendSmallFundsTxs(ctx context.Context, api api.FullNode, from address.Address, rate int) error {
	var sendSet []address.Address
	for i := 0; i < 20; i++ {
		naddr, err := api.WalletNew(ctx, "bls")
		if err != nil {
			return err
		}

		sendSet = append(sendSet, naddr)
	}

	tick := time.NewTicker(time.Second / time.Duration(rate))
	for {
		select {
		case <-tick.C:
			msg := &types.Message{
				From:     from,
				To:       sendSet[rand.Intn(20)],
				Value:    types.NewInt(1),
				GasLimit: types.NewInt(100000),
				GasPrice: types.NewInt(0),
			}

			smsg, err := api.MpoolPushMessage(ctx, msg)
			if err != nil {
				return err
			}
			fmt.Println("Message sent: ", smsg.Cid())
		case <-ctx.Done():
			return nil
		}
	}

	return nil
}
