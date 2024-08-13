package index

import "database/sql"

// TODO PRAGMA foreign_keys = ON;
// TODO: Allow for msg_cid <> eth_tx_hash for unconfirmed mpool messages
var ddls = []string{
	// Question: Is tipset_key_cid guaranteed to be unique? What if it's reverted at height h but added back at height h+x?
	// Ideally, even in that case, tipset_key_cid will be different as parent blocks will be different but confirm this.
	`CREATE TABLE IF NOT EXISTS tipsets (
        tipset_key_cid BLOB PRIMARY KEY,
        height INTEGER NOT NULL,
		reverted INTEGER NOT NULL
    )`,

	// Question: Is it better to merge tipset_messages and tipset table into one table?
	// Pros of merging the tables:
	// 	1. No need to join to lookup non-reverted messages for message<>tipset lookups
	// 	2. Redices a join for looking up events (as removes the tipset -> messages indirection)
	// Cons of merging the tables:
	// 	1. Duplication of "height" and "reverted" for EACH message
	// 	2. Insertion and deletion is slightly more complex (and so is marking tipsets as reverted)
	// 	3. Current schema maps nicely to the (tipset has messages -> messages have events -> events have event entries)
	//     relationship
	// Maybe decide after profiling queries and building indexes
	`CREATE TABLE IF NOT EXISTS tipset_messages (
		message_id INTEGER PRIMARY KEY,
        tipset_key_cid BLOB NOT NULL,
        message_cid BLOB NOT NULL,
        message_index INTEGER NOT NULL,
        eth_tx_hash TEXT,
		FOREIGN KEY (tipset_key_cid) REFERENCES tipsets(tipset_key_cid) ON DELETE CASCADE,
		UNIQUE (tipset_key_cid, message_cid)
	)`,

	`CREATE TABLE IF NOT EXISTS events (
		event_id INTEGER PRIMARY KEY,
		message_id INTEGER NOT NULL,
        event_index INTEGER NOT NULL,
        emitter_addr BLOB NOT NULL,
		FOREIGN KEY (message_id) REFERENCES tipset_messages(message_id) ON DELETE CASCADE
    )`,

	`CREATE TABLE IF NOT EXISTS event_entry (
		event_id INTEGER NOT NULL,
		indexed INTEGER NOT NULL,
		flags BLOB NOT NULL,
		key TEXT NOT NULL,
		codec INTEGER,
		value BLOB NOT NULL,
		FOREIGN KEY (event_id) REFERENCES events(event_id) ON DELETE CASCADE
	)`,

	// TODO: Build Indexes based on query patterns and `explain query` profiling
}

type preparedStatements struct {
	insertTipset                      *sql.Stmt
	removeRevertedTipsetsBeforeHeight *sql.Stmt

	insertTipsetMsg *sql.Stmt

	insertEvent      *sql.Stmt
	insertEventEntry *sql.Stmt

	tipsetExists *sql.Stmt

	revertTipset *sql.Stmt

	restoreTipset *sql.Stmt

	selectNonRevertedMessage *sql.Stmt
	selectEthTxHash          *sql.Stmt
}

func preparedStatementMapping(ps *preparedStatements) map[**sql.Stmt]string {
	return map[**sql.Stmt]string{
		// Tipsets
		&ps.insertTipset: `INSERT INTO tipsets (tipset_key_cid, height, reverted) VALUES (?, ?, ?) ON CONFLICT(tipset_key_cid) DO UPDATE SET reverted=false`,

		&ps.removeRevertedTipsetsBeforeHeight: `DELETE FROM tipsets WHERE reverted=true AND height < ?`,

		&ps.tipsetExists: `SELECT EXISTS(SELECT 1 FROM tipsets WHERE tipset_key_cid = ?)`,

		&ps.restoreTipset: `UPDATE tipsets SET reverted=false WHERE tipset_key_cid = ?`,

		&ps.revertTipset: `UPDATE tipsets SET reverted=true WHERE tipset_key_cid = ?`,

		// Tipset Messages
		&ps.insertTipsetMsg: `INSERT INTO tipset_messages (tipset_key_cid, message_cid, message_index, eth_tx_hash) VALUES (?, ?, ?, ?)`,

		&ps.selectNonRevertedMessage: `SELECT tm.tipset_key_cid, t.height
			FROM tipset_messages tm
			JOIN tipsets t ON tm.tipset_key_cid = t.tipset_key_cid
			WHERE tm.message_cid = ? AND t.reverted = 0`,

		&ps.selectEthTxHash: `SELECT eth_tx_hash FROM tipset_messages WHERE message_cid = ?`,

		// Events
		&ps.insertEvent:      `INSERT INTO events (message_id, event_index, emitter_addr) VALUES (?, ?, ?)`,
		&ps.insertEventEntry: `INSERT INTO event_entry (event_id, indexed, flags, key, codec, value) VALUES (?, ?, ?, ?, ?, ?)`,
	}
}
