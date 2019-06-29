package daemon

import (
	goctx "context"

	"gopkg.in/urfave/cli.v2"

	"github.com/filecoin-project/go-lotus/node"
)

var Cmd = &cli.Command{
	Name:  "daemon",
	Usage: "Start a lotus daemon process",
	Action: func(context *cli.Context) error {
		ctx := goctx.Background()

		api, err := node.New(ctx)
		if err != nil {
			return err
		}

		return serveRPC(api)
	},
}
