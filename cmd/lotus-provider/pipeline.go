package main

import (
	"fmt"
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/types"
	lcli "github.com/filecoin-project/lotus/cli"
	"github.com/filecoin-project/lotus/cmd/lotus-provider/deps"
	"github.com/filecoin-project/lotus/lib/harmony/harmonydb"
	"github.com/filecoin-project/lotus/provider/lpseal"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"
)

var pipelineCmd = &cli.Command{
	Name:  "pipeline",
	Usage: "Manage the sealing pipeline",
	Subcommands: []*cli.Command{
		pipelineStartCmd,
	},
}

var pipelineStartCmd = &cli.Command{
	Name:  "start",
	Usage: "Start new sealing operations manually",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:     "actor",
			Usage:    "Specify actor address to start sealing sectors for",
			Required: true,
		},
		&cli.BoolFlag{
			Name:  "now",
			Usage: "Start sealing sectors for all actors now (not on schedule)",
		},
		&cli.BoolFlag{
			Name:  "cc",
			Usage: "Start sealing new CC sectors",
		},
		&cli.IntFlag{
			Name:  "count",
			Usage: "Number of sectors to start",
			Value: 1,
		},
		&cli.BoolFlag{
			Name:  "synthetic",
			Usage: "Use synthetic PoRep",
			Value: false, // todo implement synthetic
		},
		&cli.StringSliceFlag{ // todo consider moving layers top level
			Name:  "layers",
			Usage: "list of layers to be interpreted (atop defaults). Default: base",
			Value: cli.NewStringSlice("base"),
		},
	},
	Action: func(cctx *cli.Context) error {
		if !cctx.Bool("now") {
			return xerrors.Errorf("schedule not implemented, use --now")
		}
		if !cctx.IsSet("actor") {
			return cli.ShowCommandHelp(cctx, "start")
		}
		if !cctx.Bool("cc") {
			return xerrors.Errorf("only CC sectors supported for now")
		}

		act, err := address.NewFromString(cctx.String("actor"))
		if err != nil {
			return xerrors.Errorf("parsing --actor: %w", err)
		}

		ctx := lcli.ReqContext(cctx)
		dep, err := deps.GetDepsCLI(ctx, cctx)
		if err != nil {
			return err
		}

		/*
			create table sectors_sdr_pipeline (
			    sp_id bigint not null,
			    sector_number bigint not null,

			    -- at request time
			    create_time timestamp not null,
			    reg_seal_proof int not null,
			    comm_d_cid text not null,

			    [... other not relevant fields]
		*/

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

		num, err := lpseal.AllocateSectorNumbers(ctx, dep.Full, dep.DB, act, cctx.Int("count"), func(tx *harmonydb.Tx, numbers []abi.SectorNumber) (bool, error) {
			for _, n := range numbers {
				_, err := tx.Exec("insert into sectors_sdr_pipeline (sp_id, sector_number, reg_seal_proof) values ($1, $2, $3)", mid, n, spt)
				if err != nil {
					return false, xerrors.Errorf("inserting into sectors_sdr_pipeline: %w", err)
				}
			}
			return true, nil
		})
		if err != nil {
			return xerrors.Errorf("allocating sector numbers: %w", err)
		}

		for _, number := range num {
			fmt.Println(number)
		}

		return nil
	},
}
