package cliutil

import "github.com/urfave/cli/v2"

// IsSuperVerbose is a global var signalling if we're running in super verbose
// mode or not (default: false).
var IsSuperVerbose bool

// FlagSuperVerbose enables super verbose mode, which is useful when debugging
// the CLI itself. It should be included as a flag on the top-level command
// (e.g. lotus -vv, lotus-miner -vv).
var FlagSuperVerbose = &cli.BoolFlag{
	Name:        "vv",
	Usage:       "enables super verbose mode, useful for debugging the CLI",
	Destination: &IsSuperVerbose,
}
