package index

import (
	"context"
	"database/sql"
	"errors"
	"io/fs"
	"os"
	"path"
	"sync"
	"time"

	"github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log/v2"
	_ "github.com/mattn/go-sqlite3"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
)

var log = logging.Logger("msgindex")

var dbName = "msgindex.db"
var dbDefs = []string{
	`CREATE TABLE IF NOT EXISTS messages (
     cid VARCHAR(80) PRIMARY KEY ON CONFLICT REPLACE,
     tipset_cid VARCHAR(80) NOT NULL,
     epoch INTEGER NOT NULL
   )`,
	`CREATE INDEX IF NOT EXISTS tipset_cids ON messages (tipset_cid)
  `,
	`CREATE TABLE IF NOT EXISTS _meta (
    	version UINT64 NOT NULL UNIQUE
	)`,
	`INSERT OR IGNORE INTO _meta (version) VALUES (1)`,
}

var dbPragmas = []string{
	"PRAGMA synchronous = normal",
	"PRAGMA temp_store = memory",
	"PRAGMA mmap_size = 30000000000",
	"PRAGMA page_size = 32768",
	"PRAGMA auto_vacuum = NONE",
	"PRAGMA automatic_index = OFF",
	"PRAGMA journal_mode = WAL",
	"PRAGMA read_uncommitted = ON",
}

const (
	// prepared stmts
	dbqGetMessageInfo       = "SELECT tipset_cid, epoch FROM messages WHERE cid = ?"
	dbqInsertMessage        = "INSERT INTO messages VALUES (?, ?, ?)"
	dbqDeleteTipsetMessages = "DELETE FROM messages WHERE tipset_cid = ?"
	// reconciliation
	dbqCountMessages         = "SELECT COUNT(*) FROM messages"
	dbqMinEpoch              = "SELECT MIN(epoch) FROM messages"
	dbqCountTipsetMessages   = "SELECT COUNT(*) FROM messages WHERE tipset_cid = ?"
	dbqDeleteMessagesByEpoch = "DELETE FROM messages WHERE epoch >= ?"
)

// coalescer configuration (TODO: use observer instead)
// these are exposed to make tests snappy
var (
	CoalesceMinDelay      = time.Second
	CoalesceMaxDelay      = 15 * time.Second
	CoalesceMergeInterval = time.Second
)

// chain store interface; we could use store.ChainStore directly,
// but this simplifies unit testing.
type ChainStore interface {
	SubscribeHeadChanges(f store.ReorgNotifee)
	MessagesForTipset(ctx context.Context, ts *types.TipSet) ([]types.ChainMsg, error)
	GetHeaviestTipSet() *types.TipSet
	GetTipSetFromKey(ctx context.Context, tsk types.TipSetKey) (*types.TipSet, error)
}

var _ ChainStore = (*store.ChainStore)(nil)

type msgIndex struct {
	cs ChainStore

	db               *sql.DB
	selectMsgStmt    *sql.Stmt
	insertMsgStmt    *sql.Stmt
	deleteTipSetStmt *sql.Stmt

	sema chan struct{}
	mx   sync.Mutex
	pend []headChange

	cancel  func()
	workers sync.WaitGroup
	closeLk sync.RWMutex
	closed  bool
}

var _ MsgIndex = (*msgIndex)(nil)

type headChange struct {
	rev []*types.TipSet
	app []*types.TipSet
}

func NewMsgIndex(lctx context.Context, basePath string, cs ChainStore) (MsgIndex, error) {
	var (
		dbPath string
		exists bool
		err    error
	)

	err = os.MkdirAll(basePath, 0755)
	if err != nil {
		return nil, xerrors.Errorf("error creating msgindex base directory: %w", err)
	}

	dbPath = path.Join(basePath, dbName)
	_, err = os.Stat(dbPath)
	switch {
	case err == nil:
		exists = true

	case errors.Is(err, fs.ErrNotExist):

	case err != nil:
		return nil, xerrors.Errorf("error stating msgindex database: %w", err)
	}

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		// TODO [nice to have]: automaticaly delete corrupt databases
		//      but for now we can just error and let the operator delete.
		return nil, xerrors.Errorf("error opening msgindex database: %w", err)
	}

	if err := prepareDB(db); err != nil {
		return nil, xerrors.Errorf("error creating msgindex database: %w", err)
	}

	// TODO we may consider populating the index when first creating the db
	if exists {
		if err := reconcileIndex(db, cs); err != nil {
			return nil, xerrors.Errorf("error reconciling msgindex database: %w", err)
		}
	}

	ctx, cancel := context.WithCancel(lctx)

	msgIndex := &msgIndex{
		db:     db,
		cs:     cs,
		sema:   make(chan struct{}, 1),
		cancel: cancel,
	}

	err = msgIndex.prepareStatements()
	if err != nil {
		if err := db.Close(); err != nil {
			log.Errorf("error closing msgindex database: %s", err)
		}

		return nil, xerrors.Errorf("error preparing msgindex database statements: %w", err)
	}

	rnf := store.WrapHeadChangeCoalescer(
		msgIndex.onHeadChange,
		CoalesceMinDelay,
		CoalesceMaxDelay,
		CoalesceMergeInterval,
	)
	cs.SubscribeHeadChanges(rnf)

	msgIndex.workers.Add(1)
	go msgIndex.background(ctx)

	return msgIndex, nil
}

func PopulateAfterSnapshot(lctx context.Context, basePath string, cs ChainStore) error {
	err := os.MkdirAll(basePath, 0755)
	if err != nil {
		return xerrors.Errorf("error creating msgindex base directory: %w", err)
	}

	dbPath := path.Join(basePath, dbName)

	// if a database already exists, we try to delete it and create a new one
	if _, err := os.Stat(dbPath); err == nil {
		if err = os.Remove(dbPath); err != nil {
			return xerrors.Errorf("msgindex already exists at %s and can't be deleted", dbPath)
		}
	}

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return xerrors.Errorf("error opening msgindex database: %w", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			log.Errorf("error closing msgindex database: %s", err)
		}
	}()

	if err := prepareDB(db); err != nil {
		return xerrors.Errorf("error creating msgindex database: %w", err)
	}

	tx, err := db.Begin()
	if err != nil {
		return xerrors.Errorf("error when starting transaction: %w", err)
	}

	rollback := func() {
		if err := tx.Rollback(); err != nil {
			log.Errorf("error in rollback: %s", err)
		}
	}

	insertStmt, err := tx.Prepare(dbqInsertMessage)
	if err != nil {
		rollback()
		return xerrors.Errorf("error preparing insertStmt: %w", err)
	}

	curTs := cs.GetHeaviestTipSet()
	startHeight := curTs.Height()
	for curTs != nil {
		tscid, err := curTs.Key().Cid()
		if err != nil {
			rollback()
			return xerrors.Errorf("error computing tipset cid: %w", err)
		}

		tskey := tscid.String()
		epoch := int64(curTs.Height())

		msgs, err := cs.MessagesForTipset(lctx, curTs)
		if err != nil {
			log.Infof("stopping import after %d tipsets", startHeight-curTs.Height())
			break
		}

		for _, msg := range msgs {
			key := msg.Cid().String()
			if _, err := insertStmt.Exec(key, tskey, epoch); err != nil {
				rollback()
				return xerrors.Errorf("error inserting message: %w", err)
			}
		}

		curTs, err = cs.GetTipSetFromKey(lctx, curTs.Parents())
		if err != nil {
			rollback()
			return xerrors.Errorf("error walking chain: %w", err)
		}
	}

	err = tx.Commit()
	if err != nil {
		return xerrors.Errorf("error committing transaction: %w", err)
	}

	return nil
}

// init utilities
func prepareDB(db *sql.DB) error {
	for _, stmt := range dbDefs {
		if _, err := db.Exec(stmt); err != nil {
			return xerrors.Errorf("error executing sql statement '%s': %w", stmt, err)
		}
	}

	for _, stmt := range dbPragmas {
		if _, err := db.Exec(stmt); err != nil {
			return xerrors.Errorf("error executing sql statement '%s': %w", stmt, err)
		}
	}

	return nil
}

func reconcileIndex(db *sql.DB, cs ChainStore) error {
	// Invariant: after reconciliation, every tipset in the index is in the current chain; ie either
	//  the chain head or reachable by walking the chain.
	// Algorithm:
	//  1. Count messages in index; if none, trivially reconciled.
	//     TODO we may consider populating the index in that case
	//  2. Find the minimum tipset in the index; this will mark the end of the reconciliation walk
	//  3. Walk from current tipset until we find a tipset in the index.
	//  4. Delete (revert!) all tipsets above the found tipset.
	//  5. If the walk ends in the boundary epoch, then delete everything.
	//

	row := db.QueryRow(dbqCountMessages)

	var result int64
	if err := row.Scan(&result); err != nil {
		return xerrors.Errorf("error counting messages: %w", err)
	}

	if result == 0 {
		return nil
	}

	row = db.QueryRow(dbqMinEpoch)
	if err := row.Scan(&result); err != nil {
		return xerrors.Errorf("error finding boundary epoch: %w", err)
	}

	boundaryEpoch := abi.ChainEpoch(result)

	countMsgsStmt, err := db.Prepare(dbqCountTipsetMessages)
	if err != nil {
		return xerrors.Errorf("error preparing statement: %w", err)
	}

	curTs := cs.GetHeaviestTipSet()
	for curTs != nil && curTs.Height() >= boundaryEpoch {
		tsCid, err := curTs.Key().Cid()
		if err != nil {
			return xerrors.Errorf("error computing tipset cid: %w", err)
		}

		key := tsCid.String()
		row = countMsgsStmt.QueryRow(key)
		if err := row.Scan(&result); err != nil {
			return xerrors.Errorf("error counting messages: %w", err)
		}

		if result > 0 {
			// found it!
			boundaryEpoch = curTs.Height() + 1
			break
		}

		// walk up
		parents := curTs.Parents()
		curTs, err = cs.GetTipSetFromKey(context.TODO(), parents)
		if err != nil {
			return xerrors.Errorf("error walking chain: %w", err)
		}
	}

	// delete everything above the minEpoch
	if _, err = db.Exec(dbqDeleteMessagesByEpoch, int64(boundaryEpoch)); err != nil {
		return xerrors.Errorf("error deleting stale reorged out message: %w", err)
	}

	return nil
}

func (x *msgIndex) prepareStatements() error {
	stmt, err := x.db.Prepare(dbqGetMessageInfo)
	if err != nil {
		return xerrors.Errorf("prepare selectMsgStmt: %w", err)
	}
	x.selectMsgStmt = stmt

	stmt, err = x.db.Prepare(dbqInsertMessage)
	if err != nil {
		return xerrors.Errorf("prepare insertMsgStmt: %w", err)
	}
	x.insertMsgStmt = stmt

	stmt, err = x.db.Prepare(dbqDeleteTipsetMessages)
	if err != nil {
		return xerrors.Errorf("prepare deleteTipSetStmt: %w", err)
	}
	x.deleteTipSetStmt = stmt

	return nil
}

// head change notifee
func (x *msgIndex) onHeadChange(rev, app []*types.TipSet) error {
	x.closeLk.RLock()
	defer x.closeLk.RUnlock()

	if x.closed {
		return nil
	}

	// do it in the background to avoid blocking head change processing
	x.mx.Lock()
	x.pend = append(x.pend, headChange{rev: rev, app: app})
	pendLen := len(x.pend)
	x.mx.Unlock()

	// complain loudly if this is building backlog
	if pendLen > 10 {
		log.Warnf("message index head change processing is building backlog: %d pending head changes", pendLen)
	}

	select {
	case x.sema <- struct{}{}:
	default:
	}

	return nil
}

func (x *msgIndex) background(ctx context.Context) {
	defer x.workers.Done()

	for {
		select {
		case <-x.sema:
			err := x.processHeadChanges(ctx)
			if err != nil {
				// we can't rely on an inconsistent index, so shut it down.
				log.Errorf("error processing head change notifications: %s; shutting down message index", err)
				if err2 := x.Close(); err2 != nil {
					log.Errorf("error shutting down index: %s", err2)
				}
			}

		case <-ctx.Done():
			return
		}
	}
}

func (x *msgIndex) processHeadChanges(ctx context.Context) error {
	x.mx.Lock()
	pend := x.pend
	x.pend = nil
	x.mx.Unlock()

	tx, err := x.db.Begin()
	if err != nil {
		return xerrors.Errorf("error creating transaction: %w", err)
	}

	for _, hc := range pend {
		for _, ts := range hc.rev {
			if err := x.doRevert(ctx, tx, ts); err != nil {
				if err2 := tx.Rollback(); err2 != nil {
					log.Errorf("error rolling back transaction: %s", err2)
				}
				return xerrors.Errorf("error reverting %s: %w", ts, err)
			}
		}

		for _, ts := range hc.app {
			if err := x.doApply(ctx, tx, ts); err != nil {
				if err2 := tx.Rollback(); err2 != nil {
					log.Errorf("error rolling back transaction: %s", err2)
				}
				return xerrors.Errorf("error applying %s: %w", ts, err)
			}
		}
	}

	return tx.Commit()
}

func (x *msgIndex) doRevert(ctx context.Context, tx *sql.Tx, ts *types.TipSet) error {
	tskey, err := ts.Key().Cid()
	if err != nil {
		return xerrors.Errorf("error computing tipset cid: %w", err)
	}

	key := tskey.String()
	_, err = tx.Stmt(x.deleteTipSetStmt).Exec(key)
	return err
}

func (x *msgIndex) doApply(ctx context.Context, tx *sql.Tx, ts *types.TipSet) error {
	tscid, err := ts.Key().Cid()
	if err != nil {
		return xerrors.Errorf("error computing tipset cid: %w", err)
	}

	tskey := tscid.String()
	epoch := int64(ts.Height())

	msgs, err := x.cs.MessagesForTipset(ctx, ts)
	if err != nil {
		return xerrors.Errorf("error retrieving messages for tipset %s: %w", ts, err)
	}

	insertStmt := tx.Stmt(x.insertMsgStmt)
	for _, msg := range msgs {
		key := msg.Cid().String()
		if _, err := insertStmt.Exec(key, tskey, epoch); err != nil {
			return xerrors.Errorf("error inserting message: %w", err)
		}
	}

	return nil
}

// interface
func (x *msgIndex) GetMsgInfo(ctx context.Context, m cid.Cid) (MsgInfo, error) {
	x.closeLk.RLock()
	defer x.closeLk.RUnlock()

	if x.closed {
		return MsgInfo{}, ErrClosed
	}

	var (
		tipset string
		epoch  int64
	)

	key := m.String()
	row := x.selectMsgStmt.QueryRow(key)
	err := row.Scan(&tipset, &epoch)
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
		TipSet:  tipsetCid,
		Epoch:   abi.ChainEpoch(epoch),
	}, nil
}

func (x *msgIndex) Close() error {
	x.closeLk.Lock()
	defer x.closeLk.Unlock()

	if x.closed {
		return nil
	}

	x.closed = true

	x.cancel()
	x.workers.Wait()

	return x.db.Close()
}

// informal apis for itests; not exposed in the main interface
func (x *msgIndex) CountMessages() (int64, error) {
	x.closeLk.RLock()
	defer x.closeLk.RUnlock()

	if x.closed {
		return 0, ErrClosed
	}

	var result int64
	row := x.db.QueryRow(dbqCountMessages)
	err := row.Scan(&result)
	return result, err
}
