package main

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"

	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/api/client"
	cliutil "github.com/filecoin-project/lotus/cli/util"
	"github.com/filecoin-project/lotus/cmd/curio/deps"
	"github.com/filecoin-project/lotus/curiosrc/market/lmrpc"
)

var marketCmd = &cli.Command{
	Name: "market",
	Subcommands: []*cli.Command{
		marketRPCInfoCmd,
		marketSealCmd,
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

var marketSealCmd = &cli.Command{
	Name:  "seal",
	Usage: "start sealing a deal sector early",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:     "actor",
			Usage:    "Specify actor address to start sealing sectors for",
			Required: true,
		},
		&cli.StringSliceFlag{
			Name:  "layers",
			Usage: "list of layers to be interpreted (atop defaults). Default: base",
		},
	},
	Action: func(cctx *cli.Context) error {
		act, err := address.NewFromString(cctx.String("actor"))
		if err != nil {
			return xerrors.Errorf("parsing --actor: %w", err)
		}

		if cctx.Args().Len() > 1 {
			return xerrors.Errorf("specify only one sector")
		}

		sec := cctx.Args().First()

		sector, err := strconv.ParseUint(sec, 10, 64)
		if err != nil {
			return xerrors.Errorf("failed to parse the sector number: %w", err)
		}

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

		info, ok := ts[act]
		if !ok {
			return xerrors.Errorf("no market configuration found for actor %s in the specified config layers", act)
		}

		ainfo := cliutil.ParseApiInfo(info)
		addr, err := ainfo.DialArgs("v0")
		if err != nil {
			return xerrors.Errorf("could not get DialArgs: %w", err)
		}

		type httpHead struct {
			addr   string
			header http.Header
		}

		head := httpHead{
			addr:   addr,
			header: ainfo.AuthHeader(),
		}

		market, closer, err := client.NewStorageMinerRPCV0(cctx.Context, head.addr, head.header)
		if err != nil {
			return xerrors.Errorf("failed to get market API: %w", err)
		}
		defer closer()

		return market.SectorStartSealing(cctx.Context, abi.SectorNumber(sector))
	},
}
