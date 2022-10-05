package main

import (
	"fmt"

	"github.com/urfave/cli/v2"

	"github.com/filecoin-project/lotus/chain/types"
	lcli "github.com/filecoin-project/lotus/cli"
)

var createSimCommand = &cli.Command{
	Name:      "create",
	ArgsUsage: "[tipset]",
	Action: func(cctx *cli.Context) (err error) {
		node, err := open(cctx)
		if err != nil {
			return err
		}
		defer func() {
			if cerr := node.Close(); err == nil {
				err = cerr
			}
		}()

		var ts *types.TipSet
		switch cctx.NArg() {
		case 0:
			if err := node.Chainstore.Load(cctx.Context); err != nil {
				return err
			}
			ts = node.Chainstore.GetHeaviestTipSet()
		case 1:
			ts, err = lcli.ParseTipSetRefOffline(cctx.Context, node.Chainstore, cctx.Args().Get(1))
			if err != nil {
				return err
			}
		default:
			return fmt.Errorf("expected 0 or 1 arguments")
		}
		_, err = node.CreateSim(cctx.Context, cctx.String("simulation"), ts)
		return err
	},
}
