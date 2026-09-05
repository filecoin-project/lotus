package miner

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-state-types/abi"
	stminer "github.com/filecoin-project/go-state-types/builtin/v19/miner"
	"github.com/filecoin-project/go-state-types/network"
)

func TestValidateUpgradeQualityNetworkVersion(t *testing.T) {
	t.Parallel()
	require.NoError(t, validateUpgradeQualityNetworkVersion(network.Version28))
	require.NoError(t, validateUpgradeQualityNetworkVersion(network.Version29))
	require.ErrorContains(t, validateUpgradeQualityNetworkVersion(network.Version27), "network version 28")
}

func TestLegacyUpgradeQualityCompatMessage(t *testing.T) {
	t.Parallel()
	require.ErrorContains(t, legacyUpgradeQualityCompatMessage(network.Version28), "network version 28")
	require.NoError(t, legacyUpgradeQualityCompatMessage(network.Version29))
}

func TestValidateQaPowerFilterFlags(t *testing.T) {
	t.Parallel()
	// No filters requested -> allowed, even with --fast.
	require.NoError(t, validateQaPowerFilterFlags(false, false, false))
	require.NoError(t, validateQaPowerFilterFlags(false, false, true))
	// A single filter is fine without --fast.
	require.NoError(t, validateQaPowerFilterFlags(true, false, false))
	require.NoError(t, validateQaPowerFilterFlags(false, true, false))
	// Filters are mutually exclusive.
	require.ErrorContains(t, validateQaPowerFilterFlags(true, true, false), "mutually exclusive")
	// Filters require on-chain info -> rejected with --fast.
	require.ErrorContains(t, validateQaPowerFilterFlags(true, false, true), "on-chain")
	require.ErrorContains(t, validateQaPowerFilterFlags(false, true, true), "on-chain")
}

func TestQualifyQaPowerFilter(t *testing.T) {
	t.Parallel()
	// No filter: everything shown.
	require.True(t, qualifyQaPowerFilter(true, false, false))
	require.True(t, qualifyQaPowerFilter(false, false, false))
	// --full-qa-power: only FULL_QA_POWER sectors.
	require.True(t, qualifyQaPowerFilter(true, true, false))
	require.False(t, qualifyQaPowerFilter(false, true, false))
	// --legacy-qa-power: only non-FULL_QA_POWER (legacy 1x / not-yet-on-chain) sectors.
	require.True(t, qualifyQaPowerFilter(false, false, true))
	require.False(t, qualifyQaPowerFilter(true, false, true))
}

// messageSectorCounts flattens params into the per-message sector counts for easy assertion.
func messageSectorCounts(t *testing.T, params []stminer.UpgradeSectorQualityParams) []int {
	t.Helper()
	var counts []int
	for _, p := range params {
		total := 0
		for _, up := range p.Upgrades {
			n, err := up.Sectors.Count()
			require.NoError(t, err)
			total += int(n)
		}
		counts = append(counts, total)
	}
	return counts
}

// sectorSetIn returns the sorted set of sector numbers covered by a single UpgradeSectorQuality.
func sectorSetIn(t *testing.T, up stminer.UpgradeSectorQuality) []uint64 {
	t.Helper()
	out, err := up.Sectors.All(1 << 20)
	require.NoError(t, err)
	return out
}

func TestBuildUpgradeQualityParams(t *testing.T) {
	t.Parallel()
	sectors := func(sns ...uint64) []uint64 { return sns }

	// maxSectors of 0 or negative must not divide-by-zero / loop forever.
	_, err := buildUpgradeQualityParams(map[upgradeKey][]uint64{{deadline: 0, partition: 0}: sectors(1)}, 0, nil)
	require.ErrorContains(t, err, "maxSectors")

	// An empty grouping yields no messages.
	params, err := buildUpgradeQualityParams(nil, 200, nil)
	require.NoError(t, err)
	require.Empty(t, params)

	// When everything fits under maxSectors, all (deadline, partition) upgrades are greedily packed
	// into a single message in sorted (deadline, partition) order. Each upgrade keeps its own group's
	// deadline/partition and sectors, so the result is deterministic and unit-testable.
	grouped := map[upgradeKey][]uint64{
		{deadline: 3, partition: 0}: sectors(30, 31, 32),
		{deadline: 0, partition: 7}: sectors(10, 11),
		{deadline: 1, partition: 1}: sectors(20, 21),
	}
	params, err = buildUpgradeQualityParams(grouped, 200, nil)
	require.NoError(t, err)
	require.Equal(t, 1, len(params))
	require.Equal(t, []int{7}, messageSectorCounts(t, params))
	for i, want := range []struct {
		deadline  uint64
		partition uint64
		sectors   []uint64
	}{
		{0, 7, sectors(10, 11)},
		{1, 1, sectors(20, 21)},
		{3, 0, sectors(30, 31, 32)},
	} {
		up := params[0].Upgrades[i]
		require.Equal(t, want.deadline, up.Deadline)
		require.Equal(t, want.partition, up.Partition)
		require.ElementsMatch(t, want.sectors, sectorSetIn(t, up))
	}

	// A group larger than maxSectors is split across messages, and every message stays at or under
	// the cap. Flattening all messages must recover the full input coverage with no loss/duplication,
	// and (deadline, partition) correctness is preserved on every upgrade.
	grouped = map[upgradeKey][]uint64{
		{deadline: 1, partition: 2}: sectors(1, 2, 3, 4, 5, 6, 7, 8, 9),
	}
	params, err = buildUpgradeQualityParams(grouped, 4, nil)
	require.NoError(t, err)
	reqCounts := messageSectorCounts(t, params)
	for _, n := range reqCounts {
		require.LessOrEqual(t, n, 4)
	}
	// 9 sectors at 4 per message -> ceil(9/4)=3 messages.
	require.Equal(t, 3, len(reqCounts))
	var covered []uint64
	for _, p := range params {
		for _, up := range p.Upgrades {
			require.Equal(t, uint64(1), up.Deadline, "a sub-batch must keep its group's deadline")
			require.Equal(t, uint64(2), up.Partition, "a sub-batch must keep its group's partition")
			require.Nil(t, up.NewExpiration)
			covered = append(covered, sectorSetIn(t, up)...)
		}
	}
	require.ElementsMatch(t, []uint64{1, 2, 3, 4, 5, 6, 7, 8, 9}, covered)

	// Determinism: the same grouping yields the same message structure each call despite map order.
	params2, err := buildUpgradeQualityParams(grouped, 4, nil)
	require.NoError(t, err)
	require.Equal(t, reqCounts, messageSectorCounts(t, params2))

	// newExpiration is propagated to every upgrade.
	exp := abi.ChainEpoch(1234)
	params, err = buildUpgradeQualityParams(
		map[upgradeKey][]uint64{{deadline: 2, partition: 4}: sectors(1, 2, 3, 4, 5)},
		2, &exp)
	require.NoError(t, err)
	require.Equal(t, 3, len(params))
	for _, p := range params {
		for _, up := range p.Upgrades {
			require.NotNil(t, up.NewExpiration)
			require.Equal(t, exp, *up.NewExpiration)
		}
	}
}
