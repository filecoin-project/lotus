package itests

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-bitfield"
	rlepluslazy "github.com/filecoin-project/go-bitfield/rle"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/itests/kit"
	"github.com/filecoin-project/lotus/lib/strle"
)

func TestAssignBasic(t *testing.T) {
	kit.QuietMiningLogs()

	ctx := context.Background()
	blockTime := 1 * time.Millisecond

	_, miner, ens := kit.EnsembleMinimal(t, kit.ThroughRPC(), kit.MockProofs())
	ens.InterconnectAll().BeginMiningMustPost(blockTime)

	nSectors := 2

	{
		nam, err := miner.SectorNumAssignerMeta(ctx)
		require.NoError(t, err)

		// genesis sectors start at 1, so if there are 2, we expect the Next to be 3
		require.Equal(t, abi.SectorNumber(miner.PresealSectors+1), nam.Next)
	}

	miner.PledgeSectors(ctx, nSectors, 0, nil)

	sl, err := miner.SectorsListNonGenesis(ctx)
	require.NoError(t, err)

	require.Len(t, sl, nSectors)
	require.Equal(t, abi.SectorNumber(miner.PresealSectors+1), sl[0])
	require.Equal(t, abi.SectorNumber(miner.PresealSectors+2), sl[1])
}

func rangeBitField(from, to uint64) bitfield.BitField {
	var runs []rlepluslazy.Run
	if from > 0 {
		runs = append(runs, rlepluslazy.Run{
			Val: false,
			Len: from,
		})
	}
	runs = append(runs, rlepluslazy.Run{
		Val: true,
		Len: to - from + 1,
	})

	r, err := bitfield.NewFromIter(&rlepluslazy.RunSliceIterator{Runs: runs})
	if err != nil {
		panic(err)
	}
	return r
}

func TestAssignReservation(t *testing.T) {
	kit.QuietMiningLogs()

	ctx := context.Background()
	blockTime := 1 * time.Millisecond

	_, miner, ens := kit.EnsembleMinimal(t, kit.ThroughRPC(), kit.MockProofs())
	ens.InterconnectAll().BeginMiningMustPost(blockTime)

	err := miner.SectorNumReserve(ctx, "test-reservation", rangeBitField(3, 10), false)
	require.NoError(t, err)

	// colliding name fails
	err = miner.SectorNumReserve(ctx, "test-reservation", rangeBitField(30, 33), false)
	require.Error(t, err)

	// colliding range
	err = miner.SectorNumReserve(ctx, "test-reservation2", rangeBitField(7, 12), false)
	require.Error(t, err)

	// illegal characters in the name
	err = miner.SectorNumReserve(ctx, "test/reservation", rangeBitField(99, 100), false)
	require.Error(t, err)

	nSectors := 2

	{
		nam, err := miner.SectorNumAssignerMeta(ctx)
		require.NoError(t, err)

		// reservation to 10, so we expect 11 to be first free
		require.Equal(t, abi.SectorNumber(11), nam.Next)
	}

	miner.PledgeSectors(ctx, nSectors, 0, nil)

	sl, err := miner.SectorsListNonGenesis(ctx)
	require.NoError(t, err)

	require.Len(t, sl, nSectors)
	require.Equal(t, abi.SectorNumber(11), sl[0])
	require.Equal(t, abi.SectorNumber(12), sl[1])

	// drop the reservation and see if we use the unused numbers
	err = miner.SectorNumFree(ctx, "test-reservation")
	require.NoError(t, err)

	{
		nam, err := miner.SectorNumAssignerMeta(ctx)
		require.NoError(t, err)

		// first post-genesis sector is 3
		require.Equal(t, abi.SectorNumber(3), nam.Next)
	}

	miner.PledgeSectors(ctx, 1, nSectors, nil)

	sl, err = miner.SectorsListNonGenesis(ctx)
	require.NoError(t, err)

	require.Len(t, sl, nSectors+1)
	require.Equal(t, abi.SectorNumber(3), sl[0])
	require.Equal(t, abi.SectorNumber(11), sl[1])
	require.Equal(t, abi.SectorNumber(12), sl[2])
}

func TestReserveCount(t *testing.T) {
	kit.QuietMiningLogs()

	ctx := context.Background()

	_, miner, _ := kit.EnsembleMinimal(t, kit.ThroughRPC(), kit.MockProofs())

	// with no reservations higher
	r1, err := miner.SectorNumReserveCount(ctx, "r1", 2)
	require.NoError(t, err)
	requireBitField(t, "3-4", r1)

	// reserve some higher numbers
	err = miner.SectorNumReserve(ctx, "test-reservation", rangeBitField(10, 15), false)
	require.NoError(t, err)

	// reserve a few below an existing reservation
	r2, err := miner.SectorNumReserveCount(ctx, "r2", 2)
	require.NoError(t, err)
	requireBitField(t, "5-6", r2)

	// reserve a few through an existing reservation
	r3, err := miner.SectorNumReserveCount(ctx, "r3", 6)
	require.NoError(t, err)
	requireBitField(t, "7-9,16-18", r3)

	// do one more
	r4, err := miner.SectorNumReserveCount(ctx, "r4", 4)
	require.NoError(t, err)
	requireBitField(t, "19-22", r4)

	resvs, err := miner.SectorNumReservations(ctx)
	require.NoError(t, err)

	requireBitField(t, "3-4", resvs["r1"])
	requireBitField(t, "5-6", resvs["r2"])
	requireBitField(t, "7-9,16-18", resvs["r3"])
	requireBitField(t, "19-22", resvs["r4"])
}

func requireBitField(t *testing.T, expect string, bf bitfield.BitField) {
	s, err := strle.BitfieldToHumanRanges(bf)
	require.NoError(t, err)
	require.Equal(t, expect, s)
}
