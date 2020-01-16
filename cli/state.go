package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/types"
	"golang.org/x/xerrors"

	"github.com/ipfs/go-cid"
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

func loadTipSet(ctx context.Context, cctx *cli.Context, api api.FullNode) (*types.TipSet, error) {
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
	Name:  "power",
	Usage: "Query network or miner power",
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

		ts, err := loadTipSet(ctx, cctx, api)
		if err != nil {
			return err
		}

		power, err := api.StateMinerPower(ctx, maddr, ts)
		if err != nil {
			return err
		}

		res := power.TotalPower
		if cctx.Args().Present() {
			res = power.MinerPower
		}

		fmt.Println(res.String())
		return nil
	},
}

var stateSectorsCmd = &cli.Command{
	Name:  "sectors",
	Usage: "Query the sector set of a miner",
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

		ts, err := loadTipSet(ctx, cctx, api)
		if err != nil {
			return err
		}

		sectors, err := api.StateMinerSectors(ctx, maddr, ts)
		if err != nil {
			return err
		}

		for _, s := range sectors {
			fmt.Printf("%d: %x %x\n", s.SectorID, s.CommR, s.CommD)
		}

		return nil
	},
}

var stateProvingSetCmd = &cli.Command{
	Name:  "proving",
	Usage: "Query the proving set of a miner",
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

		ts, err := loadTipSet(ctx, cctx, api)
		if err != nil {
			return err
		}

		sectors, err := api.StateMinerProvingSet(ctx, maddr, ts)
		if err != nil {
			return err
		}

		for _, s := range sectors {
			fmt.Printf("%d: %x %x\n", s.SectorID, s.CommR, s.CommD)
		}

		return nil
	},
}

var stateReplaySetCmd = &cli.Command{
	Name:  "replay",
	Usage: "Replay a particular message within a tipset",
	Action: func(cctx *cli.Context) error {
		if cctx.Args().Len() < 2 {
			fmt.Println("usage: <tipset> <message cid>")
			fmt.Println("The last cid passed will be used as the message CID")
			fmt.Println("All preceding ones will be used as the tipset")
			return nil
		}

		args := cctx.Args().Slice()
		mcid, err := cid.Decode(args[len(args)-1])
		if err != nil {
			return fmt.Errorf("message cid was invalid: %s", err)
		}

		var tscids []cid.Cid
		for _, s := range args[:len(args)-1] {
			c, err := cid.Decode(s)
			if err != nil {
				return fmt.Errorf("tipset cid was invalid: %s", err)
			}
			tscids = append(tscids, c)
		}

		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		ctx := ReqContext(cctx)

		var headers []*types.BlockHeader
		for _, c := range tscids {
			h, err := api.ChainGetBlock(ctx, c)
			if err != nil {
				return err
			}

			headers = append(headers, h)
		}

		ts, err := types.NewTipSet(headers)
		if err != nil {
			return err
		}

		res, err := api.StateReplay(ctx, ts, mcid)
		if err != nil {
			return xerrors.Errorf("replay call failed: %w", err)
		}

		fmt.Println("Replay receipt:")
		fmt.Printf("Exit code: %d\n", res.Receipt.ExitCode)
		fmt.Printf("Return: %x\n", res.Receipt.Return)
		fmt.Printf("Gas Used: %s\n", res.Receipt.GasUsed)
		if res.Receipt.ExitCode != 0 {
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

		ts, err := loadTipSet(ctx, cctx, api)
		if err != nil {
			return err
		}

		coll, err := api.StatePledgeCollateral(ctx, ts)
		if err != nil {
			return err
		}

		fmt.Println(types.FIL(coll))
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

		ts, err := loadTipSet(ctx, cctx, api)
		if err != nil {
			return err
		}

		miners, err := api.StateListMiners(ctx, ts)
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

		ts, err := loadTipSet(ctx, cctx, api)
		if err != nil {
			return err
		}

		actors, err := api.StateListActors(ctx, ts)
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
	Name:  "get-actor",
	Usage: "Print actor information",
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

		ts, err := loadTipSet(ctx, cctx, api)
		if err != nil {
			return err
		}

		a, err := api.StateGetActor(ctx, addr, ts)
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
	Name:  "lookup",
	Usage: "Find corresponding ID address",
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

		ts, err := loadTipSet(ctx, cctx, api)
		if err != nil {
			return err
		}

		a, err := api.StateLookupID(ctx, addr, ts)
		if err != nil {
			return err
		}

		fmt.Printf("%s\n", a)

		return nil
	},
}

var stateSectorSizeCmd = &cli.Command{
	Name:  "sector-size",
	Usage: "Look up miners sector size",
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

		ts, err := loadTipSet(ctx, cctx, api)
		if err != nil {
			return err
		}

		ssize, err := api.StateMinerSectorSize(ctx, addr, ts)
		if err != nil {
			return err
		}

		fmt.Printf("%d\n", ssize)
		return nil
	},
}

var stateReadStateCmd = &cli.Command{
	Name:  "read-state",
	Usage: "View a json representation of an actors state",
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

		ts, err := loadTipSet(ctx, cctx, api)
		if err != nil {
			return err
		}

		act, err := api.StateGetActor(ctx, addr, ts)
		if err != nil {
			return err
		}

		as, err := api.StateReadState(ctx, act, ts)
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

		ts, err := loadTipSet(ctx, cctx, api)
		if err != nil {
			return err
		}

		msgs, err := api.StateListMessages(ctx, &types.Message{To: toa, From: froma}, ts, toh)
		if err != nil {
			return err
		}

		for _, c := range msgs {
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
