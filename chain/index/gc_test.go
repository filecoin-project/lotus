package index

import (
	"context"
	pseudo "math/rand"
	"testing"
	"time"

	"github.com/ipfs/go-cid"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-state-types/abi"
)

const (
	epochOne   = 1
	epochTen   = 10
	epochFifty = 50
	headEpoch  = 60

	validRetentionEpochs = 20
	highRetentionEpochs  = 100
	lowRetentionEpochs   = 1
)

func TestGC(t *testing.T) {
	type tipsetData struct {
		height   abi.ChainEpoch
		reverted bool
	}

	tests := []struct {
		name                          string
		headHeight                    abi.ChainEpoch
		gcRetentionEpochs             int64
		tipsets                       []tipsetData
		expectedEpochTipsetDataCounts map[abi.ChainEpoch]int // expected data count(tipsetMsg, event, eventEntry), for each epoch
		expectedEthTxHashCount        int                    // expected eth tx hash count after gc
	}{
		{
			name:              "Basic GC with some tipsets removed",
			headHeight:        headEpoch,
			gcRetentionEpochs: validRetentionEpochs,
			tipsets: []tipsetData{
				{height: epochOne, reverted: false},
				{height: epochTen, reverted: false},
				{height: epochFifty, reverted: false},
			},
			expectedEpochTipsetDataCounts: map[abi.ChainEpoch]int{
				epochOne:   0, // Should be removed
				epochTen:   0, // Should be removed
				epochFifty: 1, // Should remain
			},
			expectedEthTxHashCount: 1, // Only the entry for height 50 should remain
		},
		{
			name:              "No GC when retention epochs is high",
			headHeight:        headEpoch,
			gcRetentionEpochs: highRetentionEpochs,
			tipsets: []tipsetData{
				{height: epochOne, reverted: false},
				{height: epochTen, reverted: false},
				{height: epochFifty, reverted: false},
			},
			expectedEpochTipsetDataCounts: map[abi.ChainEpoch]int{
				epochOne:   1, // Should remain
				epochTen:   1, // Should remain
				epochFifty: 1, // Should remain
			},
			expectedEthTxHashCount: 3, // All entries should remain
		},
		{
			name:              "No GC when gcRetentionEpochs is zero",
			headHeight:        headEpoch,
			gcRetentionEpochs: 0,
			tipsets: []tipsetData{
				{height: epochOne, reverted: false},
				{height: epochTen, reverted: false},
				{height: epochFifty, reverted: false},
			},
			expectedEpochTipsetDataCounts: map[abi.ChainEpoch]int{
				epochOne:   1, // Should remain
				epochTen:   1, // Should remain
				epochFifty: 1, // Should remain
			},
			expectedEthTxHashCount: 3, // All entries should remain
		},
		{
			name:              "GC should remove tipsets that are older than gcRetentionEpochs + gracEpochs",
			headHeight:        headEpoch,
			gcRetentionEpochs: lowRetentionEpochs, // headHeight - gcRetentionEpochs - graceEpochs = 60 - 5 - 10 = 45 (removalEpoch)
			tipsets: []tipsetData{
				{height: 1, reverted: false},
				{height: 10, reverted: false},
				{height: 50, reverted: false},
			},
			expectedEpochTipsetDataCounts: map[abi.ChainEpoch]int{
				epochOne:   0, // Should be removed
				epochTen:   0, // Should be removed
				epochFifty: 1, // Should remain
			},
			expectedEthTxHashCount: 1, // Only the entry for height 50 should remain
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			genesisTime := time.Now()
			rng := pseudo.New(pseudo.NewSource(genesisTime.UnixNano()))

			si, _, _ := setupWithHeadIndexed(t, tt.headHeight, rng)
			si.gcRetentionEpochs = tt.gcRetentionEpochs
			defer func() { _ = si.Close() }()

			tipsetKeyCids := make(map[abi.ChainEpoch]cid.Cid)

			for _, tsData := range tt.tipsets {
				t.Logf("inserting tipset at height %d", tsData.height)

				tsKeyCid, _, _ := insertRandomTipsetAtHeight(t, si, uint64(tsData.height), tsData.reverted, genesisTime)
				tipsetKeyCids[tsData.height] = tsKeyCid
			}

			si.gc(ctx)

			for height, expectedCount := range tt.expectedEpochTipsetDataCounts {
				var count int

				err := si.db.QueryRow("SELECT COUNT(*) FROM tipset_message WHERE height = ?", height).Scan(&count)
				require.NoError(t, err)
				require.Equal(t, expectedCount, count, "Unexpected tipset_message count for height %d", height)

				tsKeyCid := tipsetKeyCids[height]
				err = si.stmts.getNonRevertedTipsetEventCountStmt.QueryRow(tsKeyCid.Bytes()).Scan(&count)
				require.NoError(t, err)
				require.Equal(t, expectedCount, count, "Unexpected events count for height %d", height)

				err = si.stmts.getNonRevertedTipsetEventEntriesCountStmt.QueryRow(tsKeyCid.Bytes()).Scan(&count)
				require.NoError(t, err)
				require.Equal(t, expectedCount, count, "Unexpected event_entries count for height %d", height)
			}

			var ethTxHashCount int
			err := si.db.QueryRow("SELECT COUNT(*) FROM eth_tx_hash").Scan(&ethTxHashCount)
			require.NoError(t, err)
			require.Equal(t, tt.expectedEthTxHashCount, ethTxHashCount, "Unexpected eth_tx_hash count")

			t.Cleanup(func() {
				cleanup(t, si)
			})
		})
	}
}
