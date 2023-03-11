package index

import (
	"context"
	"database/sql"
	"errors"
	"io/fs"
	"os"
	"path"
	"time"

	"github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log/v2"
	_ "github.com/mattn/go-sqlite3"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
)

type msgIndex struct {
	cs *store.ChainStore

	db *sql.DB
}

var _ MsgIndex = (*msgIndex)(nil)

var log = logging.Logger("chain/index")

var (
	dbName = "msgindex.db"

	coalesceMinDelay      = 100 * time.Millisecond
	coalesceMaxDelay      = time.Second
	coalesceMergeInterval = 100 * time.Millisecond
)

func NewMsgIndex(basePath string, cs *store.ChainStore) (MsgIndex, error) {
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
	// TODO
	return errors.New("TODO: index.createTables")
}

func reconcileIndex(db *sql.DB, cs *store.ChainStore) error {
	// TODO
	return errors.New("TODO: index.reconcileIndex")
}

func (x *msgIndex) prepareStatements() error {
	// TODO
	return errors.New("TODO: msgIndex.prepareStatements")
}

// head change notifee
func (x *msgIndex) onHeadChange(rev, app []*types.TipSet) error {
	// TODO
	return errors.New("TODO: msgIndex.onHeadChange")
}

// interface
func (x *msgIndex) GetMsgInfo(ctx context.Context, m cid.Cid) (MsgInfo, error) {
	// TODO
	return MsgInfo{}, errors.New("TODO: msgIndex.GetMsgInfo")
}

func (x *msgIndex) Close() error {
	// TODO
	return errors.New("TODO: msgIndex.Close")
}
