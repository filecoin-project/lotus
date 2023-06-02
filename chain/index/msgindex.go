package index

import (
	"context"
	"database/sql"
	"errors"
	"github.com/filecoin-project/go-address"
	"github.com/google/uuid"
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
var dbPragmas = []string{}

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
	GetParentReceipt(ctx context.Context, header *types.BlockHeader, i int) (*types.MessageReceipt, error)
	LoadTipSet(ctx context.Context, tsk types.TipSetKey) (*types.TipSet, error)
}

var _ ChainStore = (*store.ChainStore)(nil)

const (
	TSRevert = "tsRevert"
	MsgFound = "msgFound"
)

type WaitMsgReturn struct {
	Tipset  *types.TipSet
	Receipt *types.MessageReceipt
}

//type SubscriptionMapVal struct {
//	tipset   *types.TipSet
//	channels []chan WaitMsgReturn
//}

type MsgId struct {
	From  address.Address
	Nonce uint64
}

type WaitMsgSub struct {
	Msg        cid.Cid
	Confidence uint64
	ch         chan WaitMsgReturn
}

type msgIndex struct {
	cs ChainStore

	db               *sql.DB
	selectMsgStmt    *sql.Stmt
	insertMsgStmt    *sql.Stmt
	deleteTipSetStmt *sql.Stmt

	msgSubs       map[cid.Cid]map[uuid.UUID]bool
	msgTipsets    map[cid.Cid]*types.TipSet
	msgReceipts   map[cid.Cid]*types.MessageReceipt
	subscriptions map[uuid.UUID]*WaitMsgSub
	readyEpochs   map[abi.ChainEpoch]map[uuid.UUID]bool // subscriptions which are ready at this epoch
	subEpoch      map[uuid.UUID]abi.ChainEpoch

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
		db:            db,
		cs:            cs,
		sema:          make(chan struct{}, 1),
		cancel:        cancel,
		msgSubs:       make(map[cid.Cid]map[uuid.UUID]bool),
		msgReceipts:   make(map[cid.Cid]*types.MessageReceipt),
		msgTipsets:    make(map[cid.Cid]*types.TipSet),
		subscriptions: make(map[uuid.UUID]*WaitMsgSub),
		readyEpochs:   make(map[abi.ChainEpoch]map[uuid.UUID]bool),
		subEpoch:      make(map[uuid.UUID]abi.ChainEpoch),
		//msgSubs       map[cid.Cid][]uuid.UUID
		//msgTipsets    map[cid.Cid]*types.TipSet
		//msgReceipts   map[cid.Cid]*types.MessageReceipt
		//subscriptions map[uuid.UUID]*WaitMsgSub
		//readyEpochs   map[abi.ChainEpoch]map[uuid.UUID]bool // subscriptions which are ready at this epoch
		//subEpoch      map[uuid.UUID]abi.ChainEpoch
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

	// Processing of WaitMsg subscriptions
	msgs, err := x.cs.MessagesForTipset(ctx, ts)
	if err != nil {
		return xerrors.Errorf("error retrieving messages for tipset %s: %w", ts, err)
	}
	for _, msg := range msgs {
		mcid := msg.Cid()

		// For all subs for this msg, remove any references in readyEpochs since this msg is no
		// longer valid
		for subId, _ := range x.msgSubs[mcid] {
			rEpoch, ok := x.subEpoch[subId]
			if ok {
				delete(x.readyEpochs[rEpoch], subId)
				delete(x.subEpoch, subId)
			}
		}

		delete(x.msgTipsets, mcid)
		delete(x.msgReceipts, mcid)
	}

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

	pts, err := x.cs.LoadTipSet(ctx, ts.Parents())
	if err != nil {
		return err
	}

	pmsgs, err := x.cs.MessagesForTipset(ctx, pts)
	if err != nil {
		return err
	}

	insertStmt := tx.Stmt(x.insertMsgStmt)
	for _, msg := range msgs {

		key := msg.Cid().String()
		if _, err := insertStmt.Exec(key, tskey, epoch); err != nil {
			return xerrors.Errorf("error inserting message: %w", err)
		}
	}

	// Processing of parent msgs for wait msg subscription
	for i, pmsg := range pmsgs {
		cid := pmsg.Cid()
		subIds, ok := x.msgSubs[cid]
		if ok {
			pr, err := x.cs.GetParentReceipt(ctx, ts.Blocks()[0], i)
			if err != nil {
				return xerrors.Errorf("Can't find parent receipt for message: %w", err)
			}
			x.msgReceipts[cid] = pr
			x.msgTipsets[cid] = ts

			// Run through all the Wait Subs for this message and determine the epoch we need to
			// signal the channel
			for subId, _ := range subIds {
				rEpoch := ts.Height() + abi.ChainEpoch(x.subscriptions[subId].Confidence)
				x.subEpoch[subId] = rEpoch
				_, ok := x.readyEpochs[rEpoch]
				if !ok {
					x.readyEpochs[rEpoch] = make(map[uuid.UUID]bool)
					x.readyEpochs[rEpoch][subId] = true
				} else {
					x.readyEpochs[rEpoch][subId] = true
				}
			}
			//for _, ch := range chans {
			//	ch <- WaitMsgReturn{
			//		Tipset:  ts,
			//		Receipt: pr,
			//	}
			//}
			//delete(x.subscriptions, cid)
		}
	}

	// Go through the ready epochs to check if any channel needs signaling
	if _, ok := x.readyEpochs[ts.Height()]; ok {
		for subId, _ := range x.readyEpochs[ts.Height()] {
			sub := x.subscriptions[subId]
			sub.ch <- WaitMsgReturn{
				Tipset:  x.msgTipsets[sub.Msg],
				Receipt: x.msgReceipts[sub.Msg],
			}

			// Remove all info for this subscription
			delete(x.msgSubs[sub.Msg], subId)
			if len(x.msgSubs[sub.Msg]) == 0 {
				delete(x.msgSubs, sub.Msg)
			}
			delete(x.subscriptions, subId)
			delete(x.subEpoch, subId)
		}
		delete(x.readyEpochs, ts.Height())
	}

	return nil
}

//msgSubs       map[cid.Cid][]uuid.UUID
//msgTipsets    map[cid.Cid]*types.TipSet
//msgReceipts   map[cid.Cid]*types.MessageReceipt
//subscriptions map[uuid.UUID]*WaitMsgSub
//readyEpochs   map[abi.ChainEpoch]map[uuid.UUID]bool // subscriptions which are ready at this epoch
//subEpoch      map[uuid.UUID]abi.ChainEpoch

func (x *msgIndex) WaitForMessageCid(ctx context.Context, msgCid cid.Cid, confidence uint64) (chan WaitMsgReturn, error) {

	//ch := make(chan WaitMsgReturn)
	sub := WaitMsgSub{
		Msg:        msgCid,
		Confidence: confidence,
		ch:         make(chan WaitMsgReturn),
	}
	subId := uuid.New()

	x.subscriptions[subId] = &sub

	_, ok := x.msgSubs[msgCid]
	if !ok {
		x.msgSubs[msgCid] = make(map[uuid.UUID]bool)
		x.msgSubs[msgCid][subId] = true
	} else {
		x.msgSubs[msgCid][subId] = true
	}
	return sub.ch, nil
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
