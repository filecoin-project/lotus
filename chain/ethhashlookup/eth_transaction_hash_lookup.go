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
	insertTxHash    = `INSERT INTO eth_tx_hashes (hash, cid) VALUES(?, ?) ON CONFLICT (hash) DO UPDATE SET insertion_time = CURRENT_TIMESTAMP`
	getCidFromHash  = `SELECT cid FROM eth_tx_hashes WHERE hash = ?`
	getHashFromCid  = `SELECT hash FROM eth_tx_hashes WHERE cid = ?`
	deleteOlderThan = `DELETE FROM eth_tx_hashes WHERE insertion_time < datetime('now', ?);`
)

type EthTxHashLookup struct {
	db *sql.DB

	stmtInsertTxHash    *sql.Stmt
	stmtGetCidFromHash  *sql.Stmt
	stmtGetHashFromCid  *sql.Stmt
	stmtDeleteOlderThan *sql.Stmt
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

	ei := &EthTxHashLookup{db: db}

	if err = ei.initStatements(); err != nil {
		_ = ei.Close()
		return nil, xerrors.Errorf("error preparing eth transaction hash lookup db statements: %w", err)
	}

	return ei, nil
}

func (ei *EthTxHashLookup) initStatements() (err error) {
	ei.stmtInsertTxHash, err = ei.db.Prepare(insertTxHash)
	if err != nil {
		return xerrors.Errorf("prepare stmtInsertTxHash: %w", err)
	}
	ei.stmtGetCidFromHash, err = ei.db.Prepare(getCidFromHash)
	if err != nil {
		return xerrors.Errorf("prepare stmtGetCidFromHash: %w", err)
	}
	ei.stmtGetHashFromCid, err = ei.db.Prepare(getHashFromCid)
	if err != nil {
		return xerrors.Errorf("prepare stmtGetHashFromCid: %w", err)
	}
	ei.stmtDeleteOlderThan, err = ei.db.Prepare(deleteOlderThan)
	if err != nil {
		return xerrors.Errorf("prepare stmtDeleteOlderThan: %w", err)
	}
	return nil
}

func (ei *EthTxHashLookup) UpsertHash(txHash ethtypes.EthHash, c cid.Cid) error {
	if ei.db == nil {
		return xerrors.New("db closed")
	}

	_, err := ei.stmtInsertTxHash.Exec(txHash.String(), c.String())
	return err
}

func (ei *EthTxHashLookup) GetCidFromHash(txHash ethtypes.EthHash) (cid.Cid, error) {
	if ei.db == nil {
		return cid.Undef, xerrors.New("db closed")
	}

	row := ei.stmtGetCidFromHash.QueryRow(txHash.String())
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
	if ei.db == nil {
		return ethtypes.EmptyEthHash, xerrors.New("db closed")
	}

	row := ei.stmtGetHashFromCid.QueryRow(c.String())
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
	if ei.db == nil {
		return 0, xerrors.New("db closed")
	}

	res, err := ei.stmtDeleteOlderThan.Exec("-" + strconv.Itoa(days) + " day")
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (ei *EthTxHashLookup) Close() (err error) {
	if ei.db == nil {
		return nil
	}
	db := ei.db
	ei.db = nil
	return db.Close()
}
