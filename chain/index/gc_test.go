package index

import (
	"context"
	"fmt"
	pseudo "math/rand"
	"path/filepath"
	"testing"
	"time"

	"github.com/multiformats/go-multicodec"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/chain/actors/builtin"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/types/ethtypes"
	"github.com/filecoin-project/lotus/lib/sqlite"
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

	// Tipset rows and eth tx hashes each needed several batches to get here.
	requireCounts(6, 6, 2, 2)
	var minHeight int
	require.NoError(t, si.db.QueryRow("SELECT MIN(height) FROM tipset_message").Scan(&minHeight))
	require.Equal(t, 200, minHeight)
}

func TestGCBatchRemovesWholeTipsets(t *testing.T) {
	ctx := context.Background()
	rng := pseudo.New(pseudo.NewSource(14030))

	prevBatchSize := gcBatchSize
	gcBatchSize = 2
	t.Cleanup(func() { gcBatchSize = prevBatchSize })

	msgTipset := fakeTipSet(t, rng, 10, nil)
	executionTipset := fakeTipSet(t, rng, 11, msgTipset.Cids())
	retainedTipset := fakeTipSet(t, rng, 200, executionTipset.Cids())

	cs := newDummyChainStore()
	cs.SetHeaviestTipSet(retainedTipset)
	for _, ts := range []*types.TipSet{msgTipset, executionTipset} {
		cs.SetTipsetByHeightAndKey(ts.Height(), ts.Key(), ts)
		cs.SetTipSetByCid(t, ts)
	}

	// Three messages with one event each: more rows than one batch of two.
	var msgs []types.ChainMsg
	var executed []executedMessage
	for i := range 3 {
		msg := fakeMessage(address.TestAddress, address.TestAddress)
		msg.Nonce = uint64(i)
		msgs = append(msgs, msg)
		executed = append(executed, executedMessage{
			msg: msg,
			evs: []types.Event{*fakeEvent(abi.ActorID(1), []kv{{k: "t1", v: []byte("topic")}}, nil)},
		})
	}
	cs.SetMessagesForTipset(msgTipset, msgs)

	si, err := NewSqliteIndexer(":memory:", cs, 0, false, 0)
	require.NoError(t, err)
	t.Cleanup(func() { _ = si.Close() })
	si.SetActorToDelegatedAddresFunc(func(context.Context, abi.ActorID, *types.TipSet) (address.Address, bool) {
		return address.TestAddress, true
	})
	si.setExecutedMessagesLoaderFunc(func(context.Context, ChainStore, *types.TipSet, *types.TipSet) ([]executedMessage, error) {
		return executed, nil
	})

	require.NoError(t, si.Apply(ctx, msgTipset, executionTipset))
	insertHead(t, si, retainedTipset, retainedTipset.Height())

	msgTipsetCid, err := msgTipset.Key().Cid()
	require.NoError(t, err)
	blockFilter := &EventFilter{MinHeight: -1, MaxHeight: -1, TipsetCid: msgTipsetCid, Codec: multicodec.Raw}
	events, err := si.GetEventsForFilter(ctx, blockFilter)
	require.NoError(t, err)
	require.Len(t, events, 3)

	// The first batch reaches the second row, which is in msgTipset, so it
	// removes all of msgTipset's rows and its bloom together.
	rows, blooms, done, err := si.gcTipsetBatch(ctx, 100)
	require.NoError(t, err)
	require.Equal(t, int64(3), rows)
	require.Equal(t, int64(1), blooms)
	require.False(t, done)

	// A block-hash read between batches finds the tipset unindexed rather
	// than returning a truncated event set.
	events, err = si.GetEventsForFilter(ctx, blockFilter)
	require.ErrorIs(t, err, ErrNotFound)
	require.Empty(t, events)

	// Fewer than a batch of rows remain below the cutoff: the last batch takes them all.
	rows, blooms, done, err = si.gcTipsetBatch(ctx, 100)
	require.NoError(t, err)
	require.Equal(t, int64(1), rows)
	require.Equal(t, int64(0), blooms)
	require.True(t, done)

	var heights []int
	r, err := si.db.Query("SELECT DISTINCT height FROM tipset_message")
	require.NoError(t, err)
	for r.Next() {
		var h int
		require.NoError(t, r.Scan(&h))
		heights = append(heights, h)
	}
	require.NoError(t, r.Err())
	require.NoError(t, r.Close())
	require.Equal(t, []int{200}, heights)
}

func TestGCBatchWaitsForOtherWriters(t *testing.T) {
	ctx := context.Background()
	rng := pseudo.New(pseudo.NewSource(14030))

	path := filepath.Join(t.TempDir(), DefaultDbFilename)
	si, err := NewSqliteIndexer(path, newDummyChainStore(), 0, false, 0)
	require.NoError(t, err)
	t.Cleanup(func() { _ = si.Close() })

	ts := fakeTipSet(t, rng, 10, nil)
	insertHead(t, si, ts, ts.Height())

	// A second connection holds SQLite's write lock, as an eth tx hash insert
	// does, for longer than withTx's retries take to run out.
	other, err := sqlite.Open(path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = other.Close() })
	tx, err := other.BeginTx(ctx, nil)
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, "INSERT INTO eth_tx_hash (tx_hash, message_cid) VALUES (?, ?)", ethtypes.EthHash{1}.String(), randomCid(t, rng).Bytes())
	require.NoError(t, err)
	released := make(chan error, 1)
	go func() {
		time.Sleep(700 * time.Millisecond)
		released <- tx.Commit()
	}()

	rows, _, done, err := si.gcTipsetBatch(ctx, 100)
	require.NoError(t, err)
	require.NoError(t, <-released)
	require.Equal(t, int64(1), rows)
	require.True(t, done)
}

func TestGCBatchKeepsMarkersOfRemainingHeights(t *testing.T) {
	ctx := context.Background()
	rng := pseudo.New(pseudo.NewSource(14030))

	prevBatchSize := gcBatchSize
	gcBatchSize = 2
	t.Cleanup(func() { gcBatchSize = prevBatchSize })

	si, _, _ := setupWithHeadIndexed(t, 3000, rng)
	t.Cleanup(func() { _ = si.Close() })

	// Two heights below the cutoff, each with two rows and a bloom, so one
	// batch of two covers height 10 only.
	for _, height := range []uint64{10, 20} {
		key := []byte(fmt.Sprintf("tipset-%d", height))
		for m := range 2 {
			insertTipsetMessage(t, si, tipsetMessage{
				tipsetKeyCid: key,
				height:       height,
				messageCid:   []byte(fmt.Sprintf("msg-%d-%d", height, m)),
				messageIndex: int64(m),
			})
		}
		_, err := si.stmts.insertTipsetBloomStmt.Exec(key, height, []byte{0})
		require.NoError(t, err)
	}

	rows, blooms, done, err := si.gcTipsetBatch(ctx, 100)
	require.NoError(t, err)
	require.Equal(t, int64(2), rows)
	require.Equal(t, int64(1), blooms)
	require.False(t, done)

	var bloomHeight, rowsAt20 int
	require.NoError(t, si.db.QueryRow("SELECT height FROM tipset_bloom").Scan(&bloomHeight))
	require.Equal(t, 20, bloomHeight)
	require.NoError(t, si.db.QueryRow("SELECT COUNT(*) FROM tipset_message WHERE height = 20").Scan(&rowsAt20))
	require.Equal(t, 2, rowsAt20)
}

func TestGCRetentionEpochsBounds(t *testing.T) {
	for _, epochs := range []int64{0, builtin.EpochsInDay, 7 * builtin.EpochsInDay} {
		si, err := NewSqliteIndexer(":memory:", newDummyChainStore(), epochs, false, 0)
		require.NoError(t, err, "epochs %d", epochs)
		require.NoError(t, si.Close())
	}
	for _, epochs := range []int64{-1, 1, builtin.EpochsInDay - 1} {
		_, err := NewSqliteIndexer(":memory:", newDummyChainStore(), epochs, false, 0)
		require.ErrorContains(t, err, fmt.Sprintf("must be 0 or at least %d", builtin.EpochsInDay), "epochs %d", epochs)
	}
}
