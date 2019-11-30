package main

import (
	"context"
	"fmt"

	"gopkg.in/urfave/cli.v2"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/types"
	lcli "github.com/filecoin-project/lotus/cli"
)

var infoCmd = &cli.Command{
	Name:  "info",
	Usage: "Print storage miner info",
	Action: func(cctx *cli.Context) error {
		nodeApi, closer, err := lcli.GetStorageMinerAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		api, acloser, err := lcli.GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer acloser()

		ctx := lcli.ReqContext(cctx)

		maddr, err := nodeApi.ActorAddress(ctx)
		if err != nil {
			return err
		}

		fmt.Printf("Miner: %s\n", maddr)

		// Sector size
		sizeByte, err := api.StateMinerSectorSize(ctx, maddr, nil)
		if err != nil {
			return err
		}

		fmt.Printf("Sector Size: %s\n", sizeStr(types.NewInt(sizeByte)))

		pow, err := api.StateMinerPower(ctx, maddr, nil)
		if err != nil {
			return err
		}

		percI := types.BigDiv(types.BigMul(pow.MinerPower, types.NewInt(1000)), pow.TotalPower)
		fmt.Printf("Power: %s / %s (%0.4f%%)\n", sizeStr(pow.MinerPower), sizeStr(pow.TotalPower), float64(percI.Int64())/100000*10000)

		// TODO: indicate whether the post worker is in use
		wstat, err := nodeApi.WorkerStats(ctx)
		if err != nil {
			return err
		}

		fmt.Printf("Worker use:\n")
		fmt.Printf("\tLocal: %d / %d (+%d reserved)\n", wstat.LocalTotal-wstat.LocalReserved-wstat.LocalFree, wstat.LocalTotal-wstat.LocalReserved, wstat.LocalReserved)
		fmt.Printf("\tRemote: %d / %d\n", wstat.RemotesTotal-wstat.RemotesFree, wstat.RemotesTotal)

		ppe, err := api.StateMinerElectionPeriodStart(ctx, maddr, nil)
		if err != nil {
			return err
		}
		if ppe != 0 {
			head, err := api.ChainHead(ctx)
			if err != nil {
				return err
			}
			pdiff := int64(ppe - head.Height())
			pdifft := pdiff * build.BlockDelay
			fmt.Printf("Proving Period: %d, in %d Blocks (~%dm %ds)\n", ppe, pdiff, pdifft/60, pdifft%60)
		} else {
			fmt.Printf("Proving Period: Not Proving\n")
		}

		sinfo, err := sectorsInfo(ctx, nodeApi)
		if err != nil {
			return err
		}

		fmt.Println("Sectors: ", sinfo)

		// TODO: grab actr state / info
		//  * Sealed sectors (count / bytes)
		//  * Power
		return nil
	},
}

var Units = []string{"B", "KiB", "MiB", "GiB", "TiB", "PiB", "EiB", "ZiB"}

func sizeStr(size types.BigInt) string {
	size = types.BigMul(size, types.NewInt(100))
	i := 0
	for types.BigCmp(size, types.NewInt(102400)) >= 0 && i < len(Units)-1 {
		size = types.BigDiv(size, types.NewInt(1024))
		i++
	}
	return fmt.Sprintf("%s.%s %s", types.BigDiv(size, types.NewInt(100)), types.BigMod(size, types.NewInt(100)), Units[i])
}

func sectorsInfo(ctx context.Context, napi api.StorageMiner) (map[string]int, error) {
	sectors, err := napi.SectorsList(ctx)
	if err != nil {
		return nil, err
	}

	out := map[string]int{
		"Total": len(sectors),
	}
	for _, s := range sectors {
		st, err := napi.SectorsStatus(ctx, s)
		if err != nil {
			return nil, err
		}

		out[api.SectorStateStr(st.State)]++
	}

	return out, nil
}
