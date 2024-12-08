package index

import "database/sql"

const DefaultDbFilename = "chainindex.db"

var ddls = []string{
	`CREATE TABLE IF NOT EXISTS tipset_message (
		id INTEGER PRIMARY KEY,
		tipset_key_cid BLOB NOT NULL,
		height INTEGER NOT NULL,
		reverted INTEGER NOT NULL,
		message_cid BLOB,
		message_index INTEGER,
		UNIQUE (tipset_key_cid, message_cid)
	)`,

	`CREATE TABLE IF NOT EXISTS eth_tx_hash (
		tx_hash TEXT PRIMARY KEY,
		message_cid BLOB NOT NULL,
		inserted_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	)`,

	`CREATE TABLE IF NOT EXISTS event (
		id INTEGER PRIMARY KEY,
		message_id INTEGER NOT NULL,
		event_index INTEGER NOT NULL,
		emitter_id INTEGER NOT NULL,
		emitter_addr BLOB,
		reverted INTEGER NOT NULL,
		FOREIGN KEY (message_id) REFERENCES tipset_message(id) ON DELETE CASCADE,
		UNIQUE (message_id, event_index)
	)`,

	`CREATE TABLE IF NOT EXISTS event_entry (
		event_id INTEGER NOT NULL,
		indexed INTEGER NOT NULL,
		flags BLOB NOT NULL,
		key TEXT NOT NULL,
		codec INTEGER,
		value BLOB NOT NULL,
		FOREIGN KEY (event_id) REFERENCES event(id) ON DELETE CASCADE
	)`,

	`CREATE INDEX IF NOT EXISTS insertion_time_index ON eth_tx_hash (inserted_at)`,

	`CREATE INDEX IF NOT EXISTS idx_message_cid ON tipset_message (message_cid)`,

	`CREATE INDEX IF NOT EXISTS idx_tipset_key_cid ON tipset_message (tipset_key_cid)`,

	`CREATE INDEX IF NOT EXISTS idx_event_message_id ON event (message_id)`,

	`CREATE INDEX IF NOT EXISTS idx_height ON tipset_message (height)`,

	`CREATE INDEX IF NOT EXISTS event_entry_event_id ON event_entry(event_id)`,
}

// preparedStatementMapping returns a map of fields of the preparedStatements struct to the SQL
// query that should be prepared for that field. This is used to prepare all the statements in
// the preparedStatements struct.
func preparedStatementMapping(ps *preparedStatements) map[**sql.Stmt]string {
	return map[**sql.Stmt]string{
		&ps.getNonRevertedMsgInfoStmt:                 "SELECT tipset_key_cid, height FROM tipset_message WHERE message_cid = ? AND reverted = 0 LIMIT 1",
		&ps.getMsgCidFromEthHashStmt:                  "SELECT message_cid FROM eth_tx_hash WHERE tx_hash = ? LIMIT 1",
		&ps.insertEthTxHashStmt:                       "INSERT INTO eth_tx_hash (tx_hash, message_cid) VALUES (?, ?) ON CONFLICT (tx_hash) DO UPDATE SET inserted_at = CURRENT_TIMESTAMP",
		&ps.insertTipsetMessageStmt:                   "INSERT INTO tipset_message (tipset_key_cid, height, reverted, message_cid, message_index) VALUES (?, ?, ?, ?, ?) ON CONFLICT (tipset_key_cid, message_cid) DO UPDATE SET reverted = 0",
		&ps.hasTipsetStmt:                             "SELECT EXISTS(SELECT 1 FROM tipset_message WHERE tipset_key_cid = ?)",
		&ps.updateTipsetToNonRevertedStmt:             "UPDATE tipset_message SET reverted = 0 WHERE tipset_key_cid = ?",
		&ps.updateTipsetToRevertedStmt:                "UPDATE tipset_message SET reverted = 1 WHERE tipset_key_cid = ?",
		&ps.removeTipsetsBeforeHeightStmt:             "DELETE FROM tipset_message WHERE height < ?",
		&ps.removeEthHashesOlderThanStmt:              "DELETE FROM eth_tx_hash WHERE inserted_at < datetime('now', ?)",
		&ps.updateTipsetsToRevertedFromHeightStmt:     "UPDATE tipset_message SET reverted = 1 WHERE height >= ?",
		&ps.updateEventsToRevertedFromHeightStmt:      "UPDATE event SET reverted = 1 WHERE message_id IN (SELECT id FROM tipset_message WHERE height >= ?)",
		&ps.isIndexEmptyStmt:                          "SELECT NOT EXISTS(SELECT 1 FROM tipset_message LIMIT 1)",
		&ps.getMinNonRevertedHeightStmt:               "SELECT MIN(height) FROM tipset_message WHERE reverted = 0",
		&ps.hasNonRevertedTipsetStmt:                  "SELECT EXISTS(SELECT 1 FROM tipset_message WHERE tipset_key_cid = ? AND reverted = 0)",
		&ps.updateEventsToRevertedStmt:                "UPDATE event SET reverted = 1 WHERE message_id IN (SELECT id FROM tipset_message WHERE tipset_key_cid = ?)",
		&ps.updateEventsToNonRevertedStmt:             "UPDATE event SET reverted = 0 WHERE message_id IN (SELECT id FROM tipset_message WHERE tipset_key_cid = ?)",
		&ps.getMsgIdForMsgCidAndTipsetStmt:            "SELECT id FROM tipset_message WHERE tipset_key_cid = ? AND message_cid = ? AND reverted = 0",
		&ps.insertEventStmt:                           "INSERT INTO event (message_id, event_index, emitter_addr, reverted) VALUES (?, ?, ?, ?) ON CONFLICT (message_id, event_index) DO UPDATE SET reverted = 0",
		&ps.insertEventEntryStmt:                      "INSERT INTO event_entry (event_id, indexed, flags, key, codec, value) VALUES (?, ?, ?, ?, ?, ?)",
		&ps.hasNullRoundAtHeightStmt:                  "SELECT NOT EXISTS(SELECT 1 FROM tipset_message WHERE height = ?)",
		&ps.getNonRevertedTipsetAtHeightStmt:          "SELECT tipset_key_cid FROM tipset_message WHERE height = ? AND reverted = 0 LIMIT 1",
		&ps.countTipsetsAtHeightStmt:                  "SELECT COUNT(CASE WHEN reverted = 1 THEN 1 END) AS reverted_count, COUNT(CASE WHEN reverted = 0 THEN 1 END) AS non_reverted_count FROM (SELECT tipset_key_cid, MAX(reverted) AS reverted FROM tipset_message WHERE height = ? GROUP BY tipset_key_cid) AS unique_tipsets",
		&ps.getNonRevertedTipsetMessageCountStmt:      "SELECT COUNT(*) FROM tipset_message WHERE tipset_key_cid = ? AND reverted = 0 AND message_cid IS NOT NULL",
		&ps.getNonRevertedTipsetEventCountStmt:        "SELECT COUNT(*) FROM event WHERE reverted = 0 AND message_id IN (SELECT id FROM tipset_message WHERE tipset_key_cid = ? AND reverted = 0)",
		&ps.hasRevertedEventsInTipsetStmt:             "SELECT EXISTS(SELECT 1 FROM event WHERE reverted = 1 AND message_id IN (SELECT id FROM tipset_message WHERE tipset_key_cid = ?))",
		&ps.getNonRevertedTipsetEventEntriesCountStmt: "SELECT COUNT(ee.event_id) AS entry_count FROM event_entry ee JOIN event e ON ee.event_id = e.id JOIN tipset_message tm ON e.message_id = tm.id WHERE tm.tipset_key_cid = ? AND tm.reverted = 0",
		&ps.removeRevertedTipsetsBeforeHeightStmt:     "DELETE FROM tipset_message WHERE reverted = 1 AND height < ?",
		&ps.getNonRevertedMsgInfoStmt:                 "SELECT tipset_key_cid, height FROM tipset_message WHERE message_cid = ? AND reverted = 0 LIMIT 1",
		&ps.getMsgCidFromEthHashStmt:                  "SELECT message_cid FROM eth_tx_hash WHERE tx_hash = ? LIMIT 1",
		&ps.insertEthTxHashStmt:                       "INSERT INTO eth_tx_hash (tx_hash, message_cid) VALUES (?, ?) ON CONFLICT (tx_hash) DO UPDATE SET inserted_at = CURRENT_TIMESTAMP",
		&ps.insertTipsetMessageStmt:                   "INSERT INTO tipset_message (tipset_key_cid, height, reverted, message_cid, message_index) VALUES (?, ?, ?, ?, ?) ON CONFLICT (tipset_key_cid, message_cid) DO UPDATE SET reverted = 0",
		&ps.updateTipsetToNonRevertedStmt:             "UPDATE tipset_message SET reverted = 0 WHERE tipset_key_cid = ?",
		&ps.updateTipsetToRevertedStmt:                "UPDATE tipset_message SET reverted = 1 WHERE tipset_key_cid = ?",
		&ps.removeTipsetsBeforeHeightStmt:             "DELETE FROM tipset_message WHERE height < ?",
		&ps.removeEthHashesOlderThanStmt:              "DELETE FROM eth_tx_hash WHERE inserted_at < datetime('now', ?)",
		&ps.updateTipsetsToRevertedFromHeightStmt:     "UPDATE tipset_message SET reverted = 1 WHERE height >= ?",
		&ps.updateEventsToRevertedFromHeightStmt:      "UPDATE event SET reverted = 1 WHERE message_id IN (SELECT id FROM tipset_message WHERE height >= ?)",
		&ps.getMinNonRevertedHeightStmt:               "SELECT MIN(height) FROM tipset_message WHERE reverted = 0",
		&ps.hasNonRevertedTipsetStmt:                  "SELECT EXISTS(SELECT 1 FROM tipset_message WHERE tipset_key_cid = ? AND reverted = 0)",
		&ps.updateEventsToRevertedStmt:                "UPDATE event SET reverted = 1 WHERE message_id IN (SELECT id FROM tipset_message WHERE tipset_key_cid = ?)",
		&ps.updateEventsToNonRevertedStmt:             "UPDATE event SET reverted = 0 WHERE message_id IN (SELECT id FROM tipset_message WHERE tipset_key_cid = ?)",
		&ps.getMsgIdForMsgCidAndTipsetStmt:            "SELECT id FROM tipset_message WHERE tipset_key_cid = ? AND message_cid = ? AND reverted = 0 LIMIT 1",
		&ps.insertEventStmt:                           "INSERT INTO event (message_id, event_index, emitter_id, emitter_addr, reverted) VALUES (?, ?, ?, ?, ?)",
		&ps.insertEventEntryStmt:                      "INSERT INTO event_entry (event_id, indexed, flags, key, codec, value) VALUES (?, ?, ?, ?, ?, ?)",
		&ps.getEventEntriesStmt:                       "SELECT flags, key, codec, value FROM event_entry WHERE event_id=? ORDER BY _rowid_ ASC",
		&ps.getEventIdAndEmitterIdStmt:                "SELECT e.id, e.emitter_id FROM event e JOIN tipset_message tm ON e.message_id = tm.id WHERE tm.tipset_key_cid = ? AND tm.message_cid = ? ORDER BY e.event_index ASC",
	}
}
