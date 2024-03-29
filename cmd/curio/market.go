package main

import (
	"fmt"
	"sort"

	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/cmd/curio/deps"
	"github.com/filecoin-project/lotus/curiosrc/market/lmrpc"
)

var marketCmd = &cli.Command{
	Name: "market",
	Subcommands: []*cli.Command{
		marketRPCInfoCmd,
	},
}

var marketRPCInfoCmd = &cli.Command{
	Flags: []cli.Flag{
		&cli.StringSliceFlag{
			Name:  "layers",
			Usage: "list of layers to be interpreted (atop defaults). Default: base",
		},
	},
	Action: func(cctx *cli.Context) error {
		db, err := deps.MakeDB(cctx)
		if err != nil {
			return err
		}

		cfg, err := deps.GetConfig(cctx, db)
		if err != nil {
			return xerrors.Errorf("get config: %w", err)
		}

		ts, err := lmrpc.MakeTokens(cfg)
		if err != nil {
			return xerrors.Errorf("make tokens: %w", err)
		}

		var addrTokens []struct {
			Address string
			Token   string
		}

		for address, s := range ts {
			addrTokens = append(addrTokens, struct {
				Address string
				Token   string
			}{
				Address: address.String(),
				Token:   s,
			})
		}

		sort.Slice(addrTokens, func(i, j int) bool {
			return addrTokens[i].Address < addrTokens[j].Address
		})

		for _, at := range addrTokens {
			fmt.Printf("[lotus-miner/boost compatible] %s %s\n", at.Address, at.Token)
		}

		return nil
	},
	Name: "rpc-info",
}
