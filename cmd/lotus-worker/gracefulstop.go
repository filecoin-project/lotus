package main

import (
	"context"
	"fmt"
	"time"

	"github.com/urfave/cli/v2"

	"github.com/filecoin-project/lotus/api"
	lcli "github.com/filecoin-project/lotus/cli"
	"github.com/filecoin-project/lotus/storage/sealer/sealtasks"
)

var gracefulStopCmd = &cli.Command{
	Name:  "gracefully",
	Usage: "Gracefully stop a running lotus worker",
	Flags: []cli.Flag{
		&cli.IntFlag{
			Name:  "checks",
			Usage: "Number of consecutive checks (every second without a running tasks) we wait before stopping the worker (default: 30))",
			Value: 30,
		},
	},
	Action: gracefulStopCmdAct,
}

func gracefulStopCmdAct(cctx *cli.Context) error {
	//Disable all tasks
	tasksDisableAction := taskDisableAll(api.Worker.TaskDisable)
	err := tasksDisableAction(cctx)
	if err != nil {
		return err
	}

	// Wait for current tasks to finish
	fmt.Println("Waiting for tasks to finish...")
	api, closer, err := lcli.GetWorkerAPI(cctx)
	if err != nil {
		return err
	}
	defer closer()

	ctx := lcli.ReqContext(cctx)

	consecutiveChecks := cctx.Int("checks")
	checks := 0
	for {
		start := time.Now()
		err := api.WaitQuiet(ctx)
		if err != nil {
			return err
		}

		elapsed := time.Since(start)
		if elapsed < time.Second {
			checks++
			if checks >= consecutiveChecks {
				break
			}
		} else {
			// Reset if we encounter a slow check
			checks = 0
		}
	}

	//Stop the worker
	fmt.Println("Stopping the worker...")
	err = api.StorageDetachAll(ctx)
	if err != nil {
		return err
	}

	err = api.Shutdown(ctx)
	if err != nil {
		return err
	}

	fmt.Println("Worker has been stopped gracefully.")
	return nil
}

func taskDisableAll(tf func(a api.Worker, ctx context.Context, tt sealtasks.TaskType) error) func(cctx *cli.Context) error {
	return func(cctx *cli.Context) error {
		api, closer, err := lcli.GetWorkerAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		ctx := lcli.ReqContext(cctx)

		// Disable all tasks
		for taskType := range allowSetting {
			if err := tf(api, ctx, taskType); err != nil {
				return err
			}
		}

		return nil
	}
}
