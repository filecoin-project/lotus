package index

import (
	"context"
	pseudo "math/rand"
	"testing"
	"time"

	"github.com/ipfs/go-cid"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/chain/types"
)

func TestValidateIsNullRoundSimple(t *testing.T) {
	ctx := context.Background()
	seed := time.Now().UnixNano()
	t.Logf("seed: %d", seed)
	rng := pseudo.New(pseudo.NewSource(seed))
	headHeight := abi.ChainEpoch(100)

	tests := []struct {
		name           string
		epoch          abi.ChainEpoch
		setupFunc      func(*SqliteIndexer)
		expectedResult bool
		expectError    bool
		errorContains  string
	}{
		{
			name:           "happy path - null round",
			epoch:          50,
			expectedResult: true,
		},
		{
			name:  "failure - non-null round",
			epoch: 50,
			setupFunc: func(si *SqliteIndexer) {
				insertTipsetMessage(t, si, tipsetMessage{
					tipsetKeyCid: randomCid(t, rng).Bytes(),
					height:       50,
					reverted:     false,
					messageCid:   randomCid(t, rng).Bytes(),
					messageIndex: 0,
				})
			},
			expectError:   true,
			errorContains: "index corruption",
		},
		{
			name:           "edge case - epoch 0",
			epoch:          0,
			expectedResult: true,
		},
		{
			name:           "edge case - epoch above head",
			epoch:          headHeight + 1,
			expectedResult: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			si, _, _ := setupWithHeadIndexed(t, headHeight, rng)
			t.Cleanup(func() { _ = si.Close() })

			if tt.setupFunc != nil {
				tt.setupFunc(si)
			}

			res, err := si.validateIsNullRound(ctx, tt.epoch)

			if tt.expectError {
				require.Error(t, err)
				if tt.errorContains != "" {
					require.ErrorContains(t, err, tt.errorContains)
				}
			} else {
				require.NoError(t, err)
				require.NotNil(t, res)
				require.Equal(t, tt.expectedResult, res.IsNullRound)
				require.Equal(t, tt.epoch, res.Height)
			}
		})
	}
}

func TestFailureHeadHeight(t *testing.T) {
	ctx := context.Background()
	seed := time.Now().UnixNano()
	t.Logf("seed: %d", seed)
	rng := pseudo.New(pseudo.NewSource(seed))
	headHeight := abi.ChainEpoch(100)

	si, head, _ := setupWithHeadIndexed(t, headHeight, rng)
	t.Cleanup(func() { _ = si.Close() })
	si.Start()

	_, err := si.ChainValidateIndex(ctx, head.Height(), false)
	require.Error(t, err)
	require.Contains(t, err.Error(), "cannot validate index at epoch")
}

func TestBackfillNullRound(t *testing.T) {
	ctx := context.Background()
	seed := time.Now().UnixNano()
	t.Logf("seed: %d", seed)
	rng := pseudo.New(pseudo.NewSource(seed))
	headHeight := abi.ChainEpoch(100)

	si, _, cs := setupWithHeadIndexed(t, headHeight, rng)
	t.Cleanup(func() { _ = si.Close() })
	si.Start()

	nullRoundEpoch := abi.ChainEpoch(50)
	nonNullRoundEpoch := abi.ChainEpoch(51)

	// Create a tipset with a height different from the requested epoch
	nonNullTs := fakeTipSet(t, rng, nonNullRoundEpoch, []cid.Cid{})

	// Set up the chainstore to return the non-null tipset for the null round epoch
	cs.SetTipsetByHeightAndKey(nullRoundEpoch, nonNullTs.Key(), nonNullTs)

	// Attempt to validate the null round epoch
	result, err := si.ChainValidateIndex(ctx, nullRoundEpoch, true)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, result.Backfilled)
	require.True(t, result.IsNullRound)
}

func TestBackfillReturnsError(t *testing.T) {
	ctx := context.Background()
	seed := time.Now().UnixNano()
	t.Logf("seed: %d", seed)
	rng := pseudo.New(pseudo.NewSource(seed))
	headHeight := abi.ChainEpoch(100)

	si, _, cs := setupWithHeadIndexed(t, headHeight, rng)
	t.Cleanup(func() { _ = si.Close() })
	si.Start()

	missingEpoch := abi.ChainEpoch(50)

	// Create a tipset for the missing epoch, but don't index it
	missingTs := fakeTipSet(t, rng, missingEpoch, []cid.Cid{})
	cs.SetTipsetByHeightAndKey(missingEpoch, missingTs.Key(), missingTs)

	// Attempt to validate the missing epoch with backfill flag set to false
	_, err := si.ChainValidateIndex(ctx, missingEpoch, false)
	require.Error(t, err)
	require.ErrorContains(t, err, "missing tipset at height 50 in the chain index")
}

func TestBackfillMissingEpoch(t *testing.T) {
	ctx := context.Background()
	seed := time.Now().UnixNano()
	t.Logf("seed: %d", seed)
	rng := pseudo.New(pseudo.NewSource(seed))
	headHeight := abi.ChainEpoch(100)

	si, _, cs := setupWithHeadIndexed(t, headHeight, rng)
	t.Cleanup(func() { _ = si.Close() })
	si.Start()

	// Initialize address resolver
	si.SetActorToDelegatedAddresFunc(func(ctx context.Context, emitter abi.ActorID, ts *types.TipSet) (address.Address, bool) {
		idAddr, err := address.NewIDAddress(uint64(emitter))
		if err != nil {
			return address.Undef, false
		}
		return idAddr, true
	})

	missingEpoch := abi.ChainEpoch(50)

	parentTs := fakeTipSet(t, rng, missingEpoch-1, []cid.Cid{})
	cs.SetTipsetByHeightAndKey(missingEpoch-1, parentTs.Key(), parentTs)

	missingTs := fakeTipSet(t, rng, missingEpoch, parentTs.Cids())
	cs.SetTipsetByHeightAndKey(missingEpoch, missingTs.Key(), missingTs)

	executionTs := fakeTipSet(t, rng, missingEpoch+1, missingTs.Key().Cids())
	cs.SetTipsetByHeightAndKey(missingEpoch+1, executionTs.Key(), executionTs)

	// Create fake messages and events
	fakeMsg := fakeMessage(randomIDAddr(t, rng), randomIDAddr(t, rng))
	fakeEvent := fakeEvent(1, []kv{{k: "test", v: []byte("value")}, {k: "test2", v: []byte("value2")}}, nil)

	ec := randomCid(t, rng)
	executedMsg := executedMessage{
		msg: fakeMsg,
		evs: []types.Event{*fakeEvent},
		rct: types.MessageReceipt{
			EventsRoot: &ec,
		},
	}

	cs.SetMessagesForTipset(missingTs, []types.ChainMsg{fakeMsg})
	si.setExecutedMessagesLoaderFunc(func(ctx context.Context, cs ChainStore, msgTs, rctTs *types.TipSet) ([]executedMessage, error) {
		if msgTs.Height() == missingTs.Height() {
			return []executedMessage{executedMsg}, nil
		}
		return nil, nil
	})

	// Attempt to validate and backfill the missing epoch
	result, err := si.ChainValidateIndex(ctx, missingEpoch, true)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.Backfilled)
	require.EqualValues(t, missingEpoch, result.Height)
	require.Equal(t, uint64(1), result.IndexedMessagesCount)
	require.Equal(t, uint64(1), result.IndexedEventsCount)
	require.Equal(t, uint64(2), result.IndexedEventEntriesCount)

	// Verify that the epoch is now indexed
	// fails as the events root don't match
	verificationResult, err := si.ChainValidateIndex(ctx, missingEpoch, false)
	require.ErrorContains(t, err, "events AMT root mismatch")
	require.Nil(t, verificationResult)

	tsKeyCid, err := missingTs.Key().Cid()
	require.NoError(t, err)

	root, b, err := si.amtRootForEvents(ctx, tsKeyCid, fakeMsg.Cid())
	require.NoError(t, err)
	require.True(t, b)
	executedMsg.rct.EventsRoot = &root
	si.setExecutedMessagesLoaderFunc(func(ctx context.Context, cs ChainStore, msgTs, rctTs *types.TipSet) ([]executedMessage, error) {
		if msgTs.Height() == missingTs.Height() {
			return []executedMessage{executedMsg}, nil
		}
		return nil, nil
	})

	verificationResult, err = si.ChainValidateIndex(ctx, missingEpoch, false)
	require.NoError(t, err)
	require.NotNil(t, verificationResult)
	require.False(t, verificationResult.Backfilled)
	require.Equal(t, result.IndexedMessagesCount, verificationResult.IndexedMessagesCount)
	require.Equal(t, result.IndexedEventsCount, verificationResult.IndexedEventsCount)
	require.Equal(t, result.IndexedEventEntriesCount, verificationResult.IndexedEventEntriesCount)
}

func TestIndexCorruption(t *testing.T) {
	ctx := context.Background()
	seed := time.Now().UnixNano()
	t.Logf("seed: %d", seed)
	rng := pseudo.New(pseudo.NewSource(seed))
	headHeight := abi.ChainEpoch(100)

	tests := []struct {
		name          string
		setupFunc     func(*testing.T, *SqliteIndexer, *dummyChainStore)
		epoch         abi.ChainEpoch
		errorContains string
	}{
		{
			name: "only reverted tipsets",
			setupFunc: func(t *testing.T, si *SqliteIndexer, cs *dummyChainStore) {
				epoch := abi.ChainEpoch(50)
				ts := fakeTipSet(t, rng, epoch, []cid.Cid{})
				cs.SetTipsetByHeightAndKey(epoch, ts.Key(), ts)
				keyBz, err := ts.Key().Cid()
				require.NoError(t, err)

				insertTipsetMessage(t, si, tipsetMessage{
					tipsetKeyCid: keyBz.Bytes(),
					height:       uint64(epoch),
					reverted:     true,
					messageCid:   randomCid(t, rng).Bytes(),
					messageIndex: 0,
				})
			},
			epoch:         50,
			errorContains: "index corruption: height 50 only has reverted tipsets",
		},
		{
			name: "multiple non-reverted tipsets",
			setupFunc: func(t *testing.T, si *SqliteIndexer, cs *dummyChainStore) {
				epoch := abi.ChainEpoch(50)
				ts1 := fakeTipSet(t, rng, epoch, []cid.Cid{})
				ts2 := fakeTipSet(t, rng, epoch, []cid.Cid{})
				cs.SetTipsetByHeightAndKey(epoch, ts1.Key(), ts1)

				t1Bz, err := toTipsetKeyCidBytes(ts1)
				require.NoError(t, err)
				t2Bz, err := toTipsetKeyCidBytes(ts2)
				require.NoError(t, err)

				insertTipsetMessage(t, si, tipsetMessage{
					tipsetKeyCid: t1Bz,
					height:       uint64(epoch),
					reverted:     false,
					messageCid:   randomCid(t, rng).Bytes(),
					messageIndex: 0,
				})
				insertTipsetMessage(t, si, tipsetMessage{
					tipsetKeyCid: t2Bz,
					height:       uint64(epoch),
					reverted:     false,
					messageCid:   randomCid(t, rng).Bytes(),
					messageIndex: 0,
				})
			},
			epoch:         50,
			errorContains: "index corruption: height 50 has multiple non-reverted tipsets",
		},
		{
			name: "tipset key mismatch",
			setupFunc: func(_ *testing.T, si *SqliteIndexer, cs *dummyChainStore) {
				epoch := abi.ChainEpoch(50)
				ts1 := fakeTipSet(t, rng, epoch, []cid.Cid{})
				ts2 := fakeTipSet(t, rng, epoch, []cid.Cid{})
				cs.SetTipsetByHeightAndKey(epoch, ts1.Key(), ts1)
				insertTipsetMessage(t, si, tipsetMessage{
					tipsetKeyCid: ts2.Key().Cids()[0].Bytes(),
					height:       uint64(epoch),
					reverted:     false,
					messageCid:   randomCid(t, rng).Bytes(),
					messageIndex: 0,
				})
			},
			epoch:         50,
			errorContains: "index corruption: indexed tipset at height 50 has key",
		},
		{
			name: "reverted events for executed tipset",
			setupFunc: func(_ *testing.T, si *SqliteIndexer, cs *dummyChainStore) {
				epoch := abi.ChainEpoch(50)
				ts := fakeTipSet(t, rng, epoch, []cid.Cid{})
				cs.SetTipsetByHeightAndKey(epoch, ts.Key(), ts)
				keyBz, err := ts.Key().Cid()
				require.NoError(t, err)

				messageID := insertTipsetMessage(t, si, tipsetMessage{
					tipsetKeyCid: keyBz.Bytes(),
					height:       uint64(epoch),
					reverted:     false,
					messageCid:   randomCid(t, rng).Bytes(),
					messageIndex: 0,
				})
				insertEvent(t, si, event{
					messageID:   messageID,
					eventIndex:  0,
					emitterId:   1,
					emitterAddr: randomIDAddr(t, rng).Bytes(),
					reverted:    true,
				})
				cs.SetTipsetByHeightAndKey(epoch+1, fakeTipSet(t, rng, epoch+1, ts.Key().Cids()).Key(), fakeTipSet(t, rng, epoch+1, ts.Key().Cids()))
			},
			epoch:         50,
			errorContains: "index corruption: reverted events found for an executed tipset",
		},
		{
			name: "message count mismatch",
			setupFunc: func(_ *testing.T, si *SqliteIndexer, cs *dummyChainStore) {
				epoch := abi.ChainEpoch(50)
				ts := fakeTipSet(t, rng, epoch, []cid.Cid{})
				cs.SetTipsetByHeightAndKey(epoch, ts.Key(), ts)
				keyBz, err := ts.Key().Cid()
				require.NoError(t, err)

				// Insert two messages in the index
				insertTipsetMessage(t, si, tipsetMessage{
					tipsetKeyCid: keyBz.Bytes(),
					height:       uint64(epoch),
					reverted:     false,
					messageCid:   randomCid(t, rng).Bytes(),
					messageIndex: 0,
				})
				insertTipsetMessage(t, si, tipsetMessage{
					tipsetKeyCid: keyBz.Bytes(),
					height:       uint64(epoch),
					reverted:     false,
					messageCid:   randomCid(t, rng).Bytes(),
					messageIndex: 1,
				})

				// Setup dummy event loader
				si.setExecutedMessagesLoaderFunc(func(ctx context.Context, cs ChainStore, msgTs, rctTs *types.TipSet) ([]executedMessage, error) {
					return []executedMessage{{msg: fakeMessage(randomIDAddr(t, rng), randomIDAddr(t, rng))}}, nil
				})

				// Set up the next tipset for event execution
				nextTs := fakeTipSet(t, rng, epoch+1, ts.Key().Cids())
				cs.SetTipsetByHeightAndKey(epoch+1, nextTs.Key(), nextTs)
			},
			epoch:         50,
			errorContains: "failed to verify indexed data at height 50: message count mismatch for height 50: chainstore has 1, index has 2",
		},
		{
			name: "event count mismatch",
			setupFunc: func(_ *testing.T, si *SqliteIndexer, cs *dummyChainStore) {
				epoch := abi.ChainEpoch(50)
				ts := fakeTipSet(t, rng, epoch, []cid.Cid{})
				cs.SetTipsetByHeightAndKey(epoch, ts.Key(), ts)
				keyBz, err := ts.Key().Cid()
				require.NoError(t, err)

				// Insert one message in the index
				messageID := insertTipsetMessage(t, si, tipsetMessage{
					tipsetKeyCid: keyBz.Bytes(),
					height:       uint64(epoch),
					reverted:     false,
					messageCid:   randomCid(t, rng).Bytes(),
					messageIndex: 0,
				})

				// Insert two events for the message
				insertEvent(t, si, event{
					messageID:   messageID,
					eventIndex:  0,
					emitterId:   2,
					emitterAddr: randomIDAddr(t, rng).Bytes(),
					reverted:    false,
				})
				insertEvent(t, si, event{
					messageID:   messageID,
					eventIndex:  1,
					emitterId:   3,
					emitterAddr: randomIDAddr(t, rng).Bytes(),
					reverted:    false,
				})

				// Setup dummy event loader to return only one event
				si.setExecutedMessagesLoaderFunc(func(ctx context.Context, cs ChainStore, msgTs, rctTs *types.TipSet) ([]executedMessage, error) {
					return []executedMessage{
						{
							msg: fakeMessage(randomIDAddr(t, rng), randomIDAddr(t, rng)),
							evs: []types.Event{*fakeEvent(1, []kv{{k: "test", v: []byte("value")}}, nil)},
						},
					}, nil
				})

				// Set up the next tipset for event execution
				nextTs := fakeTipSet(t, rng, epoch+1, ts.Key().Cids())
				cs.SetTipsetByHeightAndKey(epoch+1, nextTs.Key(), nextTs)
			},
			epoch:         50,
			errorContains: "failed to verify indexed data at height 50: event count mismatch for height 50: chainstore has 1, index has 2",
		},
		{
			name: "event entries count mismatch",
			setupFunc: func(_ *testing.T, si *SqliteIndexer, cs *dummyChainStore) {
				epoch := abi.ChainEpoch(50)
				ts := fakeTipSet(t, rng, epoch, []cid.Cid{})
				cs.SetTipsetByHeightAndKey(epoch, ts.Key(), ts)
				keyBz, err := ts.Key().Cid()
				require.NoError(t, err)

				// Insert one message in the index
				messageID := insertTipsetMessage(t, si, tipsetMessage{
					tipsetKeyCid: keyBz.Bytes(),
					height:       uint64(epoch),
					reverted:     false,
					messageCid:   randomCid(t, rng).Bytes(),
					messageIndex: 0,
				})

				// Insert one event with two entries for the message
				eventID := insertEvent(t, si, event{
					messageID:   messageID,
					eventIndex:  0,
					emitterId:   4,
					emitterAddr: randomIDAddr(t, rng).Bytes(),
					reverted:    false,
				})
				insertEventEntry(t, si, eventEntry{
					eventID: eventID,
					indexed: true,
					flags:   []byte{0x01},
					key:     "key1",
					codec:   1,
					value:   []byte("value1"),
				})
				insertEventEntry(t, si, eventEntry{
					eventID: eventID,
					indexed: true,
					flags:   []byte{0x00},
					key:     "key2",
					codec:   2,
					value:   []byte("value2"),
				})

				// Setup dummy event loader to return one event with only one entry
				si.setExecutedMessagesLoaderFunc(func(ctx context.Context, cs ChainStore, msgTs, rctTs *types.TipSet) ([]executedMessage, error) {
					return []executedMessage{
						{
							msg: fakeMessage(randomIDAddr(t, rng), randomIDAddr(t, rng)),
							evs: []types.Event{*fakeEvent(1, []kv{{k: "key1", v: []byte("value1")}}, nil)},
						},
					}, nil
				})

				// Set up the next tipset for event execution
				nextTs := fakeTipSet(t, rng, epoch+1, ts.Key().Cids())
				cs.SetTipsetByHeightAndKey(epoch+1, nextTs.Key(), nextTs)
			},
			epoch:         50,
			errorContains: "failed to verify indexed data at height 50: event entries count mismatch for height 50: chainstore has 1, index has 2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			si, _, cs := setupWithHeadIndexed(t, headHeight, rng)
			t.Cleanup(func() { _ = si.Close() })
			si.Start()

			tt.setupFunc(t, si, cs)

			_, err := si.ChainValidateIndex(ctx, tt.epoch, false)
			require.Error(t, err)
			require.Contains(t, err.Error(), tt.errorContains)
		})
	}
}
