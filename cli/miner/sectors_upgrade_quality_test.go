package miner

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"

	stminer "github.com/filecoin-project/go-state-types/builtin/v19/miner"

	"github.com/filecoin-project/lotus/chain/actors"
)

func sectorsInMessage(t *testing.T, m stminer.UpgradeSectorQualityParams) int {
	t.Helper()
	total := 0
	for _, u := range m.Upgrades {
		c, err := u.Sectors.Count()
		require.NoError(t, err)
		total += int(c)
	}
	return total
}

// TestPackUpgradeQualityMessages verifies (deadline, partition) grouping and cross-message splitting.
func TestPackUpgradeQualityMessages(t *testing.T) {
	// First group exceeds the cap and must be split; second group fills the remainder.
	toUpgrade := []sectorLoc{
		{deadline: 1, partition: 1, sectorNum: 10},
		{deadline: 1, partition: 1, sectorNum: 11},
		{deadline: 1, partition: 1, sectorNum: 12},
		{deadline: 2, partition: 3, sectorNum: 20},
	}

	messages := packUpgradeQualityMessages(toUpgrade, 2)
	require.Len(t, messages, 2)

	// Message 0: first two sectors of (1,1).
	require.Len(t, messages[0].Upgrades, 1)
	require.Equal(t, uint64(1), messages[0].Upgrades[0].Deadline)
	require.Equal(t, uint64(1), messages[0].Upgrades[0].Partition)
	got0, err := messages[0].Upgrades[0].Sectors.All(1 << 20)
	require.NoError(t, err)
	require.Equal(t, []uint64{10, 11}, got0)

	// Message 1: tail of (1,1) plus whole (2,3) group.
	require.Len(t, messages[1].Upgrades, 2)
	got1a, err := messages[1].Upgrades[0].Sectors.All(1 << 20)
	require.NoError(t, err)
	require.Equal(t, []uint64{12}, got1a)
	require.Equal(t, uint64(2), messages[1].Upgrades[1].Deadline)
	require.Equal(t, uint64(3), messages[1].Upgrades[1].Partition)
	got1b, err := messages[1].Upgrades[1].Sectors.All(1 << 20)
	require.NoError(t, err)
	require.Equal(t, []uint64{20}, got1b)

	// No message may exceed the cap, and every input sector is represented exactly once.
	total := 0
	for i, m := range messages {
		n := sectorsInMessage(t, m)
		require.LessOrEqual(t, n, 2, "message %d exceeds the per-message cap", i)
		total += n
	}
	require.Equal(t, len(toUpgrade), total)
}

// TestPackUpgradeQualityMessages_SingleMessage covers the common case where everything fits in one
// message, and the empty input case.
func TestPackUpgradeQualityMessages_SingleMessage(t *testing.T) {
	toUpgrade := []sectorLoc{
		{deadline: 0, partition: 0, sectorNum: 1},
		{deadline: 0, partition: 0, sectorNum: 2},
		{deadline: 5, partition: 2, sectorNum: 3},
	}

	messages := packUpgradeQualityMessages(toUpgrade, 100)
	require.Len(t, messages, 1)
	require.Len(t, messages[0].Upgrades, 2)
	require.Equal(t, 3, sectorsInMessage(t, messages[0]))

	require.Empty(t, packUpgradeQualityMessages(nil, 100))
}

// TestUpgradeQualityParamsSerialize confirms CBOR round-trip of the generated message params.
func TestUpgradeQualityParamsSerialize(t *testing.T) {
	toUpgrade := []sectorLoc{
		{deadline: 1, partition: 1, sectorNum: 10},
		{deadline: 1, partition: 1, sectorNum: 11},
		{deadline: 1, partition: 1, sectorNum: 12},
		{deadline: 2, partition: 3, sectorNum: 20},
	}

	messages := packUpgradeQualityMessages(toUpgrade, 2)
	require.NotEmpty(t, messages)

	for i := range messages {
		sp, err := actors.SerializeParams(&messages[i])
		require.NoError(t, err, "message %d params must CBOR-serialize", i)
		require.NotEmpty(t, sp)

		var decoded stminer.UpgradeSectorQualityParams
		require.NoError(t, decoded.UnmarshalCBOR(bytes.NewReader(sp)))
		require.Len(t, decoded.Upgrades, len(messages[i].Upgrades))

		for j := range messages[i].Upgrades {
			require.Equal(t, messages[i].Upgrades[j].Deadline, decoded.Upgrades[j].Deadline)
			require.Equal(t, messages[i].Upgrades[j].Partition, decoded.Upgrades[j].Partition)

			want, err := messages[i].Upgrades[j].Sectors.All(1 << 20)
			require.NoError(t, err)
			got, err := decoded.Upgrades[j].Sectors.All(1 << 20)
			require.NoError(t, err)
			require.Equal(t, want, got)
		}
	}
}
