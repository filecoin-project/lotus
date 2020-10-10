package test

import (
	"context"
	"fmt"
	"sync/atomic"

	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/extern/sector-storage/mock"
	sealing "github.com/filecoin-project/lotus/extern/storage-sealing"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/types"
	bminer "github.com/filecoin-project/lotus/miner"
	"github.com/filecoin-project/lotus/node/impl"
)

func init() {
	err := os.Setenv("BELLMAN_NO_GPU", "1")
	if err != nil {
		panic(fmt.Sprintf("failed to set BELLMAN_NO_GPU env variable: %s", err))
	}
}

func TestPledgeSector(t *testing.T, b APIBuilder, blocktime time.Duration, nSectors int) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	n, sn := b(t, OneFull, OneMiner)
	client := n[0].FullNode.(*impl.FullNodeAPI)
	miner := sn[0]

	addrinfo, err := client.NetAddrsListen(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if err := miner.NetConnect(ctx, addrinfo); err != nil {
		t.Fatal(err)
	}
	build.Clock.Sleep(time.Second)

	mine := int64(1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for atomic.LoadInt64(&mine) != 0 {
			build.Clock.Sleep(blocktime)
			if err := sn[0].MineOne(ctx, bminer.MineReq{Done: func(bool, abi.ChainEpoch, error) {

			}}); err != nil {
				t.Error(err)
			}
		}
	}()

	pledgeSectors(t, ctx, miner, nSectors, 0, nil)

	atomic.StoreInt64(&mine, 0)
	<-done
}

func pledgeSectors(t *testing.T, ctx context.Context, miner TestStorageNode, n, existing int, blockNotif <-chan struct{}) {
	for i := 0; i < n; i++ {
		err := miner.PledgeSector(ctx)
		require.NoError(t, err)
		if i%3 == 0 && blockNotif != nil {
			<-blockNotif
		}
	}

	for {
		s, err := miner.SectorsList(ctx) // Note - the test builder doesn't import genesis sectors into FSM
		require.NoError(t, err)
		fmt.Printf("Sectors: %d\n", len(s))
		if len(s) >= n+existing {
			break
		}

		build.Clock.Sleep(100 * time.Millisecond)
	}

	fmt.Printf("All sectors is fsm\n")

	s, err := miner.SectorsList(ctx)
	require.NoError(t, err)

	toCheck := map[abi.SectorNumber]struct{}{}
	for _, number := range s {
		toCheck[number] = struct{}{}
	}

	for len(toCheck) > 0 {
		for n := range toCheck {
			st, err := miner.SectorsStatus(ctx, n, false)
			require.NoError(t, err)
			if st.State == api.SectorState(sealing.Proving) {
				delete(toCheck, n)
			}
			if strings.Contains(string(st.State), "Fail") {
				t.Fatal("sector in a failed state", st.State)
			}
		}

		build.Clock.Sleep(100 * time.Millisecond)
		fmt.Printf("WaitSeal: %d\n", len(s))
	}
}

func TestWindowPost(t *testing.T, b APIBuilder, blocktime time.Duration, nSectors int) {
	for _, height := range []abi.ChainEpoch{
		1,    // before
		162,  // while sealing
		5000, // while proving
	} {
		height := height // copy to satisfy lints
		t.Run(fmt.Sprintf("upgrade-%d", height), func(t *testing.T) {
			testWindowPostUpgrade(t, b, blocktime, nSectors, height)
		})
	}

}
func testWindowPostUpgrade(t *testing.T, b APIBuilder, blocktime time.Duration, nSectors int,
	upgradeHeight abi.ChainEpoch) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	n, sn := b(t, []FullNodeOpts{FullNodeWithUpgradeAt(upgradeHeight)}, OneMiner)

	client := n[0].FullNode.(*impl.FullNodeAPI)
	miner := sn[0]

	addrinfo, err := client.NetAddrsListen(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if err := miner.NetConnect(ctx, addrinfo); err != nil {
		t.Fatal(err)
	}
	build.Clock.Sleep(time.Second)

	done := make(chan struct{})
	go func() {
		defer close(done)
		for ctx.Err() == nil {
			build.Clock.Sleep(blocktime)
			if err := sn[0].MineOne(ctx, MineNext); err != nil {
				if ctx.Err() != nil {
					// context was canceled, ignore the error.
					return
				}
				t.Error(err)
			}
		}
	}()
	defer func() {
		cancel()
		<-done
	}()

	pledgeSectors(t, ctx, miner, nSectors, 0, nil)

	maddr, err := miner.ActorAddress(ctx)
	require.NoError(t, err)

	di, err := client.StateMinerProvingDeadline(ctx, maddr, types.EmptyTSK)
	require.NoError(t, err)

	mid, err := address.IDFromAddress(maddr)
	require.NoError(t, err)

	fmt.Printf("Running one proving period\n")
	fmt.Printf("End for head.Height > %d\n", di.PeriodStart+di.WPoStProvingPeriod+2)

	for {
		head, err := client.ChainHead(ctx)
		require.NoError(t, err)

		if head.Height() > di.PeriodStart+di.WPoStProvingPeriod+2 {
			fmt.Printf("Now head.Height = %d\n", head.Height())
			break
		}
		build.Clock.Sleep(blocktime)
	}

	p, err := client.StateMinerPower(ctx, maddr, types.EmptyTSK)
	require.NoError(t, err)

	ssz, err := miner.ActorSectorSize(ctx, maddr)
	require.NoError(t, err)

	require.Equal(t, p.MinerPower, p.TotalPower)
	require.Equal(t, p.MinerPower.RawBytePower, types.NewInt(uint64(ssz)*uint64(nSectors+GenesisPreseals)))

	fmt.Printf("Drop some sectors\n")

	// Drop 2 sectors from deadline 2 partition 0 (full partition / deadline)
	{
		parts, err := client.StateMinerPartitions(ctx, maddr, 2, types.EmptyTSK)
		require.NoError(t, err)
		require.Greater(t, len(parts), 0)

		secs := parts[0].AllSectors
		n, err := secs.Count()
		require.NoError(t, err)
		require.Equal(t, uint64(2), n)

		// Drop the partition
		err = secs.ForEach(func(sid uint64) error {
			return miner.StorageMiner.(*impl.StorageMinerAPI).IStorageMgr.(*mock.SectorMgr).MarkCorrupted(abi.SectorID{
				Miner:  abi.ActorID(mid),
				Number: abi.SectorNumber(sid),
			}, true)
		})
		require.NoError(t, err)
	}

	var s abi.SectorID

	// Drop 1 sectors from deadline 3 partition 0
	{
		parts, err := client.StateMinerPartitions(ctx, maddr, 3, types.EmptyTSK)
		require.NoError(t, err)
		require.Greater(t, len(parts), 0)

		secs := parts[0].AllSectors
		n, err := secs.Count()
		require.NoError(t, err)
		require.Equal(t, uint64(2), n)

		// Drop the sector
		sn, err := secs.First()
		require.NoError(t, err)

		all, err := secs.All(2)
		require.NoError(t, err)
		fmt.Println("the sectors", all)

		s = abi.SectorID{
			Miner:  abi.ActorID(mid),
			Number: abi.SectorNumber(sn),
		}

		err = miner.StorageMiner.(*impl.StorageMinerAPI).IStorageMgr.(*mock.SectorMgr).MarkFailed(s, true)
		require.NoError(t, err)
	}

	di, err = client.StateMinerProvingDeadline(ctx, maddr, types.EmptyTSK)
	require.NoError(t, err)

	fmt.Printf("Go through another PP, wait for sectors to become faulty\n")
	fmt.Printf("End for head.Height > %d\n", di.PeriodStart+di.WPoStProvingPeriod+2)

	for {
		head, err := client.ChainHead(ctx)
		require.NoError(t, err)

		if head.Height() > di.PeriodStart+(di.WPoStProvingPeriod)+2 {
			fmt.Printf("Now head.Height = %d\n", head.Height())
			break
		}

		build.Clock.Sleep(blocktime)
	}

	p, err = client.StateMinerPower(ctx, maddr, types.EmptyTSK)
	require.NoError(t, err)

	require.Equal(t, p.MinerPower, p.TotalPower)

	sectors := p.MinerPower.RawBytePower.Uint64() / uint64(ssz)
	require.Equal(t, nSectors+GenesisPreseals-3, int(sectors)) // -3 just removed sectors

	fmt.Printf("Recover one sector\n")

	err = miner.StorageMiner.(*impl.StorageMinerAPI).IStorageMgr.(*mock.SectorMgr).MarkFailed(s, false)
	require.NoError(t, err)

	di, err = client.StateMinerProvingDeadline(ctx, maddr, types.EmptyTSK)
	require.NoError(t, err)

	fmt.Printf("End for head.Height > %d\n", di.PeriodStart+di.WPoStProvingPeriod+2)

	for {
		head, err := client.ChainHead(ctx)
		require.NoError(t, err)

		if head.Height() > di.PeriodStart+di.WPoStProvingPeriod+2 {
			fmt.Printf("Now head.Height = %d\n", head.Height())
			break
		}

		build.Clock.Sleep(blocktime)
	}

	p, err = client.StateMinerPower(ctx, maddr, types.EmptyTSK)
	require.NoError(t, err)

	require.Equal(t, p.MinerPower, p.TotalPower)

	sectors = p.MinerPower.RawBytePower.Uint64() / uint64(ssz)
	require.Equal(t, nSectors+GenesisPreseals-2, int(sectors)) // -2 not recovered sectors

	// pledge a sector after recovery

	pledgeSectors(t, ctx, miner, 1, nSectors, nil)

	{
		// Wait until proven.
		di, err = client.StateMinerProvingDeadline(ctx, maddr, types.EmptyTSK)
		require.NoError(t, err)

		waitUntil := di.PeriodStart + di.WPoStProvingPeriod + 2
		fmt.Printf("End for head.Height > %d\n", waitUntil)

		for {
			head, err := client.ChainHead(ctx)
			require.NoError(t, err)

			if head.Height() > waitUntil {
				fmt.Printf("Now head.Height = %d\n", head.Height())
				break
			}
		}
	}

	p, err = client.StateMinerPower(ctx, maddr, types.EmptyTSK)
	require.NoError(t, err)

	require.Equal(t, p.MinerPower, p.TotalPower)

	sectors = p.MinerPower.RawBytePower.Uint64() / uint64(ssz)
	require.Equal(t, nSectors+GenesisPreseals-2+1, int(sectors)) // -2 not recovered sectors + 1 just pledged
}
