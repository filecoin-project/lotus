package cli

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"os"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"
	"github.com/urfave/cli/v2"
	cbg "github.com/whyrusleeping/cbor-gen"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	actorstypes "github.com/filecoin-project/go-state-types/actors"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/exitcode"
	"github.com/filecoin-project/go-state-types/network"

	"github.com/filecoin-project/lotus/api"
	lapi "github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/api/v0api"
	"github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/adt"
	"github.com/filecoin-project/lotus/chain/actors/builtin"
	"github.com/filecoin-project/lotus/chain/actors/builtin/market"
	"github.com/filecoin-project/lotus/chain/consensus"
	"github.com/filecoin-project/lotus/chain/state"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	cliutil "github.com/filecoin-project/lotus/cli/util"
)

var StateMinerProvingDeadlineCmd = &cli.Command{
	Name:      "miner-proving-deadline",
	Usage:     "Retrieve information about a given miner's proving deadline",
	ArgsUsage: "[minerAddress]",
	Action: func(cctx *cli.Context) error {
		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		ctx := ReqContext(cctx)

		if cctx.NArg() != 1 {
			return IncorrectNumArgs(cctx)
		}

		addr, err := address.NewFromString(cctx.Args().First())
		if err != nil {
			return err
		}

		ts, err := LoadTipSet(ctx, cctx, api)
		if err != nil {
			return err
		}

		cd, err := api.StateMinerProvingDeadline(ctx, addr, ts.Key())
		if err != nil {
			return xerrors.Errorf("getting miner info: %w", err)
		}

		fmt.Printf("Period Start:\t%s\n", cd.PeriodStart)
		fmt.Printf("Index:\t\t%d\n", cd.Index)
		fmt.Printf("Open:\t\t%s\n", cd.Open)
		fmt.Printf("Close:\t\t%s\n", cd.Close)
		fmt.Printf("Challenge:\t%s\n", cd.Challenge)
		fmt.Printf("FaultCutoff:\t%s\n", cd.FaultCutoff)

		return nil
	},
}

func ParseTipSetString(ts string) ([]cid.Cid, error) {
	strs := strings.Split(ts, ",")

	var cids []cid.Cid
	for _, s := range strs {
		c, err := cid.Parse(strings.TrimSpace(s))
		if err != nil {
			return nil, err
		}
		cids = append(cids, c)
	}

	return cids, nil
}

type TipSetResolver interface {
	ChainHead(context.Context) (*types.TipSet, error)
	ChainGetTipSetByHeight(context.Context, abi.ChainEpoch, types.TipSetKey) (*types.TipSet, error)
	ChainGetTipSet(context.Context, types.TipSetKey) (*types.TipSet, error)
}

// LoadTipSet gets the tipset from the context, or the head from the API.
//
// It always gets the head from the API so commands use a consistent tipset even if time passes.
func LoadTipSet(ctx context.Context, cctx *cli.Context, api TipSetResolver) (*types.TipSet, error) {
	tss := cctx.String("tipset")
	if tss == "" {
		return api.ChainHead(ctx)
	}

	return ParseTipSetRef(ctx, api, tss)
}

func ParseTipSetRef(ctx context.Context, api TipSetResolver, tss string) (*types.TipSet, error) {
	if tss[0] == '@' {
		if tss == "@head" {
			return api.ChainHead(ctx)
		}

		var h uint64
		if _, err := fmt.Sscanf(tss, "@%d", &h); err != nil {
			return nil, xerrors.Errorf("parsing height tipset ref: %w", err)
		}

		return api.ChainGetTipSetByHeight(ctx, abi.ChainEpoch(h), types.EmptyTSK)
	}

	cids, err := ParseTipSetString(tss)
	if err != nil {
		return nil, err
	}

	if len(cids) == 0 {
		return nil, nil
	}

	k := types.NewTipSetKey(cids...)
	ts, err := api.ChainGetTipSet(ctx, k)
	if err != nil {
		return nil, err
	}

	return ts, nil
}

func ParseTipSetRefOffline(ctx context.Context, cs *store.ChainStore, tss string) (*types.TipSet, error) {
	switch {

	case tss == "" || tss == "@head":
		return cs.GetHeaviestTipSet(), nil

	case tss[0] != '@':
		cids, err := ParseTipSetString(tss)
		if err != nil {
			return nil, xerrors.Errorf("failed to parse tipset (%q): %w", tss, err)
		}
		return cs.LoadTipSet(ctx, types.NewTipSetKey(cids...))

	default:
		var h uint64
		if _, err := fmt.Sscanf(tss, "@%d", &h); err != nil {
			return nil, xerrors.Errorf("parsing height tipset ref: %w", err)
		}
		return cs.GetTipsetByHeight(ctx, abi.ChainEpoch(h), cs.GetHeaviestTipSet(), true)
	}
}

var StatePowerCmd = &cli.Command{
	Name:      "power",
	Usage:     "Query network or miner power",
	ArgsUsage: "[<minerAddress> (optional)]",
	Action: func(cctx *cli.Context) error {
		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		ctx := ReqContext(cctx)

		ts, err := LoadTipSet(ctx, cctx, api)
		if err != nil {
			return err
		}

		var maddr address.Address
		if cctx.Args().Present() {
			maddr, err = address.NewFromString(cctx.Args().First())
			if err != nil {
				return err
			}

			ma, err := api.StateGetActor(ctx, maddr, ts.Key())
			if err != nil {
				return err
			}

			if !builtin.IsStorageMinerActor(ma.Code) {
				return xerrors.New("provided address does not correspond to a miner actor")
			}
		}

		power, err := api.StateMinerPower(ctx, maddr, ts.Key())
		if err != nil {
			return err
		}

		tp := power.TotalPower
		if cctx.Args().Present() {
			mp := power.MinerPower
			fmt.Printf(
				"%s(%s) / %s(%s) ~= %0.4f%%\n",
				mp.QualityAdjPower.String(), types.SizeStr(mp.QualityAdjPower),
				tp.QualityAdjPower.String(), types.SizeStr(tp.QualityAdjPower),
				types.BigDivFloat(
					types.BigMul(mp.QualityAdjPower, big.NewInt(100)),
					tp.QualityAdjPower,
				),
			)
		} else {
			fmt.Printf("%s(%s)\n", tp.QualityAdjPower.String(), types.SizeStr(tp.QualityAdjPower))
		}

		return nil
	},
}

var StateSectorsCmd = &cli.Command{
	Name:      "sectors",
	Usage:     "Query the sector set of a miner",
	ArgsUsage: "[minerAddress]",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "show-partitions",
			Usage: "show sector deadlines and partitions",
		},
	},
	Action: func(cctx *cli.Context) error {
		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		ctx := ReqContext(cctx)

		if cctx.NArg() != 1 {
			return IncorrectNumArgs(cctx)
		}

		maddr, err := address.NewFromString(cctx.Args().First())
		if err != nil {
			return err
		}

		ts, err := LoadTipSet(ctx, cctx, api)
		if err != nil {
			return err
		}

		sectors, err := api.StateMinerSectors(ctx, maddr, nil, ts.Key())
		if err != nil {
			return err
		}

		showPartitions := cctx.Bool("show-partitions")
		header := "Sector Number, Sealed CID"
		if showPartitions {
			header = "Sector Number, Deadline, Partition, Sealed CID"
		}
		fmt.Println(header)

		for _, s := range sectors {
			if showPartitions {
				sp, err := api.StateSectorPartition(ctx, maddr, s.SectorNumber, ts.Key())
				if err != nil {
					return err
				}
				fmt.Printf("%d, %d, %d, %s\n", s.SectorNumber, sp.Deadline, sp.Partition, s.SealedCID)
			} else {
				fmt.Printf("%d, %s\n", s.SectorNumber, s.SealedCID)
			}
		}

		return nil
	},
}

var StateActiveSectorsCmd = &cli.Command{
	Name:      "active-sectors",
	Usage:     "Query the active sector set of a miner",
	ArgsUsage: "[minerAddress]",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "show-partitions",
			Usage: "show sector deadlines and partitions",
		},
	},
	Action: func(cctx *cli.Context) error {
		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		ctx := ReqContext(cctx)

		if cctx.NArg() != 1 {
			return IncorrectNumArgs(cctx)
		}

		maddr, err := address.NewFromString(cctx.Args().First())
		if err != nil {
			return err
		}

		ts, err := LoadTipSet(ctx, cctx, api)
		if err != nil {
			return err
		}

		sectors, err := api.StateMinerActiveSectors(ctx, maddr, ts.Key())
		if err != nil {
			return err
		}

		showPartitions := cctx.Bool("show-partitions")
		header := "Sector Number, Sealed CID"
		if showPartitions {
			header = "Sector Number, Deadline, Partition, Sealed CID"
		}
		fmt.Println(header)

		for _, s := range sectors {
			if showPartitions {
				sp, err := api.StateSectorPartition(ctx, maddr, s.SectorNumber, ts.Key())
				if err != nil {
					return err
				}
				fmt.Printf("%d, %d, %d, %s\n", s.SectorNumber, sp.Deadline, sp.Partition, s.SealedCID)
			} else {
				fmt.Printf("%d, %s\n", s.SectorNumber, s.SealedCID)
			}
		}

		return nil
	},
}

var StateExecTraceCmd = &cli.Command{
	Name:      "exec-trace",
	Usage:     "Get the execution trace of a given message",
	ArgsUsage: "<messageCid>",
	Action: func(cctx *cli.Context) error {
		if cctx.NArg() != 1 {
			return IncorrectNumArgs(cctx)
		}

		mcid, err := cid.Decode(cctx.Args().First())
		if err != nil {
			return fmt.Errorf("message cid was invalid: %s", err)
		}

		capi, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		ctx := ReqContext(cctx)

		msg, err := capi.ChainGetMessage(ctx, mcid)
		if err != nil {
			return err
		}

		lookup, err := capi.StateSearchMsg(ctx, mcid)
		if err != nil {
			return err
		}
		if lookup == nil {
			return fmt.Errorf("failed to find message: %s", mcid)
		}

		ts, err := capi.ChainGetTipSet(ctx, lookup.TipSet)
		if err != nil {
			return err
		}

		pts, err := capi.ChainGetTipSet(ctx, ts.Parents())
		if err != nil {
			return err
		}

		cso, err := capi.StateCompute(ctx, pts.Height(), nil, pts.Key())
		if err != nil {
			return err
		}

		var trace *api.InvocResult
		for _, t := range cso.Trace {
			if t.Msg.From == msg.From && t.Msg.Nonce == msg.Nonce {
				trace = t
				break
			}
		}
		if trace == nil {
			return fmt.Errorf("failed to find message in tipset trace output")
		}

		out, err := json.MarshalIndent(trace, "", "  ")
		if err != nil {
			return err
		}

		fmt.Println(string(out))
		return nil
	},
}

var StateReplayCmd = &cli.Command{
	Name:      "replay",
	Usage:     "Replay a particular message",
	ArgsUsage: "<messageCid>",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "show-trace",
			Usage: "print out full execution trace for given message",
		},
		&cli.BoolFlag{
			Name:  "detailed-gas",
			Usage: "print out detailed gas costs for given message",
		},
	},
	Action: func(cctx *cli.Context) error {
		if cctx.NArg() != 1 {
			return IncorrectNumArgs(cctx)
		}

		mcid, err := cid.Decode(cctx.Args().First())
		if err != nil {
			return fmt.Errorf("message cid was invalid: %s", err)
		}

		fapi, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		ctx := ReqContext(cctx)

		res, err := fapi.StateReplay(ctx, types.EmptyTSK, mcid)
		if err != nil {
			return xerrors.Errorf("replay call failed: %w", err)
		}

		fmt.Println("Replay receipt:")
		fmt.Printf("Exit code: %d\n", res.MsgRct.ExitCode)
		fmt.Printf("Return: %x\n", res.MsgRct.Return)
		fmt.Printf("Gas Used: %d\n", res.MsgRct.GasUsed)

		if cctx.Bool("detailed-gas") {
			fmt.Printf("Base Fee Burn: %d\n", res.GasCost.BaseFeeBurn)
			fmt.Printf("Overestimaton Burn: %d\n", res.GasCost.OverEstimationBurn)
			fmt.Printf("Miner Penalty: %d\n", res.GasCost.MinerPenalty)
			fmt.Printf("Miner Tip: %d\n", res.GasCost.MinerTip)
			fmt.Printf("Refund: %d\n", res.GasCost.Refund)
		}
		fmt.Printf("Total Message Cost: %d\n", res.GasCost.TotalCost)

		if res.MsgRct.ExitCode != 0 {
			fmt.Printf("Error message: %q\n", res.Error)
		}

		if cctx.Bool("show-trace") {
			fmt.Printf("%s\t%s\t%s\t%d\t%x\t%d\t%x\n", res.Msg.From, res.Msg.To, res.Msg.Value, res.Msg.Method, res.Msg.Params, res.MsgRct.ExitCode, res.MsgRct.Return)
			printInternalExecutions("\t", res.ExecutionTrace.Subcalls)
		}

		return nil
	},
}

var StateGetDealSetCmd = &cli.Command{
	Name:      "get-deal",
	Usage:     "View on-chain deal info",
	ArgsUsage: "[dealId]",
	Action: func(cctx *cli.Context) error {
		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		ctx := ReqContext(cctx)

		if cctx.NArg() != 1 {
			return IncorrectNumArgs(cctx)
		}

		dealid, err := strconv.ParseUint(cctx.Args().First(), 10, 64)
		if err != nil {
			return xerrors.Errorf("parsing deal ID: %w", err)
		}

		ts, err := LoadTipSet(ctx, cctx, api)
		if err != nil {
			return err
		}

		deal, err := api.StateMarketStorageDeal(ctx, abi.DealID(dealid), ts.Key())
		if err != nil {
			return err
		}

		data, err := json.MarshalIndent(deal, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(data))

		return nil
	},
}

var StateListMinersCmd = &cli.Command{
	Name:  "list-miners",
	Usage: "list all miners in the network",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "sort-by",
			Usage: "criteria to sort miners by (none, num-deals)",
		},
	},
	Action: func(cctx *cli.Context) error {
		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		ctx := ReqContext(cctx)

		ts, err := LoadTipSet(ctx, cctx, api)
		if err != nil {
			return err
		}

		miners, err := api.StateListMiners(ctx, ts.Key())
		if err != nil {
			return err
		}

		switch cctx.String("sort-by") {
		case "num-deals":
			ndm, err := getDealsCounts(ctx, api)
			if err != nil {
				return err
			}

			sort.Slice(miners, func(i, j int) bool {
				return ndm[miners[i]] > ndm[miners[j]]
			})

			for i := 0; i < 50 && i < len(miners); i++ {
				fmt.Printf("%s %d\n", miners[i], ndm[miners[i]])
			}
			return nil
		default:
			return fmt.Errorf("unrecognized sorting order")
		case "", "none":
		}

		for _, m := range miners {
			fmt.Println(m.String())
		}

		return nil
	},
}

func getDealsCounts(ctx context.Context, lapi v0api.FullNode) (map[address.Address]int, error) {
	allDeals, err := lapi.StateMarketDeals(ctx, types.EmptyTSK)
	if err != nil {
		return nil, err
	}

	out := make(map[address.Address]int)
	for _, d := range allDeals {
		if d.State.SectorStartEpoch != -1 {
			out[d.Proposal.Provider]++
		}
	}

	return out, nil
}

var StateListActorsCmd = &cli.Command{
	Name:  "list-actors",
	Usage: "list all actors in the network",
	Action: func(cctx *cli.Context) error {
		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		ctx := ReqContext(cctx)

		ts, err := LoadTipSet(ctx, cctx, api)
		if err != nil {
			return err
		}

		actors, err := api.StateListActors(ctx, ts.Key())
		if err != nil {
			return err
		}

		for _, a := range actors {
			fmt.Println(a.String())
		}

		return nil
	},
}

var StateGetActorCmd = &cli.Command{
	Name:      "get-actor",
	Usage:     "Print actor information",
	ArgsUsage: "[actorAddress]",
	Action: func(cctx *cli.Context) error {
		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		ctx := ReqContext(cctx)

		if cctx.NArg() != 1 {
			return IncorrectNumArgs(cctx)
		}

		addr, err := address.NewFromString(cctx.Args().First())
		if err != nil {
			return err
		}

		ts, err := LoadTipSet(ctx, cctx, api)
		if err != nil {
			return err
		}

		a, err := api.StateGetActor(ctx, addr, ts.Key())
		if err != nil {
			return err
		}

		strtype := builtin.ActorNameByCode(a.Code)

		fmt.Printf("Address:\t%s\n", addr)
		fmt.Printf("Balance:\t%s\n", types.FIL(a.Balance))
		fmt.Printf("Nonce:\t\t%d\n", a.Nonce)
		fmt.Printf("Code:\t\t%s (%s)\n", a.Code, strtype)
		fmt.Printf("Head:\t\t%s\n", a.Head)
		if a.DelegatedAddress != nil {
			fmt.Printf("Delegated address:\t\t%s\n", a.DelegatedAddress)
		}

		return nil
	},
}

var StateLookupIDCmd = &cli.Command{
	Name:      "lookup",
	Usage:     "Find corresponding ID address",
	ArgsUsage: "[address]",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:    "reverse",
			Aliases: []string{"r"},
			Usage:   "Perform reverse lookup",
		},
	},
	Action: func(cctx *cli.Context) error {
		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		ctx := ReqContext(cctx)

		if cctx.NArg() != 1 {
			return IncorrectNumArgs(cctx)
		}

		addr, err := address.NewFromString(cctx.Args().First())
		if err != nil {
			return err
		}

		ts, err := LoadTipSet(ctx, cctx, api)
		if err != nil {
			return err
		}

		var a address.Address
		if !cctx.Bool("reverse") {
			a, err = api.StateLookupID(ctx, addr, ts.Key())
		} else {
			a, err = api.StateAccountKey(ctx, addr, ts.Key())
		}

		if err != nil {
			return err
		}

		fmt.Printf("%s\n", a)

		return nil
	},
}

var StateSectorSizeCmd = &cli.Command{
	Name:      "sector-size",
	Usage:     "Look up miners sector size",
	ArgsUsage: "[minerAddress]",
	Action: func(cctx *cli.Context) error {
		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		ctx := ReqContext(cctx)

		if cctx.NArg() != 1 {
			return IncorrectNumArgs(cctx)
		}

		addr, err := address.NewFromString(cctx.Args().First())
		if err != nil {
			return err
		}

		ts, err := LoadTipSet(ctx, cctx, api)
		if err != nil {
			return err
		}

		mi, err := api.StateMinerInfo(ctx, addr, ts.Key())
		if err != nil {
			return err
		}

		fmt.Printf("%s (%d)\n", types.SizeStr(types.NewInt(uint64(mi.SectorSize))), mi.SectorSize)
		return nil
	},
}

var StateReadStateCmd = &cli.Command{
	Name:      "read-state",
	Usage:     "View a json representation of an actors state",
	ArgsUsage: "[actorAddress]",
	Action: func(cctx *cli.Context) error {
		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		ctx := ReqContext(cctx)

		if cctx.NArg() != 1 {
			return IncorrectNumArgs(cctx)
		}

		addr, err := address.NewFromString(cctx.Args().First())
		if err != nil {
			return err
		}

		ts, err := LoadTipSet(ctx, cctx, api)
		if err != nil {
			return err
		}

		as, err := api.StateReadState(ctx, addr, ts.Key())
		if err != nil {
			return err
		}

		data, err := json.MarshalIndent(as.State, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(data))

		return nil
	},
}

var StateListMessagesCmd = &cli.Command{
	Name:  "list-messages",
	Usage: "list messages on chain matching given criteria",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "to",
			Usage: "return messages to a given address",
		},
		&cli.StringFlag{
			Name:  "from",
			Usage: "return messages from a given address",
		},
		&cli.Uint64Flag{
			Name:  "toheight",
			Usage: "don't look before given block height",
		},
		&cli.BoolFlag{
			Name:  "cids",
			Usage: "print message CIDs instead of messages",
		},
	},
	Action: func(cctx *cli.Context) error {
		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		ctx := ReqContext(cctx)

		var toa, froma address.Address
		if tos := cctx.String("to"); tos != "" {
			a, err := address.NewFromString(tos)
			if err != nil {
				return fmt.Errorf("given 'to' address %q was invalid: %w", tos, err)
			}
			toa = a
		}

		if froms := cctx.String("from"); froms != "" {
			a, err := address.NewFromString(froms)
			if err != nil {
				return fmt.Errorf("given 'from' address %q was invalid: %w", froms, err)
			}
			froma = a
		}

		toh := abi.ChainEpoch(cctx.Uint64("toheight"))

		ts, err := LoadTipSet(ctx, cctx, api)
		if err != nil {
			return err
		}

		windowSize := abi.ChainEpoch(100)

		cur := ts
		for cur.Height() > toh {
			if ctx.Err() != nil {
				return ctx.Err()
			}

			end := toh
			if cur.Height()-windowSize > end {
				end = cur.Height() - windowSize
			}

			msgs, err := api.StateListMessages(ctx, &lapi.MessageMatch{To: toa, From: froma}, cur.Key(), end)
			if err != nil {
				return err
			}

			for _, c := range msgs {
				if cctx.Bool("cids") {
					fmt.Println(c.String())
					continue
				}

				m, err := api.ChainGetMessage(ctx, c)
				if err != nil {
					return err
				}
				b, err := json.MarshalIndent(m, "", "  ")
				if err != nil {
					return err
				}
				fmt.Println(string(b))
			}

			if end <= 0 {
				break
			}

			next, err := api.ChainGetTipSetByHeight(ctx, end-1, cur.Key())
			if err != nil {
				return err
			}

			cur = next
		}

		return nil
	},
}

var StateComputeStateCmd = &cli.Command{
	Name:  "compute-state",
	Usage: "Perform state computations",
	Flags: []cli.Flag{
		&cli.Uint64Flag{
			Name:  "vm-height",
			Usage: "set the height that the vm will see",
		},
		&cli.BoolFlag{
			Name:  "apply-mpool-messages",
			Usage: "apply messages from the mempool to the computed state",
		},
		&cli.BoolFlag{
			Name:  "show-trace",
			Usage: "print out full execution trace for given tipset",
		},
		&cli.BoolFlag{
			Name:  "html",
			Usage: "generate html report",
		},
		&cli.BoolFlag{
			Name:  "json",
			Usage: "generate json output",
		},
		&cli.StringFlag{
			Name:  "compute-state-output",
			Usage: "a json file containing pre-existing compute-state output, to generate html reports without rerunning state changes",
		},
		&cli.BoolFlag{
			Name:  "no-timing",
			Usage: "don't show timing information in html traces",
		},
	},
	Action: func(cctx *cli.Context) error {
		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		ctx := ReqContext(cctx)

		h := abi.ChainEpoch(cctx.Uint64("vm-height"))
		var ts *types.TipSet
		if tss := cctx.String("tipset"); tss != "" {
			ts, err = ParseTipSetRef(ctx, api, tss)
		} else if h > 0 {
			ts, err = api.ChainGetTipSetByHeight(ctx, h, types.EmptyTSK)
		} else {
			ts, err = api.ChainHead(ctx)
		}
		if err != nil {
			return err
		}

		if h == 0 {
			h = ts.Height()
		}

		var msgs []*types.Message
		if cctx.Bool("apply-mpool-messages") {
			pmsgs, err := api.MpoolSelect(ctx, ts.Key(), 1)
			if err != nil {
				return err
			}

			for _, sm := range pmsgs {
				msgs = append(msgs, &sm.Message)
			}
		}

		var stout *lapi.ComputeStateOutput
		if csofile := cctx.String("compute-state-output"); csofile != "" {
			data, err := os.ReadFile(csofile)
			if err != nil {
				return err
			}

			var o lapi.ComputeStateOutput
			if err := json.Unmarshal(data, &o); err != nil {
				return err
			}

			stout = &o
		} else {
			o, err := api.StateCompute(ctx, h, msgs, ts.Key())
			if err != nil {
				return err
			}

			stout = o
		}

		if cctx.Bool("json") {
			out, err := json.Marshal(stout)
			if err != nil {
				return err
			}
			fmt.Println(string(out))
			return nil
		}

		if cctx.Bool("html") {
			st, err := state.LoadStateTree(cbor.NewCborStore(blockstore.NewAPIBlockstore(api)), stout.Root)
			if err != nil {
				return xerrors.Errorf("loading state tree: %w", err)
			}

			codeCache := map[address.Address]cid.Cid{}
			getCode := func(addr address.Address) (cid.Cid, error) {
				if c, found := codeCache[addr]; found {
					return c, nil
				}

				c, err := st.GetActor(addr)
				if err != nil {
					return cid.Cid{}, err
				}

				codeCache[addr] = c.Code
				return c.Code, nil
			}

			_, _ = fmt.Fprintln(os.Stderr, "computed state cid: ", stout.Root)

			return ComputeStateHTMLTempl(os.Stdout, ts, stout, !cctx.Bool("no-timing"), getCode)
		}

		fmt.Println("computed state cid: ", stout.Root)
		if cctx.Bool("show-trace") {
			for _, ir := range stout.Trace {
				fmt.Printf("%s\t%s\t%s\t%d\t%x\t%d\t%x\n", ir.Msg.From, ir.Msg.To, ir.Msg.Value, ir.Msg.Method, ir.Msg.Params, ir.MsgRct.ExitCode, ir.MsgRct.Return)
				printInternalExecutions("\t", ir.ExecutionTrace.Subcalls)
			}
		}
		return nil
	},
}

func printInternalExecutions(prefix string, trace []types.ExecutionTrace) {
	for _, im := range trace {
		fmt.Printf("%s%s\t%s\t%s\t%d\t%x\t%d\t%x\n", prefix, im.Msg.From, im.Msg.To, im.Msg.Value, im.Msg.Method, im.Msg.Params, im.MsgRct.ExitCode, im.MsgRct.Return)
		printInternalExecutions(prefix+"\t", im.Subcalls)
	}
}

//go:embed compstate.html.template
var compStateTemplate string

//go:embed compstatemsg.html.template
var compStateMsg string

type compStateHTMLIn struct {
	TipSet *types.TipSet
	Comp   *api.ComputeStateOutput
}

func ComputeStateHTMLTempl(w io.Writer, ts *types.TipSet, o *api.ComputeStateOutput, printTiming bool, getCode func(addr address.Address) (cid.Cid, error)) error {
	t, err := template.New("compute_state").Funcs(map[string]interface{}{
		"GetCode":     getCode,
		"GetMethod":   getMethod,
		"ToFil":       toFil,
		"JsonParams":  JsonParams,
		"JsonReturn":  JsonReturn,
		"IsSlow":      isSlow,
		"IsVerySlow":  isVerySlow,
		"IntExit":     func(i exitcode.ExitCode) int64 { return int64(i) },
		"sumGas":      types.SumGas,
		"CodeStr":     builtin.ActorNameByCode,
		"Call":        call,
		"PrintTiming": func() bool { return printTiming },
	}).Parse(compStateTemplate)
	if err != nil {
		return err
	}
	t, err = t.New("message").Parse(compStateMsg)
	if err != nil {
		return err
	}

	return t.ExecuteTemplate(w, "compute_state", &compStateHTMLIn{
		TipSet: ts,
		Comp:   o,
	})
}

type callMeta struct {
	types.ExecutionTrace
	Subcall bool
	Hash    string
}

func call(e types.ExecutionTrace, subcall bool, hash string) callMeta {
	return callMeta{
		ExecutionTrace: e,
		Subcall:        subcall,
		Hash:           hash,
	}
}

func getMethod(code cid.Cid, method abi.MethodNum) string {
	return consensus.NewActorRegistry().Methods[code][method].Name // todo: use remote
}

func toFil(f types.BigInt) types.FIL {
	return types.FIL(f)
}

func isSlow(t time.Duration) bool {
	return t > 10*time.Millisecond
}

func isVerySlow(t time.Duration) bool {
	return t > 50*time.Millisecond
}

func JsonParams(code cid.Cid, method abi.MethodNum, params []byte) (string, error) {
	ar := consensus.NewActorRegistry()

	_, found := ar.Methods[code][method]
	if !found {
		return fmt.Sprintf("raw:%x", params), nil
	}

	p, err := stmgr.GetParamType(ar, code, method) // todo use api for correct actor registry
	if err != nil {
		return fmt.Sprintf("raw:%x; DECODE ERR: %s", params, err.Error()), nil
	}

	if err := p.UnmarshalCBOR(bytes.NewReader(params)); err != nil {
		return fmt.Sprintf("raw:%x; DECODE cbor ERR: %s", params, err.Error()), nil
	}

	b, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return "", err
	}

	return string(b), nil
}

func JsonReturn(code cid.Cid, method abi.MethodNum, ret []byte) (string, error) {
	methodMeta, found := consensus.NewActorRegistry().Methods[code][method] // TODO: use remote
	if !found {
		return "", fmt.Errorf("method %d not found on actor %s", method, code)
	}
	re := reflect.New(methodMeta.Ret.Elem())
	p := re.Interface().(cbg.CBORUnmarshaler)
	if err := p.UnmarshalCBOR(bytes.NewReader(ret)); err != nil {
		return fmt.Sprintf("raw:%x; DECODE ERR: %s", ret, err.Error()), nil
	}

	b, err := json.MarshalIndent(p, "", "  ")
	return string(b), err
}

var StateWaitMsgCmd = &cli.Command{
	Name:      "wait-msg",
	Aliases:   []string{"wait-message"},
	Usage:     "Wait for a message to appear on chain",
	ArgsUsage: "[messageCid]",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "timeout",
			Value: "10m",
		},
	},
	Action: func(cctx *cli.Context) error {
		if cctx.NArg() != 1 {
			return IncorrectNumArgs(cctx)
		}

		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		ctx := ReqContext(cctx)

		msg, err := cid.Decode(cctx.Args().First())
		if err != nil {
			return err
		}

		mw, err := api.StateWaitMsg(ctx, msg, buildconstants.MessageConfidence)
		if err != nil {
			return err
		}

		m, err := api.ChainGetMessage(ctx, msg)
		if err != nil {
			return err
		}

		return printMsg(ctx, api, msg, mw, m)
	},
}

var StateSearchMsgCmd = &cli.Command{
	Name:      "search-msg",
	Aliases:   []string{"search-message"},
	Usage:     "Search to see whether a message has appeared on chain",
	ArgsUsage: "[messageCid]",
	Action: func(cctx *cli.Context) error {
		if cctx.NArg() != 1 {
			return IncorrectNumArgs(cctx)
		}

		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		ctx := ReqContext(cctx)

		msg, err := cid.Decode(cctx.Args().First())
		if err != nil {
			return err
		}

		mw, err := api.StateSearchMsg(ctx, msg)
		if err != nil {
			return err
		}

		if mw == nil {
			return fmt.Errorf("failed to find message: %s", msg)
		}

		m, err := api.ChainGetMessage(ctx, msg)
		if err != nil {
			return err
		}

		return printMsg(ctx, api, msg, mw, m)
	},
}

func printReceiptReturn(ctx context.Context, api v0api.FullNode, m *types.Message, r types.MessageReceipt) error {
	if len(r.Return) == 0 {
		return nil
	}

	act, err := api.StateGetActor(ctx, m.To, types.EmptyTSK)
	if err != nil {
		return err
	}

	jret, err := JsonReturn(act.Code, m.Method, r.Return)
	if err != nil {
		return err
	}

	fmt.Println("Decoded return value: ", jret)

	return nil
}

func printMsg(ctx context.Context, api v0api.FullNode, msg cid.Cid, mw *lapi.MsgLookup, m *types.Message) error {
	if mw == nil {
		fmt.Println("message was not found on chain")
		return nil
	}

	if mw.Message != msg {
		fmt.Printf("Message was replaced: %s\n", mw.Message)
	}

	fmt.Printf("Executed in tipset: %s\n", mw.TipSet.Cids())
	fmt.Printf("Exit Code: %d\n", mw.Receipt.ExitCode)
	fmt.Printf("Gas Used: %d\n", mw.Receipt.GasUsed)
	fmt.Printf("Return: %x\n", mw.Receipt.Return)
	if err := printReceiptReturn(ctx, api, m, mw.Receipt); err != nil {
		return err
	}
	if mw.Receipt.EventsRoot != nil {
		fmt.Printf("Events Root: %s\n", mw.Receipt.EventsRoot)
	}

	return nil
}

var StateCallCmd = &cli.Command{
	Name:      "call",
	Usage:     "Invoke a method on an actor locally",
	ArgsUsage: "[toAddress methodId params (optional)]",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "from",
			Usage: "",
			Value: builtin.SystemActorAddr.String(),
		},
		&cli.StringFlag{
			Name:  "value",
			Usage: "specify value field for invocation",
			Value: "0",
		},
		&cli.StringFlag{
			Name:  "ret",
			Usage: "specify how to parse output (raw, decoded, base64, hex)",
			Value: "decoded",
		},
		&cli.StringFlag{
			Name:  "encoding",
			Value: "base64",
			Usage: "specify params encoding to parse (base64, hex)",
		},
	},
	Action: func(cctx *cli.Context) error {
		if cctx.NArg() < 2 {
			return ShowHelp(cctx, fmt.Errorf("must specify at least actor and method to invoke"))
		}

		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		ctx := ReqContext(cctx)

		toa, err := address.NewFromString(cctx.Args().First())
		if err != nil {
			return fmt.Errorf("given 'to' address %q was invalid: %w", cctx.Args().First(), err)
		}

		froma, err := address.NewFromString(cctx.String("from"))
		if err != nil {
			return fmt.Errorf("given 'from' address %q was invalid: %w", cctx.String("from"), err)
		}

		ts, err := LoadTipSet(ctx, cctx, api)
		if err != nil {
			return err
		}

		method, err := strconv.ParseUint(cctx.Args().Get(1), 10, 64)
		if err != nil {
			return fmt.Errorf("must pass method as a number")
		}

		value, err := types.ParseFIL(cctx.String("value"))
		if err != nil {
			return fmt.Errorf("failed to parse 'value': %s", err)
		}

		var params []byte
		// If params were passed in, decode them
		if cctx.NArg() > 2 {
			switch cctx.String("encoding") {
			case "base64":
				params, err = base64.StdEncoding.DecodeString(cctx.Args().Get(2))
				if err != nil {
					return xerrors.Errorf("decoding base64 value: %w", err)
				}
			case "hex":
				params, err = hex.DecodeString(cctx.Args().Get(2))
				if err != nil {
					return xerrors.Errorf("decoding hex value: %w", err)
				}
			default:
				return xerrors.Errorf("unrecognized encoding: %s", cctx.String("encoding"))
			}
		}

		ret, err := api.StateCall(ctx, &types.Message{
			From:   froma,
			To:     toa,
			Value:  types.BigInt(value),
			Method: abi.MethodNum(method),
			Params: params,
		}, ts.Key())
		if err != nil {
			return fmt.Errorf("state call failed: %w", err)
		}

		if ret.MsgRct.ExitCode != 0 {
			return fmt.Errorf("invocation failed (exit: %d, gasUsed: %d): %s", ret.MsgRct.ExitCode, ret.MsgRct.GasUsed, ret.Error)
		}

		fmt.Println("Call receipt:")
		fmt.Printf("Exit code: %d\n", ret.MsgRct.ExitCode)
		fmt.Printf("Gas Used: %d\n", ret.MsgRct.GasUsed)

		switch cctx.String("ret") {
		case "decoded":
			act, err := api.StateGetActor(ctx, toa, ts.Key())
			if err != nil {
				return xerrors.Errorf("getting actor: %w", err)
			}

			retStr, err := JsonReturn(act.Code, abi.MethodNum(method), ret.MsgRct.Return)
			if err != nil {
				return xerrors.Errorf("decoding return: %w", err)
			}

			fmt.Printf("Return:\n%s\n", retStr)
		case "raw":
			fmt.Printf("Return: \n%s\n", ret.MsgRct.Return)
		case "hex":
			fmt.Printf("Return: \n%x\n", ret.MsgRct.Return)
		case "base64":
			fmt.Printf("Return: \n%s\n", base64.StdEncoding.EncodeToString(ret.MsgRct.Return))
		}

		return nil
	},
}

var StateCircSupplyCmd = &cli.Command{
	Name:  "circulating-supply",
	Usage: "Get the exact current circulating supply of Filecoin",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "vm-supply",
			Usage: "calculates the approximation of the circulating supply used internally by the VM (instead of the exact amount)",
			Value: false,
		},
	},
	Action: func(cctx *cli.Context) error {
		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		ctx := ReqContext(cctx)

		ts, err := LoadTipSet(ctx, cctx, api)
		if err != nil {
			return err
		}

		if cctx.IsSet("vm-supply") {
			circ, err := api.StateVMCirculatingSupplyInternal(ctx, ts.Key())
			if err != nil {
				return err
			}

			fmt.Println("Circulating supply: ", types.FIL(circ.FilCirculating))
			fmt.Println("Mined: ", types.FIL(circ.FilMined))
			fmt.Println("Vested: ", types.FIL(circ.FilVested))
			fmt.Println("Burnt: ", types.FIL(circ.FilBurnt))
			fmt.Println("Locked: ", types.FIL(circ.FilLocked))
		} else {
			circ, err := api.StateCirculatingSupply(ctx, ts.Key())
			if err != nil {
				return err
			}

			fmt.Println("Exact circulating supply: ", types.FIL(circ))
			return nil
		}

		return nil
	},
}

var StateSectorCmd = &cli.Command{
	Name:      "sector",
	Aliases:   []string{"sector-info"},
	Usage:     "Get miner sector info",
	ArgsUsage: "[minerAddress] [sectorNumber]",
	Action: func(cctx *cli.Context) error {
		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		ctx := ReqContext(cctx)

		if cctx.NArg() != 2 {
			return IncorrectNumArgs(cctx)
		}

		ts, err := LoadTipSet(ctx, cctx, api)
		if err != nil {
			return err
		}

		maddr, err := address.NewFromString(cctx.Args().Get(0))
		if err != nil {
			return err
		}

		sid, err := strconv.ParseInt(cctx.Args().Get(1), 10, 64)
		if err != nil {
			return err
		}

		nv, err := api.StateNetworkVersion(ctx, ts.Key())
		if err != nil {
			return err
		}

		si, err := api.StateSectorGetInfo(ctx, maddr, abi.SectorNumber(sid), ts.Key())
		if err != nil {
			return err
		}
		if si == nil {
			return xerrors.Errorf("sector %d for miner %s not found", sid, maddr)
		}

		fmt.Println("SectorNumber: ", si.SectorNumber)
		fmt.Println("SealProof: ", si.SealProof)
		fmt.Println("SealedCID: ", si.SealedCID)
		if si.SectorKeyCID != nil {
			fmt.Println("SectorKeyCID: ", si.SectorKeyCID)
		}
		fmt.Println()
		fmt.Println("Activation: ", cliutil.EpochTimeTs(ts.Height(), si.Activation, ts))
		fmt.Println("Expiration: ", cliutil.EpochTimeTs(ts.Height(), si.Expiration, ts))
		fmt.Println()
		fmt.Println("DealWeight: ", si.DealWeight)
		fmt.Println("VerifiedDealWeight: ", si.VerifiedDealWeight)
		fmt.Println("InitialPledge: ", types.FIL(si.InitialPledge))
		if nv < network.Version25 {
			if si.ExpectedDayReward != nil {
				fmt.Println("ExpectedDayReward: ", types.FIL(*si.ExpectedDayReward))
			}
			if si.ExpectedStoragePledge != nil {
				fmt.Println("ExpectedStoragePledge: ", types.FIL(*si.ExpectedStoragePledge))
			}
		}
		fmt.Println()

		// Display DealIDs - try both deprecated field and new ProviderSectors method
		dealIDs := si.DeprecatedDealIDs
		fmt.Printf("DealIDs (deprecated): %v\n", dealIDs)

		// Deals were moved into market actor's ProviderSectors in NV22 / Actors v13
		if nv >= network.Version22 {
			marketDealIDs, err := GetMarketDealIDs(ctx, api, maddr, abi.SectorNumber(sid), ts.Key())
			if err != nil {
				fmt.Printf("DealIDs (market): error retrieving from market actor: %v\n", err)
			} else if len(marketDealIDs) > 0 {
				fmt.Printf("DealIDs (market): %v\n", marketDealIDs)
			} else {
				fmt.Printf("DealIDs (market): []\n")
			}
		}

		sp, err := api.StateSectorPartition(ctx, maddr, abi.SectorNumber(sid), ts.Key())
		if err != nil {
			return err
		}

		fmt.Println("Deadline: ", sp.Deadline)
		fmt.Println("Partition: ", sp.Partition)

		return nil
	},
}

var StateMarketCmd = &cli.Command{
	Name:  "market",
	Usage: "Inspect the storage market actor",
	Subcommands: []*cli.Command{
		stateMarketBalanceCmd,
		stateMarketProposalPending,
	},
}

var stateMarketBalanceCmd = &cli.Command{
	Name:      "balance",
	Usage:     "Get the market balance (locked and escrowed) for a given account",
	ArgsUsage: "[address]",
	Action: func(cctx *cli.Context) error {
		if cctx.NArg() != 1 {
			return IncorrectNumArgs(cctx)
		}

		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		ctx := ReqContext(cctx)

		ts, err := LoadTipSet(ctx, cctx, api)
		if err != nil {
			return err
		}

		addr, err := address.NewFromString(cctx.Args().First())
		if err != nil {
			return err
		}

		balance, err := api.StateMarketBalance(ctx, addr, ts.Key())
		if err != nil {
			return err
		}

		fmt.Printf("Escrow: %s\n", types.FIL(balance.Escrow))
		fmt.Printf("Locked: %s\n", types.FIL(balance.Locked))

		return nil
	},
}

var stateMarketProposalPending = &cli.Command{
	Name:      "proposal-pending",
	Usage:     "check if a given proposal CID is pending in the market actor",
	ArgsUsage: "[proposal CID]",
	Action: func(cctx *cli.Context) error {
		if cctx.NArg() != 1 {
			return IncorrectNumArgs(cctx)
		}

		api, closer, err := GetFullNodeAPIV1(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := ReqContext(cctx)

		propCid, err := cid.Decode(cctx.Args().First())
		if err != nil {
			return err
		}

		pending, err := api.StateMarketProposalPending(ctx, propCid, types.EmptyTSK)
		if err != nil {
			return err
		}

		fmt.Printf("pending: %t", pending)
		return nil
	},
}

var StateNtwkVersionCmd = &cli.Command{
	Name:  "network-version",
	Usage: "Returns the network version",
	Action: func(cctx *cli.Context) error {
		if cctx.Args().Present() {
			return ShowHelp(cctx, fmt.Errorf("doesn't expect any arguments"))
		}

		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		ctx := ReqContext(cctx)

		ts, err := LoadTipSet(ctx, cctx, api)
		if err != nil {
			return err
		}

		nv, err := api.StateNetworkVersion(ctx, ts.Key())
		if err != nil {
			return err
		}

		fmt.Printf("Network Version: %d\n", nv)

		return nil
	},
}

var StateSysActorCIDsCmd = &cli.Command{
	Name:  "actor-cids",
	Usage: "Returns the built-in actor bundle manifest ID & system actor cids",
	Flags: []cli.Flag{
		&cli.UintFlag{
			Name:  "network-version",
			Usage: "specify network version",
		},
	},
	Action: func(cctx *cli.Context) error {
		if cctx.Args().Present() {
			return ShowHelp(cctx, fmt.Errorf("doesn't expect any arguments"))
		}

		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		ctx := ReqContext(cctx)

		var nv network.Version
		if cctx.IsSet("network-version") {
			nv = network.Version(cctx.Uint64("network-version"))
		} else {
			nv, err = api.StateNetworkVersion(ctx, types.EmptyTSK)
			if err != nil {
				return err
			}
		}

		fmt.Printf("Network Version: %d\n", nv)

		actorVersion, err := actorstypes.VersionForNetwork(nv)
		if err != nil {
			return err
		}
		fmt.Printf("Actor Version: %d\n", actorVersion)

		manifestCid, ok := actors.GetManifest(actorVersion)
		if ok {
			fmt.Printf("Manifest CID: %v\n", manifestCid)
		}

		tw := tabwriter.NewWriter(os.Stdout, 2, 4, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "\nActor\tCID\t")

		actorsCids, err := api.StateActorCodeCIDs(ctx, nv)
		if err != nil {
			return err
		}

		var actorsCidTuples []struct {
			actorName string
			actorCid  cid.Cid
		}

		for name, actorCid := range actorsCids {
			keyVal := struct {
				actorName string
				actorCid  cid.Cid
			}{
				actorName: name,
				actorCid:  actorCid,
			}
			actorsCidTuples = append(actorsCidTuples, keyVal)
		}

		sort.Slice(actorsCidTuples, func(i, j int) bool {
			return actorsCidTuples[i].actorName < actorsCidTuples[j].actorName
		})

		for _, keyVal := range actorsCidTuples {
			_, _ = fmt.Fprintf(tw, "%v\t%v\n", keyVal.actorName, keyVal.actorCid)
		}
		return tw.Flush()
	},
}

// GetMarketDealIDs retrieves deal IDs for a sector from the market actor's ProviderSectors HAMT
func GetMarketDealIDs(ctx context.Context, api v0api.FullNode, maddr address.Address, sid abi.SectorNumber, tsKey types.TipSetKey) ([]abi.DealID, error) {
	// Convert miner address to actor ID
	actorID, err := getMinerActorID(ctx, api, maddr, tsKey)
	if err != nil {
		return nil, err
	}

	// Load market state
	marketState, err := loadMarketState(ctx, api, tsKey)
	if err != nil {
		return nil, err
	}

	// Extract deal IDs from ProviderSectors HAMT
	dealIDs, err := extractSectorDealIDs(marketState, actorID, sid)
	if err != nil {
		return nil, err
	}

	return dealIDs, nil
}

// getMinerActorID converts a miner address to its actor ID
func getMinerActorID(ctx context.Context, api v0api.FullNode, maddr address.Address, tsKey types.TipSetKey) (abi.ActorID, error) {
	// Convert miner address to ID address
	minerID, err := api.StateLookupID(ctx, maddr, tsKey)
	if err != nil {
		return 0, xerrors.Errorf("failed to lookup miner ID: %w", err)
	}

	// Extract the actor ID from the ID address
	id, err := address.IDFromAddress(minerID)
	if err != nil {
		return 0, xerrors.Errorf("failed to extract actor ID from address: %w", err)
	}

	return abi.ActorID(id), nil
}

// loadMarketState loads the market actor state
func loadMarketState(ctx context.Context, api v0api.FullNode, tsKey types.TipSetKey) (market.State, error) {
	// Get the market actor
	marketActor, err := api.StateGetActor(ctx, market.Address, tsKey)
	if err != nil {
		return nil, xerrors.Errorf("failed to get market actor: %w", err)
	}

	// Load the market state
	store := adt.WrapStore(ctx, cbor.NewCborStore(blockstore.NewAPIBlockstore(api)))
	marketState, err := market.Load(store, marketActor)
	if err != nil {
		return nil, xerrors.Errorf("failed to load market state: %w", err)
	}

	return marketState, nil
}

// extractSectorDealIDs extracts deal IDs for a specific sector from the ProviderSectors HAMT
func extractSectorDealIDs(marketState market.State, actorID abi.ActorID, sid abi.SectorNumber) ([]abi.DealID, error) {
	// Get the ProviderSectors interface
	providerSectors, err := marketState.ProviderSectors()
	if err != nil {
		return nil, xerrors.Errorf("failed to get provider sectors: %w", err)
	}

	// Get the sector deal IDs for this miner
	sectorDealIDs, found, err := providerSectors.Get(actorID)
	if err != nil {
		return nil, xerrors.Errorf("failed to get sector deal IDs for actor %d: %w", actorID, err)
	}
	if !found {
		return []abi.DealID{}, nil
	}

	// Get the deal IDs for the specific sector
	dealIDs, _, err := sectorDealIDs.Get(sid)
	if err != nil {
		return nil, xerrors.Errorf("failed to get deal IDs for sector %d: %w", sid, err)
	}
	return dealIDs, nil
}
