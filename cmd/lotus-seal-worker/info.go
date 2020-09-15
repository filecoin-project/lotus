package main

import (
	"fmt"

	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/chain/types"
	lcli "github.com/filecoin-project/lotus/cli"
)

var infoCmd = &cli.Command{
	Name:  "info",
	Usage: "Print worker info",
	Action: func(cctx *cli.Context) error {
		api, closer, err := lcli.GetWorkerAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		ctx := lcli.ReqContext(cctx)

		ver, err := api.Version(ctx)
		if err != nil {
			return xerrors.Errorf("getting version: %w", err)
		}

		fmt.Println("Worker version: ", ver)
		fmt.Print("CLI version: ")
		cli.VersionPrinter(cctx)
		fmt.Println()

		info, err := api.Info(ctx)
		if err != nil {
			return xerrors.Errorf("getting info: %w", err)
		}

		fmt.Printf("Hostname: %s\n", info.Hostname)
		fmt.Printf("CPUs: %d; GPUs: %v\n", info.Resources.CPUs, info.Resources.GPUs)
		fmt.Printf("RAM: %s; Swap: %s\n", types.SizeStr(types.NewInt(info.Resources.MemPhysical)), types.SizeStr(types.NewInt(info.Resources.MemSwap)))
		fmt.Printf("Reserved memory: %s\n", types.SizeStr(types.NewInt(info.Resources.MemReserved)))
		fmt.Println()

		paths, err := api.Paths(ctx)
		if err != nil {
			return xerrors.Errorf("getting path info: %w", err)
		}

		for _, path := range paths {
			fmt.Printf("%s:\n", path.ID)
			fmt.Printf("\tWeight: %d; Use: ", path.Weight)
			if path.CanSeal || path.CanStore {
				fmt.Printf("Weight: %d; Use: ", path.Weight)
				if path.CanSeal {
					fmt.Print("Seal ")
				}
				if path.CanStore {
					fmt.Print("Store")
				}
				fmt.Println("")
			} else {
				fmt.Print("Use: ReadOnly")
			}
			fmt.Printf("\tLocal: %s\n", path.LocalPath)
		}

		return nil
	},
}
