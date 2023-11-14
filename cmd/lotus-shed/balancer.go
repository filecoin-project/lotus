package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/ipfs/go-cid"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/exitcode"

	lapi "github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	lcli "github.com/filecoin-project/lotus/cli"
)

var balancerCmd = &cli.Command{
	Name:  "balancer",
	Usage: "Utility for balancing tokens between multiple wallets",
	Description: `Tokens are balanced based on the specification provided in arguments

Each argument specifies an address, role, and role parameters separated by ';'

Supported roles:
	- request;[addr];[low];[high] - request tokens when balance drops to [low], topping up to [high]
	- provide;[addr];[min] - provide tokens to other addresses as long as the balance is above [min]
`,
	Action: func(cctx *cli.Context) error {
		api, closer, err := lcli.GetFullNodeAPIV1(cctx)
		if err != nil {
			return err
		}

		defer closer()
		ctx := lcli.ReqContext(cctx)

		type request struct {
			addr      address.Address
			low, high abi.TokenAmount
		}
		type provide struct {
			addr address.Address
			min  abi.TokenAmount
		}

		var requests []request
		var provides []provide

		for i, s := range cctx.Args().Slice() {
			ss := strings.Split(s, ";")
			switch ss[0] {
			case "request":
				if len(ss) != 4 {
					return xerrors.Errorf("request role needs 4 parameters (arg %d)", i)
				}

				addr, err := address.NewFromString(ss[1])
				if err != nil {
					return xerrors.Errorf("parsing address in arg %d: %w", i, err)
				}

				low, err := types.ParseFIL(ss[2])
				if err != nil {
					return xerrors.Errorf("parsing low in arg %d: %w", i, err)
				}

				high, err := types.ParseFIL(ss[3])
				if err != nil {
					return xerrors.Errorf("parsing high in arg %d: %w", i, err)
				}

				if abi.TokenAmount(low).GreaterThanEqual(abi.TokenAmount(high)) {
					return xerrors.Errorf("low must be less than high in arg %d", i)
				}

				requests = append(requests, request{
					addr: addr,
					low:  abi.TokenAmount(low),
					high: abi.TokenAmount(high),
				})
			case "provide":
				if len(ss) != 3 {
					return xerrors.Errorf("provide role needs 3 parameters (arg %d)", i)
				}

				addr, err := address.NewFromString(ss[1])
				if err != nil {
					return xerrors.Errorf("parsing address in arg %d: %w", i, err)
				}

				min, err := types.ParseFIL(ss[2])
				if err != nil {
					return xerrors.Errorf("parsing min in arg %d: %w", i, err)
				}

				provides = append(provides, provide{
					addr: addr,
					min:  abi.TokenAmount(min),
				})
			default:
				return xerrors.Errorf("unknown role '%s' in arg %d", ss[0], i)
			}
		}

		if len(provides) == 0 {
			return xerrors.Errorf("no provides specified")
		}
		if len(requests) == 0 {
			return xerrors.Errorf("no requests specified")
		}

		const confidence = 16

		var notifs <-chan []*lapi.HeadChange
		for {
			if notifs == nil {
				notifs, err = api.ChainNotify(ctx)
				if err != nil {
					return xerrors.Errorf("chain notify error: %w", err)
				}
			}

			var ts *types.TipSet
		loop:
			for {
				time.Sleep(150 * time.Millisecond)
				select {
				case n := <-notifs:
					for _, change := range n {
						if change.Type != store.HCApply {
							continue
						}

						ts = change.Val
					}
				case <-ctx.Done():
					return nil
				default:
					break loop
				}
			}

			type send struct {
				to     address.Address
				amt    abi.TokenAmount
				filled bool
			}
			var toSend []*send

			for _, req := range requests {
				bal, err := api.StateGetActor(ctx, req.addr, ts.Key())
				if err != nil {
					return err
				}

				if bal.Balance.LessThan(req.low) {
					toSend = append(toSend, &send{
						to:  req.addr,
						amt: big.Sub(req.high, bal.Balance),
					})
				}
			}

			for _, s := range toSend {
				fmt.Printf("REQUEST %s for %s\n", types.FIL(s.amt), s.to)
			}

			var msgs []cid.Cid

			for _, prov := range provides {
				bal, err := api.StateGetActor(ctx, prov.addr, ts.Key())
				if err != nil {
					return err
				}

				avail := big.Sub(bal.Balance, prov.min)
				for _, s := range toSend {
					if s.filled {
						continue
					}
					if avail.LessThan(s.amt) {
						continue
					}

					m, err := api.MpoolPushMessage(ctx, &types.Message{
						From:  prov.addr,
						To:    s.to,
						Value: s.amt,
					}, nil)
					if err != nil {
						fmt.Printf("SEND ERROR %s\n", err.Error())
					}
					fmt.Printf("SEND %s; %s from %s TO %s\n", m.Cid(), types.FIL(s.amt), s.to, prov.addr)

					msgs = append(msgs, m.Cid())
					s.filled = true
					avail = big.Sub(avail, s.amt)
				}
			}

			if len(msgs) > 0 {
				fmt.Printf("WAITING FOR %d MESSAGES\n", len(msgs))
			}

			for _, msg := range msgs {
				ml, err := api.StateWaitMsg(ctx, msg, confidence, lapi.LookbackNoLimit, true)
				if err != nil {
					return err
				}
				if ml.Receipt.ExitCode != exitcode.Ok {
					fmt.Printf("MSG %s NON-ZERO EXITCODE: %s\n", msg, ml.Receipt.ExitCode)
				}
			}
		}
	},
}
