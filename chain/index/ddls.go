package index

import "database/sql"

// TODO PRAGMA foreign_keys = ON;
// TODO: Allow for msg_cid <> eth_tx_hash for unconfirmed mpool messages
var ddls = []string{
	`CREATE TABLE IF NOT EXISTS tipset_message (
		message_id INTEGER PRIMARY KEY,
		tipset_key_cid BLOB NOT NULL,
		height INTEGER NOT NULL,
		reverted INTEGER NOT NULL,
		message_cid BLOB NOT NULL,
		message_index INTEGER NOT NULL,
		UNIQUE (tipset_key_cid, message_cid)
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

	`CREATE TABLE eth_tx_hash (
		eth_tx_hash TEXT PRIMARY KEY,
		message_cid BLOB NOT NULL,
		inserted_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	)`,

	// TODO: Build Indexes based on query patterns and `explain query` profiling
}

type preparedStatements struct {
	stmtSelectMsg            *sql.Stmt
	stmtGetMsgCidFromEthHash *sql.Stmt
	stmtRevertTipset         *sql.Stmt
	stmtRevertEvents         *sql.Stmt
	stmtTipsetExists         *sql.Stmt
	stmtTipsetUnRevert       *sql.Stmt
	stmtInsertTipsetMessage  *sql.Stmt

	stmtEventsUnRevert *sql.Stmt

	selectMsgIdForMsgCidAndTipset *sql.Stmt

	stmtInsertEvent      *sql.Stmt
	stmtInsertEventEntry *sql.Stmt

	stmtInsertEthTxHash *sql.Stmt

	stmtRemoveRevertedTipsetsBeforeHeight *sql.Stmt
}

func preparedStatementMapping(ps *preparedStatements) map[**sql.Stmt]string {
	return map[**sql.Stmt]string{
		&ps.stmtSelectMsg:            "SELECT tipset_key_cid, height FROM tipset_message WHERE message_cid = ? AND reverted = 0",
		&ps.stmtGetMsgCidFromEthHash: "SELECT message_cid FROM eth_tx_hash WHERE eth_tx_hash = ?",
		&ps.stmtRevertTipset:         "UPDATE tipset_message SET reverted = 1 WHERE tipset_key_cid = ?",

		&ps.stmtRevertEvents: `UPDATE event SET reverted = 1 WHERE message_id IN (
			SELECT message_id FROM tipset_message WHERE tipset_key_cid = ?
		)`,

		&ps.stmtTipsetExists:   "SELECT EXISTS(SELECT 1 FROM tipset_message WHERE tipset_key_cid = ?)",
		&ps.stmtTipsetUnRevert: "UPDATE tipset_message SET reverted = 0 WHERE tipset_key_cid = ?",

		&ps.stmtInsertTipsetMessage: "INSERT INTO tipset_message (tipset_key_cid, height, reverted, message_cid, message_index) VALUES (?, ?, ?, ?, ?) ON CONFLICT (tipset_key_cid, message_cid) SET reverted = 0",

		&ps.stmtEventsUnRevert: `UPDATE event SET reverted = 0 WHERE message_id IN (
			SELECT message_id FROM tipset_message WHERE tipset_key_cid = ?
		)`,

		&ps.selectMsgIdForMsgCidAndTipset: "SELECT message_id FROM tipset_message WHERE message_cid = ? AND tipset_key_cid = ?",

		&ps.stmtInsertEvent:      "INSERT INTO event (message_id, event_index, emitter_addr, reverted) VALUES (?, ?, ?, ?)",
		&ps.stmtInsertEventEntry: "INSERT INTO event_entry (event_id, indexed, flags, key, codec, value) VALUES (?, ?, ?, ?, ?, ?)",

		&ps.stmtInsertEthTxHash: "INSERT INTO eth_tx_hash (eth_tx_hash, message_cid) VALUES (?, ?)",

		&ps.stmtRemoveRevertedTipsetsBeforeHeight: "DELETE FROM tipset_message WHERE reverted = 1 AND height < ?",
	}
}
