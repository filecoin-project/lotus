package index

import (
	"database/sql"
	"testing"

	"github.com/stretchr/testify/require"
)

const (
	tipsetKeyCid1 = "test_tipset_key"
	tipsetKeyCid2 = "test_tipset_key_2"
	messageCid1   = "test_message_cid"
	messageCid2   = "test_message_cid_2"
	emitterAddr1  = "test_emitter_addr"
)

func TestHasRevertedEventsInTipsetStmt(t *testing.T) {
	s, err := NewSqliteIndexer(":memory:", nil, 0, false, 0)
	require.NoError(t, err)

	// running on empty DB should return false
	verifyHasRevertedEventsInTipsetStmt(t, s, []byte(tipsetKeyCid1), false)

	// Insert tipset with a reverted event
	ts := tipsetMessage{
		tipsetKeyCid: []byte(tipsetKeyCid1),
		height:       1,
		reverted:     false,
		messageCid:   []byte(messageCid1),
		messageIndex: 0,
	}
	messageID := insertTipsetMessage(t, s, ts)

	// this event will be un-reverted later
	insertEvent(t, s, event{
		messageID:   messageID,
		eventIndex:  0,
		emitterId:   1,
		emitterAddr: []byte(emitterAddr1),
		reverted:    true,
	})

	// this event should not be un-reverted
	ts = tipsetMessage{
		tipsetKeyCid: []byte(tipsetKeyCid2),
		height:       1,
		reverted:     false,
		messageCid:   []byte(messageCid2),
		messageIndex: 0,
	}
	messageID2 := insertTipsetMessage(t, s, ts)
	insertEvent(t, s, event{
		messageID:   messageID2,
		eventIndex:  0,
		emitterId:   2,
		emitterAddr: []byte(emitterAddr1),
		reverted:    true,
	})

	// Verify `hasRevertedEventsInTipset` returns true
	verifyHasRevertedEventsInTipsetStmt(t, s, []byte(tipsetKeyCid1), true)
	verifyHasRevertedEventsInTipsetStmt(t, s, []byte(tipsetKeyCid2), true)

	// change event to non-reverted
	updateEventsToNonReverted(t, s, []byte(tipsetKeyCid1))

	// Verify `hasRevertedEventsInTipset` returns false
	verifyHasRevertedEventsInTipsetStmt(t, s, []byte(tipsetKeyCid1), false)
	verifyHasRevertedEventsInTipsetStmt(t, s, []byte(tipsetKeyCid2), true)
}

func TestGetNonRevertedTipsetCountStmts(t *testing.T) {
	s, err := NewSqliteIndexer(":memory:", nil, 0, false, 0)
	require.NoError(t, err)

	// running on empty DB should return 0
	verifyNonRevertedEventEntriesCount(t, s, []byte(tipsetKeyCid1), 0)
	verifyNonRevertedEventCount(t, s, []byte(tipsetKeyCid1), 0)
	verifyNonRevertedMessageCount(t, s, []byte(tipsetKeyCid1), 0)

	// Insert non-reverted tipset
	messageID := insertTipsetMessage(t, s, tipsetMessage{
		tipsetKeyCid: []byte(tipsetKeyCid1),
		height:       1,
		reverted:     false,
		messageCid:   []byte(messageCid1),
		messageIndex: 0,
	})

	// Insert event
	eventID1 := insertEvent(t, s, event{
		messageID:   messageID,
		eventIndex:  0,
		emitterId:   1,
		emitterAddr: []byte(emitterAddr1),
		reverted:    false,
	})
	eventID2 := insertEvent(t, s, event{
		messageID:   messageID,
		eventIndex:  1,
		emitterId:   2,
		emitterAddr: []byte(emitterAddr1),
		reverted:    false,
	})

	// Insert event entry
	insertEventEntry(t, s, eventEntry{
		eventID: eventID1,
		indexed: true,
		flags:   []byte("test_flags"),
		key:     "test_key",
		codec:   1,
		value:   []byte("test_value"),
	})
	insertEventEntry(t, s, eventEntry{
		eventID: eventID2,
		indexed: true,
		flags:   []byte("test_flags2"),
		key:     "test_key2",
		codec:   2,
		value:   []byte("test_value2"),
	})

	// verify 2 event entries
	verifyNonRevertedEventEntriesCount(t, s, []byte(tipsetKeyCid1), 2)

	// Verify event count
	verifyNonRevertedEventCount(t, s, []byte(tipsetKeyCid1), 2)

	// verify message count is 1
	verifyNonRevertedMessageCount(t, s, []byte(tipsetKeyCid1), 1)

	// mark tipset as reverted
	revertTipset(t, s, []byte(tipsetKeyCid1))

	// Verify `getNonRevertedTipsetEventEntriesCountStmt` returns 0
	verifyNonRevertedEventEntriesCount(t, s, []byte(tipsetKeyCid1), 0)

	// verify event count is 0
	verifyNonRevertedEventCount(t, s, []byte(tipsetKeyCid1), 0)

	// verify message count is 0
	verifyNonRevertedMessageCount(t, s, []byte(tipsetKeyCid1), 0)
}

func TestGetEventIdAndEmitterIdStmtAndGetEventEntriesStmt(t *testing.T) {
	s, err := NewSqliteIndexer(":memory:", nil, 0, false, 0)
	require.NoError(t, err)

	// Insert a tipset message
	tsKeyCid := []byte("test_tipset_key")
	msgCid := []byte("test_message_cid")
	messageID := insertTipsetMessage(t, s, tipsetMessage{
		tipsetKeyCid: tsKeyCid,
		height:       1,
		reverted:     false,
		messageCid:   msgCid,
		messageIndex: 0,
	})

	// Insert events
	event1ID := insertEvent(t, s, event{
		messageID:   messageID,
		eventIndex:  0,
		emitterId:   1,
		emitterAddr: []byte("emitter_addr_1"),
		reverted:    false,
	})
	event2ID := insertEvent(t, s, event{
		messageID:   messageID,
		eventIndex:  1,
		emitterId:   2,
		emitterAddr: []byte("emitter_addr_2"),
		reverted:    false,
	})

	// Insert event entries
	insertEventEntry(t, s, eventEntry{
		eventID: event1ID,
		indexed: true,
		flags:   []byte{0x01},
		key:     "key1",
		codec:   1,
		value:   []byte("value1"),
	})
	insertEventEntry(t, s, eventEntry{
		eventID: event1ID,
		indexed: false,
		flags:   []byte{0x00},
		key:     "key2",
		codec:   2,
		value:   []byte("value2"),
	})
	insertEventEntry(t, s, eventEntry{
		eventID: event2ID,
		indexed: true,
		flags:   []byte{0x01},
		key:     "key3",
		codec:   3,
		value:   []byte("value3"),
	})

	// Test getEventIdAndEmitterIdStmt
	rows, err := s.stmts.getEventIdAndEmitterIdStmt.Query(tsKeyCid, msgCid)
	require.NoError(t, err)
	defer func() {
		_ = rows.Close()
	}()
	var eventIDs []int64
	var emitterIDs []uint64
	for rows.Next() {
		var eventID int64
		var emitterID uint64
		err := rows.Scan(&eventID, &emitterID)
		require.NoError(t, err)
		eventIDs = append(eventIDs, eventID)
		emitterIDs = append(emitterIDs, emitterID)
	}
	require.NoError(t, rows.Err())
	require.Equal(t, []int64{event1ID, event2ID}, eventIDs)
	require.Equal(t, []uint64{1, 2}, emitterIDs)

	// Test getEventEntriesStmt for event1
	rows, err = s.stmts.getEventEntriesStmt.Query(event1ID)
	require.NoError(t, err)
	defer func() {
		_ = rows.Close()
	}()

	var entries []eventEntry
	for rows.Next() {
		var entry eventEntry
		err := rows.Scan(&entry.flags, &entry.key, &entry.codec, &entry.value)
		require.NoError(t, err)
		entries = append(entries, entry)
	}
	require.NoError(t, rows.Err())
	require.Len(t, entries, 2)
	require.Equal(t, []byte{0x01}, entries[0].flags)
	require.Equal(t, "key1", entries[0].key)
	require.Equal(t, 1, entries[0].codec)
	require.Equal(t, []byte("value1"), entries[0].value)
	require.Equal(t, []byte{0x00}, entries[1].flags)
	require.Equal(t, "key2", entries[1].key)
	require.Equal(t, 2, entries[1].codec)
	require.Equal(t, []byte("value2"), entries[1].value)

	// Test getEventEntriesStmt for event2
	rows, err = s.stmts.getEventEntriesStmt.Query(event2ID)
	require.NoError(t, err)
	defer func() {
		_ = rows.Close()
	}()

	entries = nil
	for rows.Next() {
		var entry eventEntry
		err := rows.Scan(&entry.flags, &entry.key, &entry.codec, &entry.value)
		require.NoError(t, err)
		entries = append(entries, entry)
	}
	require.NoError(t, rows.Err())
	require.Len(t, entries, 1)
	require.Equal(t, []byte{0x01}, entries[0].flags)
	require.Equal(t, "key3", entries[0].key)
	require.Equal(t, 3, entries[0].codec)
	require.Equal(t, []byte("value3"), entries[0].value)
}
func TestUpdateTipsetToNonRevertedStmt(t *testing.T) {
	s, err := NewSqliteIndexer(":memory:", nil, 0, false, 0)
	require.NoError(t, err)

	// insert a reverted tipset
	ts := tipsetMessage{
		tipsetKeyCid: []byte(tipsetKeyCid1),
		height:       1,
		reverted:     true,
		messageCid:   []byte(messageCid1),
		messageIndex: 0,
	}

	// Insert tipset
	messageId := insertTipsetMessage(t, s, ts)

	res, err := s.stmts.updateTipsetToNonRevertedStmt.Exec([]byte(tipsetKeyCid1))
	require.NoError(t, err)

	rowsAffected, err := res.RowsAffected()
	require.NoError(t, err)
	require.Equal(t, int64(1), rowsAffected)

	// verify the tipset is not reverted
	ts.reverted = false
	verifyTipsetMessage(t, s, messageId, ts)
}

func TestHasNullRoundAtHeightStmt(t *testing.T) {
	s, err := NewSqliteIndexer(":memory:", nil, 0, false, 0)
	require.NoError(t, err)

	// running on empty DB should return true
	verifyHasNullRoundAtHeightStmt(t, s, 1, true)
	verifyHasNullRoundAtHeightStmt(t, s, 0, true)

	// insert tipset
	insertTipsetMessage(t, s, tipsetMessage{
		tipsetKeyCid: []byte(tipsetKeyCid1),
		height:       1,
		reverted:     false,
		messageCid:   []byte(messageCid1),
		messageIndex: 0,
	})

	// verify not a null round
	verifyHasNullRoundAtHeightStmt(t, s, 1, false)
}

func TestHasTipsetStmt(t *testing.T) {
	s, err := NewSqliteIndexer(":memory:", nil, 0, false, 0)
	require.NoError(t, err)

	// running on empty DB should return false
	verifyHasTipsetStmt(t, s, []byte(tipsetKeyCid1), false)

	// insert tipset
	insertTipsetMessage(t, s, tipsetMessage{
		tipsetKeyCid: []byte(tipsetKeyCid1),
		height:       1,
		reverted:     false,
		messageCid:   []byte(messageCid1),
		messageIndex: 0,
	})

	// verify tipset exists
	verifyHasTipsetStmt(t, s, []byte(tipsetKeyCid1), true)

	// verify non-existent tipset
	verifyHasTipsetStmt(t, s, []byte("non_existent_tipset_key"), false)
}

func TestUpdateEventsToRevertedStmt(t *testing.T) {
	s, err := NewSqliteIndexer(":memory:", nil, 0, false, 0)
	require.NoError(t, err)

	// Insert a non-reverted tipset
	messageID := insertTipsetMessage(t, s, tipsetMessage{
		tipsetKeyCid: []byte(tipsetKeyCid1),
		height:       1,
		reverted:     false,
		messageCid:   []byte(messageCid1),
		messageIndex: 0,
	})

	// Insert non-reverted events
	insertEvent(t, s, event{
		messageID:   messageID,
		eventIndex:  0,
		emitterId:   1,
		emitterAddr: []byte(emitterAddr1),
		reverted:    false,
	})
	insertEvent(t, s, event{
		messageID:   messageID,
		eventIndex:  1,
		emitterId:   2,
		emitterAddr: []byte(emitterAddr1),
		reverted:    false,
	})

	// Verify events are not reverted
	var count int
	err = s.db.QueryRow("SELECT COUNT(*) FROM event WHERE reverted = 0 AND message_id = ?", messageID).Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 2, count)

	// Execute updateEventsToRevertedStmt
	_, err = s.stmts.updateEventsToRevertedStmt.Exec([]byte(tipsetKeyCid1))
	require.NoError(t, err)

	// Verify events are now reverted
	err = s.db.QueryRow("SELECT COUNT(*) FROM event WHERE reverted = 1 AND message_id = ?", messageID).Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 2, count)

	// Verify no non-reverted events remain
	err = s.db.QueryRow("SELECT COUNT(*) FROM event WHERE reverted = 0 AND message_id = ?", messageID).Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 0, count)
}

func TestCountTipsetsAtHeightStmt(t *testing.T) {
	s, err := NewSqliteIndexer(":memory:", nil, 0, false, 0)
	require.NoError(t, err)

	// Test empty DB
	verifyCountTipsetsAtHeightStmt(t, s, 1, 0, 0)

	// Test 0,1 case
	insertTipsetMessage(t, s, tipsetMessage{
		tipsetKeyCid: []byte("test_tipset_key_1"),
		height:       1,
		reverted:     false,
		messageCid:   []byte("test_message_cid_1"),
		messageIndex: 0,
	})
	verifyCountTipsetsAtHeightStmt(t, s, 1, 0, 1)

	// Test 0,2 case
	insertTipsetMessage(t, s, tipsetMessage{
		tipsetKeyCid: []byte("test_tipset_key_2"),
		height:       1,
		reverted:     false,
		messageCid:   []byte("test_message_cid_2"),
		messageIndex: 0,
	})
	verifyCountTipsetsAtHeightStmt(t, s, 1, 0, 2)

	// Test 1,2 case
	insertTipsetMessage(t, s, tipsetMessage{
		tipsetKeyCid: []byte("test_tipset_key_3"),
		height:       1,
		reverted:     true,
		messageCid:   []byte("test_message_cid_3"),
		messageIndex: 0,
	})
	verifyCountTipsetsAtHeightStmt(t, s, 1, 1, 2)

	// Test 2,2 case
	insertTipsetMessage(t, s, tipsetMessage{
		tipsetKeyCid: []byte("test_tipset_key_4"),
		height:       1,
		reverted:     true,
		messageCid:   []byte("test_message_cid_4"),
		messageIndex: 0,
	})
	verifyCountTipsetsAtHeightStmt(t, s, 1, 2, 2)
}

func TestNonRevertedTipsetAtHeightStmt(t *testing.T) {
	s, err := NewSqliteIndexer(":memory:", nil, 0, false, 0)
	require.NoError(t, err)

	// Test empty DB
	var et []byte
	err = s.stmts.getNonRevertedTipsetAtHeightStmt.QueryRow(10).Scan(&et)
	require.Equal(t, sql.ErrNoRows, err)

	// Insert non-reverted tipset
	insertTipsetMessage(t, s, tipsetMessage{
		tipsetKeyCid: []byte("test_tipset_key_1"),
		height:       10,
		reverted:     false,
		messageCid:   []byte("test_message_cid_1"),
		messageIndex: 0,
	})

	// Insert reverted tipset at same height
	insertTipsetMessage(t, s, tipsetMessage{
		tipsetKeyCid: []byte("test_tipset_key_2"),
		height:       10,
		reverted:     true,
		messageCid:   []byte("test_message_cid_2"),
		messageIndex: 0,
	})

	// Verify getNonRevertedTipsetAtHeightStmt returns the non-reverted tipset
	var tipsetKeyCid []byte
	err = s.stmts.getNonRevertedTipsetAtHeightStmt.QueryRow(10).Scan(&tipsetKeyCid)
	require.NoError(t, err)
	require.Equal(t, []byte("test_tipset_key_1"), tipsetKeyCid)

	// Insert another non-reverted tipset at a different height
	insertTipsetMessage(t, s, tipsetMessage{
		tipsetKeyCid: []byte("test_tipset_key_3"),
		height:       20,
		reverted:     false,
		messageCid:   []byte("test_message_cid_3"),
		messageIndex: 0,
	})

	// Verify getNonRevertedTipsetAtHeightStmt returns the correct tipset for the new height
	err = s.stmts.getNonRevertedTipsetAtHeightStmt.QueryRow(20).Scan(&tipsetKeyCid)
	require.NoError(t, err)
	require.Equal(t, []byte("test_tipset_key_3"), tipsetKeyCid)

	// Test with a height that has no tipset
	err = s.stmts.getNonRevertedTipsetAtHeightStmt.QueryRow(30).Scan(&tipsetKeyCid)
	require.Equal(t, sql.ErrNoRows, err)

	// Revert all tipsets at height 10
	_, err = s.db.Exec("UPDATE tipset_message SET reverted = 1 WHERE height = 10")
	require.NoError(t, err)

	// Verify getNonRevertedTipsetAtHeightStmt returns no rows for the reverted height
	err = s.stmts.getNonRevertedTipsetAtHeightStmt.QueryRow(10).Scan(&tipsetKeyCid)
	require.Equal(t, sql.ErrNoRows, err)
}

func TestMinNonRevertedHeightStmt(t *testing.T) {
	s, err := NewSqliteIndexer(":memory:", nil, 0, false, 0)
	require.NoError(t, err)

	// Test empty DB
	var minHeight sql.NullInt64
	err = s.stmts.getMinNonRevertedHeightStmt.QueryRow().Scan(&minHeight)
	require.NoError(t, err)
	require.False(t, minHeight.Valid)

	// Insert non-reverted tipsets
	insertTipsetMessage(t, s, tipsetMessage{
		tipsetKeyCid: []byte("test_tipset_key_1"),
		height:       10,
		reverted:     false,
		messageCid:   []byte("test_message_cid_1"),
		messageIndex: 0,
	})
	insertTipsetMessage(t, s, tipsetMessage{
		tipsetKeyCid: []byte("test_tipset_key_2"),
		height:       20,
		reverted:     false,
		messageCid:   []byte("test_message_cid_2"),
		messageIndex: 0,
	})

	// Verify minimum non-reverted height
	verifyMinNonRevertedHeightStmt(t, s, 10)

	// Insert reverted tipset with lower height
	insertTipsetMessage(t, s, tipsetMessage{
		tipsetKeyCid: []byte("test_tipset_key_4"),
		height:       5,
		reverted:     true,
		messageCid:   []byte("test_message_cid_4"),
		messageIndex: 0,
	})

	// Verify minimum non-reverted height hasn't changed
	verifyMinNonRevertedHeightStmt(t, s, 10)

	// Revert all tipsets
	_, err = s.db.Exec("UPDATE tipset_message SET reverted = 1")
	require.NoError(t, err)

	// Verify no minimum non-reverted height
	err = s.stmts.getMinNonRevertedHeightStmt.QueryRow().Scan(&minHeight)
	require.NoError(t, err)
	require.False(t, minHeight.Valid)
}

func verifyMinNonRevertedHeightStmt(t *testing.T, s *SqliteIndexer, expectedMinHeight int64) {
	var minHeight sql.NullInt64
	err := s.stmts.getMinNonRevertedHeightStmt.QueryRow().Scan(&minHeight)
	require.NoError(t, err)
	require.True(t, minHeight.Valid)
	require.Equal(t, expectedMinHeight, minHeight.Int64)
}

func TestGetMsgIdForMsgCidAndTipsetStmt(t *testing.T) {
	s, err := NewSqliteIndexer(":memory:", nil, 0, false, 0)
	require.NoError(t, err)

	// Insert a non-reverted tipset
	tipsetKeyCid := []byte(tipsetKeyCid1)
	messageCid := []byte(messageCid1)
	insertTipsetMessage(t, s, tipsetMessage{
		tipsetKeyCid: tipsetKeyCid,
		height:       1,
		reverted:     false,
		messageCid:   messageCid,
		messageIndex: 0,
	})

	// Verify getMsgIdForMsgCidAndTipset returns the correct message ID
	var messageID int64
	err = s.stmts.getMsgIdForMsgCidAndTipsetStmt.QueryRow(tipsetKeyCid, messageCid).Scan(&messageID)
	require.NoError(t, err)
	require.Equal(t, int64(1), messageID)

	// Test with non-existent message CID
	nonExistentMessageCid := []byte("non_existent_message_cid")
	err = s.stmts.getMsgIdForMsgCidAndTipsetStmt.QueryRow(tipsetKeyCid, nonExistentMessageCid).Scan(&messageID)
	require.Equal(t, sql.ErrNoRows, err)

	// Test with non-existent tipset key
	nonExistentTipsetKeyCid := []byte("non_existent_tipset_key")
	err = s.stmts.getMsgIdForMsgCidAndTipsetStmt.QueryRow(nonExistentTipsetKeyCid, messageCid).Scan(&messageID)
	require.Equal(t, sql.ErrNoRows, err)

	// Insert a reverted tipset
	revertedTipsetKeyCid := []byte("reverted_tipset_key")
	insertTipsetMessage(t, s, tipsetMessage{
		tipsetKeyCid: revertedTipsetKeyCid,
		height:       2,
		reverted:     true,
		messageCid:   messageCid,
		messageIndex: 0,
	})

	// Verify getMsgIdForMsgCidAndTipset doesn't return the message ID for a reverted tipset
	err = s.stmts.getMsgIdForMsgCidAndTipsetStmt.QueryRow(revertedTipsetKeyCid, messageCid).Scan(&messageID)
	require.Equal(t, sql.ErrNoRows, err)
}

func TestForeignKeyCascadeDelete(t *testing.T) {
	s, err := NewSqliteIndexer(":memory:", nil, 0, false, 0)
	require.NoError(t, err)

	// Insert a tipset
	messageID := insertTipsetMessage(t, s, tipsetMessage{
		tipsetKeyCid: []byte("test_tipset_key"),
		height:       1,
		reverted:     false,
		messageCid:   []byte(messageCid1),
		messageIndex: 0,
	})

	// Insert an event for the tipset
	eventID := insertEvent(t, s, event{
		messageID:   messageID,
		eventIndex:  0,
		emitterId:   2,
		emitterAddr: []byte("test_emitter_addr"),
		reverted:    false,
	})

	// Insert an event entry for the event
	insertEventEntry(t, s, eventEntry{
		eventID: eventID,
		indexed: true,
		flags:   []byte("test_flags"),
		key:     "test_key",
		codec:   1,
		value:   []byte("test_value"),
	})

	// Delete the tipset
	res, err := s.db.Exec("DELETE FROM tipset_message WHERE tipset_key_cid = ?", []byte("test_tipset_key"))
	require.NoError(t, err)
	rowsAffected, err := res.RowsAffected()
	require.NoError(t, err)
	require.Equal(t, int64(1), rowsAffected)

	// verify event is deleted
	verifyEventAbsent(t, s, eventID)
	verifyEventEntryAbsent(t, s, eventID)
}

func TestInsertTipsetMessage(t *testing.T) {
	s, err := NewSqliteIndexer(":memory:", nil, 0, false, 0)
	require.NoError(t, err)

	ts := tipsetMessage{
		tipsetKeyCid: []byte("test_tipset_key"),
		height:       1,
		reverted:     false,
		messageCid:   []byte(messageCid1),
		messageIndex: 0,
	}

	// Insert a tipset
	messageID := insertTipsetMessage(t, s, ts)

	// revert the tipset
	revertTipset(t, s, []byte("test_tipset_key"))
	ts.reverted = true
	verifyTipsetMessage(t, s, messageID, ts)

	// inserting with the same (tipset, message) should overwrite the reverted flag
	res, err := s.stmts.insertTipsetMessageStmt.Exec(ts.tipsetKeyCid, ts.height, true, ts.messageCid, ts.messageIndex)
	require.NoError(t, err)

	rowsAffected, err := res.RowsAffected()
	require.NoError(t, err)
	require.Equal(t, int64(1), rowsAffected)

	ts.reverted = false
	verifyTipsetMessage(t, s, messageID, ts)
}

type tipsetMessage struct {
	tipsetKeyCid []byte
	height       uint64
	reverted     bool
	messageCid   []byte
	messageIndex int64
}

type event struct {
	eventIndex  uint64
	emitterId   uint64
	emitterAddr []byte
	reverted    bool
	messageID   int64
}

type eventEntry struct {
	eventID int64
	indexed bool
	flags   []byte
	key     string
	codec   int
	value   []byte
}

func updateEventsToNonReverted(t *testing.T, s *SqliteIndexer, tsKeyCid []byte) {
	res, err := s.stmts.updateEventsToNonRevertedStmt.Exec(tsKeyCid)
	require.NoError(t, err)

	rowsAffected, err := res.RowsAffected()
	require.NoError(t, err)
	require.Equal(t, int64(1), rowsAffected)

	// read all events for this tipset and verify they are not reverted using a COUNT query
	var count int
	err = s.db.QueryRow("SELECT COUNT(*) FROM event e JOIN tipset_message tm ON e.message_id = tm.id WHERE tm.tipset_key_cid = ? AND e.reverted = 1", tsKeyCid).Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 0, count, "Expected no reverted events for this tipset")
}

func revertTipset(t *testing.T, s *SqliteIndexer, tipsetKeyCid []byte) {
	res, err := s.stmts.updateTipsetToRevertedStmt.Exec(tipsetKeyCid)
	require.NoError(t, err)

	rowsAffected, err := res.RowsAffected()
	require.NoError(t, err)
	require.Equal(t, int64(1), rowsAffected)

	var reverted bool
	err = s.db.QueryRow("SELECT reverted FROM tipset_message WHERE tipset_key_cid = ?", tipsetKeyCid).Scan(&reverted)
	require.NoError(t, err)
	require.True(t, reverted)
}

func verifyTipsetMessage(t *testing.T, s *SqliteIndexer, messageID int64, expectedTipsetMessage tipsetMessage) {
	var tipsetKeyCid []byte
	var height uint64
	var reverted bool
	var messageCid []byte
	var messageIndex int64
	err := s.db.QueryRow("SELECT tipset_key_cid, height, reverted, message_cid, message_index FROM tipset_message WHERE id = ?", messageID).Scan(&tipsetKeyCid, &height, &reverted, &messageCid, &messageIndex)
	require.NoError(t, err)
	require.Equal(t, expectedTipsetMessage.tipsetKeyCid, tipsetKeyCid)
	require.Equal(t, expectedTipsetMessage.height, height)
	require.Equal(t, expectedTipsetMessage.reverted, reverted)
	require.Equal(t, expectedTipsetMessage.messageCid, messageCid)
	require.Equal(t, expectedTipsetMessage.messageIndex, messageIndex)
}

func verifyEventEntryAbsent(t *testing.T, s *SqliteIndexer, eventID int64) {
	err := s.db.QueryRow("SELECT event_id FROM event_entry WHERE event_id = ?", eventID).Scan(&eventID)
	require.Equal(t, sql.ErrNoRows, err)
}

func verifyEventAbsent(t *testing.T, s *SqliteIndexer, eventID int64) {
	var eventIndex uint64
	err := s.db.QueryRow("SELECT event_index FROM event WHERE id = ?", eventID).Scan(&eventIndex)
	require.Equal(t, sql.ErrNoRows, err)
}

func verifyEvent(t *testing.T, s *SqliteIndexer, eventID int64, expectedEvent event) {
	var eventIndex uint64
	var emitterAddr []byte
	var reverted bool
	var messageID int64
	err := s.db.QueryRow("SELECT event_index, emitter_addr, reverted, message_id FROM event WHERE id = ?", eventID).Scan(&eventIndex, &emitterAddr, &reverted, &messageID)
	require.NoError(t, err)
	require.Equal(t, expectedEvent.eventIndex, eventIndex)
	require.Equal(t, expectedEvent.emitterAddr, emitterAddr)
	require.Equal(t, expectedEvent.reverted, reverted)
	require.Equal(t, expectedEvent.messageID, messageID)
}

func verifyCountTipsetsAtHeightStmt(t *testing.T, s *SqliteIndexer, height uint64, expectedRevertedCount, expectedNonRevertedCount int) {
	var revertedCount, nonRevertedCount int
	err := s.stmts.countTipsetsAtHeightStmt.QueryRow(height).Scan(&revertedCount, &nonRevertedCount)
	require.NoError(t, err)
	require.Equal(t, expectedRevertedCount, revertedCount)
	require.Equal(t, expectedNonRevertedCount, nonRevertedCount)
}

func verifyHasTipsetStmt(t *testing.T, s *SqliteIndexer, tipsetKeyCid []byte, expectedHas bool) {
	var has bool
	err := s.stmts.hasTipsetStmt.QueryRow(tipsetKeyCid).Scan(&has)
	require.NoError(t, err)
	require.Equal(t, expectedHas, has)
}

func verifyHasRevertedEventsInTipsetStmt(t *testing.T, s *SqliteIndexer, tipsetKeyCid []byte, expectedHas bool) {
	var hasRevertedEventsInTipset bool
	err := s.stmts.hasRevertedEventsInTipsetStmt.QueryRow(tipsetKeyCid).Scan(&hasRevertedEventsInTipset)
	require.NoError(t, err)
	require.Equal(t, expectedHas, hasRevertedEventsInTipset)
}

func verifyHasNullRoundAtHeightStmt(t *testing.T, s *SqliteIndexer, height uint64, expectedHasNullRound bool) {
	var hasNullRound bool
	err := s.stmts.hasNullRoundAtHeightStmt.QueryRow(height).Scan(&hasNullRound)
	require.NoError(t, err)
	require.Equal(t, expectedHasNullRound, hasNullRound)
}

func verifyNonRevertedMessageCount(t *testing.T, s *SqliteIndexer, tipsetKeyCid []byte, expectedCount int) {
	var count int
	err := s.stmts.getNonRevertedTipsetMessageCountStmt.QueryRow(tipsetKeyCid).Scan(&count)
	require.NoError(t, err)
	require.Equal(t, expectedCount, count)
}

func verifyNonRevertedEventCount(t *testing.T, s *SqliteIndexer, tipsetKeyCid []byte, expectedCount int) {
	var count int
	err := s.stmts.getNonRevertedTipsetEventCountStmt.QueryRow(tipsetKeyCid).Scan(&count)
	require.NoError(t, err)
	require.Equal(t, expectedCount, count)
}

func verifyNonRevertedEventEntriesCount(t *testing.T, s *SqliteIndexer, tipsetKeyCid []byte, expectedCount int) {
	var count int
	err := s.stmts.getNonRevertedTipsetEventEntriesCountStmt.QueryRow(tipsetKeyCid).Scan(&count)
	require.NoError(t, err)
	require.Equal(t, expectedCount, count)
}

func insertTipsetMessage(t *testing.T, s *SqliteIndexer, ts tipsetMessage) int64 {
	res, err := s.stmts.insertTipsetMessageStmt.Exec(ts.tipsetKeyCid, ts.height, ts.reverted, ts.messageCid, ts.messageIndex)
	require.NoError(t, err)

	rowsAffected, err := res.RowsAffected()
	require.NoError(t, err)
	require.Equal(t, int64(1), rowsAffected)

	messageID, err := res.LastInsertId()
	require.NoError(t, err)
	require.NotEqual(t, int64(0), messageID)

	// read back the message to verify it was inserted correctly
	verifyTipsetMessage(t, s, messageID, ts)

	return messageID
}

func insertEvent(t *testing.T, s *SqliteIndexer, e event) int64 {
	res, err := s.stmts.insertEventStmt.Exec(e.messageID, e.eventIndex, e.emitterId, e.emitterAddr, e.reverted)
	require.NoError(t, err)

	rowsAffected, err := res.RowsAffected()
	require.NoError(t, err)
	require.Equal(t, int64(1), rowsAffected)

	eventID, err := res.LastInsertId()
	require.NoError(t, err)
	require.NotEqual(t, int64(0), eventID)

	verifyEvent(t, s, eventID, e)

	return eventID
}

func insertEventEntry(t *testing.T, s *SqliteIndexer, ee eventEntry) {
	res, err := s.stmts.insertEventEntryStmt.Exec(ee.eventID, ee.indexed, ee.flags, ee.key, ee.codec, ee.value)
	require.NoError(t, err)

	rowsAffected, err := res.RowsAffected()
	require.NoError(t, err)
	require.Equal(t, int64(1), rowsAffected)
}
