package rfwp

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"sync"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/lotus/testplans/lotus-soup/testkit"
)

type ChainState struct {
	sync.Mutex

	PrevHeight abi.ChainEpoch
	DiffHeight map[string]map[string]map[abi.ChainEpoch]big.Int  // height -> value
	DiffValue  map[string]map[string]map[string][]abi.ChainEpoch // value -> []height
	DiffCmp    map[string]map[string]map[string][]abi.ChainEpoch // difference (height, height-1) -> []height
	valueTypes []string
}

func NewChainState() *ChainState {
	cs := &ChainState{}
	cs.PrevHeight = abi.ChainEpoch(-1)
	cs.DiffHeight = make(map[string]map[string]map[abi.ChainEpoch]big.Int) // height -> value
	cs.DiffValue = make(map[string]map[string]map[string][]abi.ChainEpoch) // value -> []height
	cs.DiffCmp = make(map[string]map[string]map[string][]abi.ChainEpoch)   // difference (height, height-1) -> []height
	cs.valueTypes = []string{"MinerPower", "CommittedBytes", "ProvingBytes", "Balance", "PreCommitDeposits", "LockedFunds", "AvailableFunds", "WorkerBalance", "MarketEscrow", "MarketLocked", "Faults", "ProvenSectors", "Recoveries"}
	return cs
}

var (
	cs *ChainState
)

func init() {
	cs = NewChainState()
}

func printDiff(t *testkit.TestEnvironment, mi *MinerInfo, height abi.ChainEpoch) {
	maddr := mi.MinerAddr.String()
	filename := fmt.Sprintf("%s%cdiff-%s-%d", t.TestOutputsPath, os.PathSeparator, maddr, height)

	f, err := os.Create(filename)
	if err != nil {
		panic(err)
	}
	defer f.Close()

	w := bufio.NewWriter(f)
	defer w.Flush()

	keys := make([]string, 0, len(cs.DiffCmp[maddr]))
	for k := range cs.DiffCmp[maddr] {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	fmt.Fprintln(w, "=====", maddr, "=====")
	for i, valueName := range keys {
		fmt.Fprintln(w, toCharStr(i), "=====", valueName, "=====")
		if len(cs.DiffCmp[maddr][valueName]) > 0 {
			fmt.Fprintf(w, "%s diff of             |\n", toCharStr(i))
		}

		for difference, heights := range cs.DiffCmp[maddr][valueName] {
			fmt.Fprintf(w, "%s diff of %30v at heights %v\n", toCharStr(i), difference, heights)
		}
	}
}

func recordDiff(mi *MinerInfo, ps *ProvingInfoState, height abi.ChainEpoch) {
	maddr := mi.MinerAddr.String()
	if _, ok := cs.DiffHeight[maddr]; !ok {
		cs.DiffHeight[maddr] = make(map[string]map[abi.ChainEpoch]big.Int)
		cs.DiffValue[maddr] = make(map[string]map[string][]abi.ChainEpoch)
		cs.DiffCmp[maddr] = make(map[string]map[string][]abi.ChainEpoch)

		for _, v := range cs.valueTypes {
			cs.DiffHeight[maddr][v] = make(map[abi.ChainEpoch]big.Int)
			cs.DiffValue[maddr][v] = make(map[string][]abi.ChainEpoch)
			cs.DiffCmp[maddr][v] = make(map[string][]abi.ChainEpoch)
		}
	}

	{
		value := big.Int(mi.MinerPower.MinerPower.RawBytePower)
		cs.DiffHeight[maddr]["MinerPower"][height] = value
		cs.DiffValue[maddr]["MinerPower"][value.String()] = append(cs.DiffValue[maddr]["MinerPower"][value.String()], height)

		if cs.PrevHeight != -1 {
			prevValue := cs.DiffHeight[maddr]["MinerPower"][cs.PrevHeight]
			cmp := big.Zero()
			cmp.Sub(value.Int, prevValue.Int) // value - prevValue
			if big.Cmp(cmp, big.Zero()) != 0 {
				cs.DiffCmp[maddr]["MinerPower"][cmp.String()] = append(cs.DiffCmp[maddr]["MinerPower"][cmp.String()], height)
			}
		}
	}

	{
		value := big.Int(mi.CommittedBytes)
		cs.DiffHeight[maddr]["CommittedBytes"][height] = value
		cs.DiffValue[maddr]["CommittedBytes"][value.String()] = append(cs.DiffValue[maddr]["CommittedBytes"][value.String()], height)

		if cs.PrevHeight != -1 {
			prevValue := cs.DiffHeight[maddr]["CommittedBytes"][cs.PrevHeight]
			cmp := big.Zero()
			cmp.Sub(value.Int, prevValue.Int) // value - prevValue
			if big.Cmp(cmp, big.Zero()) != 0 {
				cs.DiffCmp[maddr]["CommittedBytes"][cmp.String()] = append(cs.DiffCmp[maddr]["CommittedBytes"][cmp.String()], height)
			}
		}
	}

	{
		value := big.Int(mi.ProvingBytes)
		cs.DiffHeight[maddr]["ProvingBytes"][height] = value
		cs.DiffValue[maddr]["ProvingBytes"][value.String()] = append(cs.DiffValue[maddr]["ProvingBytes"][value.String()], height)

		if cs.PrevHeight != -1 {
			prevValue := cs.DiffHeight[maddr]["ProvingBytes"][cs.PrevHeight]
			cmp := big.Zero()
			cmp.Sub(value.Int, prevValue.Int) // value - prevValue
			if big.Cmp(cmp, big.Zero()) != 0 {
				cs.DiffCmp[maddr]["ProvingBytes"][cmp.String()] = append(cs.DiffCmp[maddr]["ProvingBytes"][cmp.String()], height)
			}
		}
	}

	{
		value := big.Int(mi.Balance)
		roundBalance(&value)
		cs.DiffHeight[maddr]["Balance"][height] = value
		cs.DiffValue[maddr]["Balance"][value.String()] = append(cs.DiffValue[maddr]["Balance"][value.String()], height)

		if cs.PrevHeight != -1 {
			prevValue := cs.DiffHeight[maddr]["Balance"][cs.PrevHeight]
			cmp := big.Zero()
			cmp.Sub(value.Int, prevValue.Int) // value - prevValue
			if big.Cmp(cmp, big.Zero()) != 0 {
				cs.DiffCmp[maddr]["Balance"][cmp.String()] = append(cs.DiffCmp[maddr]["Balance"][cmp.String()], height)
			}
		}
	}

	{
		value := big.Int(mi.PreCommitDeposits)
		cs.DiffHeight[maddr]["PreCommitDeposits"][height] = value
		cs.DiffValue[maddr]["PreCommitDeposits"][value.String()] = append(cs.DiffValue[maddr]["PreCommitDeposits"][value.String()], height)

		if cs.PrevHeight != -1 {
			prevValue := cs.DiffHeight[maddr]["PreCommitDeposits"][cs.PrevHeight]
			cmp := big.Zero()
			cmp.Sub(value.Int, prevValue.Int) // value - prevValue
			if big.Cmp(cmp, big.Zero()) != 0 {
				cs.DiffCmp[maddr]["PreCommitDeposits"][cmp.String()] = append(cs.DiffCmp[maddr]["PreCommitDeposits"][cmp.String()], height)
			}
		}
	}

	{
		value := big.Int(mi.LockedFunds)
		roundBalance(&value)
		cs.DiffHeight[maddr]["LockedFunds"][height] = value
		cs.DiffValue[maddr]["LockedFunds"][value.String()] = append(cs.DiffValue[maddr]["LockedFunds"][value.String()], height)

		if cs.PrevHeight != -1 {
			prevValue := cs.DiffHeight[maddr]["LockedFunds"][cs.PrevHeight]
			cmp := big.Zero()
			cmp.Sub(value.Int, prevValue.Int) // value - prevValue
			if big.Cmp(cmp, big.Zero()) != 0 {
				cs.DiffCmp[maddr]["LockedFunds"][cmp.String()] = append(cs.DiffCmp[maddr]["LockedFunds"][cmp.String()], height)
			}
		}
	}

	{
		value := big.Int(mi.AvailableFunds)
		roundBalance(&value)
		cs.DiffHeight[maddr]["AvailableFunds"][height] = value
		cs.DiffValue[maddr]["AvailableFunds"][value.String()] = append(cs.DiffValue[maddr]["AvailableFunds"][value.String()], height)

		if cs.PrevHeight != -1 {
			prevValue := cs.DiffHeight[maddr]["AvailableFunds"][cs.PrevHeight]
			cmp := big.Zero()
			cmp.Sub(value.Int, prevValue.Int) // value - prevValue
			if big.Cmp(cmp, big.Zero()) != 0 {
				cs.DiffCmp[maddr]["AvailableFunds"][cmp.String()] = append(cs.DiffCmp[maddr]["AvailableFunds"][cmp.String()], height)
			}
		}
	}

	{
		value := big.Int(mi.WorkerBalance)
		cs.DiffHeight[maddr]["WorkerBalance"][height] = value
		cs.DiffValue[maddr]["WorkerBalance"][value.String()] = append(cs.DiffValue[maddr]["WorkerBalance"][value.String()], height)

		if cs.PrevHeight != -1 {
			prevValue := cs.DiffHeight[maddr]["WorkerBalance"][cs.PrevHeight]
			cmp := big.Zero()
			cmp.Sub(value.Int, prevValue.Int) // value - prevValue
			if big.Cmp(cmp, big.Zero()) != 0 {
				cs.DiffCmp[maddr]["WorkerBalance"][cmp.String()] = append(cs.DiffCmp[maddr]["WorkerBalance"][cmp.String()], height)
			}
		}
	}

	{
		value := big.Int(mi.MarketEscrow)
		cs.DiffHeight[maddr]["MarketEscrow"][height] = value
		cs.DiffValue[maddr]["MarketEscrow"][value.String()] = append(cs.DiffValue[maddr]["MarketEscrow"][value.String()], height)

		if cs.PrevHeight != -1 {
			prevValue := cs.DiffHeight[maddr]["MarketEscrow"][cs.PrevHeight]
			cmp := big.Zero()
			cmp.Sub(value.Int, prevValue.Int) // value - prevValue
			if big.Cmp(cmp, big.Zero()) != 0 {
				cs.DiffCmp[maddr]["MarketEscrow"][cmp.String()] = append(cs.DiffCmp[maddr]["MarketEscrow"][cmp.String()], height)
			}
		}
	}

	{
		value := big.Int(mi.MarketLocked)
		cs.DiffHeight[maddr]["MarketLocked"][height] = value
		cs.DiffValue[maddr]["MarketLocked"][value.String()] = append(cs.DiffValue[maddr]["MarketLocked"][value.String()], height)

		if cs.PrevHeight != -1 {
			prevValue := cs.DiffHeight[maddr]["MarketLocked"][cs.PrevHeight]
			cmp := big.Zero()
			cmp.Sub(value.Int, prevValue.Int) // value - prevValue
			if big.Cmp(cmp, big.Zero()) != 0 {
				cs.DiffCmp[maddr]["MarketLocked"][cmp.String()] = append(cs.DiffCmp[maddr]["MarketLocked"][cmp.String()], height)
			}
		}
	}

	{
		value := big.NewInt(int64(ps.Faults))
		cs.DiffHeight[maddr]["Faults"][height] = value
		cs.DiffValue[maddr]["Faults"][value.String()] = append(cs.DiffValue[maddr]["Faults"][value.String()], height)

		if cs.PrevHeight != -1 {
			prevValue := cs.DiffHeight[maddr]["Faults"][cs.PrevHeight]
			cmp := big.Zero()
			cmp.Sub(value.Int, prevValue.Int) // value - prevValue
			if big.Cmp(cmp, big.Zero()) != 0 {
				cs.DiffCmp[maddr]["Faults"][cmp.String()] = append(cs.DiffCmp[maddr]["Faults"][cmp.String()], height)
			}
		}
	}

	{
		value := big.NewInt(int64(ps.ProvenSectors))
		cs.DiffHeight[maddr]["ProvenSectors"][height] = value
		cs.DiffValue[maddr]["ProvenSectors"][value.String()] = append(cs.DiffValue[maddr]["ProvenSectors"][value.String()], height)

		if cs.PrevHeight != -1 {
			prevValue := cs.DiffHeight[maddr]["ProvenSectors"][cs.PrevHeight]
			cmp := big.Zero()
			cmp.Sub(value.Int, prevValue.Int) // value - prevValue
			if big.Cmp(cmp, big.Zero()) != 0 {
				cs.DiffCmp[maddr]["ProvenSectors"][cmp.String()] = append(cs.DiffCmp[maddr]["ProvenSectors"][cmp.String()], height)
			}
		}
	}

	{
		value := big.NewInt(int64(ps.Recoveries))
		cs.DiffHeight[maddr]["Recoveries"][height] = value
		cs.DiffValue[maddr]["Recoveries"][value.String()] = append(cs.DiffValue[maddr]["Recoveries"][value.String()], height)

		if cs.PrevHeight != -1 {
			prevValue := cs.DiffHeight[maddr]["Recoveries"][cs.PrevHeight]
			cmp := big.Zero()
			cmp.Sub(value.Int, prevValue.Int) // value - prevValue
			if big.Cmp(cmp, big.Zero()) != 0 {
				cs.DiffCmp[maddr]["Recoveries"][cmp.String()] = append(cs.DiffCmp[maddr]["Recoveries"][cmp.String()], height)
			}
		}
	}
}

func roundBalance(i *big.Int) {
	*i = big.Div(*i, big.NewInt(1000000000000000))
	*i = big.Mul(*i, big.NewInt(1000000000000000))
}

func toCharStr(i int) string {
	return string('a' + i)
}
