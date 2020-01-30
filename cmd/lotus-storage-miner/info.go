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

		fmt.Printf("Sector Size: %s\n", types.NewInt(sizeByte).SizeStr())

		pow, err := api.StateMinerPower(ctx, maddr, nil)
		if err != nil {
			return err
		}

		percI := types.BigDiv(types.BigMul(pow.MinerPower, types.NewInt(1000000)), pow.TotalPower)
		fmt.Printf("Power: %s / %s (%0.4f%%)\n", pow.MinerPower.SizeStr(), pow.TotalPower.SizeStr(), float64(percI.Int64())/10000)

		secCounts, err := api.StateMinerSectorCount(ctx, maddr, nil)
		if err != nil {
			return err
		}
		fmt.Printf("\tCommitted: %s\n", types.BigMul(types.NewInt(secCounts.Sset), types.NewInt(sizeByte)).SizeStr())
		fmt.Printf("\tProving: %s\n", types.BigMul(types.NewInt(secCounts.Pset), types.NewInt(sizeByte)).SizeStr())

		// TODO: indicate whether the post worker is in use
		wstat, err := nodeApi.WorkerStats(ctx)
		if err != nil {
			return err
		}

		fmt.Printf("Worker use:\n")
		fmt.Printf("\tLocal: %d / %d (+%d reserved)\n", wstat.LocalTotal-wstat.LocalReserved-wstat.LocalFree, wstat.LocalTotal-wstat.LocalReserved, wstat.LocalReserved)
		fmt.Printf("\tRemote: %d / %d\n", wstat.RemotesTotal-wstat.RemotesFree, wstat.RemotesTotal)

		fmt.Printf("Queues:\n")
		fmt.Printf("\tAddPiece: %d\n", wstat.AddPieceWait)
		fmt.Printf("\tPreCommit: %d\n", wstat.PreCommitWait)
		fmt.Printf("\tCommit: %d\n", wstat.CommitWait)
		fmt.Printf("\tUnseal: %d\n", wstat.UnsealWait)

		eps, err := api.StateMinerElectionPeriodStart(ctx, maddr, nil)
		if err != nil {
			return err
		}
		if eps != 0 {
			head, err := api.ChainHead(ctx)
			if err != nil {
				return err
			}
			lastEps := int64(head.Height() - eps)
			lastEpsS := lastEps * build.BlockDelay

			fallback := lastEps + build.FallbackPoStDelay
			fallbackS := fallback * build.BlockDelay

			next := lastEps + build.SlashablePowerDelay
			nextS := next * build.BlockDelay

			fmt.Printf("PoSt Submissions:\n")
			fmt.Printf("\tPrevious: Epoch %d (%d block(s), ~%dm %ds ago)\n", eps, lastEps, lastEpsS/60, lastEpsS%60)
			fmt.Printf("\tFallback: Epoch %d (in %d blocks, ~%dm %ds)\n", eps+build.FallbackPoStDelay, fallback, fallbackS/60, fallbackS%60)
			fmt.Printf("\tDeadline: Epoch %d (in %d blocks, ~%dm %ds)\n", eps+build.SlashablePowerDelay, next, nextS/60, nextS%60)

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

		out[api.SectorStates[st.State]]++
	}

	return out, nil
}
