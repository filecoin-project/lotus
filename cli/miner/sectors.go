package miner

import (
	"bufio"
	"encoding/csv"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/docker/go-units"
	"github.com/fatih/color"
	cbor "github.com/ipfs/go-ipld-cbor"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-bitfield"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/network"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/chain/actors/adt"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/actors/policy"
	"github.com/filecoin-project/lotus/chain/types"
	lcli "github.com/filecoin-project/lotus/cli"
	"github.com/filecoin-project/lotus/cli/spcli"
	cliutil "github.com/filecoin-project/lotus/cli/util"
	"github.com/filecoin-project/lotus/lib/result"
	"github.com/filecoin-project/lotus/lib/strle"
	"github.com/filecoin-project/lotus/lib/tablewriter"
	sealing "github.com/filecoin-project/lotus/storage/pipeline"
)

const parallelSectorChecks = 300

var sectorsCmd = &cli.Command{
	Name:  "sectors",
	Usage: "interact with sector store",
	Subcommands: []*cli.Command{
		spcli.SectorsStatusCmd(LMActorOrEnvGetter, getOnDiskInfo),
		sectorsListCmd,
		sectorsRefsCmd,
		sectorsUpdateCmd,
		sectorsPledgeCmd,
		sectorsNumbersCmd,
		spcli.SectorPreCommitsCmd(LMActorOrEnvGetter),
		spcli.SectorsCheckExpireCmd(LMActorOrEnvGetter),
		sectorsExpiredCmd,
		spcli.SectorsExtendCmd(LMActorOrEnvGetter),
		sectorsTerminateCmd,
		sectorsRemoveCmd,
		sectorsSnapUpCmd,
		sectorsSnapAbortCmd,
		sectorsStartSealCmd,
		sectorsSealDelayCmd,
		sectorsCapacityCollateralCmd,
		sectorsBatching,
		sectorsRefreshPieceMatchingCmd,
		spcli.SectorsCompactPartitionsCmd(LMActorOrEnvGetter),
		sectorsUnsealCmd,
	},
}

func getOnDiskInfo(cctx *cli.Context, id abi.SectorNumber, onChainInfo bool) (api.SectorInfo, error) {
	minerApi, closer, err := lcli.GetStorageMinerAPI(cctx)
	if err != nil {
		return api.SectorInfo{}, err
	}
	defer closer()
	return minerApi.SectorsStatus(cctx.Context, id, onChainInfo)
}

var sectorsPledgeCmd = &cli.Command{
	Name:  "pledge",
	Usage: "store random data in a sector",
	Action: func(cctx *cli.Context) error {
		minerApi, closer, err := lcli.GetStorageMinerAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := lcli.ReqContext(cctx)

		id, err := minerApi.PledgeSector(ctx)
		if err != nil {
			return err
		}

		fmt.Println("Created CC sector: ", id.Number)

		return nil
	},
}

var sectorsListCmd = &cli.Command{
	Name:  "list",
	Usage: "List sectors",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:    "show-removed",
			Usage:   "show removed sectors",
			Aliases: []string{"r"},
		},
		&cli.BoolFlag{
			Name:    "fast",
			Usage:   "don't show on-chain info for better performance",
			Aliases: []string{"f"},
		},
		&cli.BoolFlag{
			Name:    "events",
			Usage:   "display number of events the sector has received",
			Aliases: []string{"e"},
		},
		&cli.BoolFlag{
			Name:    "initial-pledge",
			Usage:   "display initial pledge",
			Aliases: []string{"p"},
		},
		&cli.BoolFlag{
			Name:    "seal-time",
			Usage:   "display how long it took for the sector to be sealed",
			Aliases: []string{"t"},
		},
		&cli.StringFlag{
			Name:  "states",
			Usage: "filter sectors by a comma-separated list of states",
		},
		&cli.BoolFlag{
			Name:    "unproven",
			Usage:   "only show sectors which aren't in the 'Proving' state",
			Aliases: []string{"u"},
		},
		&cli.Int64Flag{
			Name:  "check-parallelism",
			Usage: "number of parallel requests to make for checking sector states",
			Value: parallelSectorChecks,
		},
	},
	Subcommands: []*cli.Command{
		sectorsListUpgradeBoundsCmd,
	},
	Action: func(cctx *cli.Context) error {
		// http mode allows for parallel json decoding/encoding, which was a bottleneck here
		minerApi, closer, err := lcli.GetStorageMinerAPI(cctx, cliutil.StorageMinerUseHttp)
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

		var list []abi.SectorNumber

		showRemoved := cctx.Bool("show-removed")
		var states []api.SectorState
		if cctx.IsSet("states") && cctx.IsSet("unproven") {
			return xerrors.Errorf("only one of --states or --unproven can be specified at once")
		}

		if cctx.IsSet("states") {
			showRemoved = true
			sList := strings.Split(cctx.String("states"), ",")
			states = make([]api.SectorState, len(sList))
			for i := range sList {
				states[i] = api.SectorState(sList[i])
			}
		}

		if cctx.Bool("unproven") {
			for state := range sealing.ExistSectorStateList {
				if state == sealing.Proving || state == sealing.Available {
					continue
				}
				states = append(states, api.SectorState(state))
			}
		}

		if len(states) == 0 {
			list, err = minerApi.SectorsList(ctx)
		} else {
			list, err = minerApi.SectorsListInStates(ctx, states)
		}

		if err != nil {
			return err
		}

		maddr, err := minerApi.ActorAddress(ctx)
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
		powerBaseEpochs := make(map[abi.SectorNumber]abi.ChainEpoch, len(sset))
		commitedIDs := make(map[abi.SectorNumber]struct{}, len(sset))
		for _, info := range sset {
			commitedIDs[info.SectorNumber] = struct{}{}
			powerBaseEpochs[info.SectorNumber] = info.PowerBaseEpoch
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
			tablewriter.Col("SealTime"),
			tablewriter.Col("Events"),
			tablewriter.Col("Deals"),
			tablewriter.Col("DealWeight"),
			tablewriter.Col("VerifiedPower"),
			tablewriter.Col("Pledge"),
			tablewriter.NewLineCol("Error"),
			tablewriter.NewLineCol("RecoveryTimeout"))

		fast := cctx.Bool("fast")

		throttle := make(chan struct{}, cctx.Int64("check-parallelism"))

		slist := make([]result.Result[api.SectorInfo], len(list))
		var wg sync.WaitGroup
		for i, s := range list {
			throttle <- struct{}{}
			wg.Add(1)
			go func(i int, s abi.SectorNumber) {
				defer wg.Done()
				defer func() { <-throttle }()
				r := result.Wrap(minerApi.SectorsStatus(ctx, s, !fast))
				if r.Error != nil {
					r.Value.SectorID = s
				}
				slist[i] = r
			}(i, s)
		}
		wg.Wait()

		for _, rsn := range slist {
			if rsn.Error != nil {
				tw.Write(map[string]interface{}{
					"ID":    rsn.Value.SectorID,
					"Error": err,
				})
				continue
			}

			st := rsn.Value
			s := st.SectorID

			if !showRemoved && st.State == api.SectorState(sealing.Removed) {
				continue
			}

			_, inSSet := commitedIDs[s]
			_, inASet := activeIDs[s]

			const verifiedPowerGainMul = 9

			dw, vp := .0, .0
			estimate := (st.Expiration-st.Activation <= 0) || sealing.IsUpgradeState(sealing.SectorState(st.State))
			if !estimate {
				rdw := big.Add(st.DealWeight, st.VerifiedDealWeight)
				powerBaseEpoch := powerBaseEpochs[st.SectorID]
				dw = float64(big.Div(rdw, big.NewInt(int64(st.Expiration-powerBaseEpoch))).Uint64())
				vp = float64(big.Div(big.Mul(st.VerifiedDealWeight, big.NewInt(verifiedPowerGainMul)), big.NewInt(int64(st.Expiration-st.Activation))).Uint64())
			} else {
				for _, piece := range st.Pieces {
					if piece.DealInfo != nil {
						dw += float64(piece.Piece.Size)
						if piece.DealInfo.DealProposal != nil && piece.DealInfo.DealProposal.VerifiedDeal {
							vp += float64(piece.Piece.Size) * verifiedPowerGainMul
						}
					}
				}
			}

			var deals int
			for _, deal := range st.Deals {
				if deal != 0 {
					deals++
				}
			}

			var pams int
			for _, p := range st.Pieces {
				if p.DealInfo != nil && p.DealInfo.PieceActivationManifest != nil {
					pams++
				}
			}

			exp := st.Expiration
			if st.OnTime > 0 && st.OnTime < exp {
				exp = st.OnTime // Can be different when the sector was CC upgraded
			}

			m := map[string]interface{}{
				"ID":      s,
				"State":   color.New(spcli.StateOrder[sealing.SectorState(st.State)].Col).Sprint(st.State),
				"OnChain": yesno(inSSet),
				"Active":  yesno(inASet),
			}

			if deals > 0 {
				m["Deals"] = color.GreenString("%d", deals)
			} else if pams > 0 {
				m["Deals"] = color.MagentaString("DDO:%d", pams)
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
					m["Expiration"] = cliutil.EpochTime(head.Height(), exp)
					if st.Early > 0 {
						m["RecoveryTimeout"] = color.YellowString(cliutil.EpochTime(head.Height(), st.Early))
					}
				}
				if inSSet && cctx.Bool("initial-pledge") {
					m["Pledge"] = types.FIL(st.InitialPledge).Short()
				}
			}

			if !fast && deals > 0 {
				estWrap := func(s string) string {
					if !estimate {
						return s
					}
					return fmt.Sprintf("[%s]", s)
				}

				m["DealWeight"] = estWrap(units.BytesSize(dw))
				if vp > 0 {
					m["VerifiedPower"] = estWrap(color.GreenString(units.BytesSize(vp)))
				}
			}

			if cctx.Bool("events") {
				var events int
				for _, sectorLog := range st.Log {
					if !strings.HasPrefix(sectorLog.Kind, "event") {
						continue
					}
					if sectorLog.Kind == "event;sealing.SectorRestart" {
						continue
					}
					events++
				}

				pieces := len(st.Deals)

				switch {
				case events < 12+pieces:
					m["Events"] = color.GreenString("%d", events)
				case events < 20+pieces:
					m["Events"] = color.YellowString("%d", events)
				default:
					m["Events"] = color.RedString("%d", events)
				}
			}

			if cctx.Bool("seal-time") && len(st.Log) > 1 {
				start := time.Unix(int64(st.Log[0].Timestamp), 0)

				for _, sectorLog := range st.Log {
					if sectorLog.Kind == "event;sealing.SectorProving" { // todo: figure out a good way to not hardcode
						end := time.Unix(int64(sectorLog.Timestamp), 0)
						dur := end.Sub(start)

						switch {
						case dur < 12*time.Hour:
							m["SealTime"] = color.GreenString("%s", dur)
						case dur < 24*time.Hour:
							m["SealTime"] = color.YellowString("%s", dur)
						default:
							m["SealTime"] = color.RedString("%s", dur)
						}

						break
					}
				}
			}

			tw.Write(m)
		}

		return tw.Flush(os.Stdout)
	},
}

var sectorsListUpgradeBoundsCmd = &cli.Command{
	Name:  "upgrade-bounds",
	Usage: "Output upgrade bounds for available sectors",
	Flags: []cli.Flag{
		&cli.IntFlag{
			Name:  "buckets",
			Value: 25,
		},
		&cli.BoolFlag{
			Name:  "csv",
			Usage: "output machine-readable values",
		},
		&cli.BoolFlag{
			Name:  "deal-terms",
			Usage: "bucket by how many deal-sectors can start at a given expiration",
		},
	},
	Action: func(cctx *cli.Context) error {
		minerApi, closer, err := lcli.GetStorageMinerAPI(cctx, cliutil.StorageMinerUseHttp)
		if err != nil {
			return err
		}
		defer closer()

		fullApi, closer2, err := lcli.GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer2()

		ctx := lcli.ReqContext(cctx)

		list, err := minerApi.SectorsListInStates(ctx, []api.SectorState{
			api.SectorState(sealing.Available),
		})
		if err != nil {
			return xerrors.Errorf("getting sector list: %w", err)
		}

		head, err := fullApi.ChainHead(ctx)
		if err != nil {
			return xerrors.Errorf("getting chain head: %w", err)
		}

		filter := bitfield.New()

		for _, s := range list {
			filter.Set(uint64(s))
		}

		maddr, err := minerApi.ActorAddress(ctx)
		if err != nil {
			return err
		}

		sset, err := fullApi.StateMinerSectors(ctx, maddr, &filter, head.Key())
		if err != nil {
			return err
		}

		if len(sset) == 0 {
			return nil
		}

		var minExpiration, maxExpiration abi.ChainEpoch

		for _, s := range sset {
			if s.Expiration < minExpiration || minExpiration == 0 {
				minExpiration = s.Expiration
			}
			if s.Expiration > maxExpiration {
				maxExpiration = s.Expiration
			}
		}

		buckets := cctx.Int("buckets")
		bucketSize := (maxExpiration - minExpiration) / abi.ChainEpoch(buckets)
		bucketCounts := make([]int, buckets+1)

		for b := range bucketCounts {
			bucketMin := minExpiration + abi.ChainEpoch(b)*bucketSize
			bucketMax := minExpiration + abi.ChainEpoch(b+1)*bucketSize

			if cctx.Bool("deal-terms") {
				bucketMax = bucketMax + policy.MarketDefaultAllocationTermBuffer
			}

			for _, s := range sset {
				isInBucket := s.Expiration >= bucketMin && s.Expiration < bucketMax

				if isInBucket {
					bucketCounts[b]++
				}
			}

		}

		// Creating CSV writer
		writer := csv.NewWriter(os.Stdout)

		// Writing CSV headers
		err = writer.Write([]string{"Max Expiration in Bucket", "Sector Count"})
		if err != nil {
			return xerrors.Errorf("writing csv headers: %w", err)
		}

		// Writing bucket details

		if cctx.Bool("csv") {
			for i := 0; i < buckets; i++ {
				maxExp := minExpiration + abi.ChainEpoch(i+1)*bucketSize

				timeStr := strconv.FormatInt(int64(maxExp), 10)

				err = writer.Write([]string{
					timeStr,
					strconv.Itoa(bucketCounts[i]),
				})
				if err != nil {
					return xerrors.Errorf("writing csv row: %w", err)
				}
			}

			// Flush to make sure all data is written to the underlying writer
			writer.Flush()

			if err := writer.Error(); err != nil {
				return xerrors.Errorf("flushing csv writer: %w", err)
			}

			return nil
		}

		tw := tablewriter.New(
			tablewriter.Col("Bucket Expiration"),
			tablewriter.Col("Sector Count"),
			tablewriter.Col("Bar"),
		)

		var barCols = 40
		var maxCount int

		for _, c := range bucketCounts {
			if c > maxCount {
				maxCount = c
			}
		}

		for i := 0; i < buckets; i++ {
			maxExp := minExpiration + abi.ChainEpoch(i+1)*bucketSize
			timeStr := cliutil.EpochTime(head.Height(), maxExp)

			tw.Write(map[string]interface{}{
				"Bucket Expiration": timeStr,
				"Sector Count":      color.YellowString("%d", bucketCounts[i]),
				"Bar":               "[" + color.GreenString(strings.Repeat("|", bucketCounts[i]*barCols/maxCount)) + strings.Repeat(" ", barCols-bucketCounts[i]*barCols/maxCount) + "]",
			})
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

var sectorsTerminateCmd = &cli.Command{
	Name:      "terminate",
	Usage:     "Terminate sector on-chain then remove (WARNING: This means losing power and collateral for the removed sector)",
	ArgsUsage: "<sectorNum>",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "really-do-it",
			Usage: "pass this flag if you know what you are doing",
		},
	},
	Subcommands: []*cli.Command{
		sectorsTerminateFlushCmd,
		sectorsTerminatePendingCmd,
	},
	Action: func(cctx *cli.Context) error {
		minerApi, closer, err := lcli.GetStorageMinerAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := lcli.ReqContext(cctx)
		if cctx.NArg() != 1 {
			return lcli.IncorrectNumArgs(cctx)
		}

		if !cctx.Bool("really-do-it") {
			return xerrors.Errorf("pass --really-do-it to confirm this action")
		}

		id, err := strconv.ParseUint(cctx.Args().Get(0), 10, 64)
		if err != nil {
			return xerrors.Errorf("could not parse sector number: %w", err)
		}

		return minerApi.SectorTerminate(ctx, abi.SectorNumber(id))
	},
}

var sectorsTerminateFlushCmd = &cli.Command{
	Name:  "flush",
	Usage: "Send a terminate message if there are sectors queued for termination",
	Action: func(cctx *cli.Context) error {
		minerApi, closer, err := lcli.GetStorageMinerAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := lcli.ReqContext(cctx)

		mcid, err := minerApi.SectorTerminateFlush(ctx)
		if err != nil {
			return err
		}

		if mcid == nil {
			return xerrors.New("no sectors were queued for termination")
		}

		fmt.Println(mcid)

		return nil
	},
}

var sectorsTerminatePendingCmd = &cli.Command{
	Name:  "pending",
	Usage: "List sector numbers of sectors pending termination",
	Action: func(cctx *cli.Context) error {
		minerAPI, closer, err := lcli.GetStorageMinerAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		api, nCloser, err := lcli.GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer nCloser()
		ctx := lcli.ReqContext(cctx)

		pending, err := minerAPI.SectorTerminatePending(ctx)
		if err != nil {
			return err
		}

		maddr, err := minerAPI.ActorAddress(ctx)
		if err != nil {
			return err
		}

		dl, err := api.StateMinerProvingDeadline(ctx, maddr, types.EmptyTSK)
		if err != nil {
			return xerrors.Errorf("getting proving deadline info failed: %w", err)
		}

		for _, id := range pending {
			loc, err := api.StateSectorPartition(ctx, maddr, id.Number, types.EmptyTSK)
			if err != nil {
				return xerrors.Errorf("finding sector partition: %w", err)
			}

			fmt.Print(id.Number)

			if loc.Deadline == (dl.Index+1)%miner.WPoStPeriodDeadlines || // not in next (in case the terminate message takes a while to get on chain)
				loc.Deadline == dl.Index || // not in current
				(loc.Deadline+1)%miner.WPoStPeriodDeadlines == dl.Index { // not in previous
				fmt.Print(" (in proving window)")
			}
			fmt.Println()
		}

		return nil
	},
}

var sectorsRemoveCmd = &cli.Command{
	Name:      "remove",
	Usage:     "Forcefully remove a sector (WARNING: This means losing power and collateral for the removed sector (use 'terminate' for lower penalty))",
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
		minerAPI, closer, err := lcli.GetStorageMinerAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := lcli.ReqContext(cctx)
		if cctx.NArg() != 1 {
			return lcli.IncorrectNumArgs(cctx)
		}

		id, err := strconv.ParseUint(cctx.Args().Get(0), 10, 64)
		if err != nil {
			return xerrors.Errorf("could not parse sector number: %w", err)
		}

		// Check if the sector exists
		_, err = minerAPI.SectorsStatus(ctx, abi.SectorNumber(id), false)
		if err != nil {
			return xerrors.Errorf("sectorID %d has not been created yet: %w", id, err)
		}

		return minerAPI.SectorRemove(ctx, abi.SectorNumber(id))
	},
}

var sectorsSnapUpCmd = &cli.Command{
	Name:      "snap-up",
	Usage:     "Mark a committed capacity sector to be filled with deals",
	ArgsUsage: "<sectorNum>",
	Action: func(cctx *cli.Context) error {
		if cctx.NArg() != 1 {
			return lcli.IncorrectNumArgs(cctx)
		}

		minerAPI, closer, err := lcli.GetStorageMinerAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		api, nCloser, err := lcli.GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer nCloser()
		ctx := lcli.ReqContext(cctx)

		nv, err := api.StateNetworkVersion(ctx, types.EmptyTSK)
		if err != nil {
			return xerrors.Errorf("failed to get network version: %w", err)
		}
		if nv < network.Version15 {
			return xerrors.Errorf("snap deals upgrades enabled in network v15")
		}

		id, err := strconv.ParseUint(cctx.Args().Get(0), 10, 64)
		if err != nil {
			return xerrors.Errorf("could not parse sector number: %w", err)
		}

		return minerAPI.SectorMarkForUpgrade(ctx, abi.SectorNumber(id), true)
	},
}

var sectorsSnapAbortCmd = &cli.Command{
	Name:      "abort-upgrade",
	Usage:     "Abort the attempted (SnapDeals) upgrade of a CC sector, reverting it to as before",
	ArgsUsage: "<sectorNum>",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "really-do-it",
			Usage: "pass this flag if you know what you are doing",
		},
	},
	Action: func(cctx *cli.Context) error {
		if cctx.NArg() != 1 {
			return lcli.ShowHelp(cctx, xerrors.Errorf("must pass sector number"))
		}

		really := cctx.Bool("really-do-it")
		if !really {
			//nolint:golint
			return fmt.Errorf("--really-do-it must be specified for this action to have an effect; you have been warned")
		}

		minerAPI, closer, err := lcli.GetStorageMinerAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := lcli.ReqContext(cctx)

		id, err := strconv.ParseUint(cctx.Args().Get(0), 10, 64)
		if err != nil {
			return xerrors.Errorf("could not parse sector number: %w", err)
		}

		return minerAPI.SectorAbortUpgrade(ctx, abi.SectorNumber(id))
	},
}

var sectorsStartSealCmd = &cli.Command{
	Name:      "seal",
	Usage:     "Manually start sealing a sector (filling any unused space with junk)",
	ArgsUsage: "<sectorNum>",
	Action: func(cctx *cli.Context) error {
		minerAPI, closer, err := lcli.GetStorageMinerAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := lcli.ReqContext(cctx)
		if cctx.NArg() != 1 {
			return lcli.IncorrectNumArgs(cctx)
		}

		id, err := strconv.ParseUint(cctx.Args().Get(0), 10, 64)
		if err != nil {
			return xerrors.Errorf("could not parse sector number: %w", err)
		}

		return minerAPI.SectorStartSealing(ctx, abi.SectorNumber(id))
	},
}

var sectorsSealDelayCmd = &cli.Command{
	Name:      "set-seal-delay",
	Usage:     "Set the time (in minutes) that a new sector waits for deals before sealing starts",
	ArgsUsage: "<time>",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "seconds",
			Usage: "Specifies that the time argument should be in seconds",
		},
	},
	Action: func(cctx *cli.Context) error {
		minerAPI, closer, err := lcli.GetStorageMinerAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := lcli.ReqContext(cctx)
		if cctx.NArg() != 1 {
			return lcli.IncorrectNumArgs(cctx)
		}

		hs, err := strconv.ParseUint(cctx.Args().Get(0), 10, 64)
		if err != nil {
			return xerrors.Errorf("could not parse sector number: %w", err)
		}

		var delay uint64
		if cctx.Bool("seconds") {
			delay = hs * uint64(time.Second)
		} else {
			delay = hs * uint64(time.Minute)
		}

		return minerAPI.SectorSetSealDelay(ctx, time.Duration(delay))
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

		nApi, nCloser, err := lcli.GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer nCloser()

		ctx := lcli.ReqContext(cctx)

		maddr, err := getActorAddress(ctx, cctx)
		if err != nil {
			return err
		}

		mi, err := nApi.StateMinerInfo(ctx, maddr, types.EmptyTSK)
		if err != nil {
			return err
		}

		nv, err := nApi.StateNetworkVersion(ctx, types.EmptyTSK)
		if err != nil {
			return err
		}

		spt, err := miner.PreferredSealProofTypeFromWindowPoStType(nv, mi.WindowPoStProofType, false)
		if err != nil {
			return err
		}

		pci := miner.SectorPreCommitInfo{
			SealProof:  spt,
			Expiration: abi.ChainEpoch(cctx.Uint64("expiration")),
		}
		if pci.Expiration == 0 {
			h, err := nApi.ChainHead(ctx)
			if err != nil {
				return err
			}

			maxExtension, err := policy.GetMaxSectorExpirationExtension(nv)
			if err != nil {
				return xerrors.Errorf("failed to get max extension: %w", err)
			}

			pci.Expiration = maxExtension + h.Height()
		}

		pc, err := nApi.StateMinerInitialPledgeCollateral(ctx, maddr, pci, types.EmptyTSK)
		if err != nil {
			return err
		}

		pcd, err := nApi.StateMinerPreCommitDepositForPower(ctx, maddr, pci, types.EmptyTSK)
		if err != nil {
			return err
		}

		fmt.Printf("Estimated collateral: %s\n", types.FIL(big.Max(pc, pcd)))

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
		minerAPI, closer, err := lcli.GetStorageMinerAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := lcli.ReqContext(cctx)
		if cctx.NArg() != 2 {
			return lcli.IncorrectNumArgs(cctx)
		}

		id, err := strconv.ParseUint(cctx.Args().Get(0), 10, 64)
		if err != nil {
			return xerrors.Errorf("could not parse sector number: %w", err)
		}

		_, err = minerAPI.SectorsStatus(ctx, abi.SectorNumber(id), false)
		if err != nil {
			return xerrors.Errorf("sector %d not found, could not change state", id)
		}

		newState := cctx.Args().Get(1)
		if _, ok := sealing.ExistSectorStateList[sealing.SectorState(newState)]; !ok {
			fmt.Printf(" \"%s\" is not a valid state. Possible states for sectors are: \n", newState)
			for state := range sealing.ExistSectorStateList {
				fmt.Printf("%s\n", string(state))
			}
			return nil
		}

		return minerAPI.SectorsUpdate(ctx, abi.SectorNumber(id), api.SectorState(cctx.Args().Get(1)))
	},
}

var sectorsExpiredCmd = &cli.Command{
	Name:  "expired",
	Usage: "Get or cleanup expired sectors",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "show-removed",
			Usage: "show removed sectors",
		},
		&cli.BoolFlag{
			Name:  "remove-expired",
			Usage: "remove expired sectors",
		},

		&cli.Int64Flag{
			Name:   "confirm-remove-count",
			Hidden: true,
		},
		&cli.Int64Flag{
			Name:        "expired-epoch",
			Usage:       "epoch at which to check sector expirations",
			DefaultText: "WinningPoSt lookback epoch",
		},
	},
	Action: func(cctx *cli.Context) error {
		minerAPI, closer, err := lcli.GetStorageMinerAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		fullApi, nCloser, err := lcli.GetFullNodeAPI(cctx)
		if err != nil {
			return xerrors.Errorf("getting fullnode api: %w", err)
		}
		defer nCloser()
		ctx := lcli.ReqContext(cctx)

		head, err := fullApi.ChainHead(ctx)
		if err != nil {
			return xerrors.Errorf("getting chain head: %w", err)
		}

		lbEpoch := abi.ChainEpoch(cctx.Int64("expired-epoch"))
		if !cctx.IsSet("expired-epoch") {
			nv, err := fullApi.StateNetworkVersion(ctx, head.Key())
			if err != nil {
				return xerrors.Errorf("getting network version: %w", err)
			}

			lbEpoch = head.Height() - policy.GetWinningPoStSectorSetLookback(nv)
			if lbEpoch < 0 {
				return xerrors.Errorf("too early to terminate sectors")
			}
		}

		if cctx.IsSet("confirm-remove-count") && !cctx.IsSet("expired-epoch") {
			return xerrors.Errorf("--expired-epoch must be specified with --confirm-remove-count")
		}

		lbts, err := fullApi.ChainGetTipSetByHeight(ctx, lbEpoch, head.Key())
		if err != nil {
			return xerrors.Errorf("getting lookback tipset: %w", err)
		}

		maddr, err := minerAPI.ActorAddress(ctx)
		if err != nil {
			return xerrors.Errorf("getting actor address: %w", err)
		}

		// toCheck is a working bitfield which will only contain terminated sectors
		toCheck := bitfield.New()
		{
			sectors, err := minerAPI.SectorsList(ctx)
			if err != nil {
				return xerrors.Errorf("getting sector list: %w", err)
			}

			for _, sector := range sectors {
				toCheck.Set(uint64(sector))
			}
		}

		mact, err := fullApi.StateGetActor(ctx, maddr, lbts.Key())
		if err != nil {
			return err
		}

		tbs := blockstore.NewTieredBstore(blockstore.NewAPIBlockstore(fullApi), blockstore.NewMemory())
		mas, err := miner.Load(adt.WrapStore(ctx, cbor.NewCborStore(tbs)), mact)
		if err != nil {
			return err
		}

		alloc, err := mas.GetAllocatedSectors()
		if err != nil {
			return xerrors.Errorf("getting allocated sectors: %w", err)
		}

		// only allocated sectors can be expired,
		toCheck, err = bitfield.IntersectBitField(toCheck, *alloc)
		if err != nil {
			return xerrors.Errorf("intersecting bitfields: %w", err)
		}

		if err := mas.ForEachDeadline(func(dlIdx uint64, dl miner.Deadline) error {
			return dl.ForEachPartition(func(partIdx uint64, part miner.Partition) error {
				live, err := part.LiveSectors()
				if err != nil {
					return err
				}

				toCheck, err = bitfield.SubtractBitField(toCheck, live)
				if err != nil {
					return err
				}

				unproven, err := part.UnprovenSectors()
				if err != nil {
					return err
				}

				toCheck, err = bitfield.SubtractBitField(toCheck, unproven)

				return err
			})
		}); err != nil {
			return err
		}

		err = mas.ForEachPrecommittedSector(func(pci miner.SectorPreCommitOnChainInfo) error {
			toCheck.Unset(uint64(pci.Info.SectorNumber))
			return nil
		})
		if err != nil {
			return err
		}

		if cctx.Bool("remove-expired") {
			color.Red("Removing sectors:\n")
		}

		// toCheck now only contains sectors which either failed to precommit or are expired/terminated
		fmt.Printf("Sector\tState\tExpiration\n")

		var toRemove []abi.SectorNumber

		err = toCheck.ForEach(func(u uint64) error {
			s := abi.SectorNumber(u)

			st, err := minerAPI.SectorsStatus(ctx, s, true)
			if err != nil {
				fmt.Printf("%d:\tError getting status: %s\n", u, err)
				return nil
			}

			rmMsg := ""

			if st.State == api.SectorState(sealing.Removed) {
				if cctx.IsSet("confirm-remove-count") || !cctx.Bool("show-removed") {
					return nil
				}
			} else { // not removed
				toRemove = append(toRemove, s)
			}

			fmt.Printf("%d%s\t%s\t%s\n", s, rmMsg, st.State, cliutil.EpochTime(head.Height(), st.Expiration))

			return nil
		})
		if err != nil {
			return err
		}

		if cctx.Bool("remove-expired") {
			if !cctx.IsSet("confirm-remove-count") {
				fmt.Println()
				fmt.Println(color.YellowString("All"), color.GreenString("%d", len(toRemove)), color.YellowString("sectors listed above will be removed\n"))
				fmt.Println(color.YellowString("To confirm removal of the above sectors, including\n all related sealed and unsealed data, run:\n"))
				fmt.Println(color.RedString("lotus-miner sectors expired --remove-expired --confirm-remove-count=%d --expired-epoch=%d\n", len(toRemove), lbts.Height()))
				fmt.Println(color.YellowString("WARNING: This operation is irreversible"))
				return nil
			}

			fmt.Println()

			if int64(len(toRemove)) != cctx.Int64("confirm-remove-count") {
				return xerrors.Errorf("value of confirm-remove-count doesn't match the number of sectors which can be removed (%d)", len(toRemove))
			}

			batchSizeReached := 0

			for _, number := range toRemove {
				fmt.Printf("Removing sector\t%s:\t", color.YellowString("%d", number))
				if batchSizeReached == 5 {
					fmt.Printf("Waiting for 2 seconds before starting next batch of 5 sectors")
					batchSizeReached = 0
					time.Sleep(2 * time.Second)
				}

				err := minerAPI.SectorRemove(ctx, number)
				if err != nil {
					color.Red("ERROR: %s\n", err.Error())
				} else {
					batchSizeReached++
					color.Green("OK\n")
				}
			}
		}

		return nil
	},
}

var sectorsBatching = &cli.Command{
	Name:  "batching",
	Usage: "manage batch sector operations",
	Subcommands: []*cli.Command{
		sectorsBatchingPendingCommit,
		sectorsBatchingPendingPreCommit,
	},
}

var sectorsBatchingPendingCommit = &cli.Command{
	Name:  "commit",
	Usage: "list sectors waiting in commit batch queue",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "publish-now",
			Usage: "send a batch now",
		},
	},
	Action: func(cctx *cli.Context) error {
		minerAPI, closer, err := lcli.GetStorageMinerAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := lcli.ReqContext(cctx)

		if cctx.Bool("publish-now") {
			res, err := minerAPI.SectorCommitFlush(ctx)
			if err != nil {
				return xerrors.Errorf("flush: %w", err)
			}
			if res == nil {
				return xerrors.Errorf("no sectors to publish")
			}

			for i, re := range res {
				fmt.Printf("Batch %d:\n", i)
				if re.Error != "" {
					fmt.Printf("\tError: %s\n", re.Error)
				} else {
					fmt.Printf("\tMessage: %s\n", re.Msg)
				}
				fmt.Printf("\tSectors:\n")
				for _, sector := range re.Sectors {
					if e, found := re.FailedSectors[sector]; found {
						fmt.Printf("\t\t%d\tERROR %s\n", sector, e)
					} else {
						fmt.Printf("\t\t%d\tOK\n", sector)
					}
				}
			}
			return nil
		}

		pending, err := minerAPI.SectorCommitPending(ctx)
		if err != nil {
			return xerrors.Errorf("getting pending deals: %w", err)
		}

		if len(pending) > 0 {
			for _, sector := range pending {
				fmt.Println(sector.Number)
			}

			reader := bufio.NewReader(os.Stdin)
			fmt.Print("Do you want to publish these sectors now? (yes/no): ")
			userInput, err := reader.ReadString('\n')
			if err != nil {
				return xerrors.Errorf("reading user input: %w", err)
			}
			userInput = strings.ToLower(strings.TrimSpace(userInput))

			if userInput == "yes" {
				err := cctx.Set("publish-now", "true")
				if err != nil {
					return xerrors.Errorf("setting publish-now flag: %w", err)
				}
				return cctx.Command.Action(cctx)
			} else if userInput == "no" {
				return nil
			}
			fmt.Println("Invalid input. Please answer with 'yes' or 'no'.")
			return nil
		}
		fmt.Println("No sectors queued to be committed")
		return nil
	},
}

var sectorsBatchingPendingPreCommit = &cli.Command{
	Name:  "precommit",
	Usage: "list sectors waiting in precommit batch queue",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "publish-now",
			Usage: "send a batch now",
		},
	},
	Action: func(cctx *cli.Context) error {
		minerAPI, closer, err := lcli.GetStorageMinerAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := lcli.ReqContext(cctx)

		if cctx.Bool("publish-now") {
			res, err := minerAPI.SectorPreCommitFlush(ctx)
			if err != nil {
				return xerrors.Errorf("flush: %w", err)
			}
			if res == nil {
				return xerrors.Errorf("no sectors to publish")
			}

			for i, re := range res {
				fmt.Printf("Batch %d:\n", i)
				if re.Error != "" {
					fmt.Printf("\tError: %s\n", re.Error)
				} else {
					fmt.Printf("\tMessage: %s\n", re.Msg)
				}
				fmt.Printf("\tSectors:\n")
				for _, sector := range re.Sectors {
					fmt.Printf("\t\t%d\tOK\n", sector)
				}
			}
			return nil
		}

		pending, err := minerAPI.SectorPreCommitPending(ctx)
		if err != nil {
			return xerrors.Errorf("getting pending deals: %w", err)
		}

		if len(pending) > 0 {
			for _, sector := range pending {
				fmt.Println(sector.Number)
			}

			reader := bufio.NewReader(os.Stdin)
			fmt.Print("Do you want to publish these sectors now? (yes/no): ")
			userInput, err := reader.ReadString('\n')
			if err != nil {
				return xerrors.Errorf("reading user input: %w", err)
			}
			userInput = strings.ToLower(strings.TrimSpace(userInput))

			if userInput == "yes" {
				err := cctx.Set("publish-now", "true")
				if err != nil {
					return xerrors.Errorf("setting publish-now flag: %w", err)
				}
				return cctx.Command.Action(cctx)
			} else if userInput == "no" {
				return nil
			}
			fmt.Println("Invalid input. Please answer with 'yes' or 'no'.")
			return nil
		}
		fmt.Println("No sectors queued to be committed")
		return nil
	},
}

var sectorsRefreshPieceMatchingCmd = &cli.Command{
	Name:  "match-pending-pieces",
	Usage: "force a refreshed match of pending pieces to open sectors without manually waiting for more deals",
	Action: func(cctx *cli.Context) error {
		minerAPI, closer, err := lcli.GetStorageMinerAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := lcli.ReqContext(cctx)

		if err := minerAPI.SectorMatchPendingPiecesToOpenSectors(ctx); err != nil {
			return err
		}

		return nil
	},
}

func yesno(b bool) string {
	if b {
		return color.GreenString("YES")
	}
	return color.RedString("NO")
}

var sectorsNumbersCmd = &cli.Command{
	Name:  "numbers",
	Usage: "manage sector number assignments",
	Subcommands: []*cli.Command{
		sectorsNumbersInfoCmd,
		sectorsNumbersReservationsCmd,
		sectorsNumbersReserveCmd,
		sectorsNumbersFreeCmd,
	},
}

var sectorsNumbersInfoCmd = &cli.Command{
	Name:  "info",
	Usage: "view sector assigner state",
	Action: func(cctx *cli.Context) error {
		minerAPI, closer, err := lcli.GetStorageMinerAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := lcli.ReqContext(cctx)

		am, err := minerAPI.SectorNumAssignerMeta(ctx)
		if err != nil {
			return err
		}

		alloc, err := strle.BitfieldToHumanRanges(am.Allocated)
		if err != nil {
			return err
		}

		reserved, err := strle.BitfieldToHumanRanges(am.Reserved)
		if err != nil {
			return err
		}

		fmt.Printf("Next free: %s\n", am.Next)
		fmt.Printf("Allocated: %s\n", alloc)
		fmt.Printf("Reserved: %s\n", reserved)

		return nil
	},
}

var sectorsNumbersReservationsCmd = &cli.Command{
	Name:  "reservations",
	Usage: "list sector number reservations",
	Action: func(cctx *cli.Context) error {
		minerAPI, closer, err := lcli.GetStorageMinerAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := lcli.ReqContext(cctx)

		rs, err := minerAPI.SectorNumReservations(ctx)
		if err != nil {
			return err
		}

		var out []string

		for name, field := range rs {
			hr, err := strle.BitfieldToHumanRanges(field)
			if err != nil {
				return err
			}
			count, err := field.Count()
			if err != nil {
				return err
			}

			out = append(out, fmt.Sprintf("%s: count=%d %s", name, count, hr))
		}

		fmt.Printf("reservations: %d\n", len(out))

		sort.Strings(out)

		for _, s := range out {
			fmt.Println(s)
		}

		return nil
	},
}

var sectorsNumbersReserveCmd = &cli.Command{
	Name:  "reserve",
	Usage: "create sector number reservations",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "force",
			Usage: "skip duplicate reservation checks (note: can lead to damaging other reservations on free)",
		},
	},
	ArgsUsage: "[reservation name] [reserved ranges]",
	Action: func(cctx *cli.Context) error {
		minerAPI, closer, err := lcli.GetStorageMinerAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := lcli.ReqContext(cctx)

		if cctx.NArg() != 2 {
			return lcli.IncorrectNumArgs(cctx)
		}

		bf, err := strle.HumanRangesToBitField(cctx.Args().Get(1))
		if err != nil {
			return xerrors.Errorf("parsing ranges: %w", err)
		}

		return minerAPI.SectorNumReserve(ctx, cctx.Args().First(), bf, cctx.Bool("force"))
	},
}

var sectorsNumbersFreeCmd = &cli.Command{
	Name:      "free",
	Usage:     "remove sector number reservations",
	ArgsUsage: "[reservation name]",
	Action: func(cctx *cli.Context) error {
		minerAPI, closer, err := lcli.GetStorageMinerAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := lcli.ReqContext(cctx)

		if cctx.NArg() != 1 {
			return lcli.IncorrectNumArgs(cctx)
		}

		return minerAPI.SectorNumFree(ctx, cctx.Args().First())
	},
}

var sectorsUnsealCmd = &cli.Command{
	Name:      "unseal",
	Usage:     "unseal a sector",
	ArgsUsage: "[sector number]",
	Action: func(cctx *cli.Context) error {
		minerAPI, closer, err := lcli.GetStorageMinerAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := lcli.ReqContext(cctx)
		if cctx.NArg() != 1 {
			return lcli.IncorrectNumArgs(cctx)
		}

		sectorNum, err := strconv.ParseUint(cctx.Args().Get(0), 10, 64)
		if err != nil {
			return xerrors.Errorf("could not parse sector number: %w", err)
		}

		fmt.Printf("Unsealing sector %d\n", sectorNum)

		return minerAPI.SectorUnseal(ctx, abi.SectorNumber(sectorNum))
	},
}
