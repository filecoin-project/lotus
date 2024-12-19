package miner

import (
	"flag"
	"fmt"
	"sort"

	"github.com/urfave/cli/v2"

	lcli "github.com/filecoin-project/lotus/cli"
	"github.com/filecoin-project/lotus/cli/spcli"
)

var _test = false

var infoAllCmd = &cli.Command{
	Name:  "all",
	Usage: "dump all related miner info",
	Action: func(cctx *cli.Context) error {
		minerApi, closer, err := lcli.GetStorageMinerAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		api, acloser, err := lcli.GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer acloser()
		_ = api

		ctx := lcli.ReqContext(cctx)

		// Top-level info

		fmt.Println("#: Version")
		if err := lcli.VersionCmd.Action(cctx); err != nil {
			fmt.Println("ERROR: ", err)
		}

		fmt.Println("\n#: Miner Info")
		if err := infoCmdAct(cctx); err != nil {
			fmt.Println("ERROR: ", err)
		}

		// Verbose info

		fmt.Println("\n#: Storage List")
		if err := storageListCmd.Action(cctx); err != nil {
			fmt.Println("ERROR: ", err)
		}

		fmt.Println("\n#: Sealing Worker List")
		if err := workersCmd(true).Action(cctx); err != nil {
			fmt.Println("ERROR: ", err)
		}

		fmt.Println("\n#: Proving Worker List")
		if err := workersCmd(false).Action(cctx); err != nil {
			fmt.Println("ERROR: ", err)
		}

		fmt.Println("\n#: PeerID")
		if err := lcli.NetId.Action(cctx); err != nil {
			fmt.Println("ERROR: ", err)
		}

		fmt.Println("\n#: Listen Addresses")
		if err := lcli.NetListen.Action(cctx); err != nil {
			fmt.Println("ERROR: ", err)
		}

		fmt.Println("\n#: Reachability")
		if err := lcli.NetReachability.Action(cctx); err != nil {
			fmt.Println("ERROR: ", err)
		}

		// Very Verbose info
		fmt.Println("\n#: Peers")
		if err := lcli.NetPeers.Action(cctx); err != nil {
			fmt.Println("ERROR: ", err)
		}

		fmt.Println("\n#: Proving Info")
		if err := spcli.ProvingInfoCmd(LMActorOrEnvGetter).Action(cctx); err != nil {
			fmt.Println("ERROR: ", err)
		}

		fmt.Println("\n#: Proving Deadlines")
		if err := spcli.ProvingDeadlinesCmd(LMActorOrEnvGetter).Action(cctx); err != nil {
			fmt.Println("ERROR: ", err)
		}

		fmt.Println("\n#: Proving Faults")
		if err := spcli.ProvingFaultsCmd(LMActorOrEnvGetter).Action(cctx); err != nil {
			fmt.Println("ERROR: ", err)
		}

		fmt.Println("\n#: Sealing Jobs")
		if err := sealingJobsCmd.Action(cctx); err != nil {
			fmt.Println("ERROR: ", err)
		}

		fmt.Println("\n#: Storage Locks")
		if err := storageLocks.Action(cctx); err != nil {
			fmt.Println("ERROR: ", err)
		}

		fmt.Println("\n#: Sched Diag")
		if err := sealingSchedDiagCmd.Action(cctx); err != nil {
			fmt.Println("ERROR: ", err)
		}

		fmt.Println("\n#: Pending Batch Terminations")
		if err := sectorsTerminatePendingCmd.Action(cctx); err != nil {
			fmt.Println("ERROR: ", err)
		}

		fmt.Println("\n#: Pending Batch PreCommit")
		if err := sectorsBatchingPendingPreCommit.Action(cctx); err != nil {
			fmt.Println("ERROR: ", err)
		}

		fmt.Println("\n#: Pending Batch Commit")
		if err := sectorsBatchingPendingCommit.Action(cctx); err != nil {
			fmt.Println("ERROR: ", err)
		}

		fmt.Println("\n#: Sector List")
		{
			fs := &flag.FlagSet{}
			for _, f := range sectorsListCmd.Flags {
				if err := f.Apply(fs); err != nil {
					fmt.Println("ERROR: ", err)
				}
			}

			if err := sectorsListCmd.Action(cli.NewContext(cctx.App, fs, cctx)); err != nil {
				fmt.Println("ERROR: ", err)
			}
		}

		fmt.Println("\n#: Storage Sector List")
		if err := storageListSectorsCmd.Action(cctx); err != nil {
			fmt.Println("ERROR: ", err)
		}

		fmt.Println("\n#: Expired Sectors")
		if err := sectorsExpiredCmd.Action(cctx); err != nil {
			fmt.Println("ERROR: ", err)
		}

		// Very Very Verbose info
		fmt.Println("\n#: Per Sector Info")

		list, err := minerApi.SectorsList(ctx)
		if err != nil {
			fmt.Println("ERROR: ", err)
		}

		sort.Slice(list, func(i, j int) bool {
			return list[i] < list[j]
		})

		for _, s := range list {
			fmt.Printf("\n##: Sector %d Status\n", s)

			fs := &flag.FlagSet{}
			for _, f := range spcli.SectorsStatusCmd(LMActorOrEnvGetter, getOnDiskInfo).Flags {
				if err := f.Apply(fs); err != nil {
					fmt.Println("ERROR: ", err)
				}
			}
			if err := fs.Parse([]string{"--log", "--on-chain-info", fmt.Sprint(s)}); err != nil {
				fmt.Println("ERROR: ", err)
			}

			if err := spcli.SectorsStatusCmd(LMActorOrEnvGetter, getOnDiskInfo).Action(cli.NewContext(cctx.App, fs, cctx)); err != nil {
				fmt.Println("ERROR: ", err)
			}

			fmt.Printf("\n##: Sector %d Storage Location\n", s)

			fs = &flag.FlagSet{}
			if err := fs.Parse([]string{fmt.Sprint(s)}); err != nil {
				fmt.Println("ERROR: ", err)
			}

			if err := storageFindCmd.Action(cli.NewContext(cctx.App, fs, cctx)); err != nil {
				fmt.Println("ERROR: ", err)
			}
		}

		if !_test {
			fmt.Println("\n#: Goroutines")
			if err := lcli.PprofGoroutinesCmd.Action(cctx); err != nil {
				fmt.Println("ERROR: ", err)
			}
		}

		return nil
	},
}
