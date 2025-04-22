package spcli

import (
	"bufio"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/fatih/color"
	cbor "github.com/ipfs/go-ipld-cbor"
	"github.com/samber/lo"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-bitfield"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/builtin"
	miner2 "github.com/filecoin-project/specs-actors/v2/actors/builtin/miner"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/adt"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/actors/builtin/verifreg"
	"github.com/filecoin-project/lotus/chain/actors/policy"
	"github.com/filecoin-project/lotus/chain/types"
	lcli "github.com/filecoin-project/lotus/cli"
	cliutil "github.com/filecoin-project/lotus/cli/util"
	"github.com/filecoin-project/lotus/lib/tablewriter"
)

type OnDiskInfoGetter func(cctx *cli.Context, id abi.SectorNumber, onChainInfo bool) (api.SectorInfo, error)

func SectorsStatusCmd(getActorAddress ActorAddressGetter, getOnDiskInfo OnDiskInfoGetter) *cli.Command {
	return &cli.Command{
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
			ctx := lcli.ReqContext(cctx)

			if cctx.NArg() != 1 {
				return lcli.IncorrectNumArgs(cctx)
			}

			id, err := strconv.ParseUint(cctx.Args().First(), 10, 64)
			if err != nil {
				return err
			}

			onChainInfo := cctx.Bool("on-chain-info")

			var status api.SectorInfo
			if getOnDiskInfo != nil {
				status, err = getOnDiskInfo(cctx, abi.SectorNumber(id), onChainInfo)
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

				fmt.Printf("\nExpiration Info\n")
				fmt.Printf("OnTime:\t\t%v\n", status.OnTime)
				fmt.Printf("Early:\t\t%v\n", status.Early)

				var pamsHeaderOnce sync.Once

				for pi, piece := range status.Pieces {
					if piece.DealInfo == nil {
						continue
					}
					if piece.DealInfo.PieceActivationManifest == nil {
						continue
					}
					pamsHeaderOnce.Do(func() {
						fmt.Printf("\nPiece Activation Manifests\n")
					})

					pam := piece.DealInfo.PieceActivationManifest

					fmt.Printf("Piece %d: %s %s verif-alloc:%+v\n", pi, pam.CID, types.SizeStr(types.NewInt(uint64(pam.Size))), pam.VerifiedAllocationKey)
					for ni, notification := range pam.Notify {
						fmt.Printf("\tNotify %d: %s (%x)\n", ni, notification.Address, notification.Payload)
					}
				}

			} else {
				onChainInfo = true
			}

			maddr, err := getActorAddress(cctx)
			if err != nil {
				return err
			}

			if onChainInfo {
				fullApi, closer, err := lcli.GetFullNodeAPI(cctx)
				if err != nil {
					return err
				}
				defer closer()

				head, err := fullApi.ChainHead(ctx)
				if err != nil {
					return xerrors.Errorf("getting chain head: %w", err)
				}

				status, err := fullApi.StateSectorGetInfo(ctx, maddr, abi.SectorNumber(id), head.Key())
				if err != nil {
					return err
				}

				if status == nil {
					fmt.Println("Sector status not found on chain")
					return nil
				}

				mid, err := address.IDFromAddress(maddr)
				if err != nil {
					return err
				}
				fmt.Printf("\nSector On Chain Info\n")
				fmt.Printf("SealProof:\t\t%x\n", status.SealProof)
				fmt.Printf("Activation:\t\t%v\n", cliutil.EpochTime(head.Height(), status.Activation))
				fmt.Printf("Expiration:\t\t%s\n", cliutil.EpochTime(head.Height(), status.Expiration))
				fmt.Printf("DealWeight:\t\t%v\n", status.DealWeight)
				fmt.Printf("VerifiedDealWeight:\t\t%v\n", status.VerifiedDealWeight)
				fmt.Printf("InitialPledge:\t\t%v\n", types.FIL(status.InitialPledge))
				fmt.Printf("SectorID:\t\t{Miner: %v, Number: %v}\n", abi.ActorID(mid), status.SectorNumber)
			}

			if cctx.Bool("partition-info") {
				fullApi, nCloser, err := lcli.GetFullNodeAPI(cctx)
				if err != nil {
					return err
				}
				defer nCloser()

				maddr, err := getActorAddress(cctx)
				if err != nil {
					return err
				}

				mact, err := fullApi.StateGetActor(ctx, maddr, types.EmptyTSK)
				if err != nil {
					return err
				}

				tbs := blockstore.NewTieredBstore(blockstore.NewAPIBlockstore(fullApi), blockstore.NewMemory())
				mas, err := miner.Load(adt.WrapStore(ctx, cbor.NewCborStore(tbs)), mact)
				if err != nil {
					return err
				}

				errFound := errors.New("found")
				if err := mas.ForEachDeadline(func(dlIdx uint64, dl miner.Deadline) error {
					return dl.ForEachPartition(func(partIdx uint64, part miner.Partition) error {
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
				if getOnDiskInfo != nil {
					fmt.Printf("--------\nEvent Log:\n")

					for i, l := range status.Log {
						fmt.Printf("%d.\t%s:\t[%s]\t%s\n", i, time.Unix(int64(l.Timestamp), 0), l.Kind, l.Message)
						if l.Trace != "" {
							fmt.Printf("\t%s\n", l.Trace)
						}
					}
				}
			}
			return nil
		},
	}
}

func SectorsListUpgradeBoundsCmd(getActorAddress ActorAddressGetter) *cli.Command {
	return &cli.Command{
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
			fullApi, closer2, err := lcli.GetFullNodeAPI(cctx)
			if err != nil {
				return err
			}
			defer closer2()

			ctx := lcli.ReqContext(cctx)

			head, err := fullApi.ChainHead(ctx)
			if err != nil {
				return xerrors.Errorf("getting chain head: %w", err)
			}

			maddr, err := getActorAddress(cctx)
			if err != nil {
				return err
			}

			list, err := fullApi.StateMinerActiveSectors(ctx, maddr, head.Key())
			if err != nil {
				return err
			}
			filter := bitfield.New()

			for _, s := range list {
				filter.Set(uint64(s.SectorNumber))
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
}

func SectorPreCommitsCmd(getActorAddress ActorAddressGetter) *cli.Command {
	return &cli.Command{
		Name:  "precommits",
		Usage: "Print on-chain precommit info",
		Action: func(cctx *cli.Context) error {
			ctx := lcli.ReqContext(cctx)
			mapi, closer, err := lcli.GetFullNodeAPI(cctx)
			if err != nil {
				return err
			}
			defer closer()
			maddr, err := getActorAddress(cctx)
			if err != nil {
				return err
			}
			mact, err := mapi.StateGetActor(ctx, maddr, types.EmptyTSK)
			if err != nil {
				return err
			}
			store := adt.WrapStore(ctx, cbor.NewCborStore(blockstore.NewAPIBlockstore(mapi)))
			mst, err := miner.Load(store, mact)
			if err != nil {
				return err
			}
			preCommitSector := make([]miner.SectorPreCommitOnChainInfo, 0)
			err = mst.ForEachPrecommittedSector(func(info miner.SectorPreCommitOnChainInfo) error {
				preCommitSector = append(preCommitSector, info)
				return err
			})
			less := func(i, j int) bool {
				return preCommitSector[i].Info.SectorNumber <= preCommitSector[j].Info.SectorNumber
			}
			sort.Slice(preCommitSector, less)
			for _, info := range preCommitSector {
				fmt.Printf("%s: %s\n", info.Info.SectorNumber, info.PreCommitEpoch)
			}

			return nil
		},
	}
}

func SectorsCheckExpireCmd(getActorAddress ActorAddressGetter) *cli.Command {
	return &cli.Command{
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

			maddr, err := getActorAddress(cctx)
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
				maxExtension, err := policy.GetMaxSectorExpirationExtension(nv)
				if err != nil {
					return xerrors.Errorf("failed to get max extension: %w", err)
				}

				MaxExtendNow := currEpoch + maxExtension

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
}

func SectorsExtendCmd(getActorAddress ActorAddressGetter) *cli.Command {
	return &cli.Command{
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
				Name:   "only-cc",
				Hidden: true,
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
			if cctx.IsSet("only-cc") {
				return xerrors.Errorf("only-cc flag has been removed, use --exclude flag instead")
			}

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

			maddr, err := getActorAddress(cctx)
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
			mas, err := miner.Load(adtStore, mact)
			if err != nil {
				return err
			}

			activeSectorsLocation := make(map[abi.SectorNumber]*miner.SectorLocation, len(activeSet))

			if err := mas.ForEachDeadline(func(dlIdx uint64, dl miner.Deadline) error {
				return dl.ForEachPartition(func(partIdx uint64, part miner.Partition) error {
					pas, err := part.ActiveSectors()
					if err != nil {
						return err
					}

					return pas.ForEach(func(i uint64) error {
						activeSectorsLocation[abi.SectorNumber(i)] = &miner.SectorLocation{
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

				sis = append(sis, si)
			}

			withinTolerance := func(a, b abi.ChainEpoch) bool {
				diff := a - b
				if diff < 0 {
					diff = -diff
				}

				return diff <= abi.ChainEpoch(cctx.Int64("tolerance"))
			}

			extensions := map[miner.SectorLocation]map[abi.ChainEpoch][]abi.SectorNumber{}
			for _, si := range sis {
				extension := abi.ChainEpoch(cctx.Int64("extension"))
				newExp := si.Expiration + extension

				if cctx.IsSet("new-expiration") {
					newExp = abi.ChainEpoch(cctx.Int64("new-expiration"))
				}

				maxExtension, err := policy.GetMaxSectorExpirationExtension(nv)
				if err != nil {
					return xerrors.Errorf("failed to get max extension: %w", err)
				}

				maxExtendNow := currEpoch + maxExtension
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
					numbersToExtend := make([]abi.SectorNumber, 0, len(numbers))
					var sectorsWithClaims []miner.SectorClaim
					for _, sectorNumber := range numbers {
						claimIdsToMaintain := make([]verifreg.ClaimId, 0)
						claimIdsToDrop := make([]verifreg.ClaimId, 0)
						cannotExtendSector := false
						claimIds, ok := claimIdsBySector[sectorNumber]
						// Nothing to check, add to ccSectors
						if !ok {
							sectorsWithoutClaimsToExtend.Set(uint64(sectorNumber))
							numbersToExtend = append(numbersToExtend, sectorNumber)
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
									if !cctx.Bool("drop-claims") {
										fmt.Printf("skipping sector %d because claim %d (client f0%s, piece %s) cannot be maintained in the extended sector (use --drop-claims to drop claims)\n", sectorNumber, claimId, claim.Client, claim.Data)
										cannotExtendSector = true
										break
									} else if currEpoch <= (claim.TermStart + claim.TermMin) {
										// FIP-0045 requires the claim minimum duration to have passed
										fmt.Printf("skipping sector %d because claim %d (client f0%s, piece %s) has not reached its minimum duration\n", sectorNumber, claimId, claim.Client, claim.Data)
										cannotExtendSector = true
										break
									} else if currEpoch <= sectorInfo.Expiration-builtin.EndOfLifeClaimDropPeriod {
										// FIP-0045 requires the sector to be in its last 30 days of life
										fmt.Printf("skipping sector %d because claim %d (client f0%s, piece %s) is not in its last 30 days of life\n", sectorNumber, claimId, claim.Client, claim.Data)
										cannotExtendSector = true
										break
									}

									claimIdsToDrop = append(claimIdsToDrop, claimId)
								}

								numbersToExtend = append(numbersToExtend, sectorNumber)
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
					scount += sectorsInDecl

					if scount > addrSectors || len(p.Extensions) >= policy.DeclarationsMax {
						params = append(params, p)
						p = miner.ExtendSectorExpiration2Params{}
						scount = sectorsInDecl
					}

					p.Extensions = append(p.Extensions, miner.ExpirationExtension2{
						Deadline:          l.Deadline,
						Partition:         l.Partition,
						Sectors:           SectorNumsToBitfield(numbersToExtend),
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

				sp, aerr := actors.SerializeParams(&params[i])
				if aerr != nil {
					return xerrors.Errorf("serializing params: %w", err)
				}

				m := &types.Message{
					From:   mi.Worker,
					To:     maddr,
					Method: builtin.MethodsMiner.ExtendSectorExpiration2,
					Value:  big.Zero(),
					Params: sp,
				}

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

					_, err = fullApi.GasEstimateMessageGas(ctx, m, spec, types.EmptyTSK)
					if err != nil {
						return xerrors.Errorf("simulating message execution: %w", err)
					}

					continue
				}

				smsg, err := fullApi.MpoolPushMessage(ctx, m, spec)
				if err != nil {
					return xerrors.Errorf("mpool push message: %w", err)
				}

				fmt.Println(smsg.Cid())
			}

			fmt.Printf("%d sectors extended\n", stotal)

			return nil
		},
	}
}

func SectorNumsToBitfield(sectors []abi.SectorNumber) bitfield.BitField {
	var numbers []uint64
	for _, sector := range sectors {
		numbers = append(numbers, uint64(sector))
	}

	return bitfield.NewFromSet(numbers)
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

type PseudoExtendSectorExpirationParams struct {
	Extensions []PseudoExpirationExtension
}

type PseudoExpirationExtension struct {
	Deadline      uint64
	Partition     uint64
	Sectors       string
	NewExpiration abi.ChainEpoch
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

func SectorsCompactPartitionsCmd(getActorAddress ActorAddressGetter) *cli.Command {
	return &cli.Command{
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
		},
		Action: func(cctx *cli.Context) error {
			fullNodeAPI, acloser, err := lcli.GetFullNodeAPI(cctx)
			if err != nil {
				return err
			}
			defer acloser()

			ctx := lcli.ReqContext(cctx)

			maddr, err := getActorAddress(cctx)
			if err != nil {
				return err
			}

			minfo, err := fullNodeAPI.StateMinerInfo(ctx, maddr, types.EmptyTSK)
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
			fmt.Printf("compacting %d partitions\n", len(parts))

			var makeMsgForPartitions func(partitionsBf bitfield.BitField) ([]*types.Message, error)
			makeMsgForPartitions = func(partitionsBf bitfield.BitField) ([]*types.Message, error) {
				params := miner.CompactPartitionsParams{
					Deadline:   deadline,
					Partitions: partitionsBf,
				}

				sp, aerr := actors.SerializeParams(&params)
				if aerr != nil {
					return nil, xerrors.Errorf("serializing params: %w", err)
				}

				msg := &types.Message{
					From:   minfo.Worker,
					To:     maddr,
					Method: builtin.MethodsMiner.CompactPartitions,
					Value:  big.Zero(),
					Params: sp,
				}

				estimatedMsg, err := fullNodeAPI.GasEstimateMessageGas(ctx, msg, nil, types.EmptyTSK)
				if err != nil && errors.Is(err, &api.ErrOutOfGas{}) {
					// the message is too big -- split into 2
					partitionsSlice, err := partitionsBf.All(math.MaxUint64)
					if err != nil {
						return nil, err
					}

					partitions1 := bitfield.New()
					for i := 0; i < len(partitionsSlice)/2; i++ {
						partitions1.Set(uint64(i))
					}

					msgs1, err := makeMsgForPartitions(partitions1)
					if err != nil {
						return nil, err
					}

					// time for the second half
					partitions2 := bitfield.New()
					for i := len(partitionsSlice) / 2; i < len(partitionsSlice); i++ {
						partitions2.Set(uint64(i))
					}

					msgs2, err := makeMsgForPartitions(partitions2)
					if err != nil {
						return nil, err
					}

					return append(msgs1, msgs2...), nil
				} else if err != nil {
					return nil, err
				}

				return []*types.Message{estimatedMsg}, nil
			}

			partitions := bitfield.New()
			for _, partition := range parts {
				partitions.Set(uint64(partition))
			}

			msgs, err := makeMsgForPartitions(partitions)
			if err != nil {
				return xerrors.Errorf("failed to make messages: %w", err)
			}

			// Actually send the messages if really-do-it provided, simulate otherwise
			if cctx.Bool("really-do-it") {
				smsgs, err := fullNodeAPI.MpoolBatchPushMessage(ctx, msgs, nil)
				if err != nil {
					return xerrors.Errorf("mpool push: %w", err)
				}

				if len(smsgs) == 1 {
					fmt.Printf("Requested compact partitions in message %s\n", smsgs[0].Cid())
				} else {
					fmt.Printf("Requested compact partitions in %d messages\n\n", len(smsgs))
					for _, v := range smsgs {
						fmt.Println(v.Cid())
					}
				}

				for _, v := range smsgs {
					wait, err := fullNodeAPI.StateWaitMsg(ctx, v.Cid(), 2)
					if err != nil {
						return err
					}

					// check it executed successfully
					if wait.Receipt.ExitCode.IsError() {
						fmt.Println(cctx.App.Writer, "compact partitions msg %s failed!", v.Cid())
						return err
					}
				}

				return nil
			}

			for i, v := range msgs {
				fmt.Printf("total of %d CompactPartitions msgs would be sent\n", len(msgs))

				estMsg, err := fullNodeAPI.GasEstimateMessageGas(ctx, v, nil, types.EmptyTSK)
				if err != nil {
					return err
				}

				fmt.Printf("msg %d would cost up to %s\n", i+1, types.FIL(estMsg.RequiredFunds()))
			}

			return nil

		},
	}
}

func TerminateSectorCmd(getActorAddress ActorAddressGetter) *cli.Command {
	return &cli.Command{
		Name:      "terminate",
		Usage:     "Forcefully terminate a sector (WARNING: This means losing power and pay a one-time termination penalty(including collateral) for the terminated sector)",
		ArgsUsage: "[sectorNum1 sectorNum2 ...]",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "actor",
				Usage: "specify the address of miner actor",
			},
			&cli.BoolFlag{
				Name:  "really-do-it",
				Usage: "pass this flag if you know what you are doing",
			},
			&cli.StringFlag{
				Name:  "from",
				Usage: "specify the address to send the terminate message from",
			},
		},
		Action: func(cctx *cli.Context) error {
			if cctx.NArg() < 1 {
				return lcli.ShowHelp(cctx, fmt.Errorf("at least one sector must be specified"))
			}

			var maddr address.Address
			if act := cctx.String("actor"); act != "" {
				var err error
				maddr, err = address.NewFromString(act)
				if err != nil {
					return fmt.Errorf("parsing address %s: %w", act, err)
				}
			}

			if !cctx.Bool("really-do-it") {
				return fmt.Errorf("this is a command for advanced users, only use it if you are sure of what you are doing")
			}

			nodeApi, closer, err := lcli.GetFullNodeAPI(cctx)
			if err != nil {
				return err
			}
			defer closer()

			ctx := lcli.ReqContext(cctx)

			if maddr.Empty() {
				maddr, err = getActorAddress(cctx)
				if err != nil {
					return err
				}
			}

			var outerErr error
			sectorNumbers := lo.Map(cctx.Args().Slice(), func(sn string, _ int) int {
				sectorNum, err := strconv.Atoi(sn)
				if err != nil {
					outerErr = fmt.Errorf("could not parse sector number: %w", err)
					return 0
				}
				return sectorNum
			})
			if outerErr != nil {
				return outerErr
			}

			confidence := uint64(cctx.Int("confidence"))

			var fromAddr address.Address
			if from := cctx.String("from"); from != "" {
				var err error
				fromAddr, err = address.NewFromString(from)
				if err != nil {
					return fmt.Errorf("parsing address %s: %w", from, err)
				}
			} else {
				mi, err := nodeApi.StateMinerInfo(ctx, maddr, types.EmptyTSK)
				if err != nil {
					return err
				}

				fromAddr = mi.Worker
			}
			smsg, err := TerminateSectors(ctx, nodeApi, maddr, sectorNumbers, fromAddr)
			if err != nil {
				return err
			}

			wait, err := nodeApi.StateWaitMsg(ctx, smsg.Cid(), confidence)
			if err != nil {
				return err
			}

			if wait.Receipt.ExitCode.IsError() {
				return fmt.Errorf("terminate sectors message returned exit %d", wait.Receipt.ExitCode)
			}
			return nil
		},
	}
}

type TerminatorNode interface {
	StateSectorPartition(ctx context.Context, maddr address.Address, sectorNumber abi.SectorNumber, tok types.TipSetKey) (*miner.SectorLocation, error)
	MpoolPushMessage(ctx context.Context, msg *types.Message, spec *api.MessageSendSpec) (*types.SignedMessage, error)
}

func TerminateSectors(ctx context.Context, full TerminatorNode, maddr address.Address, sectorNumbers []int, fromAddr address.Address) (*types.SignedMessage, error) {

	terminationDeclarationParams := []miner2.TerminationDeclaration{}

	for _, sectorNum := range sectorNumbers {

		sectorbit := bitfield.New()
		sectorbit.Set(uint64(sectorNum))

		loca, err := full.StateSectorPartition(ctx, maddr, abi.SectorNumber(sectorNum), types.EmptyTSK)
		if err != nil {
			return nil, fmt.Errorf("get state sector partition %s", err)
		}

		para := miner2.TerminationDeclaration{
			Deadline:  loca.Deadline,
			Partition: loca.Partition,
			Sectors:   sectorbit,
		}

		terminationDeclarationParams = append(terminationDeclarationParams, para)
	}

	terminateSectorParams := &miner2.TerminateSectorsParams{
		Terminations: terminationDeclarationParams,
	}

	sp, errA := actors.SerializeParams(terminateSectorParams)
	if errA != nil {
		return nil, xerrors.Errorf("serializing params: %w", errA)
	}

	smsg, err := full.MpoolPushMessage(ctx, &types.Message{
		From:   fromAddr,
		To:     maddr,
		Method: builtin.MethodsMiner.TerminateSectors,

		Value:  big.Zero(),
		Params: sp,
	}, nil)
	if err != nil {
		return nil, xerrors.Errorf("mpool push message: %w", err)
	}

	fmt.Println("sent termination message:", smsg.Cid())

	return smsg, nil
}
