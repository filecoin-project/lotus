package ethhashlookup

import (
	"context"
	"database/sql"
	"errors"
	"strconv"

	"github.com/ipfs/go-cid"
	_ "github.com/mattn/go-sqlite3"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/chain/types/ethtypes"
	"github.com/filecoin-project/lotus/lib/sqlite"
)

const DefaultDbFilename = "txhash.db"

var ErrNotFound = errors.New("not found")

var ddls = []string{
	`CREATE TABLE IF NOT EXISTS eth_tx_hashes (
		hash TEXT PRIMARY KEY NOT NULL,
		cid TEXT NOT NULL UNIQUE,
		insertion_time TIMESTAMP DEFAULT CURRENT_TIMESTAMP NOT NULL
	)`,

	`CREATE INDEX IF NOT EXISTS insertion_time_index ON eth_tx_hashes (insertion_time)`,
}

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

func NewTransactionHashLookup(ctx context.Context, path string) (*EthTxHashLookup, error) {
	db, _, err := sqlite.Open(path)
	if err != nil {
		return nil, xerrors.Errorf("failed to setup eth transaction hash lookup db: %w", err)
	}

	if err := sqlite.InitDb(ctx, "eth transaction hash lookup", db, ddls, []sqlite.MigrationFunc{}); err != nil {
		_ = db.Close()
		return nil, xerrors.Errorf("failed to init eth transaction hash lookup db: %w", err)
	}

	return &EthTxHashLookup{db: db}, nil
}

func (ei *EthTxHashLookup) Close() error {
	if ei.db == nil {
		return nil
	}
	return ei.db.Close()
}
