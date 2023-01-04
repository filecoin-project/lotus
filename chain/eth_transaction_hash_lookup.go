package chain

import (
	"database/sql"
	"math"

	"github.com/ipfs/go-cid"
	_ "github.com/mattn/go-sqlite3"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/chain/types/ethtypes"
)

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
	`CREATE TABLE IF NOT EXISTS tx_hash_lookup (
		hash TEXT PRIMARY KEY,
		cid TEXT NOT NULL,
		epoch INT NOT NULL
	)`,

	// metadata containing version of schema
	`CREATE TABLE IF NOT EXISTS _meta (
    	version UINT64 NOT NULL UNIQUE
	)`,

	// version 1.
	`INSERT OR IGNORE INTO _meta (version) VALUES (1)`,
}

const schemaVersion = 1
const MemPoolEpoch = math.MaxInt64

const (
	insertTxHash = `INSERT INTO tx_hash_lookup
	(hash, cid, epoch)
	VALUES(?, ?, ?)
	ON CONFLICT (hash) DO UPDATE SET epoch = EXCLUDED.epoch
	WHERE epoch > EXCLUDED.epoch`
)

type TransactionHashLookup struct {
	db *sql.DB
}

func (ei *TransactionHashLookup) InsertTxHash(txHash ethtypes.EthHash, c cid.Cid, epoch int64) error {
	hashEntry, err := ei.db.Prepare(insertTxHash)
	if err != nil {
		return xerrors.Errorf("prepare insert event: %w", err)
	}

	_, err = hashEntry.Exec(txHash.String(), c.String(), epoch)
	return err
}

func (ei *TransactionHashLookup) LookupCidFromTxHash(txHash ethtypes.EthHash) (cid.Cid, error) {
	q, err := ei.db.Query("SELECT cid FROM tx_hash_lookup WHERE hash = :hash;", sql.Named("hash", txHash.String()))
	if err != nil {
		return cid.Undef, err
	}

	var c string
	if !q.Next() {
		return cid.Undef, xerrors.Errorf("transaction hash %s not found", txHash.String())
	}
	err = q.Scan(&c)
	if err != nil {
		return cid.Undef, err
	}
	return cid.Decode(c)
}

func (ei *TransactionHashLookup) LookupTxHashFromCid(c cid.Cid) (ethtypes.EthHash, error) {
	q, err := ei.db.Query("SELECT hash FROM tx_hash_lookup WHERE cid = :cid;", sql.Named("cid", c.String()))
	if err != nil {
		return ethtypes.EmptyEthHash, err
	}

	var hashString string
	if !q.Next() {
		return ethtypes.EmptyEthHash, xerrors.Errorf("transaction hash %s not found", c.String())
	}
	err = q.Scan(&hashString)
	if err != nil {
		return ethtypes.EmptyEthHash, err
	}
	return ethtypes.ParseEthHash(hashString)
}

func (ei *TransactionHashLookup) RemoveEntriesOlderThan(epoch abi.ChainEpoch) (int64, error) {
	res, err := ei.db.Exec("DELETE FROM tx_hash_lookup WHERE epoch < :epoch;", sql.Named("epoch", epoch))
	if err != nil {
		return 0, err
	}

	return res.RowsAffected()
}

func NewTransactionHashLookup(path string) (*TransactionHashLookup, error) {
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

	return &TransactionHashLookup{
		db: db,
	}, nil
}

func (ei *TransactionHashLookup) Close() error {
	if ei.db == nil {
		return nil
	}
	return ei.db.Close()
}
