package itests

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/filecoin-project/go-bitfield"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/types"
	sealing "github.com/filecoin-project/lotus/extern/storage-sealing"
	"github.com/filecoin-project/lotus/itests/kit"
	"github.com/filecoin-project/lotus/node/impl"
	"github.com/stretchr/testify/require"
)

func TestTerminate(t *testing.T) {
	kit.Expensive(t)

	kit.QuietMiningLogs()

	const blocktime = 2 * time.Millisecond

	nSectors := uint64(2)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	n, sn := kit.MockMinerBuilder(t,
		[]kit.FullNodeOpts{kit.FullNodeWithLatestActorsAt(-1)},
		[]kit.StorageMiner{{Full: 0, Preseal: int(nSectors)}},
	)

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
			if err := sn[0].MineOne(ctx, kit.MineNext); err != nil {
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

	kit.PledgeSectors(t, ctx, miner, 1, 0, nil)

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
