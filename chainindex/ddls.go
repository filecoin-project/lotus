package chainindex

const DefaultDbFilename = "chainindex.db"

const (
	stmtGetNonRevertedMessageInfo = "SELECT tipset_key_cid, height FROM tipset_message WHERE message_cid = ? AND reverted = 0"
	stmtGetMsgCidFromEthHash      = "SELECT message_cid FROM eth_tx_hash WHERE tx_hash = ?"
	stmtInsertEthTxHash           = "INSERT INTO eth_tx_hash (tx_hash, message_cid) VALUES (?, ?) ON CONFLICT (tx_hash) DO UPDATE SET inserted_at = CURRENT_TIMESTAMP"

	stmtInsertTipsetMessage = "INSERT INTO tipset_message (tipset_key_cid, height, reverted, message_cid, message_index) VALUES (?, ?, ?, ?, ?) ON CONFLICT (tipset_key_cid, message_cid) DO UPDATE SET reverted = 0"

	stmtHasTipset                 = "SELECT EXISTS(SELECT 1 FROM tipset_message WHERE tipset_key_cid = ?)"
	stmtUpdateTipsetToNonReverted = "UPDATE tipset_message SET reverted = 0 WHERE tipset_key_cid = ?"

	stmtUpdateTipsetToReverted = "UPDATE tipset_message SET reverted = 1 WHERE tipset_key_cid = ?"

	stmtGetMaxNonRevertedTipset = "SELECT tipset_key_cid FROM tipset_message WHERE reverted = 0 ORDER BY height DESC LIMIT 1"

	stmtRemoveRevertedTipsetsBeforeHeight = "DELETE FROM tipset_message WHERE height < ? AND reverted = 1"
	stmtRemoveTipsetsBeforeHeight         = "DELETE FROM tipset_message WHERE height < ?"

	stmtRemoveEthHashesOlderThan = `DELETE FROM eth_tx_hash WHERE inserted_at < datetime('now', ?);`

	stmtUpdateTipsetsToRevertedFromHeight = "UPDATE tipset_message SET reverted = 1 WHERE height >= ?"

	stmtIsTipsetMessageNonEmpty = "SELECT EXISTS(SELECT 1 FROM tipset_message LIMIT 1)"

	stmtGetMinNonRevertedHeight = `SELECT MIN(height) FROM tipset_message WHERE reverted = 0`

	stmtHasNonRevertedTipset = `SELECT EXISTS(SELECT 1 FROM tipset_message WHERE tipset_key_cid = ? AND reverted = 0)`

	stmtUpdateEventsToReverted = `UPDATE event SET reverted = 1 WHERE message_id IN (
			SELECT message_id FROM tipset_message WHERE tipset_key_cid = ?
		)`

	stmtUpdateEventsToNonReverted = `UPDATE event SET reverted = 0 WHERE message_id IN (
		SELECT message_id FROM tipset_message WHERE tipset_key_cid = ?
	)`

	stmtGetMsgIdForMsgCidAndTipset = `SELECT message_id FROM tipset_message WHERE message_cid = ? AND tipset_key_cid = ?`

	stmtInsertEvent      = "INSERT INTO event (message_id, event_index, emitter_addr, reverted) VALUES (?, ?, ?, ?)"
	stmtInsertEventEntry = "INSERT INTO event_entry (event_id, indexed, flags, key, codec, value) VALUES (?, ?, ?, ?, ?, ?)"
)

var ddls = []string{
	`CREATE TABLE IF NOT EXISTS tipset_message (
		message_id INTEGER PRIMARY KEY,
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
		event_id INTEGER PRIMARY KEY,
		message_id INTEGER NOT NULL,
        event_index INTEGER NOT NULL,
        emitter_addr BLOB NOT NULL,
		reverted INTEGER NOT NULL,
		FOREIGN KEY (message_id) REFERENCES tipset_message(message_id) ON DELETE CASCADE,
		UNIQUE (message_id, event_index)
    )`,

	`CREATE TABLE IF NOT EXISTS event_entry (
		event_id INTEGER NOT NULL,
		indexed INTEGER NOT NULL,
		flags BLOB NOT NULL,
		key TEXT NOT NULL,
		codec INTEGER,
		value BLOB NOT NULL,
		FOREIGN KEY (event_id) REFERENCES event(event_id) ON DELETE CASCADE
	)`,

	`CREATE INDEX IF NOT EXISTS insertion_time_index ON eth_tx_hash (inserted_at)`,

	`CREATE INDEX IF NOT EXISTS idx_message_cid ON tipset_message (message_cid)`,

	`CREATE INDEX IF NOT EXISTS idx_tipset_key_cid ON tipset_message (tipset_key_cid)`,

	`CREATE INDEX IF NOT EXISTS idx_event_message_id ON event (message_id)`,

	`CREATE INDEX IF NOT EXISTS idx_height ON tipset_message (height)`,
}
