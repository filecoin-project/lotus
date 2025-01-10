package worker

import (
	"context"
	"strings"

	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/api"
	lcli "github.com/filecoin-project/lotus/cli"
	"github.com/filecoin-project/lotus/storage/sealer/sealtasks"
)

var tasksCmd = &cli.Command{
	Name:  "tasks",
	Usage: "Manage task processing",
	Subcommands: []*cli.Command{
		tasksEnableCmd,
		tasksDisableCmd,
	},
}

var allowSetting = map[sealtasks.TaskType]struct{}{
	sealtasks.TTAddPiece:            {},
	sealtasks.TTDataCid:             {},
	sealtasks.TTPreCommit1:          {},
	sealtasks.TTPreCommit2:          {},
	sealtasks.TTCommit2:             {},
	sealtasks.TTUnseal:              {},
	sealtasks.TTReplicaUpdate:       {},
	sealtasks.TTProveReplicaUpdate2: {},
	sealtasks.TTRegenSectorKey:      {},
}

var settableStr = func() string {
	var s []string
	for _, tt := range ttList(allowSetting) {
		s = append(s, tt.Short())
	}
	return strings.Join(s, "|")
}()

var tasksEnableCmd = &cli.Command{
	Name:      "enable",
	Usage:     "Enable a task type",
	ArgsUsage: "--all | [" + settableStr + "]",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "all",
			Usage: "Enable all task types",
		},
	},
	Action: taskAction(api.Worker.TaskEnable),
}

var tasksDisableCmd = &cli.Command{
	Name:      "disable",
	Usage:     "Disable a task type",
	ArgsUsage: "--all | [" + settableStr + "]",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "all",
			Usage: "Disable all task types",
		},
	},
	Action: taskAction(api.Worker.TaskDisable),
}

func taskAction(tf func(a api.Worker, ctx context.Context, tt sealtasks.TaskType) error) func(cctx *cli.Context) error {
	return func(cctx *cli.Context) error {
		allFlag := cctx.Bool("all")

		if cctx.NArg() == 1 && allFlag {
			return xerrors.Errorf("Cannot use --all flag with task type argument")
		}

		if cctx.NArg() != 1 && !allFlag {
			return xerrors.Errorf("Expected 1 argument or use --all flag")
		}

		var tt sealtasks.TaskType
		if cctx.NArg() == 1 {
			for taskType := range allowSetting {
				if taskType.Short() == cctx.Args().First() {
					tt = taskType
					break
				}
			}

			if tt == "" {
				return xerrors.Errorf("unknown task type '%s'", cctx.Args().First())
			}
		}

		api, closer, err := lcli.GetWorkerAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		ctx := lcli.ReqContext(cctx)

		if allFlag {
			for taskType := range allowSetting {
				if err := tf(api, ctx, taskType); err != nil {
					return err
				}
			}
			return nil
		}

		return tf(api, ctx, tt)
	}
}
