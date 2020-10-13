package main

import (
	"fmt"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	lapi "github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/types"
	lcli "github.com/filecoin-project/lotus/cli"
	"github.com/filecoin-project/specs-actors/v2/actors/builtin"
	"github.com/urfave/cli/v2"
)

var postFindCmd = &cli.Command{
	Name:        "post-find",
	Description: "return addresses of all miners who have over zero power and have posted in the last day",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "tipset",
			Usage: "specify tipset state to search on",
		},
	},
	Action: func(c *cli.Context) error {
		api, acloser, err := lcli.GetFullNodeAPI(c)
		if err != nil {
			return err
		}
		defer acloser()
		ctx := lcli.ReqContext(c)

		ts, err := lcli.LoadTipSet(ctx, c, api)
		if err != nil {
			return err
		}
		if ts == nil {
			ts, err = api.ChainHead(ctx)
			if err != nil {
				return err
			}
		}
		oneDayAgo := ts.Height() - abi.ChainEpoch(2880)

		// Get all messages over the last day
		msgs := make([]*types.Message, 0)
		for ts.Height() > oneDayAgo {
			// Get messages on ts parent
			next, err := api.ChainGetParentMessages(ctx, ts.Cids()[0])
			if err != nil {
				return err
			}
			msgs = append(msgs, messagesFromAPIMessages(next)...)

			// Next ts
			ts, err = api.ChainGetTipSet(ctx, ts.Parents())
			if err != nil {
				return err
			}
		}
		fmt.Printf("Loaded messages to height %d\n", ts.Height())

		mAddrs, err := api.StateListMiners(ctx, ts.Key())
		if err != nil {
			return err
		}

		minersWithPower := make(map[address.Address]struct{})
		for _, mAddr := range mAddrs {
			// if they have no power ignore. This filters out 14k inactive miners
			// so we can do 100x fewer expensive message queries
			power, err := api.StateMinerPower(ctx, mAddr, ts.Key())
			if err != nil {
				return err
			}
			if power.MinerPower.RawBytePower.GreaterThan(big.Zero()) {
				minersWithPower[mAddr] = struct{}{}
			}
		}
		fmt.Printf("Loaded %d miners with power\n", len(minersWithPower))

		postedMiners := make(map[address.Address]struct{})
		for _, msg := range msgs {
			_, hasPower := minersWithPower[msg.To]
			_, seenBefore := postedMiners[msg.To]

			if hasPower && !seenBefore {
				if msg.Method == builtin.MethodsMiner.SubmitWindowedPoSt {
					fmt.Printf("%s\n", msg.To)
					postedMiners[msg.To] = struct{}{}
				}
			}
		}
		return nil
	},
}

func messagesFromAPIMessages(apiMessages []lapi.Message) []*types.Message {
	messages := make([]*types.Message, len(apiMessages))
	for i, apiMessage := range apiMessages {
		messages[i] = apiMessage.Message
	}
	return messages
}
