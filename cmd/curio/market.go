package main

import (
	"fmt"
	"sort"
	"strconv"

	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/types"
	lcli "github.com/filecoin-project/lotus/cli"
	"github.com/filecoin-project/lotus/cmd/curio/deps"
	"github.com/filecoin-project/lotus/curiosrc/market/lmrpc"
	"github.com/filecoin-project/lotus/lib/harmony/harmonydb"
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

		layers := cctx.StringSlice("layers")

		cfg, err := deps.GetConfig(cctx.Context, layers, db)
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
		&cli.BoolFlag{
			Name:  "synthetic",
			Usage: "Use synthetic PoRep",
			Value: false, // todo implement synthetic
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

		ctx := lcli.ReqContext(cctx)
		dep, err := deps.GetDepsCLI(ctx, cctx)
		if err != nil {
			return err
		}

		mid, err := address.IDFromAddress(act)
		if err != nil {
			return xerrors.Errorf("getting miner id: %w", err)
		}

		mi, err := dep.Full.StateMinerInfo(ctx, act, types.EmptyTSK)
		if err != nil {
			return xerrors.Errorf("getting miner info: %w", err)
		}

		nv, err := dep.Full.StateNetworkVersion(ctx, types.EmptyTSK)
		if err != nil {
			return xerrors.Errorf("getting network version: %w", err)
		}

		wpt := mi.WindowPoStProofType
		spt, err := miner.PreferredSealProofTypeFromWindowPoStType(nv, wpt, cctx.Bool("synthetic"))
		if err != nil {
			return xerrors.Errorf("getting seal proof type: %w", err)
		}

		comm, err := dep.DB.BeginTransaction(ctx, func(tx *harmonydb.Tx) (commit bool, err error) {
			// Get current open sector pieces from DB
			var pieces []struct {
				Sector abi.SectorNumber    `db:"sector_number"`
				Size   abi.PaddedPieceSize `db:"piece_size"`
				Index  uint64              `db:"piece_index"`
			}
			err = tx.Select(&pieces, `
					SELECT
					    sector_number,
						piece_size,
						piece_index,
					FROM
						open_sector_pieces
					WHERE
						sp_id = $1 AND sector_number = $2
					ORDER BY
						piece_index DESC;`, mid, sector)
			if err != nil {
				return false, xerrors.Errorf("getting open sectors from DB")
			}

			if len(pieces) < 1 {
				return false, xerrors.Errorf("sector %d is not waiting to be sealed", sector)
			}

			cn, err := tx.Exec(`INSERT INTO sectors_sdr_pipeline (sp_id, sector_number, reg_seal_proof) VALUES ($1, $2, $3);`, mid, sector, spt)

			if err != nil {
				return false, xerrors.Errorf("adding sector to pipeline: %w", err)
			}

			if cn != 1 {
				return false, xerrors.Errorf("incorrect number of rows returned")
			}

			_, err = tx.Exec("SELECT transfer_and_delete_open_piece($1, $2)", mid, sector)
			if err != nil {
				return false, xerrors.Errorf("adding sector to pipeline: %w", err)
			}

			return true, nil

		}, harmonydb.OptionRetry())

		if err != nil {
			return xerrors.Errorf("start sealing sector: %w", err)
		}

		if !comm {
			return xerrors.Errorf("start sealing sector: commit failed")
		}

		return nil
	},
}
