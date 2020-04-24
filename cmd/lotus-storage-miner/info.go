package main

import (
	"bytes"
	"context"
	"fmt"
	"golang.org/x/xerrors"

	"gopkg.in/urfave/cli.v2"

	"github.com/filecoin-project/specs-actors/actors/builtin/miner"
	sealing "github.com/filecoin-project/storage-fsm"

	"github.com/filecoin-project/lotus/api"
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

		mact, err := api.StateGetActor(ctx, maddr, types.EmptyTSK)
		if err != nil {
			return err
		}
		var mas miner.State
		{
			rmas, err := api.ChainReadObj(ctx, mact.Head)
			if err != nil {
				return err
			}
			if err := mas.UnmarshalCBOR(bytes.NewReader(rmas)); err != nil {
				return err
			}
		}

		fmt.Printf("Miner: %s\n", maddr)

		// Sector size
		mi, err := api.StateMinerInfo(ctx, maddr, types.EmptyTSK)
		if err != nil {
			return err
		}

		fmt.Printf("Sector Size: %s\n", types.SizeStr(types.NewInt(uint64(mi.SectorSize))))

		pow, err := api.StateMinerPower(ctx, maddr, types.EmptyTSK)
		if err != nil {
			return err
		}

		rpercI := types.BigDiv(types.BigMul(pow.MinerPower.RawBytePower, types.NewInt(1000000)), pow.TotalPower.RawBytePower)
		qpercI := types.BigDiv(types.BigMul(pow.MinerPower.QualityAdjPower, types.NewInt(1000000)), pow.TotalPower.QualityAdjPower)
		fmt.Printf("Byte Power:   %s / %s (%0.4f%%)\n", types.SizeStr(pow.MinerPower.RawBytePower), types.SizeStr(pow.TotalPower.RawBytePower), float64(rpercI.Int64())/10000)
		fmt.Printf("Actual Power: %s / %s (%0.4f%%)\n", types.DecStr(pow.MinerPower.QualityAdjPower), types.DecStr(pow.TotalPower.QualityAdjPower), float64(qpercI.Int64())/10000)
		secCounts, err := api.StateMinerSectorCount(ctx, maddr, types.EmptyTSK)
		if err != nil {
			return err
		}
		faults, err := api.StateMinerFaults(ctx, maddr, types.EmptyTSK)
		if err != nil {
			return err
		}

		fmt.Printf("\tCommitted: %s\n", types.SizeStr(types.BigMul(types.NewInt(secCounts.Sset), types.NewInt(uint64(mi.SectorSize)))))
		if len(faults) == 0 {
			fmt.Printf("\tProving: %s\n", types.SizeStr(types.BigMul(types.NewInt(secCounts.Pset), types.NewInt(uint64(mi.SectorSize)))))
		} else {
			fmt.Printf("\tProving: %s (%s Faulty, %.2f%%)\n",
				types.SizeStr(types.BigMul(types.NewInt(secCounts.Pset-uint64(len(faults))), types.NewInt(uint64(mi.SectorSize)))),
				types.SizeStr(types.BigMul(types.NewInt(uint64(len(faults))), types.NewInt(uint64(mi.SectorSize)))),
				float64(10000*uint64(len(faults))/secCounts.Pset)/100.)
		}

		fmt.Printf("Miner Balance: %s\n", types.FIL(mact.Balance))
		fmt.Printf("\tPreCommit:   %s\n", types.FIL(mas.PreCommitDeposits))
		fmt.Printf("\tLocked:      %s\n", types.FIL(mas.LockedFunds))
		fmt.Printf("\tAvailable:   %s\n", types.FIL(types.BigSub(mact.Balance, types.BigAdd(mas.LockedFunds, mas.PreCommitDeposits))))
		wb, err := api.WalletBalance(ctx, mi.Worker)
		if err != nil {
			return xerrors.Errorf("getting worker balance: %w", err)
		}
		fmt.Printf("Worker Balance: %s\n", types.FIL(wb))

		mb, err := api.StateMarketBalance(ctx, maddr, types.EmptyTSK)
		if err != nil {
			return xerrors.Errorf("getting market balance: %w", err)
		}
		fmt.Printf("Market (Escrow):  %s\n", types.FIL(mb.Escrow))
		fmt.Printf("Market (Locked):  %s\n", types.FIL(mb.Locked))

		/*// TODO: indicate whether the post worker is in use
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
		fmt.Printf("\tUnseal: %d\n", wstat.UnsealWait)*/

		/*ps, err := api.StateMinerPostState(ctx, maddr, types.EmptyTSK)
		if err != nil {
			return err
		}
		if ps.ProvingPeriodStart != 0 {
			head, err := api.ChainHead(ctx)
			if err != nil {
				return err
			}

			fallback := ps.ProvingPeriodStart - head.Height()
			fallbackS := fallback * build.BlockDelay

			next := fallback + power.WindowedPostChallengeDuration
			nextS := next * build.BlockDelay

			fmt.Printf("PoSt Submissions:\n")
			fmt.Printf("\tFallback: Epoch %d (in %d blocks, ~%dm %ds)\n", ps.ProvingPeriodStart, fallback, fallbackS/60, fallbackS%60)
			fmt.Printf("\tDeadline: Epoch %d (in %d blocks, ~%dm %ds)\n", ps.ProvingPeriodStart+build.SlashablePowerDelay, next, nextS/60, nextS%60)
			fmt.Printf("\tConsecutive Failures: %d\n", ps.NumConsecutiveFailures)
		} else {
			fmt.Printf("Proving Period: Not Proving\n")
		}*/

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

func sectorsInfo(ctx context.Context, napi api.StorageMiner) (map[sealing.SectorState]int, error) {
	sectors, err := napi.SectorsList(ctx)
	if err != nil {
		return nil, err
	}

	out := map[sealing.SectorState]int{
		"Total": len(sectors),
	}
	for _, s := range sectors {
		st, err := napi.SectorsStatus(ctx, s)
		if err != nil {
			return nil, err
		}

		out[sealing.SectorState(st.State)]++
	}

	return out, nil
}
