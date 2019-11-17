package cli

import (
	"encoding/json"
	"fmt"

	"gopkg.in/urfave/cli.v2"
)

var mpoolCmd = &cli.Command{
	Name:  "mpool",
	Usage: "Manage message pool",
	Subcommands: []*cli.Command{
		mpoolPending,
		mpoolSub,
	},
}

var mpoolPending = &cli.Command{
	Name:  "pending",
	Usage: "Get pending messages",
	Action: func(cctx *cli.Context) error {
		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		ctx := ReqContext(cctx)

		msgs, err := api.MpoolPending(ctx, nil)
		if err != nil {
			return err
		}

		for _, msg := range msgs {
			out, err := json.MarshalIndent(msg, "", "  ")
			if err != nil {
				return err
			}
			fmt.Println(string(out))
		}

		return nil
	},
}

var mpoolSub = &cli.Command{
	Name:  "sub",
	Usage: "Subscibe to mpool changes",
	Action: func(cctx *cli.Context) error {
		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		ctx := ReqContext(cctx)

		sub, err := api.MpoolSub(ctx)
		if err != nil {
			return err
		}

		for {
			select {
			case update := <-sub:
				out, err := json.MarshalIndent(update, "", "  ")
				if err != nil {
					return err
				}
				fmt.Println(string(out))
			case <-ctx.Done():
				return nil
			}
		}
	},
}
