// +build !nodaemon

package daemon

import (
	"context"

	"gopkg.in/urfave/cli.v2"

	"github.com/filecoin-project/go-lotus/node"
	"github.com/filecoin-project/go-lotus/node/repo"
)

// Cmd is the `go-lotus daemon` command
var Cmd = &cli.Command{
	Name:  "daemon",
	Usage: "Start a lotus daemon process",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "api",
			Value: ":1234",
		},
	},
	Action: func(cctx *cli.Context) error {
		ctx := context.Background()
		repo := repo.NewMemory(nil)

		api, err := node.New(ctx,
			node.Online(),
			node.Repo(repo),
		)
		if err != nil {
			return err
		}

		return serveRPC(api, cctx.String("api"))
	},
}
