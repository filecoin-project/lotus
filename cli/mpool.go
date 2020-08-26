package cli

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"

	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/specs-actors/actors/abi"

	"github.com/filecoin-project/lotus/chain/types"
)

var mpoolCmd = &cli.Command{
	Name:  "mpool",
	Usage: "Manage message pool",
	Subcommands: []*cli.Command{
		mpoolPending,
		mpoolClear,
		mpoolSub,
		mpoolStat,
		mpoolReplaceCmd,
		mpoolFindCmd,
		mpoolConfig,
	},
}

var mpoolPending = &cli.Command{
	Name:  "pending",
	Usage: "Get pending messages",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "local",
			Usage: "print pending messages for addresses in local wallet only",
		},
	},
	Action: func(cctx *cli.Context) error {
		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		ctx := ReqContext(cctx)

		var filter map[address.Address]struct{}
		if cctx.Bool("local") {
			filter = map[address.Address]struct{}{}

			addrss, err := api.WalletList(ctx)
			if err != nil {
				return xerrors.Errorf("getting local addresses: %w", err)
			}

			for _, a := range addrss {
				filter[a] = struct{}{}
			}
		}

		msgs, err := api.MpoolPending(ctx, types.EmptyTSK)
		if err != nil {
			return err
		}

		for _, msg := range msgs {
			if filter != nil {
				if _, has := filter[msg.Message.From]; !has {
					continue
				}
			}

			out, err := json.MarshalIndent(msg, "", "  ")
			if err != nil {
				return err
			}
			fmt.Println(string(out))
		}

		return nil
	},
}

var mpoolClear = &cli.Command{
	Name:  "clear",
	Usage: "Clear all pending messages from the mpool (USE WITH CARE)",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "local",
			Usage: "also clear local messages",
		},
		&cli.BoolFlag{
			Name:  "really-do-it",
			Usage: "must be specified for the action to take effect",
		},
	},
	Action: func(cctx *cli.Context) error {
		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		really := cctx.Bool("really-do-it")
		if !really {
			//nolint:golint
			return fmt.Errorf("--really-do-it must be specified for this action to have an effect; you have been warned")
		}

		local := cctx.Bool("local")

		ctx := ReqContext(cctx)
		return api.MpoolClear(ctx, local)
	},
}

var mpoolSub = &cli.Command{
	Name:  "sub",
	Usage: "Subscribe to mpool changes",
	Action: func(cctx *cli.Context) error {
		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		ctx := ReqContext(cctx)

		sub, err := api.MpoolSub(ctx)
		if err != nil {
			return err
		}

		for {
			select {
			case update := <-sub:
				out, err := json.MarshalIndent(update, "", "  ")
				if err != nil {
					return err
				}
				fmt.Println(string(out))
			case <-ctx.Done():
				return nil
			}
		}
	},
}

type statBucket struct {
	msgs map[uint64]*types.SignedMessage
}
type mpStat struct {
	addr              string
	past, cur, future uint64
}

var mpoolStat = &cli.Command{
	Name:  "stat",
	Usage: "print mempool stats",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "local",
			Usage: "print stats for addresses in local wallet only",
		},
	},
	Action: func(cctx *cli.Context) error {
		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		ctx := ReqContext(cctx)

		ts, err := api.ChainHead(ctx)
		if err != nil {
			return xerrors.Errorf("getting chain head: %w", err)
		}

		var filter map[address.Address]struct{}
		if cctx.Bool("local") {
			filter = map[address.Address]struct{}{}

			addrss, err := api.WalletList(ctx)
			if err != nil {
				return xerrors.Errorf("getting local addresses: %w", err)
			}

			for _, a := range addrss {
				filter[a] = struct{}{}
			}
		}

		msgs, err := api.MpoolPending(ctx, types.EmptyTSK)
		if err != nil {
			return err
		}

		buckets := map[address.Address]*statBucket{}

		for _, v := range msgs {
			if filter != nil {
				if _, has := filter[v.Message.From]; !has {
					continue
				}
			}

			bkt, ok := buckets[v.Message.From]
			if !ok {
				bkt = &statBucket{
					msgs: map[uint64]*types.SignedMessage{},
				}
				buckets[v.Message.From] = bkt
			}

			bkt.msgs[v.Message.Nonce] = v
		}

		var out []mpStat

		for a, bkt := range buckets {
			act, err := api.StateGetActor(ctx, a, ts.Key())
			if err != nil {
				fmt.Printf("%s, err: %s\n", a, err)
				continue
			}

			cur := act.Nonce
			for {
				_, ok := bkt.msgs[cur]
				if !ok {
					break
				}
				cur++
			}

			past := uint64(0)
			future := uint64(0)
			for _, m := range bkt.msgs {
				if m.Message.Nonce < act.Nonce {
					past++
				}
				if m.Message.Nonce > cur {
					future++
				}
			}

			out = append(out, mpStat{
				addr:   a.String(),
				past:   past,
				cur:    cur - act.Nonce,
				future: future,
			})
		}

		sort.Slice(out, func(i, j int) bool {
			return out[i].addr < out[j].addr
		})

		var total mpStat

		for _, stat := range out {
			total.past += stat.past
			total.cur += stat.cur
			total.future += stat.future

			fmt.Printf("%s: past: %d, cur: %d, future: %d\n", stat.addr, stat.past, stat.cur, stat.future)
		}

		fmt.Println("-----")
		fmt.Printf("total: past: %d, cur: %d, future: %d\n", total.past, total.cur, total.future)

		return nil
	},
}

var mpoolReplaceCmd = &cli.Command{
	Name:  "replace",
	Usage: "replace a message in the mempool",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "gas-feecap",
			Usage: "gas feecap for new message",
		},
		&cli.StringFlag{
			Name:  "gas-premium",
			Usage: "gas price for new message",
		},
		&cli.Int64Flag{
			Name:  "gas-limit",
			Usage: "gas price for new message",
		},
	},
	ArgsUsage: "[from] [nonce]",
	Action: func(cctx *cli.Context) error {
		if cctx.Args().Len() < 2 {
			return cli.ShowCommandHelp(cctx, cctx.Command.Name)
		}

		from, err := address.NewFromString(cctx.Args().Get(0))
		if err != nil {
			return err
		}

		nonce, err := strconv.ParseUint(cctx.Args().Get(1), 10, 64)
		if err != nil {
			return err
		}

		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		ctx := ReqContext(cctx)

		ts, err := api.ChainHead(ctx)
		if err != nil {
			return xerrors.Errorf("getting chain head: %w", err)
		}

		pending, err := api.MpoolPending(ctx, ts.Key())
		if err != nil {
			return err
		}

		var found *types.SignedMessage
		for _, p := range pending {
			if p.Message.From == from && p.Message.Nonce == nonce {
				found = p
				break
			}
		}

		if found == nil {
			return fmt.Errorf("no pending message found from %s with nonce %d", from, nonce)
		}

		msg := found.Message

		msg.GasLimit = cctx.Int64("gas-limit")
		msg.GasPremium, err = types.BigFromString(cctx.String("gas-premium"))
		if err != nil {
			return fmt.Errorf("parsing gas-premium: %w", err)
		}
		// TODO: estimate fee cap here
		msg.GasFeeCap, err = types.BigFromString(cctx.String("gas-feecap"))
		if err != nil {
			return fmt.Errorf("parsing gas-feecap: %w", err)
		}

		smsg, err := api.WalletSignMessage(ctx, msg.From, &msg)
		if err != nil {
			return fmt.Errorf("failed to sign message: %w", err)
		}

		cid, err := api.MpoolPush(ctx, smsg)
		if err != nil {
			return fmt.Errorf("failed to push new message to mempool: %w", err)
		}

		fmt.Println("new message cid: ", cid)
		return nil
	},
}

var mpoolFindCmd = &cli.Command{
	Name:  "find",
	Usage: "find a message in the mempool",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "from",
			Usage: "search for messages with given 'from' address",
		},
		&cli.StringFlag{
			Name:  "to",
			Usage: "search for messages with given 'to' address",
		},
		&cli.Int64Flag{
			Name:  "method",
			Usage: "search for messages with given method",
		},
	},
	Action: func(cctx *cli.Context) error {
		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		ctx := ReqContext(cctx)

		pending, err := api.MpoolPending(ctx, types.EmptyTSK)
		if err != nil {
			return err
		}

		var toFilter, fromFilter address.Address
		if cctx.IsSet("to") {
			a, err := address.NewFromString(cctx.String("to"))
			if err != nil {
				return fmt.Errorf("'to' address was invalid: %w", err)
			}

			toFilter = a
		}

		if cctx.IsSet("from") {
			a, err := address.NewFromString(cctx.String("from"))
			if err != nil {
				return fmt.Errorf("'from' address was invalid: %w", err)
			}

			fromFilter = a
		}

		var methodFilter *abi.MethodNum
		if cctx.IsSet("method") {
			m := abi.MethodNum(cctx.Int64("method"))
			methodFilter = &m
		}

		var out []*types.SignedMessage
		for _, m := range pending {
			if toFilter != address.Undef && m.Message.To != toFilter {
				continue
			}

			if fromFilter != address.Undef && m.Message.From != fromFilter {
				continue
			}

			if methodFilter != nil && *methodFilter != m.Message.Method {
				continue
			}

			out = append(out, m)
		}

		b, err := json.MarshalIndent(out, "", "  ")
		if err != nil {
			return err
		}

		fmt.Println(string(b))
		return nil
	},
}

var mpoolConfig = &cli.Command{
	Name:      "config",
	Usage:     "get or set current mpool configuration",
	ArgsUsage: "[new-config]",
	Action: func(cctx *cli.Context) error {
		if cctx.Args().Len() > 1 {
			return cli.ShowCommandHelp(cctx, cctx.Command.Name)
		}

		api, closer, err := GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		ctx := ReqContext(cctx)

		if cctx.Args().Len() == 0 {
			cfg, err := api.MpoolGetConfig(ctx)
			if err != nil {
				return err
			}

			bytes, err := json.Marshal(cfg)
			if err != nil {
				return err
			}

			fmt.Println(string(bytes))
		} else {
			cfg := new(types.MpoolConfig)
			bytes := []byte(cctx.Args().Get(0))

			err := json.Unmarshal(bytes, cfg)
			if err != nil {
				return err
			}

			return api.MpoolSetConfig(ctx, cfg)
		}

		return nil
	},
}
