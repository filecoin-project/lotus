package main

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"time"

	"github.com/docker/go-units"
	"github.com/fatih/color"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"

	miner0 "github.com/filecoin-project/specs-actors/actors/builtin/miner"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/lib/tablewriter"

	lcli "github.com/filecoin-project/lotus/cli"
	sealing "github.com/filecoin-project/lotus/extern/storage-sealing"
)

var sectorsCmd = &cli.Command{
	Name:  "sectors",
	Usage: "interact with sector store",
	Subcommands: []*cli.Command{
		sectorsStatusCmd,
		sectorsListCmd,
		sectorsRefsCmd,
		sectorsUpdateCmd,
		sectorsPledgeCmd,
		sectorsRemoveCmd,
		sectorsMarkForUpgradeCmd,
		sectorsStartSealCmd,
		sectorsSealDelayCmd,
		sectorsCapacityCollateralCmd,
	},
}

var sectorsPledgeCmd = &cli.Command{
	Name:  "pledge",
	Usage: "store random data in a sector",
	Action: func(cctx *cli.Context) error {
		nodeApi, closer, err := lcli.GetStorageMinerAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := lcli.ReqContext(cctx)

		return nodeApi.PledgeSector(ctx)
	},
}

var sectorsStatusCmd = &cli.Command{
	Name:      "status",
	Usage:     "Get the seal status of a sector by its number",
	ArgsUsage: "<sectorNum>",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "log",
			Usage: "display event log",
		},
		&cli.BoolFlag{
			Name:  "on-chain-info",
			Usage: "show sector on chain info",
		},
	},
	Action: func(cctx *cli.Context) error {
		nodeApi, closer, err := lcli.GetStorageMinerAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := lcli.ReqContext(cctx)

		if !cctx.Args().Present() {
			return fmt.Errorf("must specify sector number to get status of")
		}

		id, err := strconv.ParseUint(cctx.Args().First(), 10, 64)
		if err != nil {
			return err
		}

		onChainInfo := cctx.Bool("on-chain-info")
		status, err := nodeApi.SectorsStatus(ctx, abi.SectorNumber(id), onChainInfo)
		if err != nil {
			return err
		}

		fmt.Printf("SectorID:\t%d\n", status.SectorID)
		fmt.Printf("Status:\t\t%s\n", status.State)
		fmt.Printf("CIDcommD:\t%s\n", status.CommD)
		fmt.Printf("CIDcommR:\t%s\n", status.CommR)
		fmt.Printf("Ticket:\t\t%x\n", status.Ticket.Value)
		fmt.Printf("TicketH:\t%d\n", status.Ticket.Epoch)
		fmt.Printf("Seed:\t\t%x\n", status.Seed.Value)
		fmt.Printf("SeedH:\t\t%d\n", status.Seed.Epoch)
		fmt.Printf("Precommit:\t%s\n", status.PreCommitMsg)
		fmt.Printf("Commit:\t\t%s\n", status.CommitMsg)
		fmt.Printf("Proof:\t\t%x\n", status.Proof)
		fmt.Printf("Deals:\t\t%v\n", status.Deals)
		fmt.Printf("Retries:\t%d\n", status.Retries)
		if status.LastErr != "" {
			fmt.Printf("Last Error:\t\t%s\n", status.LastErr)
		}

		if onChainInfo {
			fmt.Printf("\nSector On Chain Info\n")
			fmt.Printf("SealProof:\t\t%x\n", status.SealProof)
			fmt.Printf("Activation:\t\t%v\n", status.Activation)
			fmt.Printf("Expiration:\t\t%v\n", status.Expiration)
			fmt.Printf("DealWeight:\t\t%v\n", status.DealWeight)
			fmt.Printf("VerifiedDealWeight:\t\t%v\n", status.VerifiedDealWeight)
			fmt.Printf("InitialPledge:\t\t%v\n", status.InitialPledge)
			fmt.Printf("\nExpiration Info\n")
			fmt.Printf("OnTime:\t\t%v\n", status.OnTime)
			fmt.Printf("Early:\t\t%v\n", status.Early)
		}

		if cctx.Bool("log") {
			fmt.Printf("--------\nEvent Log:\n")

			for i, l := range status.Log {
				fmt.Printf("%d.\t%s:\t[%s]\t%s\n", i, time.Unix(int64(l.Timestamp), 0), l.Kind, l.Message)
				if l.Trace != "" {
					fmt.Printf("\t%s\n", l.Trace)
				}
			}
		}
		return nil
	},
}

var sectorsListCmd = &cli.Command{
	Name:  "list",
	Usage: "List sectors",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "show-removed",
			Usage: "show removed sectors",
		},
		&cli.BoolFlag{
			Name:    "color",
			Aliases: []string{"c"},
			Value:   true,
		},
		&cli.BoolFlag{
			Name:  "fast",
			Usage: "don't show on-chain info for better performance",
		},
	},
	Action: func(cctx *cli.Context) error {
		color.NoColor = !cctx.Bool("color")

		nodeApi, closer, err := lcli.GetStorageMinerAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		fullApi, closer2, err := lcli.GetFullNodeAPI(cctx) // TODO: consider storing full node address in config
		if err != nil {
			return err
		}
		defer closer2()

		ctx := lcli.ReqContext(cctx)

		list, err := nodeApi.SectorsList(ctx)
		if err != nil {
			return err
		}

		maddr, err := nodeApi.ActorAddress(ctx)
		if err != nil {
			return err
		}

		head, err := fullApi.ChainHead(ctx)
		if err != nil {
			return err
		}

		activeSet, err := fullApi.StateMinerActiveSectors(ctx, maddr, head.Key())
		if err != nil {
			return err
		}
		activeIDs := make(map[abi.SectorNumber]struct{}, len(activeSet))
		for _, info := range activeSet {
			activeIDs[info.SectorNumber] = struct{}{}
		}

		sset, err := fullApi.StateMinerSectors(ctx, maddr, nil, head.Key())
		if err != nil {
			return err
		}
		commitedIDs := make(map[abi.SectorNumber]struct{}, len(activeSet))
		for _, info := range sset {
			commitedIDs[info.SectorNumber] = struct{}{}
		}

		sort.Slice(list, func(i, j int) bool {
			return list[i] < list[j]
		})

		tw := tablewriter.New(
			tablewriter.Col("ID"),
			tablewriter.Col("State"),
			tablewriter.Col("OnChain"),
			tablewriter.Col("Active"),
			tablewriter.Col("Expiration"),
			tablewriter.Col("Deals"),
			tablewriter.Col("DealWeight"),
			tablewriter.NewLineCol("Error"),
			tablewriter.NewLineCol("EarlyExpiration"))

		fast := cctx.Bool("fast")

		for _, s := range list {
			st, err := nodeApi.SectorsStatus(ctx, s, !fast)
			if err != nil {
				tw.Write(map[string]interface{}{
					"ID":    s,
					"Error": err,
				})
				continue
			}

			if cctx.Bool("show-removed") || st.State != api.SectorState(sealing.Removed) {
				_, inSSet := commitedIDs[s]
				_, inASet := activeIDs[s]

				dw := .0
				if st.Expiration-st.Activation > 0 {
					dw = float64(big.Div(st.DealWeight, big.NewInt(int64(st.Expiration-st.Activation))).Uint64())
				}

				var deals int
				for _, deal := range st.Deals {
					if deal != 0 {
						deals++
					}
				}

				exp := st.Expiration
				if st.OnTime > 0 && st.OnTime < exp {
					exp = st.OnTime // Can be different when the sector was CC upgraded
				}

				m := map[string]interface{}{
					"ID":      s,
					"State":   color.New(stateOrder[sealing.SectorState(st.State)].col).Sprint(st.State),
					"OnChain": yesno(inSSet),
					"Active":  yesno(inASet),
				}

				if deals > 0 {
					m["Deals"] = color.GreenString("%d", deals)
				} else {
					m["Deals"] = color.BlueString("CC")
					if st.ToUpgrade {
						m["Deals"] = color.CyanString("CC(upgrade)")
					}
				}

				if !fast {
					if !inSSet {
						m["Expiration"] = "n/a"
					} else {
						m["Expiration"] = lcli.EpochTime(head.Height(), exp)

						if !fast && deals > 0 {
							m["DealWeight"] = units.BytesSize(dw)
						}

						if st.Early > 0 {
							m["EarlyExpiration"] = color.YellowString(lcli.EpochTime(head.Height(), st.Early))
						}
					}
				}

				tw.Write(m)
			}
		}

		return tw.Flush(os.Stdout)
	},
}

var sectorsRefsCmd = &cli.Command{
	Name:  "refs",
	Usage: "List References to sectors",
	Action: func(cctx *cli.Context) error {
		nodeApi, closer, err := lcli.GetStorageMinerAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := lcli.ReqContext(cctx)

		refs, err := nodeApi.SectorsRefs(ctx)
		if err != nil {
			return err
		}

		for name, refs := range refs {
			fmt.Printf("Block %s:\n", name)
			for _, ref := range refs {
				fmt.Printf("\t%d+%d %d bytes\n", ref.SectorID, ref.Offset, ref.Size)
			}
		}
		return nil
	},
}

var sectorsRemoveCmd = &cli.Command{
	Name:      "remove",
	Usage:     "Forcefully remove a sector (WARNING: This means losing power and collateral for the removed sector)",
	ArgsUsage: "<sectorNum>",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "really-do-it",
			Usage: "pass this flag if you know what you are doing",
		},
	},
	Action: func(cctx *cli.Context) error {
		if !cctx.Bool("really-do-it") {
			return xerrors.Errorf("this is a command for advanced users, only use it if you are sure of what you are doing")
		}
		nodeApi, closer, err := lcli.GetStorageMinerAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := lcli.ReqContext(cctx)
		if cctx.Args().Len() != 1 {
			return xerrors.Errorf("must pass sector number")
		}

		id, err := strconv.ParseUint(cctx.Args().Get(0), 10, 64)
		if err != nil {
			return xerrors.Errorf("could not parse sector number: %w", err)
		}

		return nodeApi.SectorRemove(ctx, abi.SectorNumber(id))
	},
}

var sectorsMarkForUpgradeCmd = &cli.Command{
	Name:      "mark-for-upgrade",
	Usage:     "Mark a committed capacity sector for replacement by a sector with deals",
	ArgsUsage: "<sectorNum>",
	Action: func(cctx *cli.Context) error {
		if cctx.Args().Len() != 1 {
			return lcli.ShowHelp(cctx, xerrors.Errorf("must pass sector number"))
		}

		nodeApi, closer, err := lcli.GetStorageMinerAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := lcli.ReqContext(cctx)

		id, err := strconv.ParseUint(cctx.Args().Get(0), 10, 64)
		if err != nil {
			return xerrors.Errorf("could not parse sector number: %w", err)
		}

		return nodeApi.SectorMarkForUpgrade(ctx, abi.SectorNumber(id))
	},
}

var sectorsStartSealCmd = &cli.Command{
	Name:      "seal",
	Usage:     "Manually start sealing a sector (filling any unused space with junk)",
	ArgsUsage: "<sectorNum>",
	Action: func(cctx *cli.Context) error {
		nodeApi, closer, err := lcli.GetStorageMinerAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := lcli.ReqContext(cctx)
		if cctx.Args().Len() != 1 {
			return xerrors.Errorf("must pass sector number")
		}

		id, err := strconv.ParseUint(cctx.Args().Get(0), 10, 64)
		if err != nil {
			return xerrors.Errorf("could not parse sector number: %w", err)
		}

		return nodeApi.SectorStartSealing(ctx, abi.SectorNumber(id))
	},
}

var sectorsSealDelayCmd = &cli.Command{
	Name:      "set-seal-delay",
	Usage:     "Set the time, in minutes, that a new sector waits for deals before sealing starts",
	ArgsUsage: "<minutes>",
	Action: func(cctx *cli.Context) error {
		nodeApi, closer, err := lcli.GetStorageMinerAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := lcli.ReqContext(cctx)
		if cctx.Args().Len() != 1 {
			return xerrors.Errorf("must pass duration in minutes")
		}

		hs, err := strconv.ParseUint(cctx.Args().Get(0), 10, 64)
		if err != nil {
			return xerrors.Errorf("could not parse sector number: %w", err)
		}

		delay := hs * uint64(time.Minute)

		return nodeApi.SectorSetSealDelay(ctx, time.Duration(delay))
	},
}

var sectorsCapacityCollateralCmd = &cli.Command{
	Name:  "get-cc-collateral",
	Usage: "Get the collateral required to pledge a committed capacity sector",
	Flags: []cli.Flag{
		&cli.Uint64Flag{
			Name:  "expiration",
			Usage: "the epoch when the sector will expire",
		},
	},
	Action: func(cctx *cli.Context) error {

		mApi, mCloser, err := lcli.GetStorageMinerAPI(cctx)
		if err != nil {
			return err
		}
		defer mCloser()

		nApi, nCloser, err := lcli.GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer nCloser()

		ctx := lcli.ReqContext(cctx)

		maddr, err := mApi.ActorAddress(ctx)
		if err != nil {
			return err
		}

		pci := miner.SectorPreCommitInfo{
			Expiration: abi.ChainEpoch(cctx.Uint64("expiration")),
		}
		if pci.Expiration == 0 {
			pci.Expiration = miner0.MaxSectorExpirationExtension
		}
		pc, err := nApi.StateMinerInitialPledgeCollateral(ctx, maddr, pci, types.EmptyTSK)
		if err != nil {
			return err
		}

		fmt.Printf("Estimated collateral: %s\n", types.FIL(pc))

		return nil
	},
}

var sectorsUpdateCmd = &cli.Command{
	Name:      "update-state",
	Usage:     "ADVANCED: manually update the state of a sector, this may aid in error recovery",
	ArgsUsage: "<sectorNum> <newState>",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "really-do-it",
			Usage: "pass this flag if you know what you are doing",
		},
	},
	Action: func(cctx *cli.Context) error {
		if !cctx.Bool("really-do-it") {
			return xerrors.Errorf("this is a command for advanced users, only use it if you are sure of what you are doing")
		}
		nodeApi, closer, err := lcli.GetStorageMinerAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := lcli.ReqContext(cctx)
		if cctx.Args().Len() < 2 {
			return xerrors.Errorf("must pass sector number and new state")
		}

		id, err := strconv.ParseUint(cctx.Args().Get(0), 10, 64)
		if err != nil {
			return xerrors.Errorf("could not parse sector number: %w", err)
		}

		newState := cctx.Args().Get(1)
		if _, ok := sealing.ExistSectorStateList[sealing.SectorState(newState)]; !ok {
			fmt.Printf(" \"%s\" is not a valid state. Possible states for sectors are: \n", newState)
			for state := range sealing.ExistSectorStateList {
				fmt.Printf("%s\n", string(state))
			}
			return nil
		}

		return nodeApi.SectorsUpdate(ctx, abi.SectorNumber(id), api.SectorState(cctx.Args().Get(1)))
	},
}

func yesno(b bool) string {
	if b {
		return color.GreenString("YES")
	}
	return color.RedString("NO")
}
