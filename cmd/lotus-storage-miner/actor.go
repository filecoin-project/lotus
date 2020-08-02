package main

import (
	"fmt"

	ma "github.com/multiformats/go-multiaddr"
	"github.com/urfave/cli/v2"

	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/builtin/miner"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/types"
	lcli "github.com/filecoin-project/lotus/cli"
)

var actorCmd = &cli.Command{
	Name:  "actor",
	Usage: "manipulate the miner actor",
	Subcommands: []*cli.Command{
		actorSetAddrsCmd,
	},
}

var actorSetAddrsCmd = &cli.Command{
	Name:  "set-addrs",
	Usage: "set addresses that your miner can be publicly dialed on",
	Flags: []cli.Flag{
		&cli.Int64Flag{
			Name:  "gas-limit",
			Usage: "set gas limit",
			Value: 0,
		},
	},
	Action: func(cctx *cli.Context) error {
		nodeAPI, closer, err := lcli.GetStorageMinerAPI(cctx)
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

		var addrs []abi.Multiaddrs
		for _, a := range cctx.Args().Slice() {
			maddr, err := ma.NewMultiaddr(a)
			if err != nil {
				return fmt.Errorf("failed to parse %q as a multiaddr: %w", a, err)
			}

			addrs = append(addrs, maddr.Bytes())
		}

		maddr, err := nodeAPI.ActorAddress(ctx)
		if err != nil {
			return err
		}

		minfo, err := api.StateMinerInfo(ctx, maddr, types.EmptyTSK)
		if err != nil {
			return err
		}

		params, err := actors.SerializeParams(&miner.ChangeMultiaddrsParams{NewMultiaddrs: addrs})
		if err != nil {
			return err
		}

		gasLimit := cctx.Int64("gas-limit")

		smsg, err := api.MpoolPushMessage(ctx, &types.Message{
			To:       maddr,
			From:     minfo.Worker,
			Value:    types.NewInt(0),
			GasLimit: gasLimit,
			Method:   18,
			Params:   params,
		})
		if err != nil {
			return err
		}

		fmt.Printf("Requested multiaddrs change in message %s\n", smsg.Cid())
		return nil

	},
}
