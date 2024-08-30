package chainindex

const DefaultDbFilename = "chainindex.db"

const (
	stmtGetNonRevertedMessageInfo = "SELECT tipset_key_cid, height FROM tipset_message WHERE message_cid = ? AND reverted = 0"
	stmtGetMsgCidFromEthHash      = "SELECT message_cid FROM eth_tx_hash WHERE tx_hash = ?"
	stmtInsertEthTxHash           = "INSERT INTO eth_tx_hash (tx_hash, message_cid) VALUES (?, ?) ON CONFLICT (tx_hash) DO UPDATE SET inserted_at = CURRENT_TIMESTAMP"

	stmtInsertTipsetMessage = "INSERT INTO tipset_message (tipset_key_cid, height, reverted, message_cid, message_index) VALUES (?, ?, ?, ?, ?) ON CONFLICT (tipset_key_cid, message_cid) DO UPDATE SET reverted = 0"

	stmtTipsetExists   = "SELECT EXISTS(SELECT 1 FROM tipset_message WHERE tipset_key_cid = ?)"
	stmtTipsetUnRevert = "UPDATE tipset_message SET reverted = 0 WHERE tipset_key_cid = ?"

	stmtRevertTipset = "UPDATE tipset_message SET reverted = 1 WHERE tipset_key_cid = ?"

	stmtGetMaxNonRevertedTipset = "SELECT tipset_key_cid FROM tipset_message WHERE reverted = 0 ORDER BY height DESC LIMIT 1"

	stmtRemoveRevertedTipsetsBeforeHeight = "DELETE FROM tipset_message WHERE height < ? AND reverted = 1"
	stmtRemoveTipsetsBeforeHeight         = "DELETE FROM tipset_message WHERE height < ?"

	stmtDeleteEthHashesOlderThan = `DELETE FROM eth_tx_hash WHERE inserted_at < datetime('now', ?);`
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

	`CREATE INDEX IF NOT EXISTS insertion_time_index ON eth_tx_hash (inserted_at)`,

	`CREATE INDEX IF NOT EXISTS idx_message_cid ON tipset_message (message_cid)`,

	`CREATE INDEX IF NOT EXISTS idx_tipset_key_cid ON tipset_message (tipset_key_cid)`,
}
