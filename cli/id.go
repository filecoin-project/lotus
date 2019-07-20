package cli

import (
	"fmt"

	"gopkg.in/urfave/cli.v2"
)

var idCmd = &cli.Command{
	Name:  "id",
	Usage: "Get node identity",
	Action: func(cctx *cli.Context) error {
		api, err := getAPI(cctx)
		if err != nil {
			return err
		}

		ctx := reqContext(cctx)

		pid, err := api.ID(ctx)
		if err != nil {
			return err
		}

		fmt.Println(pid)
		return nil
	},
}
