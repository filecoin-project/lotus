package test

import (
	"context"
	"fmt"
	"sort"
	"sync/atomic"

	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-bitfield"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/crypto"
	"github.com/filecoin-project/go-state-types/dline"
	"github.com/filecoin-project/go-state-types/network"
	"github.com/filecoin-project/lotus/extern/sector-storage/mock"
	sealing "github.com/filecoin-project/lotus/extern/storage-sealing"
	proof3 "github.com/filecoin-project/specs-actors/v3/actors/runtime/proof"
	"github.com/filecoin-project/specs-storage/storage"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors"
	minerActor "github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/types"
	bminer "github.com/filecoin-project/lotus/miner"
	"github.com/filecoin-project/lotus/node/impl"
)

func TestSDRUpgrade(t *testing.T, b APIBuilder, blocktime time.Duration) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	n, sn := b(t, []FullNodeOpts{FullNodeWithSDRAt(500, 1000)}, OneMiner)
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

	pledge := make(chan struct{})
	mine := int64(1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		round := 0
		for atomic.LoadInt64(&mine) != 0 {
			build.Clock.Sleep(blocktime)
			if err := sn[0].MineOne(ctx, bminer.MineReq{Done: func(bool, abi.ChainEpoch, error) {

			}}); err != nil {
				t.Error(err)
			}

			// 3 sealing rounds: before, during after.
			if round >= 3 {
				continue
			}

			head, err := client.ChainHead(ctx)
			assert.NoError(t, err)

			// rounds happen every 100 blocks, with a 50 block offset.
			if head.Height() >= abi.ChainEpoch(round*500+50) {
				round++
				pledge <- struct{}{}

				ver, err := client.StateNetworkVersion(ctx, head.Key())
				assert.NoError(t, err)
				switch round {
				case 1:
					assert.Equal(t, network.Version6, ver)
				case 2:
					assert.Equal(t, network.Version7, ver)
				case 3:
					assert.Equal(t, network.Version8, ver)
				}
			}

		}
	}()

	// before.
	pledgeSectors(t, ctx, miner, 9, 0, pledge)

	s, err := miner.SectorsList(ctx)
	require.NoError(t, err)
	sort.Slice(s, func(i, j int) bool {
		return s[i] < s[j]
	})

	for i, id := range s {
		info, err := miner.SectorsStatus(ctx, id, true)
		require.NoError(t, err)
		expectProof := abi.RegisteredSealProof_StackedDrg2KiBV1
		if i >= 3 {
			// after
			expectProof = abi.RegisteredSealProof_StackedDrg2KiBV1_1
		}
		assert.Equal(t, expectProof, info.SealProof, "sector %d, id %d", i, id)
	}

	atomic.StoreInt64(&mine, 0)
	<-done
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
		if i%3 == 0 && blockNotif != nil {
			<-blockNotif
			log.Errorf("WAIT")
		}
		log.Errorf("PLEDGING %d", i)
		_, err := miner.PledgeSector(ctx)
		require.NoError(t, err)
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
		2,    // before
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

	n, sn := b(t, []FullNodeOpts{FullNodeWithActorsV3At(upgradeHeight)}, OneMiner)

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
			return miner.StorageMiner.(*impl.StorageMinerAPI).IStorageMgr.(*mock.SectorMgr).MarkCorrupted(storage.SectorRef{
				ID: abi.SectorID{
					Miner:  abi.ActorID(mid),
					Number: abi.SectorNumber(sid),
				},
			}, true)
		})
		require.NoError(t, err)
	}

	var s storage.SectorRef

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

		s = storage.SectorRef{
			ID: abi.SectorID{
				Miner:  abi.ActorID(mid),
				Number: abi.SectorNumber(sn),
			},
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

func TestTerminate(t *testing.T, b APIBuilder, blocktime time.Duration) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	nSectors := uint64(2)

	n, sn := b(t, []FullNodeOpts{FullNodeWithActorsV3At(2)}, []StorageMiner{{Full: 0, Preseal: int(nSectors)}})

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

	maddr, err := miner.ActorAddress(ctx)
	require.NoError(t, err)

	ssz, err := miner.ActorSectorSize(ctx, maddr)
	require.NoError(t, err)

	p, err := client.StateMinerPower(ctx, maddr, types.EmptyTSK)
	require.NoError(t, err)
	require.Equal(t, p.MinerPower, p.TotalPower)
	require.Equal(t, p.MinerPower.RawBytePower, types.NewInt(uint64(ssz)*nSectors))

	fmt.Printf("Seal a sector\n")

	pledgeSectors(t, ctx, miner, 1, 0, nil)

	fmt.Printf("wait for power\n")

	{
		// Wait until proven.
		di, err := client.StateMinerProvingDeadline(ctx, maddr, types.EmptyTSK)
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

	nSectors++

	p, err = client.StateMinerPower(ctx, maddr, types.EmptyTSK)
	require.NoError(t, err)
	require.Equal(t, p.MinerPower, p.TotalPower)
	require.Equal(t, p.MinerPower.RawBytePower, types.NewInt(uint64(ssz)*nSectors))

	fmt.Println("Terminate a sector")

	toTerminate := abi.SectorNumber(3)

	err = miner.SectorTerminate(ctx, toTerminate)
	require.NoError(t, err)

	msgTriggerred := false
loop:
	for {
		si, err := miner.SectorsStatus(ctx, toTerminate, false)
		require.NoError(t, err)

		fmt.Println("state: ", si.State, msgTriggerred)

		switch sealing.SectorState(si.State) {
		case sealing.Terminating:
			if !msgTriggerred {
				{
					p, err := miner.SectorTerminatePending(ctx)
					require.NoError(t, err)
					require.Len(t, p, 1)
					require.Equal(t, abi.SectorNumber(3), p[0].Number)
				}

				c, err := miner.SectorTerminateFlush(ctx)
				require.NoError(t, err)
				if c != nil {
					msgTriggerred = true
					fmt.Println("terminate message:", c)

					{
						p, err := miner.SectorTerminatePending(ctx)
						require.NoError(t, err)
						require.Len(t, p, 0)
					}
				}
			}
		case sealing.TerminateWait, sealing.TerminateFinality, sealing.Removed:
			break loop
		}

		time.Sleep(100 * time.Millisecond)
	}

	// check power decreased
	p, err = client.StateMinerPower(ctx, maddr, types.EmptyTSK)
	require.NoError(t, err)
	require.Equal(t, p.MinerPower, p.TotalPower)
	require.Equal(t, p.MinerPower.RawBytePower, types.NewInt(uint64(ssz)*(nSectors-1)))

	// check in terminated set
	{
		parts, err := client.StateMinerPartitions(ctx, maddr, 1, types.EmptyTSK)
		require.NoError(t, err)
		require.Greater(t, len(parts), 0)

		bflen := func(b bitfield.BitField) uint64 {
			l, err := b.Count()
			require.NoError(t, err)
			return l
		}

		require.Equal(t, uint64(1), bflen(parts[0].AllSectors))
		require.Equal(t, uint64(0), bflen(parts[0].LiveSectors))
	}

	di, err := client.StateMinerProvingDeadline(ctx, maddr, types.EmptyTSK)
	require.NoError(t, err)
	for {
		head, err := client.ChainHead(ctx)
		require.NoError(t, err)

		if head.Height() > di.PeriodStart+di.WPoStProvingPeriod+2 {
			fmt.Printf("Now head.Height = %d\n", head.Height())
			break
		}
		build.Clock.Sleep(blocktime)
	}
	require.NoError(t, err)
	fmt.Printf("End for head.Height > %d\n", di.PeriodStart+di.WPoStProvingPeriod+2)

	p, err = client.StateMinerPower(ctx, maddr, types.EmptyTSK)
	require.NoError(t, err)

	require.Equal(t, p.MinerPower, p.TotalPower)
	require.Equal(t, p.MinerPower.RawBytePower, types.NewInt(uint64(ssz)*(nSectors-1)))
}

func TestWindowPostDispute(t *testing.T, b APIBuilder, blocktime time.Duration) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// First, we configure two miners. After sealing, we're going to turn off the first miner so
	// it doesn't submit proofs.
	///
	// Then we're going to manually submit bad proofs.
	n, sn := b(t, []FullNodeOpts{
		FullNodeWithActorsV3At(2),
	}, []StorageMiner{
		{Full: 0, Preseal: PresealGenesis},
		{Full: 0},
	})

	client := n[0].FullNode.(*impl.FullNodeAPI)
	chainMiner := sn[0]
	evilMiner := sn[1]

	{
		addrinfo, err := client.NetAddrsListen(ctx)
		if err != nil {
			t.Fatal(err)
		}

		if err := chainMiner.NetConnect(ctx, addrinfo); err != nil {
			t.Fatal(err)
		}

		if err := evilMiner.NetConnect(ctx, addrinfo); err != nil {
			t.Fatal(err)
		}
	}

	defaultFrom, err := client.WalletDefaultAddress(ctx)
	require.NoError(t, err)

	build.Clock.Sleep(time.Second)

	// Mine with the _second_ node (the good one).
	done := make(chan struct{})
	go func() {
		defer close(done)
		for ctx.Err() == nil {
			build.Clock.Sleep(blocktime)
			if err := chainMiner.MineOne(ctx, MineNext); err != nil {
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

	// Give the chain miner enough sectors to win every block.
	pledgeSectors(t, ctx, chainMiner, 10, 0, nil)
	// And the evil one 1 sector. No cookie for you.
	pledgeSectors(t, ctx, evilMiner, 1, 0, nil)

	// Let the evil miner's sectors gain power.
	evilMinerAddr, err := evilMiner.ActorAddress(ctx)
	require.NoError(t, err)

	di, err := client.StateMinerProvingDeadline(ctx, evilMinerAddr, types.EmptyTSK)
	require.NoError(t, err)

	fmt.Printf("Running one proving period\n")
	fmt.Printf("End for head.Height > %d\n", di.PeriodStart+di.WPoStProvingPeriod*2)

	for {
		head, err := client.ChainHead(ctx)
		require.NoError(t, err)

		if head.Height() > di.PeriodStart+di.WPoStProvingPeriod*2 {
			fmt.Printf("Now head.Height = %d\n", head.Height())
			break
		}
		build.Clock.Sleep(blocktime)
	}

	p, err := client.StateMinerPower(ctx, evilMinerAddr, types.EmptyTSK)
	require.NoError(t, err)

	ssz, err := evilMiner.ActorSectorSize(ctx, evilMinerAddr)
	require.NoError(t, err)

	// make sure it has gained power.
	require.Equal(t, p.MinerPower.RawBytePower, types.NewInt(uint64(ssz)))

	evilSectors, err := evilMiner.SectorsList(ctx)
	require.NoError(t, err)
	evilSectorNo := evilSectors[0] // only one.
	evilSectorLoc, err := client.StateSectorPartition(ctx, evilMinerAddr, evilSectorNo, types.EmptyTSK)
	require.NoError(t, err)

	fmt.Println("evil miner stopping")

	// Now stop the evil miner, and start manually submitting bad proofs.
	require.NoError(t, evilMiner.Stop(ctx))

	fmt.Println("evil miner stopped")

	// Wait until we need to prove our sector.
	for {
		di, err = client.StateMinerProvingDeadline(ctx, evilMinerAddr, types.EmptyTSK)
		require.NoError(t, err)
		if di.Index == evilSectorLoc.Deadline {
			break
		}
		build.Clock.Sleep(blocktime)
	}

	err = submitBadProof(ctx, client, evilMinerAddr, di, evilSectorLoc.Deadline, evilSectorLoc.Partition)
	require.NoError(t, err, "evil proof not accepted")

	// Wait until after the proving period.
	for {
		di, err = client.StateMinerProvingDeadline(ctx, evilMinerAddr, types.EmptyTSK)
		require.NoError(t, err)
		if di.Index != evilSectorLoc.Deadline {
			break
		}
		build.Clock.Sleep(blocktime)
	}

	fmt.Println("accepted evil proof")

	// Make sure the evil node didn't lose any power.
	p, err = client.StateMinerPower(ctx, evilMinerAddr, types.EmptyTSK)
	require.NoError(t, err)
	require.Equal(t, p.MinerPower.RawBytePower, types.NewInt(uint64(ssz)))

	// OBJECTION! The good miner files a DISPUTE!!!!
	{
		params := &minerActor.DisputeWindowedPoStParams{
			Deadline:  evilSectorLoc.Deadline,
			PoStIndex: 0,
		}

		enc, aerr := actors.SerializeParams(params)
		require.NoError(t, aerr)

		msg := &types.Message{
			To:     evilMinerAddr,
			Method: minerActor.Methods.DisputeWindowedPoSt,
			Params: enc,
			Value:  types.NewInt(0),
			From:   defaultFrom,
		}
		sm, err := client.MpoolPushMessage(ctx, msg, nil)
		require.NoError(t, err)

		fmt.Println("waiting dispute")
		rec, err := client.StateWaitMsg(ctx, sm.Cid(), build.MessageConfidence)
		require.NoError(t, err)
		require.Zero(t, rec.Receipt.ExitCode, "dispute not accepted: %s", rec.Receipt.ExitCode.Error())
	}

	// Objection SUSTAINED!
	// Make sure the evil node lost power.
	p, err = client.StateMinerPower(ctx, evilMinerAddr, types.EmptyTSK)
	require.NoError(t, err)
	require.True(t, p.MinerPower.RawBytePower.IsZero())

	// Now we begin the redemption arc.
	require.True(t, p.MinerPower.RawBytePower.IsZero())

	// First, recover the sector.

	{
		minerInfo, err := client.StateMinerInfo(ctx, evilMinerAddr, types.EmptyTSK)
		require.NoError(t, err)

		params := &minerActor.DeclareFaultsRecoveredParams{
			Recoveries: []minerActor.RecoveryDeclaration{{
				Deadline:  evilSectorLoc.Deadline,
				Partition: evilSectorLoc.Partition,
				Sectors:   bitfield.NewFromSet([]uint64{uint64(evilSectorNo)}),
			}},
		}

		enc, aerr := actors.SerializeParams(params)
		require.NoError(t, aerr)

		msg := &types.Message{
			To:     evilMinerAddr,
			Method: minerActor.Methods.DeclareFaultsRecovered,
			Params: enc,
			Value:  types.FromFil(30), // repay debt.
			From:   minerInfo.Owner,
		}
		sm, err := client.MpoolPushMessage(ctx, msg, nil)
		require.NoError(t, err)

		rec, err := client.StateWaitMsg(ctx, sm.Cid(), build.MessageConfidence)
		require.NoError(t, err)
		require.Zero(t, rec.Receipt.ExitCode, "recovery not accepted: %s", rec.Receipt.ExitCode.Error())
	}

	// Then wait for the deadline.
	for {
		di, err = client.StateMinerProvingDeadline(ctx, evilMinerAddr, types.EmptyTSK)
		require.NoError(t, err)
		if di.Index == evilSectorLoc.Deadline {
			break
		}
		build.Clock.Sleep(blocktime)
	}

	// Now try to be evil again
	err = submitBadProof(ctx, client, evilMinerAddr, di, evilSectorLoc.Deadline, evilSectorLoc.Partition)
	require.Error(t, err)
	require.Contains(t, err.Error(), "message execution failed: exit 16, reason: window post failed: invalid PoSt")

	// It didn't work because we're recovering.
}

func submitBadProof(
	ctx context.Context,
	client api.FullNode, maddr address.Address,
	di *dline.Info, dlIdx, partIdx uint64,
) error {
	head, err := client.ChainHead(ctx)
	if err != nil {
		return err
	}

	from, err := client.WalletDefaultAddress(ctx)
	if err != nil {
		return err
	}

	minerInfo, err := client.StateMinerInfo(ctx, maddr, head.Key())
	if err != nil {
		return err
	}

	commEpoch := di.Open
	commRand, err := client.ChainGetRandomnessFromTickets(
		ctx, head.Key(), crypto.DomainSeparationTag_PoStChainCommit,
		commEpoch, nil,
	)
	if err != nil {
		return err
	}
	params := &minerActor.SubmitWindowedPoStParams{
		ChainCommitEpoch: commEpoch,
		ChainCommitRand:  commRand,
		Deadline:         dlIdx,
		Partitions:       []minerActor.PoStPartition{{Index: partIdx}},
		Proofs: []proof3.PoStProof{{
			PoStProof:  minerInfo.WindowPoStProofType,
			ProofBytes: []byte("I'm soooo very evil."),
		}},
	}

	enc, aerr := actors.SerializeParams(params)
	if aerr != nil {
		return aerr
	}

	msg := &types.Message{
		To:     maddr,
		Method: minerActor.Methods.SubmitWindowedPoSt,
		Params: enc,
		Value:  types.NewInt(0),
		From:   from,
	}
	sm, err := client.MpoolPushMessage(ctx, msg, nil)
	if err != nil {
		return err
	}

	rec, err := client.StateWaitMsg(ctx, sm.Cid(), build.MessageConfidence)
	if err != nil {
		return err
	}
	if rec.Receipt.ExitCode.IsError() {
		return rec.Receipt.ExitCode
	}
	return nil
}

func TestWindowPostDisputeFails(t *testing.T, b APIBuilder, blocktime time.Duration) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	n, sn := b(t, []FullNodeOpts{FullNodeWithActorsV3At(2)}, OneMiner)

	client := n[0].FullNode.(*impl.FullNodeAPI)
	miner := sn[0]

	{
		addrinfo, err := client.NetAddrsListen(ctx)
		if err != nil {
			t.Fatal(err)
		}

		if err := miner.NetConnect(ctx, addrinfo); err != nil {
			t.Fatal(err)
		}
	}

	defaultFrom, err := client.WalletDefaultAddress(ctx)
	require.NoError(t, err)

	maddr, err := miner.ActorAddress(ctx)
	require.NoError(t, err)

	build.Clock.Sleep(time.Second)

	// Mine with the _second_ node (the good one).
	done := make(chan struct{})
	go func() {
		defer close(done)
		for ctx.Err() == nil {
			build.Clock.Sleep(blocktime)
			if err := miner.MineOne(ctx, MineNext); err != nil {
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

	pledgeSectors(t, ctx, miner, 10, 0, nil)

	di, err := client.StateMinerProvingDeadline(ctx, maddr, types.EmptyTSK)
	require.NoError(t, err)

	fmt.Printf("Running one proving period\n")
	fmt.Printf("End for head.Height > %d\n", di.PeriodStart+di.WPoStProvingPeriod*2)

	for {
		head, err := client.ChainHead(ctx)
		require.NoError(t, err)

		if head.Height() > di.PeriodStart+di.WPoStProvingPeriod*2 {
			fmt.Printf("Now head.Height = %d\n", head.Height())
			break
		}
		build.Clock.Sleep(blocktime)
	}

	ssz, err := miner.ActorSectorSize(ctx, maddr)
	require.NoError(t, err)
	expectedPower := types.NewInt(uint64(ssz) * (GenesisPreseals + 10))

	p, err := client.StateMinerPower(ctx, maddr, types.EmptyTSK)
	require.NoError(t, err)

	// make sure it has gained power.
	require.Equal(t, p.MinerPower.RawBytePower, expectedPower)

	// Wait until a proof has been submitted.
	var targetDeadline uint64
waitForProof:
	for {
		deadlines, err := client.StateMinerDeadlines(ctx, maddr, types.EmptyTSK)
		require.NoError(t, err)
		for dlIdx, dl := range deadlines {
			nonEmpty, err := dl.PostSubmissions.IsEmpty()
			require.NoError(t, err)
			if nonEmpty {
				targetDeadline = uint64(dlIdx)
				break waitForProof
			}
		}

		build.Clock.Sleep(blocktime)
	}

	for {
		di, err := client.StateMinerProvingDeadline(ctx, maddr, types.EmptyTSK)
		require.NoError(t, err)
		// wait until the deadline finishes.
		if di.Index == ((targetDeadline + 1) % di.WPoStPeriodDeadlines) {
			break
		}

		build.Clock.Sleep(blocktime)
	}

	// Try to object to the proof. This should fail.
	{
		params := &minerActor.DisputeWindowedPoStParams{
			Deadline:  targetDeadline,
			PoStIndex: 0,
		}

		enc, aerr := actors.SerializeParams(params)
		require.NoError(t, aerr)

		msg := &types.Message{
			To:     maddr,
			Method: minerActor.Methods.DisputeWindowedPoSt,
			Params: enc,
			Value:  types.NewInt(0),
			From:   defaultFrom,
		}
		_, err := client.MpoolPushMessage(ctx, msg, nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to dispute valid post (RetCode=16)")
	}
}
