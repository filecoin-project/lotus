package clicommands

import (
	"github.com/urfave/cli/v2"

	lcli "github.com/filecoin-project/lotus/cli"
)

var Commands = []*cli.Command{
	lcli.WithCategory("basic", lcli.SendCmd),
	lcli.WithCategory("basic", lcli.WalletCmd),
	lcli.WithCategory("basic", lcli.InfoCmd),
	lcli.WithCategory("basic", lcli.MultisigCmd),
	lcli.WithCategory("basic", lcli.FilplusCmd),
	lcli.WithCategory("basic", lcli.PaychCmd),
	lcli.WithCategory("developer", lcli.AuthCmd),
	lcli.WithCategory("developer", lcli.MpoolCmd),
	lcli.WithCategory("developer", StateCmd),
	lcli.WithCategory("developer", lcli.ChainCmd),
	lcli.WithCategory("developer", lcli.LogCmd),
	{
		Name:  "index",
		Usage: "Commands related to managing the chainindex",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "output",
				Usage: "output format (json or text)",
				Value: "text",
			},
			&cli.IntFlag{
				Name:     "from",
				Usage:    "from specifies the starting tipset epoch for validation (inclusive)",
				Required: true,
			},
			&cli.IntFlag{
				Name:     "to",
				Usage:    "to specifies the ending tipset epoch for validation (inclusive)",
				Required: true,
			},
			&cli.BoolFlag{
				Name:  "backfill",
				Usage: "backfill determines whether to backfill missing index entries during validation (default: true)",
				Value: true,
			},
			&cli.BoolFlag{
				Name:  "log-good",
				Usage: "log tipsets that have no detected problems",
				Value: false,
			},
			&cli.BoolFlag{
				Name:  "quiet",
				Usage: "suppress output except for errors (or good tipsets if log-good is enabled)",
			},
		},
	},
}func (c *IndexCmd) Action(ctx *cli.Context) error {
	// ...
	outputFormat := ctx.String("output")
	if outputFormat != "json" && outputFormat != "text" {
		return fmt.Errorf("invalid output format: %s", outputFormat)
	}
	// Standardise a top level output format for all lotus CLIs
}
