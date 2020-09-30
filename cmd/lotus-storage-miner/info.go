package main

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/fatih/color"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	cbor "github.com/ipfs/go-ipld-cbor"

	"github.com/filecoin-project/go-fil-markets/storagemarket"
	"github.com/filecoin-project/go-state-types/abi"
	sealing "github.com/filecoin-project/lotus/extern/storage-sealing"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/api/apibstore"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors/adt"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/types"
	lcli "github.com/filecoin-project/lotus/cli"
	"github.com/filecoin-project/lotus/lib/blockstore"
	"github.com/filecoin-project/lotus/lib/bufbstore"
)

var infoCmd = &cli.Command{
	Name:  "info",
	Usage: "Print miner info",
	Subcommands: []*cli.Command{
		infoAllCmd,
	},
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "hide-sectors-info",
			Usage: "hide sectors info",
		},
	},
	Action: infoCmdAct,
}

func infoCmdAct(cctx *cli.Context) error {
	color.NoColor = !cctx.Bool("color")

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

	maddr, err := getActorAddress(ctx, nodeApi, cctx.String("actor"))
	if err != nil {
		return err
	}

	mact, err := api.StateGetActor(ctx, maddr, types.EmptyTSK)
	if err != nil {
		return err
	}

	tbs := bufbstore.NewTieredBstore(apibstore.NewAPIBlockstore(api), blockstore.NewTemporary())
	mas, err := miner.Load(adt.WrapStore(ctx, cbor.NewCborStore(tbs)), mact)
	if err != nil {
		return err
	}

	fmt.Printf("Miner: %s\n", color.BlueString("%s", maddr))

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

	fmt.Printf("Byte Power:   %s / %s (%0.4f%%)\n",
		color.BlueString(types.SizeStr(pow.MinerPower.RawBytePower)),
		types.SizeStr(pow.TotalPower.RawBytePower),
		float64(rpercI.Int64())/10000)

	fmt.Printf("Actual Power: %s / %s (%0.4f%%)\n",
		color.GreenString(types.DeciStr(pow.MinerPower.QualityAdjPower)),
		types.DeciStr(pow.TotalPower.QualityAdjPower),
		float64(qpercI.Int64())/10000)

	secCounts, err := api.StateMinerSectorCount(ctx, maddr, types.EmptyTSK)
	if err != nil {
		return err
	}

	proving := secCounts.Active + secCounts.Faulty
	nfaults := secCounts.Faulty
	fmt.Printf("\tCommitted: %s\n", types.SizeStr(types.BigMul(types.NewInt(secCounts.Live), types.NewInt(uint64(mi.SectorSize)))))
	if nfaults == 0 {
		fmt.Printf("\tProving: %s\n", types.SizeStr(types.BigMul(types.NewInt(proving), types.NewInt(uint64(mi.SectorSize)))))
	} else {
		var faultyPercentage float64
		if secCounts.Live != 0 {
			faultyPercentage = float64(10000*nfaults/secCounts.Live) / 100.
		}
		fmt.Printf("\tProving: %s (%s Faulty, %.2f%%)\n",
			types.SizeStr(types.BigMul(types.NewInt(proving), types.NewInt(uint64(mi.SectorSize)))),
			types.SizeStr(types.BigMul(types.NewInt(nfaults), types.NewInt(uint64(mi.SectorSize)))),
			faultyPercentage)
	}

	if !pow.HasMinPower {
		fmt.Print("Below minimum power threshold, no blocks will be won")
	} else {
		expWinChance := float64(types.BigMul(qpercI, types.NewInt(build.BlocksPerEpoch)).Int64()) / 1000000
		if expWinChance > 0 {
			if expWinChance > 1 {
				expWinChance = 1
			}
			winRate := time.Duration(float64(time.Second*time.Duration(build.BlockDelaySecs)) / expWinChance)
			winPerDay := float64(time.Hour*24) / float64(winRate)

			fmt.Print("Expected block win rate: ")
			color.Blue("%.4f/day (every %s)", winPerDay, winRate.Truncate(time.Second))
		}
	}

	fmt.Println()

	deals, err := nodeApi.MarketListIncompleteDeals(ctx)
	if err != nil {
		return err
	}

	var nactiveDeals, nVerifDeals, ndeals uint64
	var activeDealBytes, activeVerifDealBytes, dealBytes abi.PaddedPieceSize
	for _, deal := range deals {
		ndeals++
		dealBytes += deal.Proposal.PieceSize

		if deal.State == storagemarket.StorageDealActive {
			nactiveDeals++
			activeDealBytes += deal.Proposal.PieceSize

			if deal.Proposal.VerifiedDeal {
				nVerifDeals++
				activeVerifDealBytes += deal.Proposal.PieceSize
			}
		}
	}

	fmt.Printf("Deals: %d, %s\n", ndeals, types.SizeStr(types.NewInt(uint64(dealBytes))))
	fmt.Printf("\tActive: %d, %s (Verified: %d, %s)\n", nactiveDeals, types.SizeStr(types.NewInt(uint64(activeDealBytes))), nVerifDeals, types.SizeStr(types.NewInt(uint64(activeVerifDealBytes))))
	fmt.Println()

	// NOTE: there's no need to unlock anything here. Funds only
	// vest on deadline boundaries, and they're unlocked by cron.
	lockedFunds, err := mas.LockedFunds()
	if err != nil {
		return xerrors.Errorf("getting locked funds: %w", err)
	}
	availBalance, err := mas.AvailableBalance(mact.Balance)
	if err != nil {
		return xerrors.Errorf("getting available balance: %w", err)
	}
	fmt.Printf("Miner Balance: %s\n", color.YellowString("%s", types.FIL(mact.Balance)))
	fmt.Printf("\tPreCommit:   %s\n", types.FIL(lockedFunds.PreCommitDeposits))
	fmt.Printf("\tPledge:      %s\n", types.FIL(lockedFunds.InitialPledgeRequirement))
	fmt.Printf("\tVesting:     %s\n", types.FIL(lockedFunds.VestingFunds))
	color.Green("\tAvailable:   %s", types.FIL(availBalance))
	wb, err := api.WalletBalance(ctx, mi.Worker)
	if err != nil {
		return xerrors.Errorf("getting worker balance: %w", err)
	}
	color.Cyan("Worker Balance: %s", types.FIL(wb))

	mb, err := api.StateMarketBalance(ctx, maddr, types.EmptyTSK)
	if err != nil {
		return xerrors.Errorf("getting market balance: %w", err)
	}
	fmt.Printf("Market (Escrow):  %s\n", types.FIL(mb.Escrow))
	fmt.Printf("Market (Locked):  %s\n", types.FIL(mb.Locked))

	fmt.Println()

	sealdur, err := nodeApi.SectorGetExpectedSealDuration(ctx)
	if err != nil {
		return err
	}

	fmt.Printf("Expected Seal Duration: %s\n\n", sealdur)

	if !cctx.Bool("hide-sectors-info") {
		fmt.Println("Sectors:")
		err = sectorsInfo(ctx, nodeApi)
		if err != nil {
			return err
		}
	}

	// TODO: grab actr state / info
	//  * Sealed sectors (count / bytes)
	//  * Power
	return nil
}

type stateMeta struct {
	i     int
	col   color.Attribute
	state sealing.SectorState
}

var stateOrder = map[sealing.SectorState]stateMeta{}
var stateList = []stateMeta{
	{col: 39, state: "Total"},
	{col: color.FgGreen, state: sealing.Proving},

	{col: color.FgBlue, state: sealing.Empty},
	{col: color.FgBlue, state: sealing.WaitDeals},

	{col: color.FgRed, state: sealing.UndefinedSectorState},
	{col: color.FgYellow, state: sealing.Packing},
	{col: color.FgYellow, state: sealing.PreCommit1},
	{col: color.FgYellow, state: sealing.PreCommit2},
	{col: color.FgYellow, state: sealing.PreCommitting},
	{col: color.FgYellow, state: sealing.PreCommitWait},
	{col: color.FgYellow, state: sealing.WaitSeed},
	{col: color.FgYellow, state: sealing.Committing},
	{col: color.FgYellow, state: sealing.SubmitCommit},
	{col: color.FgYellow, state: sealing.CommitWait},
	{col: color.FgYellow, state: sealing.FinalizeSector},

	{col: color.FgCyan, state: sealing.Removing},
	{col: color.FgCyan, state: sealing.Removed},

	{col: color.FgRed, state: sealing.FailedUnrecoverable},
	{col: color.FgRed, state: sealing.SealPreCommit1Failed},
	{col: color.FgRed, state: sealing.SealPreCommit2Failed},
	{col: color.FgRed, state: sealing.PreCommitFailed},
	{col: color.FgRed, state: sealing.ComputeProofFailed},
	{col: color.FgRed, state: sealing.CommitFailed},
	{col: color.FgRed, state: sealing.PackingFailed},
	{col: color.FgRed, state: sealing.FinalizeFailed},
	{col: color.FgRed, state: sealing.Faulty},
	{col: color.FgRed, state: sealing.FaultReported},
	{col: color.FgRed, state: sealing.FaultedFinal},
	{col: color.FgRed, state: sealing.RemoveFailed},
	{col: color.FgRed, state: sealing.DealsExpired},
	{col: color.FgRed, state: sealing.RecoverDealIDs},
}

func init() {
	for i, state := range stateList {
		stateOrder[state.state] = stateMeta{
			i:   i,
			col: state.col,
		}
	}
}

func sectorsInfo(ctx context.Context, napi api.StorageMiner) error {
	sectors, err := napi.SectorsList(ctx)
	if err != nil {
		return err
	}

	buckets := map[sealing.SectorState]int{
		"Total": len(sectors),
	}
	for _, s := range sectors {
		st, err := napi.SectorsStatus(ctx, s, false)
		if err != nil {
			return err
		}

		buckets[sealing.SectorState(st.State)]++
	}

	var sorted []stateMeta
	for state, i := range buckets {
		sorted = append(sorted, stateMeta{i: i, state: state})
	}

	sort.Slice(sorted, func(i, j int) bool {
		return stateOrder[sorted[i].state].i < stateOrder[sorted[j].state].i
	})

	for _, s := range sorted {
		_, _ = color.New(stateOrder[s.state].col).Printf("\t%s: %d\n", s.state, s.i)
	}

	return nil
}
