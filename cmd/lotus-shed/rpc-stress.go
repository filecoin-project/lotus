package main

import (
	"fmt"

	"github.com/filecoin-project/lotus/chain/types"

	lcli "github.com/filecoin-project/lotus/cli"
	"github.com/ipfs/go-cid"
	"github.com/urfave/cli/v2"
)

var rpcStressCmd = &cli.Command{
	Name:  "rpc-stress",
	Usage: "stress out a Lotus node's RPC service by StateWaitMsging for every message in the mempool, FOREVER",
	Flags: []cli.Flag{
		&cli.IntFlag{
			Name:  "workers",
			Usage: "number of workers spamming StateWaitMsg",
			Value: 20,
		},
	}, Action: func(cctx *cli.Context) error {
		if cctx.Args().Present() {
			return lcli.IncorrectNumArgs(cctx)
		}

		api, closer, err := lcli.GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}

		defer closer()
		ctx := lcli.ReqContext(cctx)

		pendingMsgCh := make(chan cid.Cid)
		count := 0
		go func() {
			for {
				pendingMsgs, err := api.MpoolPending(ctx, types.EmptyTSK)
				if err != nil {
					panic(err)
				}

				for _, pendingMsg := range pendingMsgs {
					count++
					if count%50 == 0 {
						fmt.Println("waited for ", count, "messages")
					}
					pendingMsgCh <- pendingMsg.Cid()
				}
			}
		}()

		waitMsgFunc := func() {
			for msg := range pendingMsgCh {
				_, err := api.StateWaitMsg(ctx, msg, 5)
				if err != nil {
					panic(err)
				}
			}
		}

		for i := 0; i < cctx.Int("workers"); i++ {
			go waitMsgFunc()
		}
		select {
		case <-ctx.Done():
			fmt.Println("all done, all done!")
			return nil
		}

	},
}
