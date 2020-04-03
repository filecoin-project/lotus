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
		rewardsListCmd,
		rewardsRedeemCmd,
	},
}

var rewardsListCmd = &cli.Command{
	Name:  "list",
	Usage: "Print unclaimed block rewards earned",
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

		rewards, err := api.StateListRewards(ctx, maddr, types.EmptyTSK)
		if err != nil {
			return err
		}

		for _, r := range rewards {
			fmt.Printf("%d\t%d\t%s\n", r.StartEpoch, r.EndEpoch, types.FIL(r.Value))
		}

		return nil
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

		worker, err := api.StateMinerWorker(ctx, maddr, types.EmptyTSK)
		if err != nil {
			return err
		}

		params, err := actors.SerializeParams(&maddr)
		if err != nil {
			return err
		}

		workerNonce, err := api.MpoolGetNonce(ctx, worker)
		if err != nil {
			return err
		}

		smsg, err := api.WalletSignMessage(ctx, worker, &types.Message{
			To:       builtin.RewardActorAddr,
			From:     worker,
			Nonce:    workerNonce,
			Value:    types.NewInt(0),
			GasPrice: types.NewInt(1),
			GasLimit: 100000,
			Method:   builtin.MethodsReward.WithdrawReward,
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
