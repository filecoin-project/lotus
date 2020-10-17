package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"golang.org/x/xerrors"

	"github.com/fatih/color"
	"github.com/urfave/cli/v2"

	"github.com/filecoin-project/lotus/extern/sector-storage/storiface"

	"github.com/filecoin-project/lotus/chain/types"
	lcli "github.com/filecoin-project/lotus/cli"
)

var sealingCmd = &cli.Command{
	Name:  "sealing",
	Usage: "interact with sealing pipeline",
	Subcommands: []*cli.Command{
		sealingJobsCmd,
		sealingWorkersCmd,
		sealingSchedDiagCmd,
	},
}

var sealingWorkersCmd = &cli.Command{
	Name:  "workers",
	Usage: "list workers",
	Flags: []cli.Flag{
		&cli.BoolFlag{Name: "color"},
	},
	Action: func(cctx *cli.Context) error {
		color.NoColor = !cctx.Bool("color")

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

		type sortableStat struct {
			id uint64
			storiface.WorkerStats
		}

		st := make([]sortableStat, 0, len(stats))
		for id, stat := range stats {
			st = append(st, sortableStat{id, stat})
		}

		sort.Slice(st, func(i, j int) bool {
			return st[i].id < st[j].id
		})

		for _, stat := range st {
			gpuUse := "not "
			gpuCol := color.FgBlue
			if stat.GpuUsed {
				gpuCol = color.FgGreen
				gpuUse = ""
			}

			fmt.Printf("Worker %d, host %s\n", stat.id, color.MagentaString(stat.Info.Hostname))

			var barCols = uint64(64)
			cpuBars := int(stat.CpuUse * barCols / stat.Info.Resources.CPUs)
			cpuBar := strings.Repeat("|", cpuBars) + strings.Repeat(" ", int(barCols)-cpuBars)

			fmt.Printf("\tCPU:  [%s] %d/%d core(s) in use\n",
				color.GreenString(cpuBar), stat.CpuUse, stat.Info.Resources.CPUs)

			ramBarsRes := int(stat.Info.Resources.MemReserved * barCols / stat.Info.Resources.MemPhysical)
			ramBarsUsed := int(stat.MemUsedMin * barCols / stat.Info.Resources.MemPhysical)
			ramBar := color.YellowString(strings.Repeat("|", ramBarsRes)) +
				color.GreenString(strings.Repeat("|", ramBarsUsed)) +
				strings.Repeat(" ", int(barCols)-ramBarsUsed-ramBarsRes)

			vmem := stat.Info.Resources.MemPhysical + stat.Info.Resources.MemSwap

			vmemBarsRes := int(stat.Info.Resources.MemReserved * barCols / vmem)
			vmemBarsUsed := int(stat.MemUsedMax * barCols / vmem)
			vmemBar := color.YellowString(strings.Repeat("|", vmemBarsRes)) +
				color.GreenString(strings.Repeat("|", vmemBarsUsed)) +
				strings.Repeat(" ", int(barCols)-vmemBarsUsed-vmemBarsRes)

			fmt.Printf("\tRAM:  [%s] %d%% %s/%s\n", ramBar,
				(stat.Info.Resources.MemReserved+stat.MemUsedMin)*100/stat.Info.Resources.MemPhysical,
				types.SizeStr(types.NewInt(stat.Info.Resources.MemReserved+stat.MemUsedMin)),
				types.SizeStr(types.NewInt(stat.Info.Resources.MemPhysical)))

			fmt.Printf("\tVMEM: [%s] %d%% %s/%s\n", vmemBar,
				(stat.Info.Resources.MemReserved+stat.MemUsedMax)*100/vmem,
				types.SizeStr(types.NewInt(stat.Info.Resources.MemReserved+stat.MemUsedMax)),
				types.SizeStr(types.NewInt(vmem)))

			for _, gpu := range stat.Info.Resources.GPUs {
				fmt.Printf("\tGPU: %s\n", color.New(gpuCol).Sprintf("%s, %sused", gpu, gpuUse))
			}
		}

		return nil
	},
}

var sealingJobsCmd = &cli.Command{
	Name:  "jobs",
	Usage: "list workers",
	Flags: []cli.Flag{
		&cli.BoolFlag{Name: "color"},
	},
	Action: func(cctx *cli.Context) error {
		color.NoColor = !cctx.Bool("color")

		nodeApi, closer, err := lcli.GetStorageMinerAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		ctx := lcli.ReqContext(cctx)

		jobs, err := nodeApi.WorkerJobs(ctx)
		if err != nil {
			return xerrors.Errorf("getting worker jobs: %w", err)
		}

		type line struct {
			storiface.WorkerJob
			wid uint64
		}

		lines := make([]line, 0)

		for wid, jobs := range jobs {
			for _, job := range jobs {
				lines = append(lines, line{
					WorkerJob: job,
					wid:       wid,
				})
			}
		}

		// oldest first
		sort.Slice(lines, func(i, j int) bool {
			if lines[i].RunWait != lines[j].RunWait {
				return lines[i].RunWait < lines[j].RunWait
			}
			return lines[i].Start.Before(lines[j].Start)
		})

		workerHostnames := map[uint64]string{}

		wst, err := nodeApi.WorkerStats(ctx)
		if err != nil {
			return xerrors.Errorf("getting worker stats: %w", err)
		}

		for wid, st := range wst {
			workerHostnames[wid] = st.Info.Hostname
		}

		tw := tabwriter.NewWriter(os.Stdout, 2, 4, 2, ' ', 0)
		_, _ = fmt.Fprintf(tw, "ID\tSector\tWorker\tHostname\tTask\tState\tTime\n")

		for _, l := range lines {
			state := "running"
			if l.RunWait != 0 {
				state = fmt.Sprintf("assigned(%d)", l.RunWait-1)
			}
			_, _ = fmt.Fprintf(tw, "%d\t%d\t%d\t%s\t%s\t%s\t%s\n", l.ID, l.Sector.Number, l.wid, workerHostnames[l.wid], l.Task.Short(), state, time.Now().Sub(l.Start).Truncate(time.Millisecond*100))
		}

		return tw.Flush()
	},
}

var sealingSchedDiagCmd = &cli.Command{
	Name:  "sched-diag",
	Usage: "Dump internal scheduler state",
	Action: func(cctx *cli.Context) error {
		nodeApi, closer, err := lcli.GetStorageMinerAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		ctx := lcli.ReqContext(cctx)

		st, err := nodeApi.SealingSchedDiag(ctx)
		if err != nil {
			return err
		}

		j, err := json.MarshalIndent(&st, "", "  ")
		if err != nil {
			return err
		}

		fmt.Println(string(j))

		return nil
	},
}
