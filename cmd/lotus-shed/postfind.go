package main

import (
	"fmt"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/chain/types"
	lcli "github.com/filecoin-project/lotus/cli"
	"github.com/filecoin-project/specs-actors/v2/actors/builtin"
	"github.com/urfave/cli/v2"
)

var postFindCmd = &cli.Command{
	Name:        "post-find",
	Description: "return addresses of all miners who have posted in the last day",
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

		mAddrs, err := api.StateListMiners(ctx, ts.Key())
		if err != nil {
			return err
		}

		for _, mAddr := range mAddrs {
			// if they have no power ignore. This filters out 14k inactive miners
			// so we can do 100x fewer expensive message queries
			power, err := api.StateMinerPower(ctx, mAddr, ts.Key())
			if err != nil {
				return err
			}
			if !power.HasMinPower {
				continue
			}
			query := &types.Message{To: mAddr}
			mCids, err := api.StateListMessages(ctx, query, ts.Key(), oneDayAgo)
			if err != nil {
				return err
			}
			for _, mCid := range mCids {
				msg, err := api.ChainGetMessage(ctx, mCid)
				if err != nil {
					return err
				}
				if msg.Method == builtin.MethodsMiner.SubmitWindowedPoSt {
					fmt.Printf("%s\n", mAddr)
					break // go to next mAddr
				}
			}
		}

		return nil
	},
}
