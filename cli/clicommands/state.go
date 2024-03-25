// Package clicommands contains only the cli.Command definitions that are
// common to sptool and miner. These are here because they can't be referenced
// in cli/spcli or cli/ because of the import cycle with all the other cli functions.
package clicommands

import (
	"github.com/urfave/cli/v2"

	"github.com/filecoin-project/go-address"

	lcli "github.com/filecoin-project/lotus/cli"
	"github.com/filecoin-project/lotus/cli/spcli"
)

var StateCmd = &cli.Command{
	Name:  "state",
	Usage: "Interact with and query filecoin chain state",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "tipset",
			Usage: "specify tipset to call method on (pass comma separated array of cids)",
		},
	},
	Subcommands: []*cli.Command{
		lcli.StatePowerCmd,
		lcli.StateSectorsCmd,
		lcli.StateActiveSectorsCmd,
		lcli.StateListActorsCmd,
		lcli.StateListMinersCmd,
		lcli.StateCircSupplyCmd,
		lcli.StateSectorCmd,
		lcli.StateGetActorCmd,
		lcli.StateLookupIDCmd,
		lcli.StateReplayCmd,
		lcli.StateSectorSizeCmd,
		lcli.StateReadStateCmd,
		lcli.StateListMessagesCmd,
		lcli.StateComputeStateCmd,
		lcli.StateCallCmd,
		lcli.StateGetDealSetCmd,
		lcli.StateWaitMsgCmd,
		lcli.StateSearchMsgCmd,
		StateMinerInfo,
		lcli.StateMarketCmd,
		lcli.StateExecTraceCmd,
		lcli.StateNtwkVersionCmd,
		lcli.StateMinerProvingDeadlineCmd,
		lcli.StateSysActorCIDsCmd,
	},
}

var StateMinerInfo = &cli.Command{
	Name:      "miner-info",
	Usage:     "Retrieve miner information",
	ArgsUsage: "[minerAddress]",
	Action: func(cctx *cli.Context) error {
		addressGetter := func(_ *cli.Context) (address.Address, error) {
			if cctx.NArg() != 1 {
				return address.Address{}, lcli.IncorrectNumArgs(cctx)
			}

			return address.NewFromString(cctx.Args().First())
		}
		err := spcli.InfoCmd(addressGetter).Action(cctx)
		if err != nil {
			return err
		}
		return nil
	},
}
