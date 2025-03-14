package itests

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/ipld/go-ipld-prime"
	"github.com/ipld/go-ipld-prime/codec/dagcbor"
	"github.com/ipld/go-ipld-prime/node/basicnode"
	"github.com/multiformats/go-multicodec"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-bitfield"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/itests/kit"
	"github.com/filecoin-project/lotus/lib/must"
	sealing "github.com/filecoin-project/lotus/storage/pipeline"
)

func TestTerminate(t *testing.T) {

	kit.Expensive(t)

	kit.QuietMiningLogs()

	var (
		blocktime = 2 * time.Millisecond
		nSectors  = 2
		ctx       = context.Background()
	)

	client, miner, ens := kit.EnsembleMinimal(t, kit.MockProofs(), kit.PresealSectors(nSectors))
	ens.InterconnectAll().BeginMining(blocktime)

	maddr, err := miner.ActorAddress(ctx)
	require.NoError(t, err)

	ssz, err := miner.ActorSectorSize(ctx, maddr)
	require.NoError(t, err)

	p, err := client.StateMinerPower(ctx, maddr, types.EmptyTSK)
	require.NoError(t, err)
	require.Equal(t, p.MinerPower, p.TotalPower)
	require.Equal(t, p.MinerPower.RawBytePower, types.NewInt(uint64(ssz)*uint64(nSectors)))

	t.Log("Seal a sector")

	miner.PledgeSectors(ctx, 1, 0, nil)

	t.Log("wait for power")

	{
		// Wait until proven.
		di, err := client.StateMinerProvingDeadline(ctx, maddr, types.EmptyTSK)
		require.NoError(t, err)

		waitUntil := di.Open + di.WPoStProvingPeriod
		t.Logf("End for head.Height > %d", waitUntil)

		ts := client.WaitTillChain(ctx, kit.HeightAtLeast(waitUntil))
		t.Logf("Now head.Height = %d", ts.Height())
	}

	nSectors++

	p, err = client.StateMinerPower(ctx, maddr, types.EmptyTSK)
	require.NoError(t, err)
	require.Equal(t, p.MinerPower, p.TotalPower)
	require.Equal(t, types.NewInt(uint64(ssz)*uint64(nSectors)), p.MinerPower.RawBytePower)

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

	// need to wait for message to be mined and applied.
	time.Sleep(5 * time.Second)

	// check power decreased
	p, err = client.StateMinerPower(ctx, maddr, types.EmptyTSK)
	require.NoError(t, err)
	require.Equal(t, p.MinerPower, p.TotalPower)
	require.Equal(t, types.NewInt(uint64(ssz)*uint64(nSectors-1)), p.MinerPower.RawBytePower)

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

	waitUntil := di.PeriodStart + di.WPoStProvingPeriod + 20 // slack like above
	t.Logf("End for head.Height > %d", waitUntil)
	ts := client.WaitTillChain(ctx, kit.HeightAtLeast(waitUntil))
	t.Logf("Now head.Height = %d", ts.Height())

	p, err = client.StateMinerPower(ctx, maddr, types.EmptyTSK)
	require.NoError(t, err)

	require.Equal(t, p.MinerPower, p.TotalPower)
	require.Equal(t, types.NewInt(uint64(ssz)*uint64(nSectors-1)), p.MinerPower.RawBytePower)

	// check "sector-terminated" actor event
	var epochZero abi.ChainEpoch
	allEvents, err := miner.FullNode.GetActorEventsRaw(ctx, &types.ActorEventFilter{
		FromHeight: &epochZero,
	})
	require.NoError(t, err)
	for _, key := range []string{"sector-precommitted", "sector-activated", "sector-terminated"} {
		var found bool
		keyBytes := must.One(ipld.Encode(basicnode.NewString(key), dagcbor.Encode))
		for _, event := range allEvents {
			for _, e := range event.Entries {
				if e.Key == "$type" && bytes.Equal(e.Value, keyBytes) {
					found = true
					if key == "sector-terminated" {
						expectedEntries := []types.EventEntry{
							{Flags: 0x03, Codec: uint64(multicodec.Cbor), Key: "$type", Value: keyBytes},
							{Flags: 0x03, Codec: uint64(multicodec.Cbor), Key: "sector", Value: must.One(ipld.Encode(basicnode.NewInt(int64(toTerminate)), dagcbor.Encode))},
						}
						require.Equal(t, expectedEntries, event.Entries)
					}
					break
				}
			}
		}
		require.True(t, found, "expected to find event %s", key)
	}
}
