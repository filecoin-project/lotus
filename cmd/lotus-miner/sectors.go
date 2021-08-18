package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/docker/go-units"
	"github.com/fatih/color"
	"github.com/hako/durafmt"
	cbor "github.com/ipfs/go-ipld-cbor"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-bitfield"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	miner5 "github.com/filecoin-project/specs-actors/v5/actors/builtin/miner"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/api/v0api"
	"github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/adt"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/actors/policy"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/lib/tablewriter"

	apitypes "github.com/filecoin-project/lotus/api/types"
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
		sectorsCheckExpireCmd,
		sectorsRenewCmd,
		sectorsExtendCmd,
		sectorsTerminateCmd,
		sectorsRemoveCmd,
		sectorsMarkForUpgradeCmd,
		sectorsStartSealCmd,
		sectorsSealDelayCmd,
		sectorsCapacityCollateralCmd,
		sectorsBatching,
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

		id, err := nodeApi.PledgeSector(ctx)
		if err != nil {
			return err
		}

		fmt.Println("Created CC sector: ", id.Number)

		return nil
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
			Name:        "color",
			Usage:       "use color in display output",
			DefaultText: "depends on output being a TTY",
			Aliases:     []string{"c"},
		},
		&cli.BoolFlag{
			Name:  "fast",
			Usage: "don't show on-chain info for better performance",
		},
		&cli.BoolFlag{
			Name:  "events",
			Usage: "display number of events the sector has received",
		},
		&cli.BoolFlag{
			Name:  "seal-time",
			Usage: "display how long it took for the sector to be sealed",
		},
		&cli.StringFlag{
			Name:  "states",
			Usage: "filter sectors by a comma-separated list of states",
		},
	},
	Action: func(cctx *cli.Context) error {
		if cctx.IsSet("color") {
			color.NoColor = !cctx.Bool("color")
		}

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

		var list []abi.SectorNumber

		showRemoved := cctx.Bool("show-removed")
		states := cctx.String("states")
		if len(states) == 0 {
			list, err = nodeApi.SectorsList(ctx)
		} else {
			showRemoved = true
			sList := strings.Split(states, ",")
			ss := make([]api.SectorState, len(sList))
			for i := range sList {
				ss[i] = api.SectorState(sList[i])
			}
			list, err = nodeApi.SectorsListInStates(ctx, ss)
		}

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
		commitedIDs := make(map[abi.SectorNumber]struct{}, len(sset))
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
			tablewriter.Col("SealTime"),
			tablewriter.Col("Events"),
			tablewriter.Col("Deals"),
			tablewriter.Col("DealWeight"),
			tablewriter.Col("VerifiedPower"),
			tablewriter.NewLineCol("Error"),
			tablewriter.NewLineCol("RecoveryTimeout"))

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

			if showRemoved || st.State != api.SectorState(sealing.Removed) {
				_, inSSet := commitedIDs[s]
				_, inASet := activeIDs[s]

				dw, vp := .0, .0
				if st.Expiration-st.Activation > 0 {
					rdw := big.Add(st.DealWeight, st.VerifiedDealWeight)
					dw = float64(big.Div(rdw, big.NewInt(int64(st.Expiration-st.Activation))).Uint64())
					vp = float64(big.Div(big.Mul(st.VerifiedDealWeight, big.NewInt(9)), big.NewInt(int64(st.Expiration-st.Activation))).Uint64())
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
							if vp > 0 {
								m["VerifiedPower"] = color.GreenString(units.BytesSize(vp))
							}
						}

						if st.Early > 0 {
							m["RecoveryTimeout"] = color.YellowString(lcli.EpochTime(head.Height(), st.Early))
						}
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
						if sectorLog.Kind == "event;sealing.SectorProving" {
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
		}

		return tw.Flush(os.Stdout)
	},
}

var sectorsRefsCmd = &cli.Command{
	Name:  "refs",
	Usage: "List References to sectors",
	Action: func(cctx *cli.Context) error {
		nodeApi, closer, err := lcli.GetMarketsAPI(cctx)
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

var sectorsCheckExpireCmd = &cli.Command{
	Name:  "check-expire",
	Usage: "Inspect expiring sectors",
	Flags: []cli.Flag{
		&cli.Int64Flag{
			Name:  "cutoff",
			Usage: "skip sectors whose current expiration is more than <cutoff> epochs from now, defaults to 60 days",
			Value: 172800,
		},
	},
	Action: func(cctx *cli.Context) error {
		fullApi, nCloser, err := lcli.GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer nCloser()

		ctx := lcli.ReqContext(cctx)

		maddr, err := getActorAddress(ctx, cctx)
		if err != nil {
			return err
		}

		head, err := fullApi.ChainHead(ctx)
		if err != nil {
			return err
		}
		currEpoch := head.Height()

		nv, err := fullApi.StateNetworkVersion(ctx, types.EmptyTSK)
		if err != nil {
			return err
		}

		sectors, err := fullApi.StateMinerActiveSectors(ctx, maddr, types.EmptyTSK)
		if err != nil {
			return err
		}

		n := 0
		for _, s := range sectors {
			if s.Expiration-currEpoch <= abi.ChainEpoch(cctx.Int64("cutoff")) {
				sectors[n] = s
				n++
			}
		}
		sectors = sectors[:n]

		sort.Slice(sectors, func(i, j int) bool {
			if sectors[i].Expiration == sectors[j].Expiration {
				return sectors[i].SectorNumber < sectors[j].SectorNumber
			}
			return sectors[i].Expiration < sectors[j].Expiration
		})

		tw := tablewriter.New(
			tablewriter.Col("ID"),
			tablewriter.Col("SealProof"),
			tablewriter.Col("InitialPledge"),
			tablewriter.Col("Activation"),
			tablewriter.Col("Expiration"),
			tablewriter.Col("MaxExpiration"),
			tablewriter.Col("MaxExtendNow"))

		for _, sector := range sectors {
			MaxExpiration := sector.Activation + policy.GetSectorMaxLifetime(sector.SealProof, nv)
			MaxExtendNow := currEpoch + policy.GetMaxSectorExpirationExtension()

			if MaxExtendNow > MaxExpiration {
				MaxExtendNow = MaxExpiration
			}

			tw.Write(map[string]interface{}{
				"ID":            sector.SectorNumber,
				"SealProof":     sector.SealProof,
				"InitialPledge": types.FIL(sector.InitialPledge).Short(),
				"Activation":    epochTime(currEpoch, sector.Activation),
				"Expiration":    epochTime(currEpoch, sector.Expiration),
				"MaxExpiration": epochTime(currEpoch, MaxExpiration),
				"MaxExtendNow":  epochTime(currEpoch, MaxExtendNow),
			})
		}

		return tw.Flush(os.Stdout)
	},
}

var sectorsRenewCmd = &cli.Command{
	Name:  "renew",
	Usage: "Renew expiring sectors while not exceeding each sector's max life",
	Flags: []cli.Flag{
		&cli.Int64Flag{
			Name:  "from",
			Usage: "only consider sectors whose current expiration epoch is in the range of [from, to], <from> defaults to: now + 120 (1 hour)",
		},
		&cli.Int64Flag{
			Name:  "to",
			Usage: "only consider sectors whose current expiration epoch is in the range of [from, to], <to> defaults to: now + 92160 (32 days)",
		},
		&cli.StringFlag{
			Name:  "sector-file",
			Usage: "provide a file containing one sector number in each line, ignoring above selecting criteria",
		},
		&cli.StringFlag{
			Name:  "exclude",
			Usage: "optionally provide a file containing excluding sectors",
		},
		&cli.Int64Flag{
			Name:  "extension",
			Usage: "try to extend selected sectors by this number of epochs, defaults to 540 days",
			Value: 1555200,
		},
		&cli.Int64Flag{
			Name:  "new-expiration",
			Usage: "try to extend selected sectors to this epoch, ignoring extension",
		},
		&cli.Int64Flag{
			Name:  "tolerance",
			Usage: "don't try to extend sectors by fewer than this number of epochs, defaults to 7 days",
			Value: 20160,
		},
		&cli.StringFlag{
			Name:  "max-fee",
			Usage: "use up to this amount of FIL for one message. pass this flag to avoid message congestion.",
			Value: "0",
		},
		&cli.BoolFlag{
			Name:  "really-do-it",
			Usage: "pass this flag to really renew sectors, otherwise will only print out json representation of parameters",
		},
	},
	Action: func(cctx *cli.Context) error {
		fullApi, nCloser, err := lcli.GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer nCloser()

		ctx := lcli.ReqContext(cctx)

		maddr, err := getActorAddress(ctx, cctx)
		if err != nil {
			return err
		}

		// Step 1: Create location (deadline & partition) map for all active sectors
		asl, err := getActiveSectorsLocation(ctx, fullApi, maddr)
		if err != nil {
			return err
		}

		head, err := fullApi.ChainHead(ctx)
		if err != nil {
			return err
		}
		currEpoch := head.Height()

		// Step 2: Select renewable sectors according to user-provided selecting criteria
		selected, err := selectRenewableSectors(ctx, cctx, fullApi, currEpoch, maddr)
		if err != nil {
			return err
		}

		nv, err := fullApi.StateNetworkVersion(ctx, types.EmptyTSK)
		if err != nil {
			return err
		}

		// Step 3: Sort selected sectors by their location and new expiration epoch
		extensions, err := genExtensions(cctx, currEpoch, nv, asl, selected)
		if err != nil {
			return err
		}

		// Step 4: Generate mpool-ready extension params
		params, err := genExtensionParams(nv, extensions)
		if err != nil {
			return err
		}

		// Step 5: Handle extension params: display or send messages
		return handleExtensionParams(ctx, cctx, fullApi, maddr, params)
	},
}

func getActiveSectorsLocation(ctx context.Context, fullApi v0api.FullNode, maddr address.Address) (map[abi.SectorNumber]*miner.SectorLocation, error) {
	mact, err := fullApi.StateGetActor(ctx, maddr, types.EmptyTSK)
	if err != nil {
		return nil, err
	}

	tbs := blockstore.NewTieredBstore(blockstore.NewAPIBlockstore(fullApi), blockstore.NewMemory())
	mas, err := miner.Load(adt.WrapStore(ctx, cbor.NewCborStore(tbs)), mact)
	if err != nil {
		return nil, err
	}

	asl := make(map[abi.SectorNumber]*miner.SectorLocation)

	if err := mas.ForEachDeadline(func(dlIdx uint64, dl miner.Deadline) error {
		return dl.ForEachPartition(func(partIdx uint64, part miner.Partition) error {
			pas, err := part.ActiveSectors()
			if err != nil {
				return err
			}

			return pas.ForEach(func(i uint64) error {
				asl[abi.SectorNumber(i)] = &miner.SectorLocation{
					Deadline:  dlIdx,
					Partition: partIdx,
				}
				return nil
			})
		})
	}); err != nil {
		return nil, err
	}

	return asl, nil
}

func selectRenewableSectors(ctx context.Context, cctx *cli.Context, fullApi v0api.FullNode, currEpoch abi.ChainEpoch, maddr address.Address) ([]*miner.SectorOnChainInfo, error) {
	activeSet, err := fullApi.StateMinerActiveSectors(ctx, maddr, types.EmptyTSK)
	if err != nil {
		return nil, err
	}

	activeSectorsInfo := make(map[abi.SectorNumber]*miner.SectorOnChainInfo, len(activeSet))
	for _, info := range activeSet {
		activeSectorsInfo[info.SectorNumber] = info
	}

	excludeSet := make(map[uint64]struct{})

	if cctx.IsSet("exclude") {
		excludeSectors, err := getSectorsFromFile(cctx.String("exclude"))
		if err != nil {
			return nil, err
		}

		for _, id := range excludeSectors {
			excludeSet[id] = struct{}{}
		}
	}

	var selected []*miner.SectorOnChainInfo

	if cctx.IsSet("sector-file") {
		sectors, err := getSectorsFromFile(cctx.String("sector-file"))
		if err != nil {
			return nil, err
		}

		for _, id := range sectors {
			if _, exclude := excludeSet[id]; exclude {
				continue
			}

			si, found := activeSectorsInfo[abi.SectorNumber(id)]
			if !found {
				return nil, xerrors.Errorf("sector %d is not active", id)
			}

			selected = append(selected, si)
		}
	} else {
		from := currEpoch + 120
		to := currEpoch + 92160

		if cctx.IsSet("from") {
			from = abi.ChainEpoch(cctx.Int64("from"))
		}

		if cctx.IsSet("to") {
			to = abi.ChainEpoch(cctx.Int64("to"))
		}

		for _, si := range activeSet {
			if si.Expiration >= from && si.Expiration <= to {
				if _, exclude := excludeSet[uint64(si.SectorNumber)]; !exclude {
					selected = append(selected, si)
				}
			}
		}
	}

	return selected, nil
}

func genExtensions(cctx *cli.Context, currEpoch abi.ChainEpoch, nv apitypes.NetworkVersion, asl map[abi.SectorNumber]*miner.SectorLocation,
	selected []*miner.SectorOnChainInfo) (map[miner.SectorLocation]map[abi.ChainEpoch][]uint64, error) {

	extensions := map[miner.SectorLocation]map[abi.ChainEpoch][]uint64{}

	withinTolerance := func(a, b abi.ChainEpoch) bool {
		diff := a - b
		if diff < 0 {
			diff = -diff
		}

		return diff <= abi.ChainEpoch(cctx.Int64("tolerance"))
	}

	for _, si := range selected {
		extension := abi.ChainEpoch(cctx.Int64("extension"))
		newExp := si.Expiration + extension

		if cctx.IsSet("new-expiration") {
			newExp = abi.ChainEpoch(cctx.Int64("new-expiration"))
		}

		maxExtendNow := currEpoch + policy.GetMaxSectorExpirationExtension()
		if newExp > maxExtendNow {
			newExp = maxExtendNow
		}

		maxExp := si.Activation + policy.GetSectorMaxLifetime(si.SealProof, nv)
		if newExp > maxExp {
			newExp = maxExp
		}

		if newExp <= si.Expiration || withinTolerance(newExp, si.Expiration) {
			continue
		}

		l, found := asl[si.SectorNumber]
		if !found {
			return nil, xerrors.Errorf("location for sector %d not found", si.SectorNumber)
		}

		es, found := extensions[*l]
		if !found {
			ne := make(map[abi.ChainEpoch][]uint64)
			ne[newExp] = []uint64{uint64(si.SectorNumber)}
			extensions[*l] = ne
		} else {
			added := false
			for exp := range es {
				if withinTolerance(newExp, exp) {
					es[exp] = append(es[exp], uint64(si.SectorNumber))
					added = true
					break
				}
			}

			if !added {
				es[newExp] = []uint64{uint64(si.SectorNumber)}
			}
		}
	}

	return extensions, nil
}

func genExtensionParams(nv apitypes.NetworkVersion, extensions map[miner.SectorLocation]map[abi.ChainEpoch][]uint64) ([]miner5.ExtendSectorExpirationParams, error) {
	addressedMax, err := policy.GetAddressedSectorsMax(nv)
	if err != nil {
		return nil, xerrors.Errorf("failed to get addressed sectors max: %w", err)
	}

	declMax, err := policy.GetDeclarationsMax(nv)
	if err != nil {
		return nil, xerrors.Errorf("failed to get declarations max: %w", err)
	}

	var params []miner5.ExtendSectorExpirationParams

	p := miner5.ExtendSectorExpirationParams{}
	scount := 0

	for l, exts := range extensions {
		for newExp, numbers := range exts {
			scount += len(numbers)

			if scount > addressedMax || len(p.Extensions) == declMax {
				params = append(params, p)
				p = miner5.ExtendSectorExpirationParams{}
				scount = len(numbers)
			}

			p.Extensions = append(p.Extensions, miner5.ExpirationExtension{
				Deadline:      l.Deadline,
				Partition:     l.Partition,
				Sectors:       bitfield.NewFromSet(numbers),
				NewExpiration: newExp,
			})
		}
	}

	// if we have any sectors, then one last append is needed here
	if scount != 0 {
		params = append(params, p)
	}

	return params, nil
}

func handleExtensionParams(ctx context.Context, cctx *cli.Context, fullApi v0api.FullNode, maddr address.Address, params []miner5.ExtendSectorExpirationParams) error {
	if len(params) == 0 {
		fmt.Println("nothing to renew")
		return nil
	}

	mi, err := fullApi.StateMinerInfo(ctx, maddr, types.EmptyTSK)
	if err != nil {
		return xerrors.Errorf("getting miner info: %w", err)
	}

	mf, err := types.ParseFIL(cctx.String("max-fee"))
	if err != nil {
		return err
	}

	spec := &api.MessageSendSpec{MaxFee: abi.TokenAmount(mf)}

	stotal := 0

	for i := range params {
		scount := 0
		for _, ext := range params[i].Extensions {
			count, err := ext.Sectors.Count()
			if err != nil {
				return err
			}
			scount += int(count)
		}
		fmt.Printf("Renewing %d sectors: ", scount)
		stotal += scount

		if !cctx.Bool("really-do-it") {
			pp, err := newPseudoExtendParams(&params[i])
			if err != nil {
				return err
			}

			data, err := json.MarshalIndent(pp, "", "  ")
			if err != nil {
				return err
			}

			fmt.Println()
			fmt.Println(string(data))
			continue
		}

		sp, aerr := actors.SerializeParams(&params[i])
		if aerr != nil {
			return xerrors.Errorf("serializing params: %w", err)
		}

		smsg, err := fullApi.MpoolPushMessage(ctx, &types.Message{
			From:   mi.Worker,
			To:     maddr,
			Method: miner.Methods.ExtendSectorExpiration,
			Value:  big.Zero(),
			Params: sp,
		}, spec)
		if err != nil {
			return xerrors.Errorf("mpool push message: %w", err)
		}

		fmt.Println(smsg.Cid())
	}

	fmt.Printf("%d sectors renewed\n", stotal)

	return nil
}

var sectorsExtendCmd = &cli.Command{
	Name:      "extend",
	Usage:     "Extend sector expiration",
	ArgsUsage: "<sectorNumbers...>",
	Flags: []cli.Flag{
		&cli.Int64Flag{
			Name:     "new-expiration",
			Usage:    "new expiration epoch",
			Required: false,
		},
		&cli.BoolFlag{
			Name:     "v1-sectors",
			Usage:    "renews all v1 sectors up to the maximum possible lifetime",
			Required: false,
		},
		&cli.Int64Flag{
			Name:     "tolerance",
			Value:    20160,
			Usage:    "when extending v1 sectors, don't try to extend sectors by fewer than this number of epochs",
			Required: false,
		},
		&cli.Int64Flag{
			Name:     "expiration-ignore",
			Value:    120,
			Usage:    "when extending v1 sectors, skip sectors whose current expiration is less than <ignore> epochs from now",
			Required: false,
		},
		&cli.Int64Flag{
			Name:     "expiration-cutoff",
			Usage:    "when extending v1 sectors, skip sectors whose current expiration is more than <cutoff> epochs from now (infinity if unspecified)",
			Required: false,
		},
		&cli.StringFlag{},
	},
	Action: func(cctx *cli.Context) error {

		api, nCloser, err := lcli.GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer nCloser()

		ctx := lcli.ReqContext(cctx)

		maddr, err := getActorAddress(ctx, cctx)
		if err != nil {
			return err
		}

		var params []miner5.ExtendSectorExpirationParams

		if cctx.Bool("v1-sectors") {

			head, err := api.ChainHead(ctx)
			if err != nil {
				return err
			}

			nv, err := api.StateNetworkVersion(ctx, types.EmptyTSK)
			if err != nil {
				return err
			}

			extensions := map[miner.SectorLocation]map[abi.ChainEpoch][]uint64{}

			// are given durations within tolerance epochs
			withinTolerance := func(a, b abi.ChainEpoch) bool {
				diff := a - b
				if diff < 0 {
					diff = b - a
				}

				return diff <= abi.ChainEpoch(cctx.Int64("tolerance"))
			}

			sis, err := api.StateMinerActiveSectors(ctx, maddr, types.EmptyTSK)
			if err != nil {
				return xerrors.Errorf("getting miner sector infos: %w", err)
			}

			for _, si := range sis {
				if si.SealProof >= abi.RegisteredSealProof_StackedDrg2KiBV1_1 {
					continue
				}

				if si.Expiration < (head.Height() + abi.ChainEpoch(cctx.Int64("expiration-ignore"))) {
					continue
				}

				if cctx.IsSet("expiration-cutoff") {
					if si.Expiration > (head.Height() + abi.ChainEpoch(cctx.Int64("expiration-cutoff"))) {
						continue
					}
				}

				ml := policy.GetSectorMaxLifetime(si.SealProof, nv)
				// if the sector's missing less than "tolerance" of its maximum possible lifetime, don't bother extending it
				if withinTolerance(si.Expiration-si.Activation, ml) {
					continue
				}

				// Set the new expiration to 48 hours less than the theoretical maximum lifetime
				newExp := ml - (miner5.WPoStProvingPeriod * 2) + si.Activation
				if withinTolerance(si.Expiration, newExp) || si.Expiration >= newExp {
					continue
				}

				p, err := api.StateSectorPartition(ctx, maddr, si.SectorNumber, types.EmptyTSK)
				if err != nil {
					return xerrors.Errorf("getting sector location for sector %d: %w", si.SectorNumber, err)
				}

				if p == nil {
					return xerrors.Errorf("sector %d not found in any partition", si.SectorNumber)
				}

				es, found := extensions[*p]
				if !found {
					ne := make(map[abi.ChainEpoch][]uint64)
					ne[newExp] = []uint64{uint64(si.SectorNumber)}
					extensions[*p] = ne
				} else {
					added := false
					for exp := range es {
						if withinTolerance(exp, newExp) && newExp >= exp && exp > si.Expiration {
							es[exp] = append(es[exp], uint64(si.SectorNumber))
							added = true
							break
						}
					}

					if !added {
						es[newExp] = []uint64{uint64(si.SectorNumber)}
					}
				}
			}

			addressedMax, err := policy.GetAddressedSectorsMax(nv)
			if err != nil {
				return xerrors.Errorf("failed to get addressed sectors max: %w", err)
			}

			declMax, err := policy.GetDeclarationsMax(nv)
			if err != nil {
				return xerrors.Errorf("failed to get declarations max: %w", err)
			}

			p := miner5.ExtendSectorExpirationParams{}
			scount := 0

			for l, exts := range extensions {
				for newExp, numbers := range exts {
					scount += len(numbers)

					if scount > addressedMax || len(p.Extensions) == declMax {
						params = append(params, p)
						p = miner5.ExtendSectorExpirationParams{}
						scount = len(numbers)
					}

					p.Extensions = append(p.Extensions, miner5.ExpirationExtension{
						Deadline:      l.Deadline,
						Partition:     l.Partition,
						Sectors:       bitfield.NewFromSet(numbers),
						NewExpiration: newExp,
					})
				}
			}

			// if we have any sectors, then one last append is needed here
			if scount != 0 {
				params = append(params, p)
			}

		} else {
			if !cctx.Args().Present() || !cctx.IsSet("new-expiration") {
				return xerrors.Errorf("must pass at least one sector number and new expiration")
			}
			sectors := map[miner.SectorLocation][]uint64{}

			for i, s := range cctx.Args().Slice() {
				id, err := strconv.ParseUint(s, 10, 64)
				if err != nil {
					return xerrors.Errorf("could not parse sector %d: %w", i, err)
				}

				p, err := api.StateSectorPartition(ctx, maddr, abi.SectorNumber(id), types.EmptyTSK)
				if err != nil {
					return xerrors.Errorf("getting sector location for sector %d: %w", id, err)
				}

				if p == nil {
					return xerrors.Errorf("sector %d not found in any partition", id)
				}

				sectors[*p] = append(sectors[*p], id)
			}

			p := miner5.ExtendSectorExpirationParams{}
			for l, numbers := range sectors {

				// TODO: Dedup with above loop
				p.Extensions = append(p.Extensions, miner5.ExpirationExtension{
					Deadline:      l.Deadline,
					Partition:     l.Partition,
					Sectors:       bitfield.NewFromSet(numbers),
					NewExpiration: abi.ChainEpoch(cctx.Int64("new-expiration")),
				})
			}

			params = append(params, p)
		}

		if len(params) == 0 {
			fmt.Println("nothing to extend")
			return nil
		}

		mi, err := api.StateMinerInfo(ctx, maddr, types.EmptyTSK)
		if err != nil {
			return xerrors.Errorf("getting miner info: %w", err)
		}

		for i := range params {
			sp, aerr := actors.SerializeParams(&params[i])
			if aerr != nil {
				return xerrors.Errorf("serializing params: %w", err)
			}

			smsg, err := api.MpoolPushMessage(ctx, &types.Message{
				From:   mi.Worker,
				To:     maddr,
				Method: miner.Methods.ExtendSectorExpiration,

				Value:  big.Zero(),
				Params: sp,
			}, nil)
			if err != nil {
				return xerrors.Errorf("mpool push message: %w", err)
			}

			fmt.Println(smsg.Cid())
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
		if !cctx.Bool("really-do-it") {
			return xerrors.Errorf("pass --really-do-it to confirm this action")
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

		return nodeApi.SectorTerminate(ctx, abi.SectorNumber(id))
	},
}

var sectorsTerminateFlushCmd = &cli.Command{
	Name:  "flush",
	Usage: "Send a terminate message if there are sectors queued for termination",
	Action: func(cctx *cli.Context) error {
		nodeApi, closer, err := lcli.GetStorageMinerAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := lcli.ReqContext(cctx)

		mcid, err := nodeApi.SectorTerminateFlush(ctx)
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
		nodeApi, closer, err := lcli.GetStorageMinerAPI(cctx)
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

		pending, err := nodeApi.SectorTerminatePending(ctx)
		if err != nil {
			return err
		}

		maddr, err := nodeApi.ActorAddress(ctx)
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

		spt, err := miner.PreferredSealProofTypeFromWindowPoStType(nv, mi.WindowPoStProofType)
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

			pci.Expiration = policy.GetMaxSectorExpirationExtension() + h.Height()
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
		api, closer, err := lcli.GetStorageMinerAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := lcli.ReqContext(cctx)

		if cctx.Bool("publish-now") {
			res, err := api.SectorCommitFlush(ctx)
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

		pending, err := api.SectorCommitPending(ctx)
		if err != nil {
			return xerrors.Errorf("getting pending deals: %w", err)
		}

		if len(pending) > 0 {
			for _, sector := range pending {
				fmt.Println(sector.Number)
			}
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
		api, closer, err := lcli.GetStorageMinerAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := lcli.ReqContext(cctx)

		if cctx.Bool("publish-now") {
			res, err := api.SectorPreCommitFlush(ctx)
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

		pending, err := api.SectorPreCommitPending(ctx)
		if err != nil {
			return xerrors.Errorf("getting pending deals: %w", err)
		}

		if len(pending) > 0 {
			for _, sector := range pending {
				fmt.Println(sector.Number)
			}
			return nil
		}

		fmt.Println("No sectors queued to be committed")
		return nil
	},
}

func yesno(b bool) string {
	if b {
		return color.GreenString("YES")
	}
	return color.RedString("NO")
}

func epochTime(curr, e abi.ChainEpoch) string {
	if build.BuildType == build.BuildMainnet {
		start := time.Date(2020, 8, 24, 22, 0, 0, 0, time.UTC)
		eTime := start.Add(time.Second * time.Duration(int64(build.BlockDelaySecs)*int64(e)))
		return fmt.Sprintf("%d (%s)", e, eTime.Format("06-01-02 15:04 MST"))
	}

	switch {
	case curr > e:
		return fmt.Sprintf("%d (%s ago)", e, durafmt.Parse(time.Second*time.Duration(int64(build.BlockDelaySecs)*int64(curr-e))).LimitFirstN(2))
	case curr == e:
		return fmt.Sprintf("%d (now)", e)
	case curr < e:
		return fmt.Sprintf("%d (in %s)", e, durafmt.Parse(time.Second*time.Duration(int64(build.BlockDelaySecs)*int64(e-curr))).LimitFirstN(2))
	}

	panic("math broke")
}

func getSectorsFromFile(filePath string) ([]uint64, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}

	scanner := bufio.NewScanner(file)
	sectors := make([]uint64, 0)

	for scanner.Scan() {
		line := scanner.Text()

		id, err := strconv.ParseUint(line, 10, 64)
		if err != nil {
			return nil, xerrors.Errorf("could not parse %s as sector id: %s", line, err)
		}

		sectors = append(sectors, id)
	}

	if err = file.Close(); err != nil {
		return nil, err
	}

	return sectors, nil
}

type pseudoExpirationExtension struct {
	Deadline      uint64
	Partition     uint64
	Sectors       string
	NewExpiration abi.ChainEpoch
}

type pseudoExtendSectorExpirationParams struct {
	Extensions []pseudoExpirationExtension
}

func newPseudoExtendParams(p *miner5.ExtendSectorExpirationParams) (*pseudoExtendSectorExpirationParams, error) {
	res := pseudoExtendSectorExpirationParams{}
	for _, ext := range p.Extensions {
		scount, err := ext.Sectors.Count()
		if err != nil {
			return nil, err
		}

		sectors, err := ext.Sectors.All(scount)
		if err != nil {
			return nil, err
		}

		res.Extensions = append(res.Extensions, pseudoExpirationExtension{
			Deadline:      ext.Deadline,
			Partition:     ext.Partition,
			Sectors:       SliceToString(sectors),
			NewExpiration: ext.NewExpiration,
		})
	}
	return &res, nil
}

// SliceToString Example: {1,3,4,5,8,9} -> "1,3-5,8-9"
func SliceToString(sli []uint64) string {
	if len(sli) == 0 {
		return ""
	}

	sort.Slice(sli, func(i, j int) bool {
		return sli[i] < sli[j]
	})

	var segments [][2]uint64
	start := sli[0]
	end := sli[0]

	for _, elm := range sli[1:] {
		if elm == end || elm == end+1 {
			end = elm
		} else {
			segments = append(segments, [2]uint64{start, end})
			start = elm
			end = elm
		}
	}

	segments = append(segments, [2]uint64{start, end})

	var ss []string

	for _, seg := range segments {
		start := seg[0]
		end := seg[1]

		if end == start {
			ss = append(ss, strconv.FormatUint(start, 10))
		} else {
			ss = append(ss, strconv.FormatUint(start, 10)+"-"+strconv.FormatUint(end, 10))
		}
	}

	return strings.Join(ss, ",")
}
