package index

import (
	"context"
	"database/sql"
	"errors"
	"io/fs"
	"os"
	"path"
	"time"

	logging "github.com/ipfs/go-log/v2"
	_ "github.com/mattn/go-sqlite3"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/ipfs/go-cid"
)

// chain store interface; we could use store.ChainStore directly,
// but this simplifies unit testing.
type ChainStore interface {
	SubscribeHeadChanges(f store.ReorgNotifee)
}

var _ ChainStore = (*store.ChainStore)(nil)

type msgIndex struct {
	cs ChainStore

	db               *sql.DB
	selectMsgStmt    *sql.Stmt
	deleteTipSetStmt *sql.Stmt
}

var _ MsgIndex = (*msgIndex)(nil)

var log = logging.Logger("chain/index")

var (
	dbName = "msgindex.db"

	coalesceMinDelay      = 100 * time.Millisecond
	coalesceMaxDelay      = time.Second
	coalesceMergeInterval = 100 * time.Millisecond
)

func NewMsgIndex(basePath string, cs ChainStore) (MsgIndex, error) {
	var (
		mkdb   bool
		dbPath string
		err    error
	)

	if basePath == ":memory:" {
		// for testing
		mkdb = true
		dbPath = basePath
		goto opendb
	}

	err = os.MkdirAll(basePath, 0755)
	if err != nil {
		return nil, xerrors.Errorf("error creating msgindex base directory: %w", err)
	}

	dbPath = path.Join(basePath, dbName)
	_, err = os.Stat(dbPath)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		mkdb = true

	case err != nil:
		return nil, xerrors.Errorf("error stating msgindex database: %w", err)
	}

opendb:
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		// TODO [nice to have]: automaticaly delete corrupt databases
		//      but for now we can just error and let the operator delete.
		return nil, xerrors.Errorf("error opening msgindex database: %w", err)
	}

	if mkdb {
		err = createTables(db)
		if err != nil {
			return nil, xerrors.Errorf("error creating msgindex database: %w", err)
		}
	} else {
		err = reconcileIndex(db, cs)
		if err != nil {
			return nil, xerrors.Errorf("error reconciling msgindex database: %w", err)
		}
	}

	msgIndex := &msgIndex{db: db, cs: cs}
	err = msgIndex.prepareStatements()
	if err != nil {
		err2 := db.Close()
		if err2 != nil {
			log.Errorf("error closing msgindex database: %s", err2)
		}

		return nil, xerrors.Errorf("error preparing msgindex database statements: %w", err)
	}

	rnf := store.WrapHeadChangeCoalescer(
		msgIndex.onHeadChange,
		coalesceMinDelay,
		coalesceMaxDelay,
		coalesceMergeInterval,
	)
	cs.SubscribeHeadChanges(rnf)

	return msgIndex, nil
}

// init utilities
func createTables(db *sql.DB) error {
	// Just a single table for now; ghetto, but this an index so we denormalize to avoid joins.
	if _, err := db.Exec("CREATE TABLE Messages (cid VARCHAR(80) PRIMARY KEY, tipset VARCHAR(80), xepoch INTEGER, xindex INTEGER)"); err != nil {
		return err
	}

	// TODO Should we add an index for tipset to speed up deletion on revert?
	return nil
}

func reconcileIndex(db *sql.DB, cs ChainStore) error {
	// TODO
	return errors.New("TODO: index.reconcileIndex")
}

func (x *msgIndex) prepareStatements() error {
	stmt, err := x.db.Prepare("SELECT (tipset, xepoch, xindex) FROM Messages WHERE cid = ?")
	if err != nil {
		return err
	}
	x.selectMsgStmt = stmt

	stmt, err = x.db.Prepare("DELETE FROM Messages WHERE tipset = ?")
	if err != nil {
		return err
	}
	x.deleteTipSetStmt = stmt

	// TODO reconciliation stmts
	return nil
}

// head change notifee
func (x *msgIndex) onHeadChange(rev, app []*types.TipSet) error {
	// TODO
	return errors.New("TODO: msgIndex.onHeadChange")
}

// interface
func (x *msgIndex) GetMsgInfo(ctx context.Context, m cid.Cid) (MsgInfo, error) {
	var (
		tipset string
		epoch  int64
		index  int64
	)

	key := m.String()
	row := x.selectMsgStmt.QueryRow(key)
	err := row.Scan(&tipset, &epoch, &index)
	switch {
	case err == sql.ErrNoRows:
		return MsgInfo{}, ErrNotFound

	case err != nil:
		return MsgInfo{}, xerrors.Errorf("error querying msgindex database: %w", err)
	}

	tipsetCid, err := cid.Decode(tipset)
	if err != nil {
		return MsgInfo{}, xerrors.Errorf("error decoding tipset cid: %w", err)
	}

	return MsgInfo{
		Message: m,
		Tipset:  tipsetCid,
		Epoch:   abi.ChainEpoch(epoch),
		Index:   int(index),
	}, nil
}

func (x *msgIndex) Close() error {
	// TODO
	return errors.New("TODO: msgIndex.Close")
}
