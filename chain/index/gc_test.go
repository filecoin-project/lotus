package index

import (
	"context"
	pseudo "math/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-state-types/abi"
)

func TestCleanupRevertedTipsets(t *testing.T) {
	ctx := context.Background()
	rng := pseudo.New(pseudo.NewSource(time.Now().UnixNano()))

	tests := []struct {
		name           string
		headHeight     int64
		revertedHeight uint64
		expectRemoved  bool
	}{
		{
			name:           "reverted tipset within finality, should not be removed",
			headHeight:     1000,
			revertedHeight: 980,
			expectRemoved:  false,
		},
		{
			name:           "reverted tipset outside finality, should be removed",
			headHeight:     5000,
			revertedHeight: 400,
			expectRemoved:  true,
		},
		{
			name:           "not enough tipsets",
			headHeight:     2000,
			revertedHeight: 1,
			expectRemoved:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			si, _, _ := setupWithHeadIndexed(t, abi.ChainEpoch(tt.headHeight), rng)
			si.gcRetentionEpochs = 0
			defer func() { _ = si.Close() }()

			revertedTsCid := randomCid(t, rng)
			insertTipsetMessage(t, si, tipsetMessage{
				tipsetKeyCid: revertedTsCid.Bytes(),
				height:       tt.revertedHeight,
				reverted:     true,
				messageCid:   randomCid(t, rng).Bytes(),
				messageIndex: 0,
			})

			si.cleanUpRevertedTipsets(ctx)

			var count int
			err := si.db.QueryRow("SELECT COUNT(*) FROM tipset_message WHERE height = ?", tt.revertedHeight).Scan(&count)
			require.NoError(t, err)

			if tt.expectRemoved {
				require.Equal(t, 0, count, "expected reverted tipset to be removed")
			} else {
				require.Equal(t, 1, count, "expected reverted tipset to not be removed")
			}
		})
	}
}

func TestGC(t *testing.T) {
	ctx := context.Background()
	rng := pseudo.New(pseudo.NewSource(time.Now().UnixNano()))

	// head at height 60
	// insert tipsets at heigh 1,10,50.
	// retention epochs is 20
	si, _, _ := setupWithHeadIndexed(t, 60, rng)
	si.gcRetentionEpochs = 20
	defer func() { _ = si.Close() }()

	tsCid1 := randomCid(t, rng)
	tsCid10 := randomCid(t, rng)
	tsCid50 := randomCid(t, rng)

	insertTipsetMessage(t, si, tipsetMessage{
		tipsetKeyCid: tsCid1.Bytes(),
		height:       1,
		reverted:     false,
		messageCid:   randomCid(t, rng).Bytes(),
		messageIndex: 0,
	})

	insertTipsetMessage(t, si, tipsetMessage{
		tipsetKeyCid: tsCid10.Bytes(),
		height:       10,
		reverted:     false,
		messageCid:   randomCid(t, rng).Bytes(),
		messageIndex: 0,
	})

	insertTipsetMessage(t, si, tipsetMessage{
		tipsetKeyCid: tsCid50.Bytes(),
		height:       50,
		reverted:     false,
		messageCid:   randomCid(t, rng).Bytes(),
		messageIndex: 0,
	})

	si.gc(ctx)

	// tipset at height 1 and 10 should be removed
	var count int
	err := si.db.QueryRow("SELECT COUNT(*) FROM tipset_message WHERE height = 1").Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 0, count)

	err = si.db.QueryRow("SELECT COUNT(*) FROM tipset_message WHERE height = 10").Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 0, count)

	// tipset at height 50 should not be removed
	err = si.db.QueryRow("SELECT COUNT(*) FROM tipset_message WHERE height = 50").Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 1, count)
}
