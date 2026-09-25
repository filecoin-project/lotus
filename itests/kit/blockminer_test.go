package kit

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-bitfield"
	"github.com/filecoin-project/go-state-types/builtin"
	minertypes "github.com/filecoin-project/go-state-types/builtin/v8/miner"

	"github.com/filecoin-project/lotus/chain/types"
)

func TestPartitionTrackerCreditsOnlyItsDeadline(t *testing.T) {
	maddr, err := address.NewIDAddress(1000)
	require.NoError(t, err)
	const dlIdx = 7

	postFor := func(deadline uint64) *types.Message {
		params := minertypes.SubmitWindowedPoStParams{
			Deadline:   deadline,
			Partitions: []minertypes.PoStPartition{{Index: 0, Skipped: bitfield.New()}},
		}
		var buf bytes.Buffer
		require.NoError(t, params.MarshalCBOR(&buf))
		return &types.Message{To: maddr, Method: builtin.MethodsMiner.SubmitWindowedPoSt, Params: buf.Bytes()}
	}

	tracker := &partitionTracker{minerAddr: maddr, dlIdx: dlIdx, mustProve: []uint64{0}, posted: bitfield.New()}

	done, err := tracker.recordIfPost(postFor(dlIdx + 1))
	require.NoError(t, err)
	require.False(t, done, "a post for the next deadline must not prove this one")

	done, err = tracker.recordIfPost(postFor(dlIdx))
	require.NoError(t, err)
	require.True(t, done, "a post for this deadline proves its partition")
}
