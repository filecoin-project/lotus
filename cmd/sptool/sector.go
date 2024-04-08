package main

import (
	"fmt"
	"os"
	"sort"

	"github.com/docker/go-units"
	"github.com/fatih/color"
	cbor "github.com/ipfs/go-ipld-cbor"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-bitfield"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"

	"github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/chain/actors/adt"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/actors/policy"
	"github.com/filecoin-project/lotus/chain/types"
	lcli "github.com/filecoin-project/lotus/cli"
	"github.com/filecoin-project/lotus/cli/spcli"
	cliutil "github.com/filecoin-project/lotus/cli/util"
	"github.com/filecoin-project/lotus/lib/tablewriter"
)

var sectorsCmd = &cli.Command{
	Name:  "sectors",
	Usage: "interact with sector store",
	Subcommands: []*cli.Command{
		spcli.SectorsStatusCmd(SPTActorGetter, nil),
		sectorsListCmd, // in-house b/c chain-only is so different. Needs Curio *web* implementation
		spcli.SectorPreCommitsCmd(SPTActorGetter),
		spcli.SectorsCheckExpireCmd(SPTActorGetter),
		sectorsExpiredCmd, // in-house b/c chain-only is so different
		spcli.SectorsExtendCmd(SPTActorGetter),
		spcli.TerminateSectorCmd(SPTActorGetter),
		spcli.SectorsCompactPartitionsCmd(SPTActorGetter),
	}}

var sectorsExpiredCmd = &cli.Command{
	Name:  "expired",
	Usage: "Get or cleanup expired sectors",
	Flags: []cli.Flag{
		&cli.Int64Flag{
			Name:        "expired-epoch",
			Usage:       "epoch at which to check sector expirations",
			DefaultText: "WinningPoSt lookback epoch",
		},
	},
	Action: func(cctx *cli.Context) error {
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

		maddr, err := SPTActorGetter(cctx)
		if err != nil {
			return xerrors.Errorf("getting actor address: %w", err)
		}

		// toCheck is a working bitfield which will only contain terminated sectors
		toCheck := bitfield.New()
		{
			sectors, err := fullApi.StateMinerSectors(ctx, maddr, nil, lbts.Key())
			if err != nil {
				return xerrors.Errorf("getting sector on chain info: %w", err)
			}

			for _, sector := range sectors {
				if sector.Expiration <= lbts.Height() {
					toCheck.Set(uint64(sector.SectorNumber))
				}
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

		// toCheck now only contains sectors which either failed to precommit or are expired/terminated
		fmt.Printf("Sectors that either failed to precommit or are expired/terminated:\n")

		err = toCheck.ForEach(func(u uint64) error {
			fmt.Println(abi.SectorNumber(u))

			return nil
		})
		if err != nil {
			return err
		}

		return nil
	},
}

var sectorsListCmd = &cli.Command{
	Name:  "list",
	Usage: "List sectors",
	Flags: []cli.Flag{
		/*
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
		*/
	},
	Subcommands: []*cli.Command{
		//sectorsListUpgradeBoundsCmd,
	},
	Action: func(cctx *cli.Context) error {
		fullApi, closer2, err := lcli.GetFullNodeAPI(cctx) // TODO: consider storing full node address in config
		if err != nil {
			return err
		}
		defer closer2()

		ctx := lcli.ReqContext(cctx)

		maddr, err := SPTActorGetter(cctx)
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

		sort.Slice(sset, func(i, j int) bool {
			return sset[i].SectorNumber < sset[j].SectorNumber
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

		for _, st := range sset {
			s := st.SectorNumber
			_, inSSet := commitedIDs[s]
			_, inASet := activeIDs[s]

			const verifiedPowerGainMul = 9
			dw, vp := .0, .0
			{
				rdw := big.Add(st.DealWeight, st.VerifiedDealWeight)
				dw = float64(big.Div(rdw, big.NewInt(int64(st.Expiration-st.Activation))).Uint64())
				vp = float64(big.Div(big.Mul(st.VerifiedDealWeight, big.NewInt(verifiedPowerGainMul)), big.NewInt(int64(st.Expiration-st.Activation))).Uint64())
			}

			var deals int
			for _, deal := range st.DealIDs {
				if deal != 0 {
					deals++
				}
			}

			exp := st.Expiration
			// if st.OnTime > 0 && st.OnTime < exp {
			// 	exp = st.OnTime // Can be different when the sector was CC upgraded
			// }

			m := map[string]interface{}{
				"ID": s,
				//"State":   color.New(spcli.StateOrder[sealing.SectorState(st.State)].Col).Sprint(st.State),
				"OnChain": yesno(inSSet),
				"Active":  yesno(inASet),
			}

			if deals > 0 {
				m["Deals"] = color.GreenString("%d", deals)
			} else {
				m["Deals"] = color.BlueString("CC")
				// if st.ToUpgrade {
				// 	m["Deals"] = color.CyanString("CC(upgrade)")
				// }
			}

			if !fast {
				if !inSSet {
					m["Expiration"] = "n/a"
				} else {
					m["Expiration"] = cliutil.EpochTime(head.Height(), exp)
					// if st.Early > 0 {
					// 	m["RecoveryTimeout"] = color.YellowString(cliutil.EpochTime(head.Height(), st.Early))
					// }
				}
				if inSSet && cctx.Bool("initial-pledge") {
					m["Pledge"] = types.FIL(st.InitialPledge).Short()
				}
			}

			if !fast && deals > 0 {
				m["DealWeight"] = units.BytesSize(dw)
				if vp > 0 {
					m["VerifiedPower"] = color.GreenString(units.BytesSize(vp))
				}
			}

			tw.Write(m)
		}

		return tw.Flush(os.Stdout)
	},
}

func yesno(b bool) string {
	if b {
		return color.GreenString("YES")
	}
	return color.RedString("NO")
}
