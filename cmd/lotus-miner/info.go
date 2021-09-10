package main

import (
	"context"
	"fmt"
	"math"
	corebig "math/big"
	"os"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/fatih/color"
	"github.com/mattn/go-isatty"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	cbor "github.com/ipfs/go-ipld-cbor"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-fil-markets/retrievalmarket"
	"github.com/filecoin-project/go-fil-markets/storagemarket"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/lotus/api/v0api"
	sealing "github.com/filecoin-project/lotus/extern/storage-sealing"
	"github.com/filecoin-project/specs-actors/actors/builtin"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors/adt"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/actors/builtin/reward"
	"github.com/filecoin-project/lotus/chain/types"
	lcli "github.com/filecoin-project/lotus/cli"
	"github.com/filecoin-project/lotus/journal/alerting"
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
		&cli.IntFlag{
			Name:  "blocks",
			Usage: "Log of produced <blocks> newest blocks and rewards(Miner Fee excluded)",
		},
	},
	Action: infoCmdAct,
}

func infoCmdAct(cctx *cli.Context) error {
	minerApi, closer, err := lcli.GetStorageMinerAPI(cctx)
	if err != nil {
		return err
	}
	defer closer()

	marketsApi, closer, err := lcli.GetMarketsAPI(cctx)
	if err != nil {
		return err
	}
	defer closer()

	fullapi, acloser, err := lcli.GetFullNodeAPI(cctx)
	if err != nil {
		return err
	}
	defer acloser()

	ctx := lcli.ReqContext(cctx)

	subsystems, err := minerApi.RuntimeSubsystems(ctx)
	if err != nil {
		return err
	}

	fmt.Println("Enabled subsystems (from miner API):", subsystems)

	subsystems, err = marketsApi.RuntimeSubsystems(ctx)
	if err != nil {
		return err
	}

	fmt.Println("Enabled subsystems (from markets API):", subsystems)

	fmt.Print("Chain: ")

	head, err := fullapi.ChainHead(ctx)
	if err != nil {
		return err
	}

	switch {
	case time.Now().Unix()-int64(head.MinTimestamp()) < int64(build.BlockDelaySecs*3/2): // within 1.5 epochs
		fmt.Printf("[%s]", color.GreenString("sync ok"))
	case time.Now().Unix()-int64(head.MinTimestamp()) < int64(build.BlockDelaySecs*5): // within 5 epochs
		fmt.Printf("[%s]", color.YellowString("sync slow (%s behind)", time.Now().Sub(time.Unix(int64(head.MinTimestamp()), 0)).Truncate(time.Second)))
	default:
		fmt.Printf("[%s]", color.RedString("sync behind! (%s behind)", time.Now().Sub(time.Unix(int64(head.MinTimestamp()), 0)).Truncate(time.Second)))
	}

	basefee := head.MinTicketBlock().ParentBaseFee
	gasCol := []color.Attribute{color.FgBlue}
	switch {
	case basefee.GreaterThan(big.NewInt(7000_000_000)): // 7 nFIL
		gasCol = []color.Attribute{color.BgRed, color.FgBlack}
	case basefee.GreaterThan(big.NewInt(3000_000_000)): // 3 nFIL
		gasCol = []color.Attribute{color.FgRed}
	case basefee.GreaterThan(big.NewInt(750_000_000)): // 750 uFIL
		gasCol = []color.Attribute{color.FgYellow}
	case basefee.GreaterThan(big.NewInt(100_000_000)): // 100 uFIL
		gasCol = []color.Attribute{color.FgGreen}
	}
	fmt.Printf(" [basefee %s]", color.New(gasCol...).Sprint(types.FIL(basefee).Short()))

	fmt.Println()

	alerts, err := minerApi.LogAlerts(ctx)
	if err != nil {
		return xerrors.Errorf("getting alerts: %w", err)
	}

	activeAlerts := make([]alerting.Alert, 0)
	for _, alert := range alerts {
		if alert.Active {
			activeAlerts = append(activeAlerts, alert)
		}
	}
	if len(activeAlerts) > 0 {
		fmt.Printf("%s (check %s)\n", color.RedString("⚠ %d Active alerts", len(activeAlerts)), color.YellowString("lotus-miner log alerts"))
	}

	err = handleMiningInfo(ctx, cctx, fullapi, minerApi)
	if err != nil {
		return err
	}

	err = handleMarketsInfo(ctx, marketsApi)
	if err != nil {
		return err
	}

	return nil
}

func handleMiningInfo(ctx context.Context, cctx *cli.Context, fullapi v0api.FullNode, nodeApi api.StorageMiner) error {
	maddr, err := getActorAddress(ctx, cctx)
	if err != nil {
		return err
	}

	mact, err := fullapi.StateGetActor(ctx, maddr, types.EmptyTSK)
	if err != nil {
		return err
	}

	tbs := blockstore.NewTieredBstore(blockstore.NewAPIBlockstore(fullapi), blockstore.NewMemory())

	mas, err := miner.Load(adt.WrapStore(ctx, cbor.NewCborStore(tbs)), mact)
	if err != nil {
		return err
	}

	// Sector size
	mi, err := fullapi.StateMinerInfo(ctx, maddr, types.EmptyTSK)

	if err != nil {
		return err
	}

	ssize := types.SizeStr(types.NewInt(uint64(mi.SectorSize)))
	fmt.Printf("Miner: %s (%s sectors)\n", color.BlueString("%s", maddr), ssize)

	pow, err := fullapi.StateMinerPower(ctx, maddr, types.EmptyTSK)
	if err != nil {
		return err
	}

	fmt.Printf("Power: %s / %s (%0.4f%%)\n",
		color.GreenString(types.DeciStr(pow.MinerPower.QualityAdjPower)),
		types.DeciStr(pow.TotalPower.QualityAdjPower),
		types.BigDivFloat(
			types.BigMul(pow.MinerPower.QualityAdjPower, big.NewInt(100)),
			pow.TotalPower.QualityAdjPower,
		),
	)

	fmt.Printf("\tRaw: %s / %s (%0.4f%%)\n",
		color.BlueString(types.SizeStr(pow.MinerPower.RawBytePower)),
		types.SizeStr(pow.TotalPower.RawBytePower),
		types.BigDivFloat(
			types.BigMul(pow.MinerPower.RawBytePower, big.NewInt(100)),
			pow.TotalPower.RawBytePower,
		),
	)
	secCounts, err := fullapi.StateMinerSectorCount(ctx, maddr, types.EmptyTSK)

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
			faultyPercentage = float64(100*nfaults) / float64(secCounts.Live)
		}
		fmt.Printf("\tProving: %s (%s Faulty, %.2f%%)\n",
			types.SizeStr(types.BigMul(types.NewInt(proving), types.NewInt(uint64(mi.SectorSize)))),
			types.SizeStr(types.BigMul(types.NewInt(nfaults), types.NewInt(uint64(mi.SectorSize)))),
			faultyPercentage)
	}

	if !pow.HasMinPower {
		fmt.Print("Below minimum power threshold, no blocks will be won")
	} else {

		winRatio := new(corebig.Rat).SetFrac(
			types.BigMul(pow.MinerPower.QualityAdjPower, types.NewInt(build.BlocksPerEpoch)).Int,
			pow.TotalPower.QualityAdjPower.Int,
		)

		if winRatioFloat, _ := winRatio.Float64(); winRatioFloat > 0 {

			// if the corresponding poisson distribution isn't infinitely small then
			// throw it into the mix as well, accounting for multi-wins
			winRationWithPoissonFloat := -math.Expm1(-winRatioFloat)
			winRationWithPoisson := new(corebig.Rat).SetFloat64(winRationWithPoissonFloat)
			if winRationWithPoisson != nil {
				winRatio = winRationWithPoisson
				winRatioFloat = winRationWithPoissonFloat
			}

			weekly, _ := new(corebig.Rat).Mul(
				winRatio,
				new(corebig.Rat).SetInt64(7*builtin.EpochsInDay),
			).Float64()

			avgDuration, _ := new(corebig.Rat).Mul(
				new(corebig.Rat).SetInt64(builtin.EpochDurationSeconds),
				new(corebig.Rat).Inv(winRatio),
			).Float64()

			fmt.Print("Projected average block win rate: ")
			color.Blue(
				"%.02f/week (every %s)",
				weekly,
				(time.Second * time.Duration(avgDuration)).Truncate(time.Second).String(),
			)

			// Geometric distribution of P(Y < k) calculated as described in https://en.wikipedia.org/wiki/Geometric_distribution#Probability_Outcomes_Examples
			// https://www.wolframalpha.com/input/?i=t+%3E+0%3B+p+%3E+0%3B+p+%3C+1%3B+c+%3E+0%3B+c+%3C1%3B+1-%281-p%29%5E%28t%29%3Dc%3B+solve+t
			// t == how many dice-rolls (epochs) before win
			// p == winRate == ( minerPower / netPower )
			// c == target probability of win ( 99.9% in this case )
			fmt.Print("Projected block win with ")
			color.Green(
				"99.9%% probability every %s",
				(time.Second * time.Duration(
					builtin.EpochDurationSeconds*math.Log(1-0.999)/
						math.Log(1-winRatioFloat),
				)).Truncate(time.Second).String(),
			)
			fmt.Println("(projections DO NOT account for future network and miner growth)")
		}
	}

	fmt.Println()

	spendable := big.Zero()

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
	spendable = big.Add(spendable, availBalance)

	fmt.Printf("Miner Balance:    %s\n", color.YellowString("%s", types.FIL(mact.Balance).Short()))
	fmt.Printf("      PreCommit:  %s\n", types.FIL(lockedFunds.PreCommitDeposits).Short())
	fmt.Printf("      Pledge:     %s\n", types.FIL(lockedFunds.InitialPledgeRequirement).Short())
	fmt.Printf("      Vesting:    %s\n", types.FIL(lockedFunds.VestingFunds).Short())
	colorTokenAmount("      Available:  %s\n", availBalance)

	mb, err := fullapi.StateMarketBalance(ctx, maddr, types.EmptyTSK)

	if err != nil {
		return xerrors.Errorf("getting market balance: %w", err)
	}
	spendable = big.Add(spendable, big.Sub(mb.Escrow, mb.Locked))

	fmt.Printf("Market Balance:   %s\n", types.FIL(mb.Escrow).Short())
	fmt.Printf("       Locked:    %s\n", types.FIL(mb.Locked).Short())
	colorTokenAmount("       Available: %s\n", big.Sub(mb.Escrow, mb.Locked))

	wb, err := fullapi.WalletBalance(ctx, mi.Worker)

	if err != nil {
		return xerrors.Errorf("getting worker balance: %w", err)
	}
	spendable = big.Add(spendable, wb)
	color.Cyan("Worker Balance:   %s", types.FIL(wb).Short())
	if len(mi.ControlAddresses) > 0 {
		cbsum := big.Zero()
		for _, ca := range mi.ControlAddresses {
			b, err := fullapi.WalletBalance(ctx, ca)
			if err != nil {
				return xerrors.Errorf("getting control address balance: %w", err)
			}
			cbsum = big.Add(cbsum, b)
		}
		spendable = big.Add(spendable, cbsum)

		fmt.Printf("       Control:   %s\n", types.FIL(cbsum).Short())
	}
	colorTokenAmount("Total Spendable:  %s\n", spendable)

	fmt.Println()

	if !cctx.Bool("hide-sectors-info") {
		fmt.Println("Sectors:")
		err = sectorsInfo(ctx, nodeApi)
		if err != nil {
			return err
		}
	}

	if cctx.IsSet("blocks") {
		fmt.Println("Produced newest blocks:")
		err = producedBlocks(ctx, cctx.Int("blocks"), maddr, fullapi)
		if err != nil {
			return err
		}
	}
	// TODO: grab actr state / info
	//  * Sealed sectors (count / bytes)
	//  * Power

	return nil
}

func handleMarketsInfo(ctx context.Context, nodeApi api.StorageMiner) error {
	deals, err := nodeApi.MarketListIncompleteDeals(ctx)
	if err != nil {
		return err
	}

	type dealStat struct {
		count, verifCount int
		bytes, verifBytes uint64
	}
	dsAdd := func(ds *dealStat, deal storagemarket.MinerDeal) {
		ds.count++
		ds.bytes += uint64(deal.Proposal.PieceSize)
		if deal.Proposal.VerifiedDeal {
			ds.verifCount++
			ds.verifBytes += uint64(deal.Proposal.PieceSize)
		}
	}

	showDealStates := map[storagemarket.StorageDealStatus]struct{}{
		storagemarket.StorageDealActive:               {},
		storagemarket.StorageDealAcceptWait:           {},
		storagemarket.StorageDealReserveProviderFunds: {},
		storagemarket.StorageDealProviderFunding:      {},
		storagemarket.StorageDealTransferring:         {},
		storagemarket.StorageDealValidating:           {},
		storagemarket.StorageDealStaged:               {},
		storagemarket.StorageDealAwaitingPreCommit:    {},
		storagemarket.StorageDealSealing:              {},
		storagemarket.StorageDealPublish:              {},
		storagemarket.StorageDealCheckForAcceptance:   {},
		storagemarket.StorageDealPublishing:           {},
	}

	var total dealStat
	perState := map[storagemarket.StorageDealStatus]*dealStat{}
	for _, deal := range deals {
		if _, ok := showDealStates[deal.State]; !ok {
			continue
		}
		if perState[deal.State] == nil {
			perState[deal.State] = new(dealStat)
		}

		dsAdd(&total, deal)
		dsAdd(perState[deal.State], deal)
	}

	type wstr struct {
		str    string
		status storagemarket.StorageDealStatus
	}
	sorted := make([]wstr, 0, len(perState))
	for status, stat := range perState {
		st := strings.TrimPrefix(storagemarket.DealStates[status], "StorageDeal")
		sorted = append(sorted, wstr{
			str:    fmt.Sprintf("      %s:\t%d\t\t%s\t(Verified: %d\t%s)\n", st, stat.count, types.SizeStr(types.NewInt(stat.bytes)), stat.verifCount, types.SizeStr(types.NewInt(stat.verifBytes))),
			status: status,
		},
		)
	}
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].status == storagemarket.StorageDealActive || sorted[j].status == storagemarket.StorageDealActive {
			return sorted[i].status == storagemarket.StorageDealActive
		}
		return sorted[i].status > sorted[j].status
	})

	fmt.Println()
	fmt.Printf("Storage Deals: %d, %s\n", total.count, types.SizeStr(types.NewInt(total.bytes)))

	tw := tabwriter.NewWriter(os.Stdout, 1, 1, 1, ' ', 0)
	for _, e := range sorted {
		_, _ = tw.Write([]byte(e.str))
	}

	_ = tw.Flush()
	fmt.Println()

	retrievals, err := nodeApi.MarketListRetrievalDeals(ctx)
	if err != nil {
		return xerrors.Errorf("getting retrieval deal list: %w", err)
	}

	var retrComplete dealStat
	for _, retrieval := range retrievals {
		if retrieval.Status == retrievalmarket.DealStatusCompleted {
			retrComplete.count++
			retrComplete.bytes += retrieval.TotalSent
		}
	}

	fmt.Printf("Retrieval Deals (complete): %d, %s\n", retrComplete.count, types.SizeStr(types.NewInt(retrComplete.bytes)))

	fmt.Println()

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
	{col: color.FgBlue, state: sealing.AddPiece},

	{col: color.FgRed, state: sealing.UndefinedSectorState},
	{col: color.FgYellow, state: sealing.Packing},
	{col: color.FgYellow, state: sealing.GetTicket},
	{col: color.FgYellow, state: sealing.PreCommit1},
	{col: color.FgYellow, state: sealing.PreCommit2},
	{col: color.FgYellow, state: sealing.PreCommitting},
	{col: color.FgYellow, state: sealing.PreCommitWait},
	{col: color.FgYellow, state: sealing.SubmitPreCommitBatch},
	{col: color.FgYellow, state: sealing.PreCommitBatchWait},
	{col: color.FgYellow, state: sealing.WaitSeed},
	{col: color.FgYellow, state: sealing.Committing},
	{col: color.FgYellow, state: sealing.CommitFinalize},
	{col: color.FgYellow, state: sealing.SubmitCommit},
	{col: color.FgYellow, state: sealing.CommitWait},
	{col: color.FgYellow, state: sealing.SubmitCommitAggregate},
	{col: color.FgYellow, state: sealing.CommitAggregateWait},
	{col: color.FgYellow, state: sealing.FinalizeSector},

	{col: color.FgCyan, state: sealing.Terminating},
	{col: color.FgCyan, state: sealing.TerminateWait},
	{col: color.FgCyan, state: sealing.TerminateFinality},
	{col: color.FgCyan, state: sealing.TerminateFailed},
	{col: color.FgCyan, state: sealing.Removing},
	{col: color.FgCyan, state: sealing.Removed},

	{col: color.FgRed, state: sealing.FailedUnrecoverable},
	{col: color.FgRed, state: sealing.AddPieceFailed},
	{col: color.FgRed, state: sealing.SealPreCommit1Failed},
	{col: color.FgRed, state: sealing.SealPreCommit2Failed},
	{col: color.FgRed, state: sealing.PreCommitFailed},
	{col: color.FgRed, state: sealing.ComputeProofFailed},
	{col: color.FgRed, state: sealing.CommitFailed},
	{col: color.FgRed, state: sealing.CommitFinalizeFailed},
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

func sectorsInfo(ctx context.Context, mapi api.StorageMiner) error {
	summary, err := mapi.SectorsSummary(ctx)
	if err != nil {
		return err
	}

	buckets := make(map[sealing.SectorState]int)
	var total int
	for s, c := range summary {
		buckets[sealing.SectorState(s)] = c
		total += c
	}
	buckets["Total"] = total

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

func colorTokenAmount(format string, amount abi.TokenAmount) {
	if amount.GreaterThan(big.Zero()) {
		color.Green(format, types.FIL(amount).Short())
	} else if amount.Equals(big.Zero()) {
		color.Yellow(format, types.FIL(amount).Short())
	} else {
		color.Red(format, types.FIL(amount).Short())
	}
}

func producedBlocks(ctx context.Context, count int, maddr address.Address, napi v0api.FullNode) error {
	var err error
	head, err := napi.ChainHead(ctx)
	if err != nil {
		return err
	}

	tbs := blockstore.NewTieredBstore(blockstore.NewAPIBlockstore(napi), blockstore.NewMemory())

	tty := isatty.IsTerminal(os.Stderr.Fd())

	ts := head
	fmt.Printf(" Epoch   | Block ID                                                       | Reward\n")
	for count > 0 {
		tsk := ts.Key()
		bhs := ts.Blocks()
		for _, bh := range bhs {
			if ctx.Err() != nil {
				return ctx.Err()
			}

			if bh.Miner == maddr {
				if tty {
					_, _ = fmt.Fprint(os.Stderr, "\r\x1b[0K")
				}

				rewardActor, err := napi.StateGetActor(ctx, reward.Address, tsk)
				if err != nil {
					return err
				}

				rewardActorState, err := reward.Load(adt.WrapStore(ctx, cbor.NewCborStore(tbs)), rewardActor)
				if err != nil {
					return err
				}
				blockReward, err := rewardActorState.ThisEpochReward()
				if err != nil {
					return err
				}

				minerReward := types.BigDiv(types.BigMul(types.NewInt(uint64(bh.ElectionProof.WinCount)),
					blockReward), types.NewInt(uint64(builtin.ExpectedLeadersPerEpoch)))

				fmt.Printf("%8d | %s | %s\n", ts.Height(), bh.Cid(), types.FIL(minerReward))
				count--
			} else if tty && bh.Height%120 == 0 {
				_, _ = fmt.Fprintf(os.Stderr, "\r\x1b[0KChecking epoch %s", lcli.EpochTime(head.Height(), bh.Height))
			}
		}
		tsk = ts.Parents()
		ts, err = napi.ChainGetTipSet(ctx, tsk)
		if err != nil {
			return err
		}
	}

	if tty {
		_, _ = fmt.Fprint(os.Stderr, "\r\x1b[0K")
	}

	return nil
}
