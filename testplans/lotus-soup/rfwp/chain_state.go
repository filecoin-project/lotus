package rfwp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	corebig "math/big"
	"os"
	"sort"
	"text/tabwriter"
	"time"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/build"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/api/v0api"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"

	"github.com/filecoin-project/lotus/testplans/lotus-soup/testkit"

	"github.com/filecoin-project/go-state-types/abi"
	sealing "github.com/filecoin-project/lotus/extern/storage-sealing"

	"github.com/filecoin-project/lotus/chain/actors/builtin"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	tsync "github.com/filecoin-project/lotus/tools/stats/sync"
)

func UpdateChainState(t *testkit.TestEnvironment, m *testkit.LotusMiner) error {
	height := 0
	headlag := 3

	ctx := context.Background()

	tipsetsCh, err := tsync.BufferedTipsetChannel(ctx, &v0api.WrapperV1Full{FullNode: m.FullApi}, abi.ChainEpoch(height), headlag)
	if err != nil {
		return err
	}

	jsonFilename := fmt.Sprintf("%s%cchain-state.ndjson", t.TestOutputsPath, os.PathSeparator)
	jsonFile, err := os.Create(jsonFilename)
	if err != nil {
		return err
	}
	defer jsonFile.Close()
	jsonEncoder := json.NewEncoder(jsonFile)

	for tipset := range tipsetsCh {
		maddrs, err := m.FullApi.StateListMiners(ctx, tipset.Key())
		if err != nil {
			return err
		}

		snapshot := ChainSnapshot{
			Height:      tipset.Height(),
			MinerStates: make(map[string]*MinerStateSnapshot),
		}

		err = func() error {
			cs.Lock()
			defer cs.Unlock()

			for _, maddr := range maddrs {
				err := func() error {
					filename := fmt.Sprintf("%s%cstate-%s-%d", t.TestOutputsPath, os.PathSeparator, maddr, tipset.Height())

					f, err := os.Create(filename)
					if err != nil {
						return err
					}
					defer f.Close()

					w := bufio.NewWriter(f)
					defer w.Flush()

					minerInfo, err := info(t, m, maddr, w, tipset.Height())
					if err != nil {
						return err
					}
					writeText(w, minerInfo)

					if tipset.Height()%100 == 0 {
						printDiff(t, minerInfo, tipset.Height())
					}

					faultState, err := provingFaults(t, m, maddr, tipset.Height())
					if err != nil {
						return err
					}
					writeText(w, faultState)

					provState, err := provingInfo(t, m, maddr, tipset.Height())
					if err != nil {
						return err
					}
					writeText(w, provState)

					// record diff
					recordDiff(minerInfo, provState, tipset.Height())

					deadlines, err := provingDeadlines(t, m, maddr, tipset.Height())
					if err != nil {
						return err
					}
					writeText(w, deadlines)

					sectorInfo, err := sectorsList(t, m, maddr, w, tipset.Height())
					if err != nil {
						return err
					}
					writeText(w, sectorInfo)

					snapshot.MinerStates[maddr.String()] = &MinerStateSnapshot{
						Info:        minerInfo,
						Faults:      faultState,
						ProvingInfo: provState,
						Deadlines:   deadlines,
						Sectors:     sectorInfo,
					}

					return jsonEncoder.Encode(snapshot)
				}()
				if err != nil {
					return err
				}
			}

			cs.PrevHeight = tipset.Height()

			return nil
		}()
		if err != nil {
			return err
		}
	}

	return nil
}

type ChainSnapshot struct {
	Height abi.ChainEpoch

	MinerStates map[string]*MinerStateSnapshot
}

type MinerStateSnapshot struct {
	Info        *MinerInfo
	Faults      *ProvingFaultState
	ProvingInfo *ProvingInfoState
	Deadlines   *ProvingDeadlines
	Sectors     *SectorInfo
}

// writeText marshals m to text and writes to w, swallowing any errors along the way.
func writeText(w io.Writer, m plainTextMarshaler) {
	b, err := m.MarshalPlainText()
	if err != nil {
		return
	}
	_, _ = w.Write(b)
}

// if we make our structs `encoding.TextMarshaler`s, they all get stringified when marshaling to JSON
// instead of just using the default struct marshaler.
// so here's encoding.TextMarshaler with a different name, so that doesn't happen.
type plainTextMarshaler interface {
	MarshalPlainText() ([]byte, error)
}

type ProvingFaultState struct {
	// FaultedSectors is a slice per-deadline faulty sectors. If the miner
	// has no faulty sectors, this will be nil.
	FaultedSectors [][]uint64
}

func (s *ProvingFaultState) MarshalPlainText() ([]byte, error) {
	w := &bytes.Buffer{}

	if len(s.FaultedSectors) == 0 {
		fmt.Fprintf(w, "no faulty sectors\n")
		return w.Bytes(), nil
	}

	tw := tabwriter.NewWriter(w, 2, 4, 2, ' ', 0)
	_, _ = fmt.Fprintf(tw, "deadline\tsectors")
	for deadline, sectors := range s.FaultedSectors {
		for _, num := range sectors {
			_, _ = fmt.Fprintf(tw, "%d\t%d\n", deadline, num)
		}
	}

	return w.Bytes(), nil
}

func provingFaults(t *testkit.TestEnvironment, m *testkit.LotusMiner, maddr address.Address, height abi.ChainEpoch) (*ProvingFaultState, error) {
	api := m.FullApi
	ctx := context.Background()

	head, err := api.ChainHead(ctx)
	if err != nil {
		return nil, err
	}
	deadlines, err := api.StateMinerDeadlines(ctx, maddr, head.Key())
	if err != nil {
		return nil, err
	}
	faultedSectors := make([][]uint64, len(deadlines))
	hasFaults := false
	for dlIdx := range deadlines {
		partitions, err := api.StateMinerPartitions(ctx, maddr, uint64(dlIdx), types.EmptyTSK)
		if err != nil {
			return nil, err
		}

		for _, partition := range partitions {
			faulty, err := partition.FaultySectors.All(10000000)
			if err != nil {
				return nil, err
			}

			if len(faulty) > 0 {
				hasFaults = true
			}

			faultedSectors[dlIdx] = append(faultedSectors[dlIdx], faulty...)
		}
	}
	result := new(ProvingFaultState)
	if hasFaults {
		result.FaultedSectors = faultedSectors
	}

	return result, nil
}

type ProvingInfoState struct {
	CurrentEpoch abi.ChainEpoch

	ProvingPeriodStart abi.ChainEpoch

	Faults        uint64
	ProvenSectors uint64
	FaultPercent  float64
	Recoveries    uint64

	DeadlineIndex       uint64
	DeadlineSectors     uint64
	DeadlineOpen        abi.ChainEpoch
	DeadlineClose       abi.ChainEpoch
	DeadlineChallenge   abi.ChainEpoch
	DeadlineFaultCutoff abi.ChainEpoch

	WPoStProvingPeriod abi.ChainEpoch
}

func (s *ProvingInfoState) MarshalPlainText() ([]byte, error) {
	w := &bytes.Buffer{}
	fmt.Fprintf(w, "Current Epoch:           %d\n", s.CurrentEpoch)
	fmt.Fprintf(w, "Chain Period:            %d\n", s.CurrentEpoch/s.WPoStProvingPeriod)
	fmt.Fprintf(w, "Chain Period Start:      %s\n", epochTime(s.CurrentEpoch, (s.CurrentEpoch/s.WPoStProvingPeriod)*s.WPoStProvingPeriod))
	fmt.Fprintf(w, "Chain Period End:        %s\n\n", epochTime(s.CurrentEpoch, (s.CurrentEpoch/s.WPoStProvingPeriod+1)*s.WPoStProvingPeriod))

	fmt.Fprintf(w, "Proving Period Boundary: %d\n", s.ProvingPeriodStart%s.WPoStProvingPeriod)
	fmt.Fprintf(w, "Proving Period Start:    %s\n", epochTime(s.CurrentEpoch, s.ProvingPeriodStart))
	fmt.Fprintf(w, "Next Period Start:       %s\n\n", epochTime(s.CurrentEpoch, s.ProvingPeriodStart+s.WPoStProvingPeriod))

	fmt.Fprintf(w, "Faults:      %d (%.2f%%)\n", s.Faults, s.FaultPercent)
	fmt.Fprintf(w, "Recovering:  %d\n", s.Recoveries)
	//fmt.Fprintf(w, "New Sectors: %d\n\n", s.NewSectors)

	fmt.Fprintf(w, "Deadline Index:       %d\n", s.DeadlineIndex)
	fmt.Fprintf(w, "Deadline Sectors:     %d\n", s.DeadlineSectors)

	fmt.Fprintf(w, "Deadline Open:        %s\n", epochTime(s.CurrentEpoch, s.DeadlineOpen))
	fmt.Fprintf(w, "Deadline Close:       %s\n", epochTime(s.CurrentEpoch, s.DeadlineClose))
	fmt.Fprintf(w, "Deadline Challenge:   %s\n", epochTime(s.CurrentEpoch, s.DeadlineChallenge))
	fmt.Fprintf(w, "Deadline FaultCutoff: %s\n", epochTime(s.CurrentEpoch, s.DeadlineFaultCutoff))

	return w.Bytes(), nil
}

func provingInfo(t *testkit.TestEnvironment, m *testkit.LotusMiner, maddr address.Address, height abi.ChainEpoch) (*ProvingInfoState, error) {
	lapi := m.FullApi
	ctx := context.Background()

	head, err := lapi.ChainHead(ctx)
	if err != nil {
		return nil, err
	}

	cd, err := lapi.StateMinerProvingDeadline(ctx, maddr, head.Key())
	if err != nil {
		return nil, err
	}

	deadlines, err := lapi.StateMinerDeadlines(ctx, maddr, head.Key())
	if err != nil {
		return nil, err
	}

	parts := map[uint64][]api.Partition{}
	for dlIdx := range deadlines {
		part, err := lapi.StateMinerPartitions(ctx, maddr, uint64(dlIdx), types.EmptyTSK)
		if err != nil {
			return nil, err
		}

		parts[uint64(dlIdx)] = part
	}

	proving := uint64(0)
	faults := uint64(0)
	recovering := uint64(0)

	for _, partitions := range parts {
		for _, partition := range partitions {
			sc, err := partition.LiveSectors.Count()
			if err != nil {
				return nil, err
			}
			proving += sc

			fc, err := partition.FaultySectors.Count()
			if err != nil {
				return nil, err
			}
			faults += fc

			rc, err := partition.RecoveringSectors.Count()
			if err != nil {
				return nil, err
			}
			recovering += rc
		}
	}

	var faultPerc float64
	if proving > 0 {
		faultPerc = float64(faults*10000/proving) / 100
	}

	s := ProvingInfoState{
		CurrentEpoch:        cd.CurrentEpoch,
		ProvingPeriodStart:  cd.PeriodStart,
		Faults:              faults,
		ProvenSectors:       proving,
		FaultPercent:        faultPerc,
		Recoveries:          recovering,
		DeadlineIndex:       cd.Index,
		DeadlineOpen:        cd.Open,
		DeadlineClose:       cd.Close,
		DeadlineChallenge:   cd.Challenge,
		DeadlineFaultCutoff: cd.FaultCutoff,
		WPoStProvingPeriod:  cd.WPoStProvingPeriod,
	}

	if cd.Index < cd.WPoStPeriodDeadlines {
		for _, partition := range parts[cd.Index] {
			sc, err := partition.LiveSectors.Count()
			if err != nil {
				return nil, err
			}
			s.DeadlineSectors += sc
		}
	}

	return &s, nil
}

func epochTime(curr, e abi.ChainEpoch) string {
	switch {
	case curr > e:
		return fmt.Sprintf("%d (%s ago)", e, time.Second*time.Duration(int64(build.BlockDelaySecs)*int64(curr-e)))
	case curr == e:
		return fmt.Sprintf("%d (now)", e)
	case curr < e:
		return fmt.Sprintf("%d (in %s)", e, time.Second*time.Duration(int64(build.BlockDelaySecs)*int64(e-curr)))
	}

	panic("math broke")
}

type ProvingDeadlines struct {
	Deadlines []DeadlineInfo
}

type DeadlineInfo struct {
	Sectors    uint64
	Partitions int
	Proven     uint64
	Current    bool
}

func (d *ProvingDeadlines) MarshalPlainText() ([]byte, error) {
	w := new(bytes.Buffer)
	tw := tabwriter.NewWriter(w, 2, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "deadline\tsectors\tpartitions\tproven")

	for i, di := range d.Deadlines {
		var cur string
		if di.Current {
			cur += "\t(current)"
		}
		_, _ = fmt.Fprintf(tw, "%d\t%d\t%d\t%d%s\n", i, di.Sectors, di.Partitions, di.Proven, cur)
	}
	tw.Flush()
	return w.Bytes(), nil
}

func provingDeadlines(t *testkit.TestEnvironment, m *testkit.LotusMiner, maddr address.Address, height abi.ChainEpoch) (*ProvingDeadlines, error) {
	lapi := m.FullApi
	ctx := context.Background()

	deadlines, err := lapi.StateMinerDeadlines(ctx, maddr, types.EmptyTSK)
	if err != nil {
		return nil, err
	}

	di, err := lapi.StateMinerProvingDeadline(ctx, maddr, types.EmptyTSK)
	if err != nil {
		return nil, err
	}

	infos := make([]DeadlineInfo, 0, len(deadlines))
	for dlIdx, deadline := range deadlines {
		partitions, err := lapi.StateMinerPartitions(ctx, maddr, uint64(dlIdx), types.EmptyTSK)
		if err != nil {
			return nil, err
		}

		provenPartitions, err := deadline.PostSubmissions.Count()
		if err != nil {
			return nil, err
		}

		var cur string
		if di.Index == uint64(dlIdx) {
			cur += "\t(current)"
		}

		outInfo := DeadlineInfo{
			//Sectors:    c,
			Partitions: len(partitions),
			Proven:     provenPartitions,
			Current:    di.Index == uint64(dlIdx),
		}
		infos = append(infos, outInfo)
		//_, _ = fmt.Fprintf(tw, "%d\t%d\t%d%s\n", dlIdx, len(partitions), provenPartitions, cur)
	}

	return &ProvingDeadlines{Deadlines: infos}, nil
}

type SectorInfo struct {
	Sectors      []abi.SectorNumber
	SectorStates map[abi.SectorNumber]api.SectorInfo
	Committed    []abi.SectorNumber
	Proving      []abi.SectorNumber
}

func (i *SectorInfo) MarshalPlainText() ([]byte, error) {
	provingIDs := make(map[abi.SectorNumber]struct{}, len(i.Proving))
	for _, id := range i.Proving {
		provingIDs[id] = struct{}{}
	}
	commitedIDs := make(map[abi.SectorNumber]struct{}, len(i.Committed))
	for _, id := range i.Committed {
		commitedIDs[id] = struct{}{}
	}

	w := new(bytes.Buffer)
	tw := tabwriter.NewWriter(w, 8, 4, 1, ' ', 0)

	for _, s := range i.Sectors {
		_, inSSet := commitedIDs[s]
		_, inPSet := provingIDs[s]

		st, ok := i.SectorStates[s]
		if !ok {
			continue
		}

		fmt.Fprintf(tw, "%d: %s\tsSet: %s\tpSet: %s\ttktH: %d\tseedH: %d\tdeals: %v\n",
			s,
			st.State,
			yesno(inSSet),
			yesno(inPSet),
			st.Ticket.Epoch,
			st.Seed.Epoch,
			st.Deals,
		)
	}

	if err := tw.Flush(); err != nil {
		return nil, err
	}
	return w.Bytes(), nil
}

func sectorsList(t *testkit.TestEnvironment, m *testkit.LotusMiner, maddr address.Address, w io.Writer, height abi.ChainEpoch) (*SectorInfo, error) {
	node := m.FullApi
	ctx := context.Background()

	list, err := m.MinerApi.SectorsList(ctx)
	if err != nil {
		return nil, err
	}

	activeSet, err := node.StateMinerActiveSectors(ctx, maddr, types.EmptyTSK)
	if err != nil {
		return nil, err
	}
	activeIDs := make(map[abi.SectorNumber]struct{}, len(activeSet))
	for _, info := range activeSet {
		activeIDs[info.SectorNumber] = struct{}{}
	}

	sset, err := node.StateMinerSectors(ctx, maddr, nil, types.EmptyTSK)
	if err != nil {
		return nil, err
	}
	commitedIDs := make(map[abi.SectorNumber]struct{}, len(activeSet))
	for _, info := range sset {
		commitedIDs[info.SectorNumber] = struct{}{}
	}

	sort.Slice(list, func(i, j int) bool {
		return list[i] < list[j]
	})

	i := SectorInfo{Sectors: list, SectorStates: make(map[abi.SectorNumber]api.SectorInfo, len(list))}

	for _, s := range list {
		st, err := m.MinerApi.SectorsStatus(ctx, s, true)
		if err != nil {
			fmt.Fprintf(w, "%d:\tError: %s\n", s, err)
			continue
		}
		i.SectorStates[s] = st
	}
	return &i, nil
}

func yesno(b bool) string {
	if b {
		return "YES"
	}
	return "NO"
}

type MinerInfo struct {
	MinerAddr  address.Address
	SectorSize string

	MinerPower *api.MinerPower

	CommittedBytes   big.Int
	ProvingBytes     big.Int
	FaultyBytes      big.Int
	FaultyPercentage float64

	Balance           big.Int
	PreCommitDeposits big.Int
	LockedFunds       big.Int
	AvailableFunds    big.Int
	WorkerBalance     big.Int
	MarketEscrow      big.Int
	MarketLocked      big.Int

	SectorStateCounts map[sealing.SectorState]int
}

func (i *MinerInfo) MarshalPlainText() ([]byte, error) {
	w := new(bytes.Buffer)
	fmt.Fprintf(w, "Miner: %s\n", i.MinerAddr)
	fmt.Fprintf(w, "Sector Size: %s\n", i.SectorSize)

	pow := i.MinerPower

	fmt.Fprintf(w, "Byte Power:   %s / %s (%0.4f%%)\n",
		types.SizeStr(pow.MinerPower.RawBytePower),
		types.SizeStr(pow.TotalPower.RawBytePower),
		types.BigDivFloat(
			types.BigMul(pow.MinerPower.RawBytePower, big.NewInt(100)),
			pow.TotalPower.RawBytePower,
		),
	)

	fmt.Fprintf(w, "Actual Power: %s / %s (%0.4f%%)\n",
		types.DeciStr(pow.MinerPower.QualityAdjPower),
		types.DeciStr(pow.TotalPower.QualityAdjPower),
		types.BigDivFloat(
			types.BigMul(pow.MinerPower.QualityAdjPower, big.NewInt(100)),
			pow.TotalPower.QualityAdjPower,
		),
	)

	fmt.Fprintf(w, "\tCommitted: %s\n", types.SizeStr(i.CommittedBytes))

	if i.FaultyBytes.Int == nil || i.FaultyBytes.IsZero() {
		fmt.Fprintf(w, "\tProving: %s\n", types.SizeStr(i.ProvingBytes))
	} else {
		fmt.Fprintf(w, "\tProving: %s (%s Faulty, %.2f%%)\n",
			types.SizeStr(i.ProvingBytes),
			types.SizeStr(i.FaultyBytes),
			i.FaultyPercentage)
	}

	if !i.MinerPower.HasMinPower {
		fmt.Fprintf(w, "Below minimum power threshold, no blocks will be won\n")
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

			fmt.Fprintf(w, "Projected average block win rate: %.02f/week (every %s)\n",
				weekly,
				(time.Second * time.Duration(avgDuration)).Truncate(time.Second).String(),
			)

			// Geometric distribution of P(Y < k) calculated as described in https://en.wikipedia.org/wiki/Geometric_distribution#Probability_Outcomes_Examples
			// https://www.wolframalpha.com/input/?i=t+%3E+0%3B+p+%3E+0%3B+p+%3C+1%3B+c+%3E+0%3B+c+%3C1%3B+1-%281-p%29%5E%28t%29%3Dc%3B+solve+t
			// t == how many dice-rolls (epochs) before win
			// p == winRate == ( minerPower / netPower )
			// c == target probability of win ( 99.9% in this case )
			fmt.Fprintf(w, "Projected block win with 99.9%% probability every %s\n",
				(time.Second * time.Duration(
					builtin.EpochDurationSeconds*math.Log(1-0.999)/
						math.Log(1-winRatioFloat),
				)).Truncate(time.Second).String(),
			)
			fmt.Fprintln(w, "(projections DO NOT account for future network and miner growth)")
		}
	}

	fmt.Fprintf(w, "Miner Balance: %s\n", types.FIL(i.Balance))
	fmt.Fprintf(w, "\tPreCommit:   %s\n", types.FIL(i.PreCommitDeposits))
	fmt.Fprintf(w, "\tLocked:      %s\n", types.FIL(i.LockedFunds))
	fmt.Fprintf(w, "\tAvailable:   %s\n", types.FIL(i.AvailableFunds))
	fmt.Fprintf(w, "Worker Balance: %s\n", types.FIL(i.WorkerBalance))
	fmt.Fprintf(w, "Market (Escrow):  %s\n", types.FIL(i.MarketEscrow))
	fmt.Fprintf(w, "Market (Locked):  %s\n\n", types.FIL(i.MarketLocked))

	buckets := i.SectorStateCounts

	var sorted []stateMeta
	for state, i := range buckets {
		sorted = append(sorted, stateMeta{i: i, state: state})
	}

	sort.Slice(sorted, func(i, j int) bool {
		return stateOrder[sorted[i].state].i < stateOrder[sorted[j].state].i
	})

	for _, s := range sorted {
		_, _ = fmt.Fprintf(w, "\t%s: %d\n", s.state, s.i)
	}

	return w.Bytes(), nil
}

func info(t *testkit.TestEnvironment, m *testkit.LotusMiner, maddr address.Address, w io.Writer, height abi.ChainEpoch) (*MinerInfo, error) {
	api := m.FullApi
	ctx := context.Background()

	ts, err := api.ChainHead(ctx)
	if err != nil {
		return nil, err
	}

	mact, err := api.StateGetActor(ctx, maddr, ts.Key())
	if err != nil {
		return nil, err
	}

	i := MinerInfo{MinerAddr: maddr}

	// Sector size
	mi, err := api.StateMinerInfo(ctx, maddr, ts.Key())
	if err != nil {
		return nil, err
	}

	i.SectorSize = types.SizeStr(types.NewInt(uint64(mi.SectorSize)))

	i.MinerPower, err = api.StateMinerPower(ctx, maddr, ts.Key())
	if err != nil {
		return nil, err
	}

	secCounts, err := api.StateMinerSectorCount(ctx, maddr, ts.Key())
	if err != nil {
		return nil, err
	}
	faults, err := api.StateMinerFaults(ctx, maddr, ts.Key())
	if err != nil {
		return nil, err
	}

	nfaults, err := faults.Count()
	if err != nil {
		return nil, err
	}

	i.CommittedBytes = types.BigMul(types.NewInt(secCounts.Live), types.NewInt(uint64(mi.SectorSize)))
	i.ProvingBytes = types.BigMul(types.NewInt(secCounts.Active), types.NewInt(uint64(mi.SectorSize)))

	if nfaults != 0 {
		if secCounts.Live != 0 {
			i.FaultyPercentage = float64(10000*nfaults/secCounts.Live) / 100.
		}
		i.FaultyBytes = types.BigMul(types.NewInt(nfaults), types.NewInt(uint64(mi.SectorSize)))
	}

	stor := store.ActorStore(ctx, blockstore.NewAPIBlockstore(api))
	mas, err := miner.Load(stor, mact)
	if err != nil {
		return nil, err
	}

	funds, err := mas.LockedFunds()
	if err != nil {
		return nil, err
	}

	i.Balance = mact.Balance
	i.PreCommitDeposits = funds.PreCommitDeposits
	i.LockedFunds = funds.VestingFunds
	i.AvailableFunds, err = mas.AvailableBalance(mact.Balance)
	if err != nil {
		return nil, err
	}

	wb, err := api.WalletBalance(ctx, mi.Worker)
	if err != nil {
		return nil, err
	}
	i.WorkerBalance = wb

	mb, err := api.StateMarketBalance(ctx, maddr, types.EmptyTSK)
	if err != nil {
		return nil, err
	}
	i.MarketEscrow = mb.Escrow
	i.MarketLocked = mb.Locked

	sectors, err := m.MinerApi.SectorsList(ctx)
	if err != nil {
		return nil, err
	}

	buckets := map[sealing.SectorState]int{
		"Total": len(sectors),
	}
	for _, s := range sectors {
		st, err := m.MinerApi.SectorsStatus(ctx, s, true)
		if err != nil {
			return nil, err
		}

		buckets[sealing.SectorState(st.State)]++
	}
	i.SectorStateCounts = buckets

	return &i, nil
}

type stateMeta struct {
	i     int
	state sealing.SectorState
}

var stateOrder = map[sealing.SectorState]stateMeta{}
var stateList = []stateMeta{
	{state: "Total"},
	{state: sealing.Proving},

	{state: sealing.UndefinedSectorState},
	{state: sealing.Empty},
	{state: sealing.Packing},
	{state: sealing.PreCommit1},
	{state: sealing.PreCommit2},
	{state: sealing.PreCommitting},
	{state: sealing.PreCommitWait},
	{state: sealing.WaitSeed},
	{state: sealing.Committing},
	{state: sealing.CommitWait},
	{state: sealing.FinalizeSector},

	{state: sealing.FailedUnrecoverable},
	{state: sealing.SealPreCommit1Failed},
	{state: sealing.SealPreCommit2Failed},
	{state: sealing.PreCommitFailed},
	{state: sealing.ComputeProofFailed},
	{state: sealing.CommitFailed},
	{state: sealing.PackingFailed},
	{state: sealing.FinalizeFailed},
	{state: sealing.Faulty},
	{state: sealing.FaultReported},
	{state: sealing.FaultedFinal},
}

func init() {
	for i, state := range stateList {
		stateOrder[state.state] = stateMeta{
			i: i,
		}
	}
}
