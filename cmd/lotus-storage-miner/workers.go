package main

import (
	"fmt"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/types"
	"gopkg.in/urfave/cli.v2"
	"sort"

	lcli "github.com/filecoin-project/lotus/cli"
)

var workersCmd = &cli.Command{
	Name:  "workers",
	Usage: "interact with workers",
	Subcommands: []*cli.Command{
		workersListCmd,
	},
}

var workersListCmd = &cli.Command{
	Name:  "list",
	Usage: "list workers",
	Action: func(cctx *cli.Context) error {
		nodeApi, closer, err := lcli.GetStorageMinerAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		ctx := lcli.ReqContext(cctx)

		stats, err := nodeApi.WorkerStats(ctx)
		if err != nil {
			return err
		}

		st := make([]struct {
			id uint64
			api.WorkerStats
		}, 0, len(stats))
		for id, stat := range stats {
			st = append(st, struct {
				id uint64
				api.WorkerStats
			}{id, stat})
		}

		sort.Slice(st, func(i, j int) bool {
			return st[i].id < st[j].id
		})

		for _, stat := range st {
			gpuUse := "not "
			if stat.GpuUsed {
				gpuUse = ""
			}

			fmt.Printf("Worker %d, host %s\n", stat.id, stat.Info.Hostname)

			if stat.CpuUse != -1 {
				fmt.Printf("\tCPU: %d core(s) in use\n", stat.CpuUse)
			} else {
				fmt.Printf("\tCPU: all cores in use\n")
			}

			for _, gpu := range stat.Info.Resources.GPUs {
				fmt.Printf("\tGPU: %s, %sused\n", gpu, gpuUse)
			}

			fmt.Printf("\tMemory: System: Physical %s, Swap %s, Reserved %s (%d%% phys)\n",
				types.SizeStr(types.NewInt(stat.Info.Resources.MemPhysical)),
				types.SizeStr(types.NewInt(stat.Info.Resources.MemSwap)),
				types.SizeStr(types.NewInt(stat.Info.Resources.MemReserved)),
				stat.Info.Resources.MemReserved*100/stat.Info.Resources.MemPhysical)

			fmt.Printf("\t\tUsed: Physical %s (%d%% phys), Virtual %s (%d%% phys, %d%% virt)\n",
				types.SizeStr(types.NewInt(stat.MemUsedMin)),
				stat.MemUsedMin*100/stat.Info.Resources.MemPhysical,
				types.SizeStr(types.NewInt(stat.MemUsedMax)),
				stat.MemUsedMax*100/stat.Info.Resources.MemPhysical,
				stat.MemUsedMax*100/(stat.Info.Resources.MemPhysical+stat.Info.Resources.MemSwap))
		}

		return nil
	},
}
