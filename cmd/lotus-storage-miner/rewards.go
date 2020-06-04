package main

import (
	"fmt"

	"gopkg.in/urfave/cli.v2"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/types"
	lcli "github.com/filecoin-project/lotus/cli"
	"github.com/filecoin-project/specs-actors/actors/builtin"
	"github.com/filecoin-project/specs-actors/actors/builtin/miner"
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
	Flags: []cli.Flag{
		&cli.Int64Flag{
			Name:  "gas-limit",
			Usage: "set gas limit",
			Value: 100000,
		},
	},
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

		mact, err := api.StateGetActor(ctx, maddr, types.EmptyTSK)
		if err != nil {
			return err
		}

		params, err := actors.SerializeParams(&miner.WithdrawBalanceParams{
			mact.Balance, // Default to attempting to withdraw all the extra funds in the miner actor
		})
		if err != nil {
			return err
		}

		gasLimit := cctx.Int64("gas-limit")

		smsg, err := api.MpoolPushMessage(ctx, &types.Message{
			To:       maddr,
			From:     mi.Owner,
			Value:    types.NewInt(0),
			GasPrice: types.NewInt(1),
			GasLimit: gasLimit,
			Method:   builtin.MethodsMiner.WithdrawBalance,
			Params:   params,
		})
		if err != nil {
			return err
		}

		fmt.Printf("Requested rewards withdrawal in message %s\n", smsg.Cid())

		return nil
	},
}
