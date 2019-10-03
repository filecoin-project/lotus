package cli

import (
	"fmt"

	"gopkg.in/urfave/cli.v2"
)

var mpoolCmd = &cli.Command{
	Name:  "mpool",
	Usage: "Manage message pool",
	Subcommands: []*cli.Command{
		mpoolPending,
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
			fmt.Println(msg)
		}

		return nil
	},
}
