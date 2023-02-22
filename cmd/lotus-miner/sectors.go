package main

import (
	"bufio"
	"encoding/json"
	"errors"
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
	"github.com/filecoin-project/go-state-types/builtin"
	"github.com/filecoin-project/go-state-types/builtin/v9/miner"
	"github.com/filecoin-project/go-state-types/network"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/adt"
	lminer "github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/actors/builtin/verifreg"
	"github.com/filecoin-project/lotus/chain/actors/policy"
	"github.com/filecoin-project/lotus/chain/types"
	lcli "github.com/filecoin-project/lotus/cli"
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
		sectorsStatusCmd,
		sectorsListCmd,
		sectorsRefsCmd,
		sectorsUpdateCmd,
		sectorsPledgeCmd,
		sectorsNumbersCmd,
		sectorPreCommitsCmd,
		sectorsCheckExpireCmd,
		sectorsExpiredCmd,
		sectorsExtendCmd,
		sectorsTerminateCmd,
		sectorsRemoveCmd,
		sectorsSnapUpCmd,
		sectorsSnapAbortCmd,
		sectorsStartSealCmd,
		sectorsSealDelayCmd,
		sectorsCapacityCollateralCmd,
		sectorsBatching,
		sectorsRefreshPieceMatchingCmd,
		sectorsCompactPartitionsCmd,
	},
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

var sectorsStatusCmd = &cli.Command{
	Name:      "status",
	Usage:     "Get the seal status of a sector by its number",
	ArgsUsage: "<sectorNum>",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:    "log",
			Usage:   "display event log",
			Aliases: []string{"l"},
		},
		&cli.BoolFlag{
			Name:    "on-chain-info",
			Usage:   "show sector on chain info",
			Aliases: []string{"c"},
		},
		&cli.BoolFlag{
			Name:    "partition-info",
			Usage:   "show partition related info",
			Aliases: []string{"p"},
		},
		&cli.BoolFlag{
			Name:  "proof",
			Usage: "print snark proof bytes as hex",
		},
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

		id, err := strconv.ParseUint(cctx.Args().First(), 10, 64)
		if err != nil {
			return err
		}

		onChainInfo := cctx.Bool("on-chain-info")
		status, err := minerApi.SectorsStatus(ctx, abi.SectorNumber(id), onChainInfo)
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
		if cctx.Bool("proof") {
			fmt.Printf("Proof:\t\t%x\n", status.Proof)
		}
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
			fmt.Printf("InitialPledge:\t\t%v\n", types.FIL(status.InitialPledge))
			fmt.Printf("\nExpiration Info\n")
			fmt.Printf("OnTime:\t\t%v\n", status.OnTime)
			fmt.Printf("Early:\t\t%v\n", status.Early)
		}

		if cctx.Bool("partition-info") {
			fullApi, nCloser, err := lcli.GetFullNodeAPI(cctx)
			if err != nil {
				return err
			}
			defer nCloser()

			maddr, err := getActorAddress(ctx, cctx)
			if err != nil {
				return err
			}

			mact, err := fullApi.StateGetActor(ctx, maddr, types.EmptyTSK)
			if err != nil {
				return err
			}

			tbs := blockstore.NewTieredBstore(blockstore.NewAPIBlockstore(fullApi), blockstore.NewMemory())
			mas, err := lminer.Load(adt.WrapStore(ctx, cbor.NewCborStore(tbs)), mact)
			if err != nil {
				return err
			}

			errFound := errors.New("found")
			if err := mas.ForEachDeadline(func(dlIdx uint64, dl lminer.Deadline) error {
				return dl.ForEachPartition(func(partIdx uint64, part lminer.Partition) error {
					pas, err := part.AllSectors()
					if err != nil {
						return err
					}

					set, err := pas.IsSet(id)
					if err != nil {
						return err
					}
					if set {
						fmt.Printf("\nDeadline:\t%d\n", dlIdx)
						fmt.Printf("Partition:\t%d\n", partIdx)

						checkIn := func(name string, bg func() (bitfield.BitField, error)) error {
							bf, err := bg()
							if err != nil {
								return err
							}

							set, err := bf.IsSet(id)
							if err != nil {
								return err
							}
							setstr := "no"
							if set {
								setstr = "yes"
							}
							fmt.Printf("%s:   \t%s\n", name, setstr)
							return nil
						}

						if err := checkIn("Unproven", part.UnprovenSectors); err != nil {
							return err
						}
						if err := checkIn("Live", part.LiveSectors); err != nil {
							return err
						}
						if err := checkIn("Active", part.ActiveSectors); err != nil {
							return err
						}
						if err := checkIn("Faulty", part.FaultySectors); err != nil {
							return err
						}
						if err := checkIn("Recovering", part.RecoveringSectors); err != nil {
							return err
						}

						return errFound
					}

					return nil
				})
			}); err != errFound {
				if err != nil {
					return err
				}

				fmt.Println("\nNot found in any partition")
			}
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
				dw = float64(big.Div(rdw, big.NewInt(int64(st.Expiration-st.Activation))).Uint64())
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
				"Activation":    cliutil.EpochTime(currEpoch, sector.Activation),
				"Expiration":    cliutil.EpochTime(currEpoch, sector.Expiration),
				"MaxExpiration": cliutil.EpochTime(currEpoch, MaxExpiration),
				"MaxExtendNow":  cliutil.EpochTime(currEpoch, MaxExtendNow),
			})
		}

		return tw.Flush(os.Stdout)
	},
}

type PseudoExpirationExtension struct {
	Deadline      uint64
	Partition     uint64
	Sectors       string
	NewExpiration abi.ChainEpoch
}

type PseudoExtendSectorExpirationParams struct {
	Extensions []PseudoExpirationExtension
}

func NewPseudoExtendParams(p *miner.ExtendSectorExpiration2Params) (*PseudoExtendSectorExpirationParams, error) {
	res := PseudoExtendSectorExpirationParams{}
	for _, ext := range p.Extensions {
		scount, err := ext.Sectors.Count()
		if err != nil {
			return nil, err
		}

		sectors, err := ext.Sectors.All(scount)
		if err != nil {
			return nil, err
		}

		res.Extensions = append(res.Extensions, PseudoExpirationExtension{
			Deadline:      ext.Deadline,
			Partition:     ext.Partition,
			Sectors:       ArrayToString(sectors),
			NewExpiration: ext.NewExpiration,
		})
	}
	return &res, nil
}

// ArrayToString Example: {1,3,4,5,8,9} -> "1,3-5,8-9"
func ArrayToString(array []uint64) string {
	sort.Slice(array, func(i, j int) bool {
		return array[i] < array[j]
	})

	var sarray []string
	s := ""

	for i, elm := range array {
		if i == 0 {
			s = strconv.FormatUint(elm, 10)
			continue
		}
		if elm == array[i-1] {
			continue // filter out duplicates
		} else if elm == array[i-1]+1 {
			s = strings.Split(s, "-")[0] + "-" + strconv.FormatUint(elm, 10)
		} else {
			sarray = append(sarray, s)
			s = strconv.FormatUint(elm, 10)
		}
	}

	if s != "" {
		sarray = append(sarray, s)
	}

	return strings.Join(sarray, ",")
}

func getSectorsFromFile(filePath string) ([]abi.SectorNumber, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}

	scanner := bufio.NewScanner(file)
	sectors := make([]abi.SectorNumber, 0)

	for scanner.Scan() {
		line := scanner.Text()

		id, err := strconv.ParseUint(line, 10, 64)
		if err != nil {
			return nil, xerrors.Errorf("could not parse %s as sector id: %s", line, err)
		}

		sectors = append(sectors, abi.SectorNumber(id))
	}

	if err = file.Close(); err != nil {
		return nil, err
	}

	return sectors, nil
}

func SectorNumsToBitfield(sectors []abi.SectorNumber) bitfield.BitField {
	var numbers []uint64
	for _, sector := range sectors {
		numbers = append(numbers, uint64(sector))
	}

	return bitfield.NewFromSet(numbers)
}

var sectorsExtendCmd = &cli.Command{
	Name:      "extend",
	Usage:     "Extend expiring sectors while not exceeding each sector's max life",
	ArgsUsage: "<sectorNumbers...(optional)>",
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
		&cli.BoolFlag{
			Name:  "only-cc",
			Usage: "only extend CC sectors (useful for making sector ready for snap upgrade)",
		},
		&cli.BoolFlag{
			Name:  "drop-claims",
			Usage: "drop claims for sectors that can be extended, but only by dropping some of their verified power claims",
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
		&cli.Int64Flag{
			Name:  "max-sectors",
			Usage: "the maximum number of sectors contained in each message",
		},
		&cli.BoolFlag{
			Name:  "really-do-it",
			Usage: "pass this flag to really extend sectors, otherwise will only print out json representation of parameters",
		},
	},
	Action: func(cctx *cli.Context) error {
		mf, err := types.ParseFIL(cctx.String("max-fee"))
		if err != nil {
			return err
		}

		spec := &api.MessageSendSpec{MaxFee: abi.TokenAmount(mf)}

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

		activeSet, err := fullApi.StateMinerActiveSectors(ctx, maddr, types.EmptyTSK)
		if err != nil {
			return err
		}

		activeSectorsInfo := make(map[abi.SectorNumber]*miner.SectorOnChainInfo, len(activeSet))
		for _, info := range activeSet {
			activeSectorsInfo[info.SectorNumber] = info
		}

		mact, err := fullApi.StateGetActor(ctx, maddr, types.EmptyTSK)
		if err != nil {
			return err
		}

		tbs := blockstore.NewTieredBstore(blockstore.NewAPIBlockstore(fullApi), blockstore.NewMemory())
		adtStore := adt.WrapStore(ctx, cbor.NewCborStore(tbs))
		mas, err := lminer.Load(adtStore, mact)
		if err != nil {
			return err
		}

		activeSectorsLocation := make(map[abi.SectorNumber]*lminer.SectorLocation, len(activeSet))

		if err := mas.ForEachDeadline(func(dlIdx uint64, dl lminer.Deadline) error {
			return dl.ForEachPartition(func(partIdx uint64, part lminer.Partition) error {
				pas, err := part.ActiveSectors()
				if err != nil {
					return err
				}

				return pas.ForEach(func(i uint64) error {
					activeSectorsLocation[abi.SectorNumber(i)] = &lminer.SectorLocation{
						Deadline:  dlIdx,
						Partition: partIdx,
					}
					return nil
				})
			})
		}); err != nil {
			return err
		}

		excludeSet := make(map[abi.SectorNumber]struct{})
		if cctx.IsSet("exclude") {
			excludeSectors, err := getSectorsFromFile(cctx.String("exclude"))
			if err != nil {
				return err
			}

			for _, id := range excludeSectors {
				excludeSet[id] = struct{}{}
			}
		}

		var sectors []abi.SectorNumber
		if cctx.Args().Present() {
			if cctx.IsSet("sector-file") {
				return xerrors.Errorf("sector-file specified along with command line params")
			}

			for i, s := range cctx.Args().Slice() {
				id, err := strconv.ParseUint(s, 10, 64)
				if err != nil {
					return xerrors.Errorf("could not parse sector %d: %w", i, err)
				}

				sectors = append(sectors, abi.SectorNumber(id))
			}
		} else if cctx.IsSet("sector-file") {
			sectors, err = getSectorsFromFile(cctx.String("sector-file"))
			if err != nil {
				return err
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
					sectors = append(sectors, si.SectorNumber)
				}
			}
		}

		var sis []*miner.SectorOnChainInfo
		for _, id := range sectors {
			if _, exclude := excludeSet[id]; exclude {
				continue
			}

			si, found := activeSectorsInfo[id]
			if !found {
				return xerrors.Errorf("sector %d is not active", id)
			}
			if len(si.DealIDs) > 0 && cctx.Bool("only-cc") {
				continue
			}

			sis = append(sis, si)
		}

		withinTolerance := func(a, b abi.ChainEpoch) bool {
			diff := a - b
			if diff < 0 {
				diff = -diff
			}

			return diff <= abi.ChainEpoch(cctx.Int64("tolerance"))
		}

		extensions := map[lminer.SectorLocation]map[abi.ChainEpoch][]abi.SectorNumber{}
		for _, si := range sis {
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

			l, found := activeSectorsLocation[si.SectorNumber]
			if !found {
				return xerrors.Errorf("location for sector %d not found", si.SectorNumber)
			}

			es, found := extensions[*l]
			if !found {
				ne := make(map[abi.ChainEpoch][]abi.SectorNumber)
				ne[newExp] = []abi.SectorNumber{si.SectorNumber}
				extensions[*l] = ne
			} else {
				added := false
				for exp := range es {
					if withinTolerance(newExp, exp) {
						es[exp] = append(es[exp], si.SectorNumber)
						added = true
						break
					}
				}

				if !added {
					es[newExp] = []abi.SectorNumber{si.SectorNumber}
				}
			}
		}

		verifregAct, err := fullApi.StateGetActor(ctx, builtin.VerifiedRegistryActorAddr, types.EmptyTSK)
		if err != nil {
			return xerrors.Errorf("failed to lookup verifreg actor: %w", err)
		}

		verifregSt, err := verifreg.Load(adtStore, verifregAct)
		if err != nil {
			return xerrors.Errorf("failed to load verifreg state: %w", err)
		}

		claimsMap, err := verifregSt.GetClaims(maddr)
		if err != nil {
			return xerrors.Errorf("failed to lookup claims for miner: %w", err)
		}

		claimIdsBySector, err := verifregSt.GetClaimIdsBySector(maddr)
		if err != nil {
			return xerrors.Errorf("failed to lookup claim IDs by sector: %w", err)
		}

		sectorsMax, err := policy.GetAddressedSectorsMax(nv)
		if err != nil {
			return err
		}

		declMax, err := policy.GetDeclarationsMax(nv)
		if err != nil {
			return err
		}

		addrSectors := sectorsMax
		if cctx.Int("max-sectors") != 0 {
			addrSectors = cctx.Int("max-sectors")
			if addrSectors > sectorsMax {
				return xerrors.Errorf("the specified max-sectors exceeds the maximum limit")
			}
		}

		var params []miner.ExtendSectorExpiration2Params

		p := miner.ExtendSectorExpiration2Params{}
		scount := 0

		for l, exts := range extensions {
			for newExp, numbers := range exts {
				sectorsWithoutClaimsToExtend := bitfield.New()
				var sectorsWithClaims []miner.SectorClaim
				for _, sectorNumber := range numbers {
					claimIdsToMaintain := make([]verifreg.ClaimId, 0)
					claimIdsToDrop := make([]verifreg.ClaimId, 0)
					cannotExtendSector := false
					claimIds, ok := claimIdsBySector[sectorNumber]
					// Nothing to check, add to ccSectors
					if !ok {
						sectorsWithoutClaimsToExtend.Set(uint64(sectorNumber))
					} else {
						for _, claimId := range claimIds {
							claim, ok := claimsMap[claimId]
							if !ok {
								return xerrors.Errorf("failed to find claim for claimId %d", claimId)
							}
							claimExpiration := claim.TermStart + claim.TermMax
							// can be maintained in the extended sector
							if claimExpiration > newExp {
								claimIdsToMaintain = append(claimIdsToMaintain, claimId)
							} else {
								sectorInfo, ok := activeSectorsInfo[sectorNumber]
								if !ok {
									return xerrors.Errorf("failed to find sector in active sector set: %w", err)
								}
								if !cctx.Bool("drop-claims") ||
									// FIP-0045 requires the claim minimum duration to have passed
									currEpoch <= (claim.TermStart+claim.TermMin) ||
									// FIP-0045 requires the sector to be in its last 30 days of life
									(currEpoch <= sectorInfo.Expiration-builtin.EndOfLifeClaimDropPeriod) {
									fmt.Printf("skipping sector %d because claim %d does not live long enough \n", sectorNumber, claimId)
									cannotExtendSector = true
									break
								}

								claimIdsToDrop = append(claimIdsToDrop, claimId)
							}
						}
						if cannotExtendSector {
							continue
						}

						if len(claimIdsToMaintain)+len(claimIdsToDrop) != 0 {
							sectorsWithClaims = append(sectorsWithClaims, miner.SectorClaim{
								SectorNumber:   sectorNumber,
								MaintainClaims: claimIdsToMaintain,
								DropClaims:     claimIdsToDrop,
							})
						}
					}
				}

				sectorsWithoutClaimsCount, err := sectorsWithoutClaimsToExtend.Count()
				if err != nil {
					return xerrors.Errorf("failed to count cc sectors: %w", err)
				}

				sectorsInDecl := int(sectorsWithoutClaimsCount) + len(sectorsWithClaims)

				if scount+sectorsInDecl > addrSectors || len(p.Extensions) >= declMax {
					params = append(params, p)
					p = miner.ExtendSectorExpiration2Params{}
					scount = sectorsInDecl
				}

				p.Extensions = append(p.Extensions, miner.ExpirationExtension2{
					Deadline:          l.Deadline,
					Partition:         l.Partition,
					Sectors:           SectorNumsToBitfield(numbers),
					SectorsWithClaims: sectorsWithClaims,
					NewExpiration:     newExp,
				})

			}
		}

		// if we have any sectors, then one last append is needed here
		if scount != 0 {
			params = append(params, p)
		}

		if len(params) == 0 {
			fmt.Println("nothing to extend")
			return nil
		}

		mi, err := fullApi.StateMinerInfo(ctx, maddr, types.EmptyTSK)
		if err != nil {
			return xerrors.Errorf("getting miner info: %w", err)
		}

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
			fmt.Printf("Extending %d sectors: ", scount)
			stotal += scount

			if !cctx.Bool("really-do-it") {
				pp, err := NewPseudoExtendParams(&params[i])
				if err != nil {
					return err
				}

				data, err := json.MarshalIndent(pp, "", "  ")
				if err != nil {
					return err
				}

				fmt.Println("\n", string(data))
				continue
			}

			sp, aerr := actors.SerializeParams(&params[i])
			if aerr != nil {
				return xerrors.Errorf("serializing params: %w", err)
			}

			smsg, err := fullApi.MpoolPushMessage(ctx, &types.Message{
				From:   mi.Worker,
				To:     maddr,
				Method: builtin.MethodsMiner.ExtendSectorExpiration,
				Value:  big.Zero(),
				Params: sp,
			}, spec)
			if err != nil {
				return xerrors.Errorf("mpool push message: %w", err)
			}

			fmt.Println(smsg.Cid())
		}

		fmt.Printf("%d sectors extended\n", stotal)

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

		spt, err := lminer.PreferredSealProofTypeFromWindowPoStType(nv, mi.WindowPoStProofType)
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
		mas, err := lminer.Load(adt.WrapStore(ctx, cbor.NewCborStore(tbs)), mact)
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

		if err := mas.ForEachDeadline(func(dlIdx uint64, dl lminer.Deadline) error {
			return dl.ForEachPartition(func(partIdx uint64, part lminer.Partition) error {
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

			for _, number := range toRemove {
				fmt.Printf("Removing sector\t%s:\t", color.YellowString("%d", number))

				err := minerAPI.SectorRemove(ctx, number)
				if err != nil {
					color.Red("ERROR: %s\n", err.Error())
				} else {
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

// TODO simulate this call if --really-do-it is not used
var sectorsCompactPartitionsCmd = &cli.Command{
	Name:  "compact-partitions",
	Usage: "removes dead sectors from partitions and reduces the number of partitions used if possible",
	Flags: []cli.Flag{
		&cli.Uint64Flag{
			Name:     "deadline",
			Usage:    "the deadline to compact the partitions in",
			Required: true,
		},
		&cli.Int64SliceFlag{
			Name:     "partitions",
			Usage:    "list of partitions to compact sectors in",
			Required: true,
		},
		&cli.BoolFlag{
			Name:  "really-do-it",
			Usage: "Actually send transaction performing the action",
			Value: false,
		},
		&cli.StringFlag{
			Name:  "actor",
			Usage: "Specify the address of the miner to run this command",
		},
	},
	Action: func(cctx *cli.Context) error {
		if !cctx.Bool("really-do-it") {
			fmt.Println("Pass --really-do-it to actually execute this action")
			return nil
		}

		api, acloser, err := lcli.GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer acloser()

		ctx := lcli.ReqContext(cctx)

		maddr, err := getActorAddress(ctx, cctx)
		if err != nil {
			return err
		}

		minfo, err := api.StateMinerInfo(ctx, maddr, types.EmptyTSK)
		if err != nil {
			return err
		}

		deadline := cctx.Uint64("deadline")
		if deadline > miner.WPoStPeriodDeadlines {
			return fmt.Errorf("deadline %d out of range", deadline)
		}

		parts := cctx.Int64Slice("partitions")
		if len(parts) <= 0 {
			return fmt.Errorf("must include at least one partition to compact")
		}
		fmt.Printf("compacting %d paritions\n", len(parts))

		partitions := bitfield.New()
		for _, partition := range parts {
			partitions.Set(uint64(partition))
		}

		params := miner.CompactPartitionsParams{
			Deadline:   deadline,
			Partitions: partitions,
		}

		sp, err := actors.SerializeParams(&params)
		if err != nil {
			return xerrors.Errorf("serializing params: %w", err)
		}

		smsg, err := api.MpoolPushMessage(ctx, &types.Message{
			From:   minfo.Worker,
			To:     maddr,
			Method: builtin.MethodsMiner.CompactPartitions,
			Value:  big.Zero(),
			Params: sp,
		}, nil)
		if err != nil {
			return xerrors.Errorf("mpool push: %w", err)
		}

		fmt.Printf("Requested compact partitions in message %s\n", smsg.Cid())

		wait, err := api.StateWaitMsg(ctx, smsg.Cid(), 0)
		if err != nil {
			return err
		}

		// check it executed successfully
		if wait.Receipt.ExitCode.IsError() {
			fmt.Println(cctx.App.Writer, "compact partitions failed!")
			return err
		}

		return nil
	},
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
