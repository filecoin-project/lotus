// +build nodaemon

package daemon

import (
	"errors"

	"gopkg.in/urfave/cli.v2"
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
		return errors.New("daemon support not included in this binary")
	},
}
