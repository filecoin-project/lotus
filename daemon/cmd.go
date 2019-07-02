package daemon

import (
	"context"

	"gopkg.in/urfave/cli.v2"

	"github.com/filecoin-project/go-lotus/node"
)

var Cmd = &cli.Command{
	Name:  "daemon",
	Usage: "Start a lotus daemon process",
	Action: func(cctx *cli.Context) error {
		ctx := context.Background()

		api, err := node.New(ctx)
		if err != nil {
			return err
		}

		return serveRPC(api)
	},
}
