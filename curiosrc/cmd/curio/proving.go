package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/dline"

	curio "github.com/filecoin-project/lotus/curiosrc"
	"github.com/filecoin-project/lotus/curiosrc/deps"
	"github.com/filecoin-project/lotus/lib/harmony/harmonydb"
)

var testCmd = &cli.Command{
	Name:  "test",
	Usage: "Utility functions for testing",
	Subcommands: []*cli.Command{
		//provingInfoCmd,
		wdPostCmd,
	},
	Before: func(cctx *cli.Context) error {
		return nil
	},
}

var wdPostCmd = &cli.Command{
	Name:    "window-post",
	Aliases: []string{"wd", "windowpost", "wdpost"},
	Usage:   "Compute a proof-of-spacetime for a sector (requires the sector to be pre-sealed). These will not send to the chain.",
	Subcommands: []*cli.Command{
		wdPostHereCmd,
		wdPostTaskCmd,
	},
}

// wdPostTaskCmd writes to harmony_task and wdpost_partition_tasks, then waits for the result.
// It is intended to be used to test the windowpost scheduler.
// The end of the compute task puts the task_id onto wdpost_proofs, which is read by the submit task.
// The submit task will not send test tasks to the chain, and instead will write the result to harmony_test.
// The result is read by this command, and printed to stdout.
var wdPostTaskCmd = &cli.Command{
	Name:    "task",
	Aliases: []string{"scheduled", "schedule", "async", "asynchronous"},
	Usage:   "Test the windowpost scheduler by running it on the next available curio. ",
	Flags: []cli.Flag{
		&cli.Uint64Flag{
			Name:  "deadline",
			Usage: "deadline to compute WindowPoSt for ",
			Value: 0,
		},
		&cli.StringSliceFlag{
			Name:  "layers",
			Usage: "list of layers to be interpreted (atop defaults). Default: base",
		},
	},
	Action: func(cctx *cli.Context) error {
		ctx := context.Background()

		deps, err := deps.GetDeps(ctx, cctx)
		if err != nil {
			return xerrors.Errorf("get config: %w", err)
		}

		ts, err := deps.Full.ChainHead(ctx)
		if err != nil {
			return xerrors.Errorf("cannot get chainhead %w", err)
		}
		ht := ts.Height()

		// It's not important to be super-accurate as it's only for basic testing.
		addr, err := address.NewFromString(deps.Cfg.Addresses[0].MinerAddresses[0])
		if err != nil {
			return xerrors.Errorf("cannot get miner address %w", err)
		}
		maddr, err := address.IDFromAddress(addr)
		if err != nil {
			return xerrors.Errorf("cannot get miner id %w", err)
		}
		var taskId int64

		_, err = deps.DB.BeginTransaction(ctx, func(tx *harmonydb.Tx) (commit bool, err error) {
			err = tx.QueryRow(`INSERT INTO harmony_task (name, posted_time, added_by) VALUES ('WdPost', CURRENT_TIMESTAMP, 123) RETURNING id`).Scan(&taskId)
			if err != nil {
				log.Error("inserting harmony_task: ", err)
				return false, xerrors.Errorf("inserting harmony_task: %w", err)
			}
			_, err = tx.Exec(`INSERT INTO wdpost_partition_tasks 
			(task_id, sp_id, proving_period_start, deadline_index, partition_index) VALUES ($1, $2, $3, $4, $5)`,
				taskId, maddr, ht, cctx.Uint64("deadline"), 0)
			if err != nil {
				log.Error("inserting wdpost_partition_tasks: ", err)
				return false, xerrors.Errorf("inserting wdpost_partition_tasks: %w", err)
			}
			_, err = tx.Exec("INSERT INTO harmony_test (task_id) VALUES ($1)", taskId)
			if err != nil {
				return false, xerrors.Errorf("inserting into harmony_tests: %w", err)
			}
			return true, nil
		}, harmonydb.OptionRetry())
		if err != nil {
			return xerrors.Errorf("writing SQL transaction: %w", err)
		}
		fmt.Printf("Inserted task %v. Waiting for success ", taskId)
		var result sql.NullString
		for {
			time.Sleep(time.Second)
			err = deps.DB.QueryRow(ctx, `SELECT result FROM harmony_test WHERE task_id=$1`, taskId).Scan(&result)
			if err != nil {
				return xerrors.Errorf("reading result from harmony_test: %w", err)
			}
			if result.Valid {
				break
			}
			fmt.Print(".")
		}
		fmt.Println()
		log.Infof("Result: %s", result.String)
		return nil
	},
}

// This command is intended to be used to verify PoSt compute performance.
// It will not send any messages to the chain. Since it can compute any deadline, output may be incorrectly timed for the chain.
// The entire processing happens in this process while you wait. It does not use the scheduler.
var wdPostHereCmd = &cli.Command{
	Name:    "here",
	Aliases: []string{"cli"},
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
		},
		&cli.StringFlag{
			Name:  "storage-json",
			Usage: "path to json file containing storage config",
			Value: "~/.curio/storage.json",
		},
		&cli.Uint64Flag{
			Name:  "partition",
			Usage: "partition to compute WindowPoSt for",
			Value: 0,
		},
	},
	Action: func(cctx *cli.Context) error {

		ctx := context.Background()
		deps, err := deps.GetDeps(ctx, cctx)
		if err != nil {
			return err
		}

		wdPostTask, wdPoStSubmitTask, derlareRecoverTask, err := curio.WindowPostScheduler(
			ctx, deps.Cfg.Fees, deps.Cfg.Proving, deps.Full, deps.Verif, nil, nil,
			deps.As, deps.Maddrs, deps.DB, deps.Stor, deps.Si, deps.Cfg.Subsystems.WindowPostMaxTasks)
		if err != nil {
			return err
		}
		_, _ = wdPoStSubmitTask, derlareRecoverTask

		if len(deps.Maddrs) == 0 {
			return errors.New("no miners to compute WindowPoSt for")
		}
		head, err := deps.Full.ChainHead(ctx)
		if err != nil {
			return xerrors.Errorf("failed to get chain head: %w", err)
		}

		di := dline.NewInfo(head.Height(), cctx.Uint64("deadline"), 0, 0, 0, 10 /*challenge window*/, 0, 0)

		for maddr := range deps.Maddrs {
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
