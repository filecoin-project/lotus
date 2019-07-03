package cli

import (
	"gopkg.in/urfave/cli.v2"
)

// Commands is the root group of CLI commands
var Commands = []*cli.Command{
	versionCmd,
}
