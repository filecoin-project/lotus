//go:build nodaemon

package lotus

import (
	"errors"

	"github.com/urfave/cli/v2"
)

// DaemonCmd is the `go-lotus daemon` command
var DaemonCmd = &cli.Command{
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
