package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"os"
	"time"

	"github.com/filecoin-project/specs-actors/actors/crypto"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/types"
	lcli "github.com/filecoin-project/lotus/cli"

	"github.com/urfave/cli/v2"
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
	Flags: []cli.Flag{
		&cli.IntFlag{
			Name:  "rate",
			Usage: "specify message sending rate to attempt",
			Value: 10,
		},
		&cli.Int64Flag{
			Name:  "gasPrice",
			Usage: "Specify gas price to use (0 for auto)",
			Value: 0,
		},
	},
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

		return sendSmallFundsTxs(ctx, api, addr, cctx.Int("rate"), cctx.Int64("gasPrice"))
	},
}

func sendSmallFundsTxs(ctx context.Context, api api.FullNode, from address.Address, rate int, gasprice int64) error {
	var sendSet []address.Address
	for i := 0; i < 20; i++ {
		naddr, err := api.WalletNew(ctx, crypto.SigTypeSecp256k1)
		if err != nil {
			return err
		}

		sendSet = append(sendSet, naddr)
	}

	curnonce, err := api.MpoolGetNonce(ctx, from)
	if err != nil {
		return err
	}

	tick := build.Clock.Ticker(time.Second / time.Duration(rate))
	for {
		select {
		case <-tick.C:
			msg := &types.Message{
				From:     from,
				To:       sendSet[rand.Intn(20)],
				Value:    types.NewInt(1),
				GasLimit: 1300000,
				GasPrice: types.NewInt(uint64(gasprice)),
				Nonce:    curnonce,
			}
			curnonce++

			s := time.Now()
			smsg, err := api.WalletSignMessage(ctx, from, msg)
			if err != nil {
				return err
			}
			signtime := time.Since(s)

			s = time.Now()
			c, err := api.MpoolPush(ctx, smsg)
			if err != nil {
				return err
			}
			pushtime := time.Since(s)
			log.Println("Message sent: ", c, signtime, pushtime)
		case <-ctx.Done():
			return nil
		}
	}

	return nil
}
