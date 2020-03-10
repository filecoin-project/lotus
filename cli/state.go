package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/filecoin-project/specs-actors/actors/builtin"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/builtin/market"
	miner2 "github.com/filecoin-project/specs-actors/actors/builtin/miner"
	samsig "github.com/filecoin-project/specs-actors/actors/builtin/multisig"
	"github.com/filecoin-project/specs-actors/actors/builtin/paych"
	"github.com/filecoin-project/specs-actors/actors/builtin/power"
	"github.com/libp2p/go-libp2p-core/peer"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/vm"
	"github.com/filecoin-project/lotus/miner"

	"github.com/docker/go-units"
	"github.com/ipfs/go-cid"
	cbg "github.com/whyrusleeping/cbor-gen"
	"gopkg.in/urfave/cli.v2"
)

var stateCmd = &cli.Command{
	Name:  "state",
	Usage: "Interact with and query filecoin chain state",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "tipset",
			Usage: "specify tipset to call method on (pass comma separated array of cids)",
		},
	},
	Subcommands: []*cli.Command{
		statePowerCmd,
		stateSectorsCmd,
		stateProvingSetCmd,
		statePledgeCollateralCmd,
		stateListActorsCmd,
		stateListMinersCmd,
		stateGetActorCmd,
		stateLookupIDCmd,
		stateReplaySetCmd,
		stateSectorSizeCmd,
		stateReadStateCmd,
		stateListMessagesCmd,
		stateComputeStateCmd,
		stateCallCmd,
		stateGetDealSetCmd,
		stateWaitMsgCmd,
		stateMinerInfo,
	},
}

var stateMinerInfo = &cli.Command{
	Name:      "miner-info",
	Usage:     "Retrieve miner information",
	ArgsUsage: "[minerAddress]",
	Action: func(cctx *cli.Context) error {
		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		ctx := ReqContext(cctx)

		if !cctx.Args().Present() {
			return fmt.Errorf("must specify miner to get information for")
		}

		addr, err := address.NewFromString(cctx.Args().First())
		if err != nil {
			return err
		}

		ts, err := LoadTipSet(ctx, cctx, api)
		if err != nil {
			return err
		}

		act, err := api.StateGetActor(ctx, addr, ts.Key())
		if err != nil {
			return err
		}

		aso, err := api.ChainReadObj(ctx, act.Head)
		if err != nil {
			return err
		}

		var mst miner2.State
		if err := mst.UnmarshalCBOR(bytes.NewReader(aso)); err != nil {
			return err
		}

		mi := mst.Info

		fmt.Printf("Owner:\t%s\n", mi.Owner)
		fmt.Printf("Worker:\t%s\n", mi.Worker)
		fmt.Printf("PeerID:\t%s\n", mi.PeerId)
		fmt.Printf("SectorSize:\t%s (%d)\n", units.BytesSize(float64(mi.SectorSize)), mi.SectorSize)

		return nil
	},
}

func parseTipSetString(cctx *cli.Context) ([]cid.Cid, error) {
	ts := cctx.String("tipset")
	if ts == "" {
		return nil, nil
	}

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

func LoadTipSet(ctx context.Context, cctx *cli.Context, api api.FullNode) (*types.TipSet, error) {
	cids, err := parseTipSetString(cctx)
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

var statePowerCmd = &cli.Command{
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

		var maddr address.Address
		if cctx.Args().Present() {
			maddr, err = address.NewFromString(cctx.Args().First())
			if err != nil {
				return err
			}
		}

		ts, err := LoadTipSet(ctx, cctx, api)
		if err != nil {
			return err
		}

		power, err := api.StateMinerPower(ctx, maddr, ts.Key())
		if err != nil {
			return err
		}

		tp := power.TotalPower
		if cctx.Args().Present() {
			mp := power.MinerPower
			percI := types.BigDiv(types.BigMul(mp, types.NewInt(1000000)), tp)
			fmt.Printf("%s(%s) / %s(%s) ~= %0.4f%%\n", mp.String(), types.SizeStr(mp), tp.String(), types.SizeStr(tp), float64(percI.Int64())/10000)
		} else {
			fmt.Printf("%s(%s)\n", tp.String(), types.SizeStr(tp))
		}

		return nil
	},
}

var stateSectorsCmd = &cli.Command{
	Name:      "sectors",
	Usage:     "Query the sector set of a miner",
	ArgsUsage: "[minerAddress]",
	Action: func(cctx *cli.Context) error {
		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		ctx := ReqContext(cctx)

		if !cctx.Args().Present() {
			return fmt.Errorf("must specify miner to list sectors for")
		}

		maddr, err := address.NewFromString(cctx.Args().First())
		if err != nil {
			return err
		}

		ts, err := LoadTipSet(ctx, cctx, api)
		if err != nil {
			return err
		}

		sectors, err := api.StateMinerSectors(ctx, maddr, ts.Key())
		if err != nil {
			return err
		}

		for _, s := range sectors {
			fmt.Printf("%d: %x\n", s.Info.Info.SectorNumber, s.Info.Info.SealedCID)
		}

		return nil
	},
}

var stateProvingSetCmd = &cli.Command{
	Name:      "proving",
	Usage:     "Query the proving set of a miner",
	ArgsUsage: "[minerAddress]",
	Action: func(cctx *cli.Context) error {
		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		ctx := ReqContext(cctx)

		if !cctx.Args().Present() {
			return fmt.Errorf("must specify miner to list sectors for")
		}

		maddr, err := address.NewFromString(cctx.Args().First())
		if err != nil {
			return err
		}

		ts, err := LoadTipSet(ctx, cctx, api)
		if err != nil {
			return err
		}

		sectors, err := api.StateMinerProvingSet(ctx, maddr, ts.Key())
		if err != nil {
			return err
		}

		for _, s := range sectors {
			fmt.Printf("%d: %x\n", s.Info.Info.SectorNumber, s.Info.Info.SealedCID)
		}

		return nil
	},
}

var stateReplaySetCmd = &cli.Command{
	Name:      "replay",
	Usage:     "Replay a particular message within a tipset",
	ArgsUsage: "[tipsetKey messageCid]",
	Action: func(cctx *cli.Context) error {
		if cctx.Args().Len() < 1 {
			fmt.Println("usage: [tipset] <message cid>")
			fmt.Println("The last cid passed will be used as the message CID")
			fmt.Println("All preceding ones will be used as the tipset")
			return nil
		}

		args := cctx.Args().Slice()
		mcid, err := cid.Decode(args[len(args)-1])
		if err != nil {
			return fmt.Errorf("message cid was invalid: %s", err)
		}

		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		ctx := ReqContext(cctx)

		var ts *types.TipSet
		{
			var tscids []cid.Cid
			for _, s := range args[:len(args)-1] {
				c, err := cid.Decode(s)
				if err != nil {
					return fmt.Errorf("tipset cid was invalid: %s", err)
				}
				tscids = append(tscids, c)
			}

			if len(tscids) > 0 {
				var headers []*types.BlockHeader
				for _, c := range tscids {
					h, err := api.ChainGetBlock(ctx, c)
					if err != nil {
						return err
					}

					headers = append(headers, h)
				}

				ts, err = types.NewTipSet(headers)
			} else {
				r, err := api.StateWaitMsg(ctx, mcid)
				if err != nil {
					return xerrors.Errorf("finding message in chain: %w", err)
				}

				ts, err = api.ChainGetTipSet(ctx, r.TipSet.Parents())
			}
			if err != nil {
				return err
			}

		}

		res, err := api.StateReplay(ctx, ts.Key(), mcid)
		if err != nil {
			return xerrors.Errorf("replay call failed: %w", err)
		}

		fmt.Println("Replay receipt:")
		fmt.Printf("Exit code: %d\n", res.MsgRct.ExitCode)
		fmt.Printf("Return: %x\n", res.MsgRct.Return)
		fmt.Printf("Gas Used: %s\n", res.MsgRct.GasUsed)
		if res.MsgRct.ExitCode != 0 {
			fmt.Printf("Error message: %q\n", res.Error)
		}

		return nil
	},
}

var statePledgeCollateralCmd = &cli.Command{
	Name:  "pledge-collateral",
	Usage: "Get minimum miner pledge collateral",
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

		coll, err := api.StatePledgeCollateral(ctx, ts.Key())
		if err != nil {
			return err
		}

		fmt.Println(types.FIL(coll))
		return nil
	},
}

var stateGetDealSetCmd = &cli.Command{
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

		if !cctx.Args().Present() {
			return fmt.Errorf("must specify deal ID")
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

var stateListMinersCmd = &cli.Command{
	Name:  "list-miners",
	Usage: "list all miners in the network",
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

		for _, m := range miners {
			fmt.Println(m.String())
		}

		return nil
	},
}

var stateListActorsCmd = &cli.Command{
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

var stateGetActorCmd = &cli.Command{
	Name:      "get-actor",
	Usage:     "Print actor information",
	ArgsUsage: "[actorrAddress]",
	Action: func(cctx *cli.Context) error {
		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		ctx := ReqContext(cctx)

		if !cctx.Args().Present() {
			return fmt.Errorf("must pass address of actor to get")
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

		fmt.Printf("Address:\t%s\n", addr)
		fmt.Printf("Balance:\t%s\n", types.FIL(a.Balance))
		fmt.Printf("Nonce:\t\t%d\n", a.Nonce)
		fmt.Printf("Code:\t\t%s\n", a.Code)
		fmt.Printf("Head:\t\t%s\n", a.Head)

		return nil
	},
}

var stateLookupIDCmd = &cli.Command{
	Name:      "lookup",
	Usage:     "Find corresponding ID address",
	ArgsUsage: "[address]",
	Action: func(cctx *cli.Context) error {
		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		ctx := ReqContext(cctx)

		if !cctx.Args().Present() {
			return fmt.Errorf("must pass address of actor to get")
		}

		addr, err := address.NewFromString(cctx.Args().First())
		if err != nil {
			return err
		}

		ts, err := LoadTipSet(ctx, cctx, api)
		if err != nil {
			return err
		}

		a, err := api.StateLookupID(ctx, addr, ts.Key())
		if err != nil {
			return err
		}

		fmt.Printf("%s\n", a)

		return nil
	},
}

var stateSectorSizeCmd = &cli.Command{
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

		if !cctx.Args().Present() {
			return fmt.Errorf("must pass miner's address")
		}

		addr, err := address.NewFromString(cctx.Args().First())
		if err != nil {
			return err
		}

		ts, err := LoadTipSet(ctx, cctx, api)
		if err != nil {
			return err
		}

		ssize, err := api.StateMinerSectorSize(ctx, addr, ts.Key())
		if err != nil {
			return err
		}

		fmt.Printf("%d\n", ssize)
		return nil
	},
}

var stateReadStateCmd = &cli.Command{
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

		if !cctx.Args().Present() {
			return fmt.Errorf("must pass address of actor to get")
		}

		addr, err := address.NewFromString(cctx.Args().First())
		if err != nil {
			return err
		}

		ts, err := LoadTipSet(ctx, cctx, api)
		if err != nil {
			return err
		}

		act, err := api.StateGetActor(ctx, addr, ts.Key())
		if err != nil {
			return err
		}

		as, err := api.StateReadState(ctx, act, ts.Key())
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

var stateListMessagesCmd = &cli.Command{
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

		toh := cctx.Uint64("toheight")

		ts, err := LoadTipSet(ctx, cctx, api)
		if err != nil {
			return err
		}

		msgs, err := api.StateListMessages(ctx, &types.Message{To: toa, From: froma}, ts.Key(), abi.ChainEpoch(toh))
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

		return nil
	},
}

var stateComputeStateCmd = &cli.Command{
	Name:  "compute-state",
	Usage: "Perform state computations",
	Flags: []cli.Flag{
		&cli.Uint64Flag{
			Name:  "height",
			Usage: "set the height to compute state at",
		},
		&cli.BoolFlag{
			Name:  "apply-mpool-messages",
			Usage: "apply messages from the mempool to the computed state",
		},
		&cli.BoolFlag{
			Name:  "show-trace",
			Usage: "print out full execution trace for given tipset",
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

		h := abi.ChainEpoch(cctx.Uint64("height"))
		if h == 0 {
			if ts == nil {
				head, err := api.ChainHead(ctx)
				if err != nil {
					return err
				}
				ts = head
			}
			h = ts.Height()
		}

		var msgs []*types.Message
		if cctx.Bool("apply-mpool-messages") {
			pmsgs, err := api.MpoolPending(ctx, ts.Key())
			if err != nil {
				return err
			}

			pmsgs, err = miner.SelectMessages(ctx, api.StateGetActor, ts, pmsgs)
			if err != nil {
				return err
			}

			for _, sm := range pmsgs {
				msgs = append(msgs, &sm.Message)
			}
		}

		stout, err := api.StateCompute(ctx, h, msgs, ts.Key())
		if err != nil {
			return err
		}

		fmt.Println("computed state cid: ", stout.Root)
		if cctx.Bool("show-trace") {
			for _, ir := range stout.Trace {
				fmt.Printf("%s\t%s\t%s\t%d\t%x\t%d\t%x\n", ir.Msg.From, ir.Msg.To, ir.Msg.Value, ir.Msg.Method, ir.Msg.Params, ir.MsgRct.ExitCode, ir.MsgRct.Return)
				printInternalExecutions("\t", ir.InternalExecutions)
			}
		}
		return nil
	},
}

func printInternalExecutions(prefix string, trace []*vm.ExecutionResult) {
	for _, im := range trace {
		fmt.Printf("%s%s\t%s\t%s\t%d\t%x\t%d\t%x\n", prefix, im.Msg.From, im.Msg.To, im.Msg.Value, im.Msg.Method, im.Msg.Params, im.MsgRct.ExitCode, im.MsgRct.Return)
		printInternalExecutions(prefix+"\t", im.Subcalls)
	}
}

var stateWaitMsgCmd = &cli.Command{
	Name:      "wait-msg",
	Usage:     "Wait for a message to appear on chain",
	ArgsUsage: "[messageCid]",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "timeout",
			Value: "10m",
		},
	},
	Action: func(cctx *cli.Context) error {
		if !cctx.Args().Present() {
			return fmt.Errorf("must specify message cid to wait for")
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

		mw, err := api.StateWaitMsg(ctx, msg)
		if err != nil {
			return err
		}

		fmt.Printf("message was executed in tipset: %s", mw.TipSet.Cids())
		fmt.Printf("Exit Code: %d", mw.Receipt.ExitCode)
		fmt.Printf("Gas Used: %s", mw.Receipt.GasUsed)
		fmt.Printf("Return: %x", mw.Receipt.Return)
		return nil
	},
}

var stateCallCmd = &cli.Command{
	Name:      "call",
	Usage:     "Invoke a method on an actor locally",
	ArgsUsage: "[toAddress methodId <param1 param2 ...> (optional)]",
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
			Usage: "specify how to parse output (auto, raw, addr, big)",
			Value: "auto",
		},
	},
	Action: func(cctx *cli.Context) error {
		if cctx.Args().Len() < 2 {
			return fmt.Errorf("must specify at least actor and method to invoke")
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

		act, err := api.StateGetActor(ctx, toa, ts.Key())
		if err != nil {
			return fmt.Errorf("failed to lookup target actor: %s", err)
		}

		params, err := parseParamsForMethod(act.Code, method, cctx.Args().Slice()[2:])
		if err != nil {
			return fmt.Errorf("failed to parse params: %s", err)
		}

		ret, err := api.StateCall(ctx, &types.Message{
			From:     froma,
			To:       toa,
			Value:    types.BigInt(value),
			GasLimit: types.NewInt(10000000000),
			GasPrice: types.NewInt(0),
			Method:   abi.MethodNum(method),
			Params:   params,
		}, ts.Key())
		if err != nil {
			return fmt.Errorf("state call failed: %s", err)
		}

		if ret.MsgRct.ExitCode != 0 {
			return fmt.Errorf("invocation failed (exit: %d): %s", ret.MsgRct.ExitCode, ret.Error)
		}

		s, err := formatOutput(cctx.String("ret"), ret.MsgRct.Return)
		if err != nil {
			return fmt.Errorf("failed to format output: %s", err)
		}

		fmt.Printf("return: %s\n", s)

		return nil
	},
}

func formatOutput(t string, val []byte) (string, error) {
	switch t {
	case "raw", "hex":
		return fmt.Sprintf("%x", val), nil
	case "address", "addr", "a":
		a, err := address.NewFromBytes(val)
		if err != nil {
			return "", err
		}
		return a.String(), nil
	case "big", "int", "bigint":
		bi := types.BigFromBytes(val)
		return bi.String(), nil
	case "fil":
		bi := types.FIL(types.BigFromBytes(val))
		return bi.String(), nil
	case "pid", "peerid", "peer":
		pid, err := peer.IDFromBytes(val)
		if err != nil {
			return "", err
		}

		return pid.Pretty(), nil
	case "auto":
		if len(val) == 0 {
			return "", nil
		}

		a, err := address.NewFromBytes(val)
		if err == nil {
			return "address: " + a.String(), nil
		}

		pid, err := peer.IDFromBytes(val)
		if err == nil {
			return "peerID: " + pid.Pretty(), nil
		}

		bi := types.BigFromBytes(val)
		return "bigint: " + bi.String(), nil
	default:
		return "", fmt.Errorf("unrecognized output type: %q", t)
	}
}

func parseParamsForMethod(act cid.Cid, method uint64, args []string) ([]byte, error) {
	if len(args) == 0 {
		return nil, nil
	}

	var f interface{}
	switch act {
	case builtin.StorageMarketActorCodeID:
		f = market.Actor{}.Exports()[method]
	case builtin.StorageMinerActorCodeID:
		f = miner2.Actor{}.Exports()[method]
	case builtin.StoragePowerActorCodeID:
		f = power.Actor{}.Exports()[method]
	case builtin.MultisigActorCodeID:
		f = samsig.Actor{}.Exports()[method]
	case builtin.PaymentChannelActorCodeID:
		f = paych.Actor{}.Exports()[method]
	default:
		return nil, fmt.Errorf("the lazy devs didnt add support for that actor to this call yet")
	}

	rf := reflect.TypeOf(f)
	if rf.NumIn() != 3 {
		return nil, fmt.Errorf("expected referenced method to have three arguments")
	}

	paramObj := rf.In(2).Elem()
	if paramObj.NumField() != len(args) {
		return nil, fmt.Errorf("not enough arguments given to call that method (expecting %d)", paramObj.NumField())
	}

	p := reflect.New(paramObj)
	for i := 0; i < len(args); i++ {
		switch paramObj.Field(i).Type {
		case reflect.TypeOf(address.Address{}):
			a, err := address.NewFromString(args[i])
			if err != nil {
				return nil, fmt.Errorf("failed to parse address: %s", err)
			}
			p.Elem().Field(i).Set(reflect.ValueOf(a))
		case reflect.TypeOf(uint64(0)):
			val, err := strconv.ParseUint(args[i], 10, 64)
			if err != nil {
				return nil, err
			}
			p.Elem().Field(i).Set(reflect.ValueOf(val))
		case reflect.TypeOf(peer.ID("")):
			pid, err := peer.IDB58Decode(args[i])
			if err != nil {
				return nil, fmt.Errorf("failed to parse peer ID: %s", err)
			}
			p.Elem().Field(i).Set(reflect.ValueOf(pid))
		default:
			return nil, fmt.Errorf("unsupported type for call (TODO): %s", paramObj.Field(i).Type)
		}
	}

	m := p.Interface().(cbg.CBORMarshaler)
	buf := new(bytes.Buffer)
	if err := m.MarshalCBOR(buf); err != nil {
		return nil, fmt.Errorf("failed to marshal param object: %s", err)
	}
	return buf.Bytes(), nil
}
