package itests

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/filecoin-project/go-bitfield"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/chain/types"
	sealing "github.com/filecoin-project/lotus/extern/storage-sealing"
	"github.com/filecoin-project/lotus/itests/kit2"
	"github.com/stretchr/testify/require"
)

func TestTerminate(t *testing.T) {
	if os.Getenv("LOTUS_TEST_WINDOW_POST") != "1" {
		t.Skip("this takes a few minutes, set LOTUS_TEST_WINDOW_POST=1 to run")
	}

	kit2.QuietMiningLogs()

	const blocktime = 2 * time.Millisecond

	nSectors := uint64(2)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	opts := kit2.ConstructorOpts(kit2.LatestActorsAt(-1))
	client, miner, ens := kit2.EnsembleMinimal(t, kit2.MockProofs(), opts)
	ens.InterconnectAll().BeginMining(blocktime)

	maddr, err := miner.ActorAddress(ctx)
	require.NoError(t, err)

	ssz, err := miner.ActorSectorSize(ctx, maddr)
	require.NoError(t, err)

	p, err := client.StateMinerPower(ctx, maddr, types.EmptyTSK)
	require.NoError(t, err)
	require.Equal(t, p.MinerPower, p.TotalPower)
	require.Equal(t, p.MinerPower.RawBytePower, types.NewInt(uint64(ssz)*nSectors))

	t.Log("Seal a sector")

	miner.PledgeSectors(ctx, 1, 0, nil)

	t.Log("wait for power")

	{
		// Wait until proven.
		di, err := client.StateMinerProvingDeadline(ctx, maddr, types.EmptyTSK)
		require.NoError(t, err)

		waitUntil := di.PeriodStart + di.WPoStProvingPeriod + 2
		t.Logf("End for head.Height > %d", waitUntil)

		height := kit2.WaitTillChainHeight(ctx, t, client, blocktime, int(waitUntil))
		t.Logf("Now head.Height = %d", height)
	}

	nSectors++

	p, err = client.StateMinerPower(ctx, maddr, types.EmptyTSK)
	require.NoError(t, err)
	require.Equal(t, p.MinerPower, p.TotalPower)
	require.Equal(t, p.MinerPower.RawBytePower, types.NewInt(uint64(ssz)*nSectors))

	t.Log("Terminate a sector")

	toTerminate := abi.SectorNumber(3)

	err = miner.SectorTerminate(ctx, toTerminate)
	require.NoError(t, err)

	msgTriggerred := false
loop:
	for {
		si, err := miner.SectorsStatus(ctx, toTerminate, false)
		require.NoError(t, err)

		t.Log("state: ", si.State, msgTriggerred)

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
					t.Log("terminate message:", c)

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

	waitUntil := di.PeriodStart + di.WPoStProvingPeriod + 2
	t.Logf("End for head.Height > %d", waitUntil)
	height := kit2.WaitTillChainHeight(ctx, t, client, blocktime, int(waitUntil))
	t.Logf("Now head.Height = %d", height)

	p, err = client.StateMinerPower(ctx, maddr, types.EmptyTSK)
	require.NoError(t, err)

	require.Equal(t, p.MinerPower, p.TotalPower)
	require.Equal(t, p.MinerPower.RawBytePower, types.NewInt(uint64(ssz)*(nSectors-1)))
}
