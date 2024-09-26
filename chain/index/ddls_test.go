package index

import (
	"database/sql"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHasRevertedEventsInTipsetStmt(t *testing.T) {
	s, err := NewSqliteIndexer(":memory:", nil, 0, false, 0)
	require.NoError(t, err)

	// running on empty DB should return false
	verifyHasRevertedEventsInTipsetStmt(t, s, []byte("test_tipset_key"), false)

	// Insert tipset with a reverted event
	insertTipsetMessage(t, s.db, []byte("test_tipset_key"), 1, true, []byte("test_message_cid"), 0)

	messageID := 1
	insertEvent(t, s.db, messageID, 0, []byte("test_emitter_addr"), true)

	// Verify `hasRevertedEventsInTipset` returns true
	verifyHasRevertedEventsInTipsetStmt(t, s, []byte("test_tipset_key"), true)

	// change event to non-reverted
	_, err = s.db.Exec("UPDATE event SET reverted = 0 WHERE message_id = ? AND event_index = 0", messageID)
	require.NoError(t, err)

	// Verify `hasRevertedEventsInTipset` returns false
	verifyHasRevertedEventsInTipsetStmt(t, s, []byte("test_tipset_key"), false)
}

func TestGetNonRevertedTipsetCountStmts(t *testing.T) {
	s, err := NewSqliteIndexer(":memory:", nil, 0, false, 0)
	require.NoError(t, err)
	// great comment

	// running on empty DB should return 0
	verifyNonRevertedEventEntriesCount(t, s, []byte("test_tipset_key"), 0)
	verifyNonRevertedEventCount(t, s, []byte("test_tipset_key"), 0)
	verifyNonRevertedMessageCount(t, s, []byte("test_tipset_key"), 0)

	// Insert non-reverted tipset
	insertTipsetMessage(t, s.db, []byte("test_tipset_key"), 1, false, []byte("test_message_cid"), 0)

	// Insert event
	messageID := 1
	insertEvent(t, s.db, messageID, 0, []byte("test_emitter_addr"), false)
	insertEvent(t, s.db, messageID, 1, []byte("test_emitter_addr"), false)

	eventID := 1
	// Insert event entry
	insertEventEntry(t, s.db, eventID, true, []byte("test_flags"), "test_key", 1, []byte("test_value"))

	// Verify `getNonRevertedTipsetEventEntriesCountStmt` returns 1
	verifyNonRevertedEventEntriesCount(t, s, []byte("test_tipset_key"), 1)

	// Verify event count
	verifyNonRevertedEventCount(t, s, []byte("test_tipset_key"), 2)

	// verify message count is 1
	verifyNonRevertedMessageCount(t, s, []byte("test_tipset_key"), 1)

	// mark tipset as reverted
	_, err = s.db.Exec("UPDATE tipset_message SET reverted = 1 WHERE tipset_key_cid = ?", []byte("test_tipset_key"))
	require.NoError(t, err)

	// Verify `getNonRevertedTipsetEventEntriesCountStmt` returns 0
	verifyNonRevertedEventEntriesCount(t, s, []byte("test_tipset_key"), 0)

	// verify event count is 0
	verifyNonRevertedEventCount(t, s, []byte("test_tipset_key"), 0)

	// verify message count is 0
	verifyNonRevertedMessageCount(t, s, []byte("test_tipset_key"), 0)
}

func TestHasNullRoundAtHeightStmt(t *testing.T) {
	s, err := NewSqliteIndexer(":memory:", nil, 0, false, 0)
	require.NoError(t, err)

	// running on empty DB should return false
	verifyHasNullRoundAtHeightStmt(t, s, 1, true)

	// insert tipset with null round
	insertTipsetMessage(t, s.db, []byte("test_tipset_key"), 1, false, []byte("test_message_cid"), 0)

	// verify count
	verifyHasNullRoundAtHeightStmt(t, s, 1, false)
}

func TestHasTipsetStmt(t *testing.T) {
	s, err := NewSqliteIndexer(":memory:", nil, 0, false, 0)
	require.NoError(t, err)

	// running on empty DB should return false
	verifyHasTipsetStmt(t, s, []byte("test_tipset_key"), false)

	// insert tipset
	insertTipsetMessage(t, s.db, []byte("test_tipset_key"), 1, false, []byte("test_message_cid"), 0)

	// verify tipset exists
	verifyHasTipsetStmt(t, s, []byte("test_tipset_key"), true)

	// verify non-existent tipset
	verifyHasTipsetStmt(t, s, []byte("non_existent_tipset_key"), false)
}

func TestUpdateEventsToRevertedStmt(t *testing.T) {
	s, err := NewSqliteIndexer(":memory:", nil, 0, false, 0)
	require.NoError(t, err)

	// Insert a non-reverted tipset
	insertTipsetMessage(t, s.db, []byte("test_tipset_key"), 1, false, []byte("test_message_cid"), 0)

	// Insert non-reverted events
	messageID := 1
	insertEvent(t, s.db, messageID, 0, []byte("test_emitter_addr"), false)
	insertEvent(t, s.db, messageID, 1, []byte("test_emitter_addr"), false)

	// Verify events are not reverted
	var count int
	err = s.db.QueryRow("SELECT COUNT(*) FROM event WHERE reverted = 0").Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 2, count)

	// Execute updateEventsToRevertedStmt
	_, err = s.stmts.updateEventsToRevertedStmt.Exec([]byte("test_tipset_key"))
	require.NoError(t, err)

	// Verify events are now reverted
	err = s.db.QueryRow("SELECT COUNT(*) FROM event WHERE reverted = 1").Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 2, count)

	// Verify no non-reverted events remain
	err = s.db.QueryRow("SELECT COUNT(*) FROM event WHERE reverted = 0").Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 0, count)
}

func TestCountTipsetsAtHeightStmt(t *testing.T) {
	s, err := NewSqliteIndexer(":memory:", nil, 0, false, 0)
	require.NoError(t, err)

	// Test empty DB
	verifyCountTipsetsAtHeightStmt(t, s, 1, 0, 0)

	// Test 0,1 case
	insertTipsetMessage(t, s.db, []byte("test_tipset_key_1"), 1, false, []byte("test_message_cid_1"), 0)
	verifyCountTipsetsAtHeightStmt(t, s, 1, 0, 1)

	// Test 0,2 case
	insertTipsetMessage(t, s.db, []byte("test_tipset_key_2"), 1, false, []byte("test_message_cid_2"), 0)
	verifyCountTipsetsAtHeightStmt(t, s, 1, 0, 2)

	// Test 1,2 case
	insertTipsetMessage(t, s.db, []byte("test_tipset_key_3"), 1, true, []byte("test_message_cid_3"), 0)
	verifyCountTipsetsAtHeightStmt(t, s, 1, 1, 2)

	// Test 2,2 case
	insertTipsetMessage(t, s.db, []byte("test_tipset_key_4"), 1, true, []byte("test_message_cid_4"), 0)
	verifyCountTipsetsAtHeightStmt(t, s, 1, 2, 2)

}

func TestNonRevertedTipsetAtHeightStmt(t *testing.T) {
	s, err := NewSqliteIndexer(":memory:", nil, 0, false, 0)
	require.NoError(t, err)

	// Insert non-reverted tipset
	insertTipsetMessage(t, s.db, []byte("test_tipset_key_1"), 10, false, []byte("test_message_cid_1"), 0)

	// Insert reverted tipset at same height
	insertTipsetMessage(t, s.db, []byte("test_tipset_key_2"), 10, true, []byte("test_message_cid_2"), 0)

	// Verify getNonRevertedTipsetAtHeightStmt returns the non-reverted tipset
	var tipsetKeyCid []byte
	err = s.stmts.getNonRevertedTipsetAtHeightStmt.QueryRow(10).Scan(&tipsetKeyCid)
	require.NoError(t, err)
	require.Equal(t, []byte("test_tipset_key_1"), tipsetKeyCid)

	// Insert another non-reverted tipset at a different height
	insertTipsetMessage(t, s.db, []byte("test_tipset_key_3"), 20, false, []byte("test_message_cid_3"), 0)

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
	insertTipsetMessage(t, s.db, []byte("test_tipset_key_1"), 10, false, []byte("test_message_cid_1"), 0)
	insertTipsetMessage(t, s.db, []byte("test_tipset_key_2"), 20, false, []byte("test_message_cid_2"), 0)
	insertTipsetMessage(t, s.db, []byte("test_tipset_key_3"), 30, false, []byte("test_message_cid_3"), 0)

	// Verify minimum non-reverted height
	err = s.stmts.getMinNonRevertedHeightStmt.QueryRow().Scan(&minHeight)
	require.NoError(t, err)
	require.True(t, minHeight.Valid)
	require.Equal(t, int64(10), minHeight.Int64)

	// Insert reverted tipset with lower height
	insertTipsetMessage(t, s.db, []byte("test_tipset_key_4"), 5, true, []byte("test_message_cid_4"), 0)

	// Verify minimum non-reverted height hasn't changed
	err = s.stmts.getMinNonRevertedHeightStmt.QueryRow().Scan(&minHeight)
	require.NoError(t, err)
	require.True(t, minHeight.Valid)
	require.Equal(t, int64(10), minHeight.Int64)

	// Revert all tipsets
	_, err = s.db.Exec("UPDATE tipset_message SET reverted = 1")
	require.NoError(t, err)

	// Verify no minimum non-reverted height
	err = s.stmts.getMinNonRevertedHeightStmt.QueryRow().Scan(&minHeight)
	require.NoError(t, err)
	require.False(t, minHeight.Valid)
}

func TestGetMsgIdForMsgCidAndTipsetStmt(t *testing.T) {
	s, err := NewSqliteIndexer(":memory:", nil, 0, false, 0)
	require.NoError(t, err)

	// Insert a non-reverted tipset
	tipsetKeyCid := []byte("test_tipset_key")
	messageCid := []byte("test_message_cid")
	insertTipsetMessage(t, s.db, tipsetKeyCid, 1, false, messageCid, 0)

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
	insertTipsetMessage(t, s.db, revertedTipsetKeyCid, 2, true, messageCid, 0)

	// Verify getMsgIdForMsgCidAndTipset doesn't return the message ID for a reverted tipset
	err = s.stmts.getMsgIdForMsgCidAndTipsetStmt.QueryRow(revertedTipsetKeyCid, messageCid).Scan(&messageID)
	require.Equal(t, sql.ErrNoRows, err)
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

func insertTipsetMessage(t *testing.T, db *sql.DB, tipsetKeyCid []byte, height uint64, reverted bool,
	messageCid []byte, messageIndex uint64) {
	_, err := db.Exec("INSERT INTO tipset_message (tipset_key_cid, height, reverted, message_cid, message_index) VALUES (?, ?, ?, ?, ?)",
		tipsetKeyCid, height, reverted, messageCid, messageIndex)
	require.NoError(t, err)
}

func insertEvent(t *testing.T, db *sql.DB, messageID int, eventIndex uint64, emitterAddr []byte, reverted bool) {
	_, err := db.Exec("INSERT INTO event (message_id, event_index, emitter_addr, reverted) VALUES (?, ?, ?, ?)",
		messageID, eventIndex, emitterAddr, reverted)
	require.NoError(t, err)
}

func insertEventEntry(t *testing.T, db *sql.DB, eventID int, indexed bool, flags []byte, key string, codec int, value []byte) {
	_, err := db.Exec("INSERT INTO event_entry (event_id, indexed, flags, key, codec, value) VALUES (?, ?, ?, ?, ?, ?)",
		eventID, indexed, flags, key, codec, value)
	require.NoError(t, err)
}
