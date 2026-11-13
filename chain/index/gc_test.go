package index

import (
	"context"
	"fmt"
	pseudo "math/rand"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/chain/actors/builtin"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/types/ethtypes"
)

func TestGC(t *testing.T) {
	ctx := context.Background()
	seed := time.Now().UnixNano()
	t.Logf("seed: %d", seed)
	rng := pseudo.New(pseudo.NewSource(seed))
	headHeight := abi.ChainEpoch(60)
	si, _, cs := setupWithHeadIndexed(t, headHeight, rng)
	t.Cleanup(func() { _ = si.Close() })

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

	si.SetActorToDelegatedAddresFunc(func(ctx context.Context, emitter abi.ActorID, ts *types.TipSet) (address.Address, bool) {
		idAddr, err := address.NewIDAddress(uint64(emitter))
		if err != nil {
			return address.Undef, false
		}

		return idAddr, true
	})

	si.setExecutedMessagesLoaderFunc(func(ctx context.Context, cs ChainStore, msgTs, rctTs *types.TipSet) ([]executedMessage, error) {
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
	cs.SetTipSetByCid(t, fakeTipSet1)
	cs.SetTipSetByCid(t, fakeTipSet2)
	cs.SetTipSetByCid(t, fakeTipSet3)

	cs.SetMessagesForTipset(fakeTipSet1, []types.ChainMsg{fm})

	// index tipset and events
	require.NoError(t, si.Apply(ctx, fakeTipSet1, fakeTipSet2))
	require.NoError(t, si.Apply(ctx, fakeTipSet2, fakeTipSet3))

	// getLogs works for height 1
	filter := &EventFilter{
		MinHeight: 1,
		MaxHeight: 1,
	}
	ces, err := si.GetEventsForFilter(ctx, filter)
	require.NoError(t, err)
	require.Len(t, ces, 2)

	si.gc(ctx)

	// getLogs does not work for height 1
	_, err = si.GetEventsForFilter(ctx, filter)
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

func TestGCBatchesUnderWriterLock(t *testing.T) {
	ctx := context.Background()
	rng := pseudo.New(pseudo.NewSource(13964))

	prevBatchSize := gcBatchSize
	gcBatchSize = 2
	t.Cleanup(func() { gcBatchSize = prevBatchSize })

	cs := newDummyChainStore()
	cs.SetHeaviestTipSet(fakeTipSet(t, rng, 3000, nil))

	// A file rather than :memory:, so the GC goroutine and the test see the same
	// database whichever pooled connection they get.
	si, err := NewSqliteIndexer(filepath.Join(t.TempDir(), DefaultDbFilename), cs, 0, false, 0)
	require.NoError(t, err)
	t.Cleanup(func() { _ = si.Close() })
	si.gcRetentionEpochs = builtin.EpochsInDay // removal epoch 3000 - 2880 - 10 = 110

	// Five tipsets below the removal epoch and two above, each with a bloom and
	// three messages that each carry one event with one entry.
	for i, height := range []uint64{10, 20, 30, 40, 50, 200, 300} {
		key := []byte(fmt.Sprintf("tipset-%d", i))
		for m := range 3 {
			msgID := insertTipsetMessage(t, si, tipsetMessage{
				tipsetKeyCid: key,
				height:       height,
				messageCid:   []byte(fmt.Sprintf("msg-%d-%d", i, m)),
				messageIndex: int64(m),
			})
			eventID := insertEvent(t, si, event{messageID: msgID, emitterId: 1})
			insertEventEntry(t, si, eventEntry{eventID: eventID, flags: []byte{0}, key: "k", value: []byte("v")})
		}
		_, err := si.stmts.insertTipsetBloomStmt.Exec(key, height, []byte{0})
		require.NoError(t, err)
	}

	// Five eth tx hashes past retention, two within it.
	for i := range 7 {
		insertEthTxHash(t, si, ethtypes.EthHash{byte(i)}, randomCid(t, rng))
	}
	_, err = si.db.Exec("UPDATE eth_tx_hash SET inserted_at = datetime('now', '-2 day') WHERE rowid <= 5")
	require.NoError(t, err)

	count := func(table string) int {
		var n int
		require.NoError(t, si.db.QueryRow("SELECT COUNT(*) FROM "+table).Scan(&n))
		return n
	}
	requireCounts := func(tipsetMessages, events, blooms, ethHashes int) {
		t.Helper()
		require.Equal(t, tipsetMessages, count("tipset_message"))
		require.Equal(t, events, count("event"))
		require.Equal(t, events, count("event_entry"))
		require.Equal(t, blooms, count("tipset_bloom"))
		require.Equal(t, ethHashes, count("eth_tx_hash"))
	}
	requireCounts(21, 21, 7, 7)

	si.writerLk.Lock()
	done := make(chan struct{})
	go func() {
		si.gc(ctx)
		close(done)
	}()
	select {
	case <-done:
		t.Fatal("gc completed while writerLk was held")
	case <-time.After(200 * time.Millisecond):
	}
	requireCounts(21, 21, 7, 7)
	si.writerLk.Unlock()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("gc did not complete after writerLk was released")
	}

	// Each table needed several batches of two to reach these counts.
	requireCounts(6, 6, 2, 2)
	var minHeight int
	require.NoError(t, si.db.QueryRow("SELECT MIN(height) FROM tipset_message").Scan(&minHeight))
	require.Equal(t, 200, minHeight)
}
