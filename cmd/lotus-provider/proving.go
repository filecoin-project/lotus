package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/dline"

	"github.com/filecoin-project/lotus/lib/harmony/harmonydb"
	"github.com/filecoin-project/lotus/provider"
)

var provingCmd = &cli.Command{
	Name:  "proving",
	Usage: "Utility functions for proving sectors",
	Subcommands: []*cli.Command{
		//provingInfoCmd,
		provingCompute,
	},
}

var provingCompute = &cli.Command{
	Name:  "compute",
	Usage: "Compute a proof-of-spacetime for a sector (requires the sector to be pre-sealed)",
	Subcommands: []*cli.Command{
		provingComputeWindowPoStCmd,
		scheduleWindowPostCmd,
	},
}

var scheduleWindowPostCmd = &cli.Command{
	Name:  "test-window-post",
	Usage: "FOR TESTING: a way to test the windowpost scheduler. Use with 1 lotus-provider running only",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name: "i_have_set_LOTUS_PROVDER_NO_SEND_to_true",
		},
		&cli.Uint64Flag{
			Name:  "deadline",
			Usage: "deadline to compute WindowPoSt for ",
			Value: 0,
		},
		&cli.StringSliceFlag{
			Name:  "layers",
			Usage: "list of layers to be interpreted (atop defaults). Default: base",
			Value: cli.NewStringSlice("base"),
		},
	},
	Action: func(cctx *cli.Context) error {
		ctx := context.Background()

		if !cctx.Bool("i_have_set_LOTUS_PROVDER_NO_SEND_to_true") {
			log.Info("This command is for testing only. It will not send any messages to the chain. If you want to run it, set LOTUS_PROVDER_NO_SEND=true on the lotus-provider environment.")
			return nil
		}
		deps, err := getDeps(ctx, cctx)
		if err != nil {
			return err
		}

		ts, err := deps.full.ChainHead(ctx)
		if err != nil {
			return xerrors.Errorf("cannot get chainhead %w", err)
		}

		ht := ts.Height()
		maddr, err := address.IDFromAddress(address.Address(deps.maddrs[0]))
		if err != nil {
			return xerrors.Errorf("cannot get miner id %w", err)
		}
		_ = ht
		deps.db.BeginTransaction(ctx, func(tx *harmonydb.Tx) (commit bool, err error) {
			_, err = tx.Exec(`INSERT INTO harmony_task (name) VALUES ('WdPost')`)
			if err != nil {
				return false, xerrors.Errorf("inserting harmony_task: %w", err)
			}
			var id int64
			if err = tx.QueryRow(`SELECT id FROM harmony_task ORDER BY update_time DESC LIMIT 1`).Scan(&id); err != nil {
				return false, xerrors.Errorf("getting inserted id: %w", err)
			}
			_, err = tx.Exec(`INSERT INTO wdpost_partition_tasks 
			(task_id, sp_id, proving_period_start, deadline_index, partition_index) VALUES ($1, $2, $3, $4, $5)`,
				id, maddr, ht, 1, cctx.Uint64("deadline"), 0)
			if err != nil {
				return false, xerrors.Errorf("inserting wdpost_partition_tasks: %w", err)
			}
			return true, nil
		})
		return nil
	},
}

var provingComputeWindowPoStCmd = &cli.Command{
	Name:    "windowed-post",
	Aliases: []string{"window-post"},
	Usage:   "Compute WindowPoSt for performance and configuration testing.",
	Description: `Note: This command is intended to be used to verify PoSt compute performance.
It will not send any messages to the chain. Since it can compute any deadline, output may be incorrectly timed for the chain.`,
	ArgsUsage: "[deadline index]",
	Flags: []cli.Flag{
		&cli.Uint64Flag{
			Name:  "deadline",
			Usage: "deadline to compute WindowPoSt for ",
			Value: 0,
		},
		&cli.StringSliceFlag{
			Name:  "layers",
			Usage: "list of layers to be interpreted (atop defaults). Default: base",
			Value: cli.NewStringSlice("base"),
		},
		&cli.StringFlag{
			Name:  "storage-json",
			Usage: "path to json file containing storage config",
			Value: "~/.lotus-provider/storage.json",
		},
		&cli.Uint64Flag{
			Name:  "partition",
			Usage: "partition to compute WindowPoSt for",
			Value: 0,
		},
	},
	Action: func(cctx *cli.Context) error {

		ctx := context.Background()
		deps, err := getDeps(ctx, cctx)
		if err != nil {
			return err
		}

		wdPostTask, wdPoStSubmitTask, derlareRecoverTask, err := provider.WindowPostScheduler(ctx, deps.cfg.Fees, deps.cfg.Proving, deps.full, deps.verif, deps.lw,
			deps.as, deps.maddrs, deps.db, deps.stor, deps.si, deps.cfg.Subsystems.WindowPostMaxTasks)
		if err != nil {
			return err
		}
		_, _ = wdPoStSubmitTask, derlareRecoverTask

		if len(deps.maddrs) == 0 {
			return errors.New("no miners to compute WindowPoSt for")
		}
		head, err := deps.full.ChainHead(ctx)
		if err != nil {
			return xerrors.Errorf("failed to get chain head: %w", err)
		}

		di := dline.NewInfo(head.Height(), cctx.Uint64("deadline"), 0, 0, 0, 10 /*challenge window*/, 0, 0)

		for _, maddr := range deps.maddrs {
			out, err := wdPostTask.DoPartition(ctx, head, address.Address(maddr), di, cctx.Uint64("partition"))
			if err != nil {
				fmt.Println("Error computing WindowPoSt for miner", maddr, err)
				continue
			}
			fmt.Println("Computed WindowPoSt for miner", maddr, ":")
			err = json.NewEncoder(os.Stdout).Encode(out)
			if err != nil {
				fmt.Println("Could not encode WindowPoSt output for miner", maddr, err)
				continue
			}
		}

		return nil
	},
}
