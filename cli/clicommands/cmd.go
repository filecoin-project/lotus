package clicommands

import (
    "github.com/urfave/cli/v2"
    lcli "github.com/filecoin-project/lotus/cli"
)

var Commands = []*cli.Command{
    lcli.WithCategory("basic", wrapCommandWithOutput(lcli.SendCmd)),
    lcli.WithCategory("basic", wrapCommandWithOutput(lcli.WalletCmd)),
    lcli.WithCategory("basic", wrapCommandWithOutput(lcli.InfoCmd)),
    lcli.WithCategory("basic", wrapCommandWithOutput(lcli.MultisigCmd)),
    lcli.WithCategory("basic", wrapCommandWithOutput(lcli.FilplusCmd)),
    lcli.WithCategory("basic", wrapCommandWithOutput(lcli.PaychCmd)),
    lcli.WithCategory("developer", wrapCommandWithOutput(lcli.AuthCmd)),
    lcli.WithCategory("developer", wrapCommandWithOutput(lcli.MpoolCmd)),
    lcli.WithCategory("developer", wrapCommandWithOutput(StateCmd)),
    lcli.WithCategory("developer", wrapCommandWithOutput(lcli.ChainCmd)),
    lcli.WithCategory("developer", wrapCommandWithOutput(lcli.LogCmd)),
    lcli.WithCategory("developer", wrapCommandWithOutput(lcli.WaitApiCmd)),
    lcli.WithCategory("developer", wrapCommandWithOutput(lcli.FetchParamCmd)),
    lcli.WithCategory("developer", wrapCommandWithOutput(lcli.EvmCmd)),
    lcli.WithCategory("developer", wrapCommandWithOutput(lcli.IndexCmd)),
    lcli.WithCategory("network", wrapCommandWithOutput(lcli.NetCmd)),
    lcli.WithCategory("network", wrapCommandWithOutput(lcli.SyncCmd)),
    lcli.WithCategory("network", wrapCommandWithOutput(lcli.F3Cmd)),
    lcli.WithCategory("status", wrapCommandWithOutput(lcli.StatusCmd)),
    wrapCommandWithOutput(lcli.PprofCmd),
    wrapCommandWithOutput(lcli.VersionCmd),
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
        Action: func(cctx *cli.Context) error {
            // Your command logic here
            data := getDataFromContext(cctx)
            return util.FormatOutput(cctx, data)
        },
    },
    {
        Name:  "miner",
        Usage: "Commands related to managing the miner",
        Flags: []cli.Flag{
            &cli.StringFlag{
                Name:  "output",
                Usage: "output format (json or text)",
                Value: "text",
            },
        },
        Action: func(cctx *cli.Context) error {
            // Your command logic here
            data := getDataFromContext(cctx)
            return util.FormatOutput(cctx, data)
        },
    },
}

func wrapCommandWithOutput(cmd *cli.Command) *cli.Command {
    originalAction := cmd.Action
    cmd.Action = func(cctx *cli.Context) error {
        if originalAction != nil {
            err := originalAction(cctx)
            if err != nil {
                return err
            }
        }
        // Assuming data is the output data you want to format
        data := getDataFromContext(cctx)
        return util.FormatOutput(cctx, data)
    }
    return cmd
}

func getDataFromContext(cctx *cli.Context) interface{} {
    // Implement this function to extract the data you want to format from the context
    return nil
}