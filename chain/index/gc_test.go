package index

import (
	"context"
	pseudo "math/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/chain/types"
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
	headHeight := abi.ChainEpoch(60)
	si, _, cs := setupWithHeadIndexed(t, headHeight, rng)
	defer func() { _ = si.Close() }()

	si.gcRetentionEpochs = 20

	ev1 := fakeEvent(
		abi.ActorID(1),
		[]kv{
			{k: "type", v: []byte("approval")},
			{k: "signer", v: []byte("addr1")},
		},
		[]kv{
			{k: "amount", v: []byte("2988181")},
		},
	)

	ev2 := fakeEvent(
		abi.ActorID(2),
		[]kv{
			{k: "type", v: []byte("approval")},
			{k: "signer", v: []byte("addr2")},
		},
		[]kv{
			{k: "amount", v: []byte("2988181")},
		},
	)

	events := []types.Event{*ev1, *ev2}

	fm := fakeMessage(address.TestAddress, address.TestAddress)
	em1 := executedMessage{
		msg: fm,
		evs: events,
	}

	si.SetIdToRobustAddrFunc(func(ctx context.Context, emitter abi.ActorID, ts *types.TipSet) (address.Address, bool) {
		idAddr, err := address.NewIDAddress(uint64(emitter))
		if err != nil {
			return address.Undef, false
		}

		return idAddr, true
	})

	si.SetEventLoaderFunc(func(ctx context.Context, cs ChainStore, msgTs, rctTs *types.TipSet) ([]executedMessage, error) {
		if msgTs.Height() == 1 {
			return []executedMessage{em1}, nil
		}
		return nil, nil
	})

	// Create a fake tipset at height 1
	fakeTipSet1 := fakeTipSet(t, rng, 1, nil)
	fakeTipSet2 := fakeTipSet(t, rng, 10, nil)
	fakeTipSet3 := fakeTipSet(t, rng, 50, nil)

	// Set the dummy chainstore to return this tipset for height 1
	cs.SetTipsetByHeightAndKey(1, fakeTipSet1.Key(), fakeTipSet1)  // empty DB
	cs.SetTipsetByHeightAndKey(10, fakeTipSet2.Key(), fakeTipSet2) // empty DB
	cs.SetTipsetByHeightAndKey(50, fakeTipSet3.Key(), fakeTipSet3) // empty DB

	cs.SetMessagesForTipset(fakeTipSet1, []types.ChainMsg{fm})

	// index tipset and events
	require.NoError(t, si.Apply(ctx, fakeTipSet1, fakeTipSet2))
	require.NoError(t, si.Apply(ctx, fakeTipSet2, fakeTipSet3))

	// getLogs works for height 1
	filter := &EventFilter{
		MinHeight: 1,
		MaxHeight: 1,
	}
	ces, err := si.GetEventsForFilter(ctx, filter, false)
	require.NoError(t, err)
	require.Len(t, ces, 2)

	si.gc(ctx)

	// getLogs does not work for height 1
	_, err = si.GetEventsForFilter(ctx, filter, false)
	require.Error(t, err)

	// Verify that the tipset at height 1 is removed
	var count int
	err = si.db.QueryRow("SELECT COUNT(*) FROM tipset_message WHERE height = 1").Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 0, count)

	// Verify that the tipset at height 10 is not removed
	err = si.db.QueryRow("SELECT COUNT(*) FROM tipset_message WHERE height = 10").Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 0, count)

	// Verify that the tipset at height 50 is not removed
	err = si.db.QueryRow("SELECT COUNT(*) FROM tipset_message WHERE height = 50").Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 1, count)
}
