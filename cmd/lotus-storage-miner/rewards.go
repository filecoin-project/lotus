package main

import (
	"fmt"

	"gopkg.in/urfave/cli.v2"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/types"
	lcli "github.com/filecoin-project/lotus/cli"

	"github.com/filecoin-project/specs-actors/actors/builtin"
)

var rewardsCmd = &cli.Command{
	Name: "rewards",
	Subcommands: []*cli.Command{
		rewardsRedeemCmd,
	},
}

var rewardsRedeemCmd = &cli.Command{
	Name:  "redeem",
	Usage: "Redeem block rewards",
	Action: func(cctx *cli.Context) error {
		nodeApi, closer, err := lcli.GetStorageMinerAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		api, acloser, err := lcli.GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer acloser()

		ctx := lcli.ReqContext(cctx)

		maddr, err := nodeApi.ActorAddress(ctx)
		if err != nil {
			return err
		}

		mi, err := api.StateMinerInfo(ctx, maddr, types.EmptyTSK)
		if err != nil {
			return err
		}

		params, err := actors.SerializeParams(&maddr)
		if err != nil {
			return err
		}

		workerNonce, err := api.MpoolGetNonce(ctx, mi.Worker)
		if err != nil {
			return err
		}

		panic("todo correct method; call miner actor")

		smsg, err := api.WalletSignMessage(ctx, mi.Worker, &types.Message{
			To:       builtin.RewardActorAddr,
			From:     mi.Worker,
			Nonce:    workerNonce,
			Value:    types.NewInt(0),
			GasPrice: types.NewInt(1),
			GasLimit: 100000,
			Method:   0,
			Params:   params,
		})
		if err != nil {
			return err
		}

		mcid, err := api.MpoolPush(ctx, smsg)
		if err != nil {
			return err
		}

		fmt.Printf("Requested rewards withdrawal in message %s\n", mcid)

		return nil
	},
}
