package ethhashlookup

import (
	"database/sql"
	"errors"
	"strconv"

	"github.com/ipfs/go-cid"
	_ "github.com/mattn/go-sqlite3"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/chain/types/ethtypes"
)

var ErrNotFound = errors.New("not found")

var pragmas = []string{
	"PRAGMA synchronous = normal",
	"PRAGMA temp_store = memory",
	"PRAGMA mmap_size = 30000000000",
	"PRAGMA page_size = 32768",
	"PRAGMA auto_vacuum = NONE",
	"PRAGMA automatic_index = OFF",
	"PRAGMA journal_mode = WAL",
	"PRAGMA read_uncommitted = ON",
}

var ddls = []string{
	`CREATE TABLE IF NOT EXISTS eth_tx_hashes (
		hash TEXT PRIMARY KEY NOT NULL,
		cid TEXT NOT NULL UNIQUE,
		insertion_time TIMESTAMP DEFAULT CURRENT_TIMESTAMP NOT NULL
	)`,

	`CREATE INDEX IF NOT EXISTS insertion_time_index ON eth_tx_hashes (insertion_time)`,

	// metadata containing version of schema
	`CREATE TABLE IF NOT EXISTS _meta (
    	version UINT64 NOT NULL UNIQUE
	)`,

	// version 1.
	`INSERT OR IGNORE INTO _meta (version) VALUES (1)`,
}

const schemaVersion = 1

const (
	insertTxHash = `INSERT INTO eth_tx_hashes
	(hash, cid)
	VALUES(?, ?)
	ON CONFLICT (hash) DO UPDATE SET insertion_time = CURRENT_TIMESTAMP`
)

type EthTxHashLookup struct {
	db *sql.DB
}

func (ei *EthTxHashLookup) UpsertHash(txHash ethtypes.EthHash, c cid.Cid) error {
	hashEntry, err := ei.db.Prepare(insertTxHash)
	if err != nil {
		return xerrors.Errorf("prepare insert event: %w", err)
	}

	_, err = hashEntry.Exec(txHash.String(), c.String())
	return err
}

func (ei *EthTxHashLookup) GetCidFromHash(txHash ethtypes.EthHash) (cid.Cid, error) {
	row := ei.db.QueryRow("SELECT cid FROM eth_tx_hashes WHERE hash = :hash;", sql.Named("hash", txHash.String()))

	var c string
	err := row.Scan(&c)
	if err != nil {
		if err == sql.ErrNoRows {
			return cid.Undef, ErrNotFound
		}
		return cid.Undef, err
	}
	return cid.Decode(c)
}

func (ei *EthTxHashLookup) GetHashFromCid(c cid.Cid) (ethtypes.EthHash, error) {
	row := ei.db.QueryRow("SELECT hash FROM eth_tx_hashes WHERE cid = :cid;", sql.Named("cid", c.String()))

	var hashString string
	err := row.Scan(&c)
	if err != nil {
		if err == sql.ErrNoRows {
			return ethtypes.EmptyEthHash, ErrNotFound
		}
		return ethtypes.EmptyEthHash, err
	}
	return ethtypes.ParseEthHash(hashString)
}

func (ei *EthTxHashLookup) DeleteEntriesOlderThan(days int) (int64, error) {
	res, err := ei.db.Exec("DELETE FROM eth_tx_hashes WHERE insertion_time < datetime('now', ?);", "-"+strconv.Itoa(days)+" day")
	if err != nil {
		return 0, err
	}

	return res.RowsAffected()
}

func NewTransactionHashLookup(path string) (*EthTxHashLookup, error) {
	db, err := sql.Open("sqlite3", path+"?mode=rwc")
	if err != nil {
		return nil, xerrors.Errorf("open sqlite3 database: %w", err)
	}

	for _, pragma := range pragmas {
		if _, err := db.Exec(pragma); err != nil {
			_ = db.Close()
			return nil, xerrors.Errorf("exec pragma %q: %w", pragma, err)
		}
	}

	q, err := db.Query("SELECT name FROM sqlite_master WHERE type='table' AND name='_meta';")
	if err == sql.ErrNoRows || !q.Next() {
		// empty database, create the schema
		for _, ddl := range ddls {
			if _, err := db.Exec(ddl); err != nil {
				_ = db.Close()
				return nil, xerrors.Errorf("exec ddl %q: %w", ddl, err)
			}
		}
	} else if err != nil {
		_ = db.Close()
		return nil, xerrors.Errorf("looking for _meta table: %w", err)
	} else {
		// Ensure we don't open a database from a different schema version

		row := db.QueryRow("SELECT max(version) FROM _meta")
		var version int
		err := row.Scan(&version)
		if err != nil {
			_ = db.Close()
			return nil, xerrors.Errorf("invalid database version: no version found")
		}
		if version != schemaVersion {
			_ = db.Close()
			return nil, xerrors.Errorf("invalid database version: got %d, expected %d", version, schemaVersion)
		}
	}

	return &EthTxHashLookup{
		db: db,
	}, nil
}

func (ei *EthTxHashLookup) Close() error {
	if ei.db == nil {
		return nil
	}
	return ei.db.Close()
}
