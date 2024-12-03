package stages

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	minertypes "github.com/filecoin-project/go-state-types/builtin/v9/miner"

	"github.com/filecoin-project/lotus/chain/actors/policy"
)

func TestCommitQueue(t *testing.T) {
	var q commitQueue
	addr1, err := address.NewIDAddress(1000)
	require.NoError(t, err)
	proofType := abi.RegisteredSealProof_StackedDrg64GiBV1_1
	require.NoError(t, q.enqueueProveCommit(addr1, 0, minertypes.SectorPreCommitInfo{
		SealProof:    proofType,
		SectorNumber: 0,
	}))
	require.NoError(t, q.enqueueProveCommit(addr1, 0, minertypes.SectorPreCommitInfo{
		SealProof:    proofType,
		SectorNumber: 1,
	}))
	require.NoError(t, q.enqueueProveCommit(addr1, 1, minertypes.SectorPreCommitInfo{
		SealProof:    proofType,
		SectorNumber: 2,
	}))
	require.NoError(t, q.enqueueProveCommit(addr1, 1, minertypes.SectorPreCommitInfo{
		SealProof:    proofType,
		SectorNumber: 3,
	}))
	require.NoError(t, q.enqueueProveCommit(addr1, 3, minertypes.SectorPreCommitInfo{
		SealProof:    proofType,
		SectorNumber: 4,
	}))
	require.NoError(t, q.enqueueProveCommit(addr1, 4, minertypes.SectorPreCommitInfo{
		SealProof:    proofType,
		SectorNumber: 5,
	}))
	require.NoError(t, q.enqueueProveCommit(addr1, 6, minertypes.SectorPreCommitInfo{
		SealProof:    proofType,
		SectorNumber: 6,
	}))

	epoch := abi.ChainEpoch(0)
	q.advanceEpoch(epoch)
	_, _, ok := q.nextMiner()
	require.False(t, ok)

	epoch += policy.GetPreCommitChallengeDelay()
	q.advanceEpoch(epoch)
	_, _, ok = q.nextMiner()
	require.False(t, ok)

	// 0 : empty + non-empty
	epoch++
	q.advanceEpoch(epoch)
	addr, sectors, ok := q.nextMiner()
	require.True(t, ok)
	require.Equal(t, sectors.count(), 2)
	require.Equal(t, addr, addr1)
	sectors.finish(proofType, 1)
	require.Equal(t, sectors.count(), 1)
	require.EqualValues(t, []abi.SectorNumber{1}, sectors[proofType])

	// 1 : non-empty + non-empty
	epoch++
	q.advanceEpoch(epoch)
	addr, sectors, ok = q.nextMiner()
	require.True(t, ok)
	require.Equal(t, addr, addr1)
	require.Equal(t, sectors.count(), 3)
	require.EqualValues(t, []abi.SectorNumber{1, 2, 3}, sectors[proofType])
	sectors.finish(proofType, 3)
	require.Equal(t, sectors.count(), 0)

	// 2 : empty + empty
	epoch++
	q.advanceEpoch(epoch)
	_, _, ok = q.nextMiner()
	require.False(t, ok)

	// 3 : empty + non-empty
	epoch++
	q.advanceEpoch(epoch)
	_, sectors, ok = q.nextMiner()
	require.True(t, ok)
	require.Equal(t, sectors.count(), 1)
	require.EqualValues(t, []abi.SectorNumber{4}, sectors[proofType])

	// 4 : non-empty + non-empty
	epoch++
	q.advanceEpoch(epoch)
	_, sectors, ok = q.nextMiner()
	require.True(t, ok)
	require.Equal(t, sectors.count(), 2)
	require.EqualValues(t, []abi.SectorNumber{4, 5}, sectors[proofType])

	// 5 : empty + non-empty
	epoch++
	q.advanceEpoch(epoch)
	_, sectors, ok = q.nextMiner()
	require.True(t, ok)
	require.Equal(t, sectors.count(), 2)
	require.EqualValues(t, []abi.SectorNumber{4, 5}, sectors[proofType])
	sectors.finish(proofType, 1)
	require.EqualValues(t, []abi.SectorNumber{5}, sectors[proofType])

	// 6
	epoch++
	q.advanceEpoch(epoch)
	_, sectors, ok = q.nextMiner()
	require.True(t, ok)
	require.Equal(t, sectors.count(), 2)
	require.EqualValues(t, []abi.SectorNumber{5, 6}, sectors[proofType])

	// 8
	epoch += 2
	q.advanceEpoch(epoch)
	_, sectors, ok = q.nextMiner()
	require.True(t, ok)
	require.Equal(t, sectors.count(), 2)
	require.EqualValues(t, []abi.SectorNumber{5, 6}, sectors[proofType])
}
