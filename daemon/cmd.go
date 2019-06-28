package daemon

import (
	"gopkg.in/urfave/cli.v2"
)

var Cmd = &cli.Command{
	Name:  "daemon",
	Usage: "Start a lotus daemon process",
	Action: func(context *cli.Context) error {
		return serveRPC()
	},
}
