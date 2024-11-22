package miner

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/fatih/color"
	"github.com/google/uuid"
	"github.com/mitchellh/go-homedir"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-padreader"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/chain/types"
	lcli "github.com/filecoin-project/lotus/cli"
	"github.com/filecoin-project/lotus/lib/httpreader"
	"github.com/filecoin-project/lotus/storage/sealer/sealtasks"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
)

var sealingCmd = &cli.Command{
	Name:  "sealing",
	Usage: "interact with sealing pipeline",
	Subcommands: []*cli.Command{
		sealingJobsCmd,
		workersCmd(true),
		sealingSchedDiagCmd,
		sealingAbortCmd,
		sealingDataCidCmd,
	},
}

func workersCmd(sealing bool) *cli.Command {
	return &cli.Command{
		Name:  "workers",
		Usage: "list workers",
		Action: func(cctx *cli.Context) error {
			minerApi, closer, err := lcli.GetStorageMinerAPI(cctx)
			if err != nil {
				return err
			}
			defer closer()

			ctx := lcli.ReqContext(cctx)

			stats, err := minerApi.WorkerStats(ctx)
			if err != nil {
				return err
			}

			type sortableStat struct {
				id uuid.UUID
				storiface.WorkerStats
			}

			st := make([]sortableStat, 0, len(stats))
			for id, stat := range stats {
				if len(stat.Tasks) > 0 {
					if (stat.Tasks[0].WorkerType() != sealtasks.WorkerSealing) == sealing {
						continue
					}
				}

				st = append(st, sortableStat{id, stat})
			}

			sort.Slice(st, func(i, j int) bool {
				return st[i].id.String() < st[j].id.String()
			})

			/*
				Example output:

				Worker c4d65451-07f8-4230-98ad-4f33dea2a8cc, host myhostname
				        TASK: PC1(1/4) AP(15/15) GET(3)
				        CPU:  [||||||||                                                        ] 16/128 core(s) in use
				        RAM:  [||||||||                                                        ] 12% 125.8 GiB/1008 GiB
				        VMEM: [||||||||                                                        ] 12% 125.8 GiB/1008 GiB
				        GPU:  [                                                                ] 0% 0.00/1 gpu(s) in use
				        GPU: NVIDIA GeForce RTX 3090, not used
			*/

			for _, stat := range st {
				// Worker uuid + name

				var disabled string
				if !stat.Enabled {
					disabled = color.RedString(" (disabled)")
				}

				fmt.Printf("Worker %s, host %s%s\n", stat.id, color.MagentaString(stat.Info.Hostname), disabled)

				// Task counts
				tc := make([][]string, 0, len(stat.TaskCounts))

				for st, c := range stat.TaskCounts {
					if c == 0 {
						continue
					}

					stt, err := sealtasks.SttFromString(st)
					if err != nil {
						return err
					}

					str := fmt.Sprint(c)
					if max := stat.Info.Resources.ResourceSpec(stt.RegisteredSealProof, stt.TaskType).MaxConcurrent; max > 0 {
						switch {
						case c < max:
							str = color.GreenString(str)
						case c >= max:
							str = color.YellowString(str)
						}
						str = fmt.Sprintf("%s/%d", str, max)
					} else {
						str = color.CyanString(str)
					}
					str = fmt.Sprintf("%s(%s)", color.BlueString(stt.Short()), str)

					tc = append(tc, []string{string(stt.TaskType), str})
				}
				sort.Slice(tc, func(i, j int) bool {
					return sealtasks.TaskType(tc[i][0]).Less(sealtasks.TaskType(tc[j][0]))
				})
				var taskStr string
				for _, t := range tc {
					taskStr += t[1] + " "
				}
				if taskStr != "" {
					fmt.Printf("\tTASK: %s\n", taskStr)
				}

				// CPU use

				fmt.Printf("\tCPU:  [%s] %d/%d core(s) in use\n",
					lcli.BarString(float64(stat.Info.Resources.CPUs), 0, float64(stat.CpuUse)), stat.CpuUse, stat.Info.Resources.CPUs)

				// RAM use

				ramTotal := stat.Info.Resources.MemPhysical
				ramTasks := stat.MemUsedMin
				ramUsed := stat.Info.Resources.MemUsed
				var ramReserved uint64
				if ramUsed > ramTasks {
					ramReserved = ramUsed - ramTasks
				}
				ramBar := lcli.BarString(float64(ramTotal), float64(ramReserved), float64(ramTasks))

				fmt.Printf("\tRAM:  [%s] %d%% %s/%s\n", ramBar,
					(ramTasks+ramReserved)*100/stat.Info.Resources.MemPhysical,
					types.SizeStr(types.NewInt(ramTasks+ramUsed)),
					types.SizeStr(types.NewInt(stat.Info.Resources.MemPhysical)))

				// VMEM use (ram+swap)

				vmemTotal := stat.Info.Resources.MemPhysical + stat.Info.Resources.MemSwap
				vmemTasks := stat.MemUsedMax
				vmemUsed := stat.Info.Resources.MemUsed + stat.Info.Resources.MemSwapUsed
				var vmemReserved uint64
				if vmemUsed > vmemTasks {
					vmemReserved = vmemUsed - vmemTasks
				}
				vmemBar := lcli.BarString(float64(vmemTotal), float64(vmemReserved), float64(vmemTasks))

				fmt.Printf("\tVMEM: [%s] %d%% %s/%s\n", vmemBar,
					(vmemTasks+vmemReserved)*100/vmemTotal,
					types.SizeStr(types.NewInt(vmemTasks+vmemReserved)),
					types.SizeStr(types.NewInt(vmemTotal)))

				// GPU use

				if len(stat.Info.Resources.GPUs) > 0 {
					gpuBar := lcli.BarString(float64(len(stat.Info.Resources.GPUs)), 0, stat.GpuUsed)
					fmt.Printf("\tGPU:  [%s] %.f%% %.2f/%d gpu(s) in use\n", color.GreenString(gpuBar),
						stat.GpuUsed*100/float64(len(stat.Info.Resources.GPUs)),
						stat.GpuUsed, len(stat.Info.Resources.GPUs))
				}

				gpuUse := "not "
				gpuCol := color.FgBlue
				if stat.GpuUsed > 0 {
					gpuCol = color.FgGreen
					gpuUse = ""
				}
				for _, gpu := range stat.Info.Resources.GPUs {
					fmt.Printf("\tGPU: %s\n", color.New(gpuCol).Sprintf("%s, %sused", gpu, gpuUse))
				}
			}

			return nil
		},
	}
}

var sealingJobsCmd = &cli.Command{
	Name:  "jobs",
	Usage: "list running jobs",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "show-ret-done",
			Usage: "show returned but not consumed calls",
		},
	},
	Action: func(cctx *cli.Context) error {
		minerApi, closer, err := lcli.GetStorageMinerAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		ctx := lcli.ReqContext(cctx)

		jobs, err := minerApi.WorkerJobs(ctx)
		if err != nil {
			return xerrors.Errorf("getting worker jobs: %w", err)
		}

		type line struct {
			storiface.WorkerJob
			wid uuid.UUID
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
			if lines[i].Start.Equal(lines[j].Start) {
				return lines[i].ID.ID.String() < lines[j].ID.ID.String()
			}
			return lines[i].Start.Before(lines[j].Start)
		})

		workerHostnames := map[uuid.UUID]string{}

		wst, err := minerApi.WorkerStats(ctx)
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
			switch {
			case l.RunWait > 1:
				state = fmt.Sprintf("assigned(%d)", l.RunWait-1)
			case l.RunWait == storiface.RWPrepared:
				state = "prepared"
			case l.RunWait == storiface.RWRetDone:
				if !cctx.Bool("show-ret-done") {
					continue
				}
				state = "ret-done"
			case l.RunWait == storiface.RWReturned:
				state = "returned"
			case l.RunWait == storiface.RWRetWait:
				state = "ret-wait"
			}
			dur := "n/a"
			if !l.Start.IsZero() {
				dur = time.Since(l.Start).Truncate(time.Millisecond * 100).String()
			}

			hostname, ok := workerHostnames[l.wid]
			if !ok {
				hostname = l.Hostname
			}

			_, _ = fmt.Fprintf(tw, "%s\t%d\t%s\t%s\t%s\t%s\t%s\n",
				hex.EncodeToString(l.ID.ID[:4]),
				l.Sector.Number,
				hex.EncodeToString(l.wid[:4]),
				hostname,
				l.Task.Short(),
				state,
				dur)
		}

		return tw.Flush()
	},
}

var sealingSchedDiagCmd = &cli.Command{
	Name:  "sched-diag",
	Usage: "Dump internal scheduler state",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name: "force-sched",
		},
	},
	Action: func(cctx *cli.Context) error {
		minerApi, closer, err := lcli.GetStorageMinerAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		ctx := lcli.ReqContext(cctx)

		st, err := minerApi.SealingSchedDiag(ctx, cctx.Bool("force-sched"))
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

var sealingAbortCmd = &cli.Command{
	Name:      "abort",
	Usage:     "Abort a running job",
	ArgsUsage: "[callid]",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "sched",
			Usage: "Specifies that the argument is UUID of the request to be removed from scheduler",
		},
	},
	Action: func(cctx *cli.Context) error {
		if cctx.NArg() != 1 {
			return lcli.IncorrectNumArgs(cctx)
		}

		minerApi, closer, err := lcli.GetStorageMinerAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		ctx := lcli.ReqContext(cctx)

		if cctx.Bool("sched") {
			err = minerApi.SealingRemoveRequest(ctx, uuid.Must(uuid.Parse(cctx.Args().First())))
			if err != nil {
				return xerrors.Errorf("Failed to removed the request with UUID %s: %w", cctx.Args().First(), err)
			}
			return nil
		}

		jobs, err := minerApi.WorkerJobs(ctx)
		if err != nil {
			return xerrors.Errorf("getting worker jobs: %w", err)
		}

		var job *storiface.WorkerJob
	outer:
		for _, workerJobs := range jobs {
			for _, j := range workerJobs {
				if strings.HasPrefix(j.ID.ID.String(), cctx.Args().First()) {
					j := j
					job = &j
					break outer
				}
			}
		}

		if job == nil {
			return xerrors.Errorf("job with specified id prefix not found")
		}

		fmt.Printf("aborting job %s, task %s, sector %d, running on host %s\n", job.ID.String(), job.Task.Short(), job.Sector.Number, job.Hostname)

		return minerApi.SealingAbort(ctx, job.ID)
	},
}

var sealingDataCidCmd = &cli.Command{
	Name:      "data-cid",
	Usage:     "Compute data CID using workers",
	ArgsUsage: "[file/url] <padded piece size>",
	Flags: []cli.Flag{
		&cli.Uint64Flag{
			Name:  "file-size",
			Usage: "real file size",
		},
	},
	Action: func(cctx *cli.Context) error {
		if cctx.NArg() < 1 || cctx.NArg() > 2 {
			return lcli.ShowHelp(cctx, xerrors.Errorf("expected 1 or 2 arguments"))
		}

		minerApi, closer, err := lcli.GetStorageMinerAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		ctx := lcli.ReqContext(cctx)

		var r io.Reader
		sz := cctx.Uint64("file-size")

		if strings.HasPrefix(cctx.Args().First(), "http://") || strings.HasPrefix(cctx.Args().First(), "https://") {
			r = &httpreader.HttpReader{
				URL: cctx.Args().First(),
			}

			if !cctx.IsSet("file-size") {
				resp, err := http.Head(cctx.Args().First())
				if err != nil {
					return xerrors.Errorf("http head: %w", err)
				}

				if resp.ContentLength < 0 {
					return xerrors.Errorf("head response didn't contain content length; specify --file-size")
				}
				sz = uint64(resp.ContentLength)
			}
		} else {
			p, err := homedir.Expand(cctx.Args().First())
			if err != nil {
				return xerrors.Errorf("expanding path: %w", err)
			}

			f, err := os.OpenFile(p, os.O_RDONLY, 0)
			if err != nil {
				return xerrors.Errorf("opening source file: %w", err)
			}

			if !cctx.IsSet("file-size") {
				st, err := f.Stat()
				if err != nil {
					return xerrors.Errorf("stat: %w", err)
				}
				sz = uint64(st.Size())
			}

			r = f
		}

		var psize abi.PaddedPieceSize
		if cctx.NArg() == 2 {
			rps, err := humanize.ParseBytes(cctx.Args().Get(1))
			if err != nil {
				return xerrors.Errorf("parsing piece size: %w", err)
			}
			psize = abi.PaddedPieceSize(rps)
			if err := psize.Validate(); err != nil {
				return xerrors.Errorf("checking piece size: %w", err)
			}
			if sz > uint64(psize.Unpadded()) {
				return xerrors.Errorf("file larger than the piece")
			}
		} else {
			psize = padreader.PaddedSize(sz).Padded()
		}

		pc, err := minerApi.ComputeDataCid(ctx, psize.Unpadded(), r)
		if err != nil {
			return xerrors.Errorf("computing data CID: %w", err)
		}

		fmt.Println(pc.PieceCID, " ", pc.Size)
		return nil
	},
}
