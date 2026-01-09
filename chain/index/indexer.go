package index

import (
	"context"
	"database/sql"
	"sync"

	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/crypto"

	"github.com/filecoin-project/lotus/chain/actors/builtin"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/types/ethtypes"
	"github.com/filecoin-project/lotus/lib/sqlite"
)

var _ Indexer = (*SqliteIndexer)(nil)

// ActorToDelegatedAddressFunc is a function type that resolves an actor ID to a DelegatedAddress if one exists for that actor, otherwise returns nil
type ActorToDelegatedAddressFunc func(ctx context.Context, emitter abi.ActorID, ts *types.TipSet) (address.Address, bool)
type emsLoaderFunc func(ctx context.Context, cs ChainStore, msgTs, rctTs *types.TipSet) ([]executedMessage, error)
type RecomputeTipSetStateFunc func(ctx context.Context, ts *types.TipSet) error

type preparedStatements struct {
	insertEthTxHashStmt                   *sql.Stmt
	getNonRevertedMsgInfoStmt             *sql.Stmt
	getMsgCidFromEthHashStmt              *sql.Stmt
	insertTipsetMessageStmt               *sql.Stmt
	updateTipsetToRevertedStmt            *sql.Stmt
	hasTipsetStmt                         *sql.Stmt
	updateTipsetToNonRevertedStmt         *sql.Stmt
	removeTipsetsBeforeHeightStmt         *sql.Stmt
	removeEthHashesOlderThanStmt          *sql.Stmt
	updateTipsetsToRevertedFromHeightStmt *sql.Stmt
	updateEventsToRevertedFromHeightStmt  *sql.Stmt
	isIndexEmptyStmt                      *sql.Stmt
	getMinNonRevertedHeightStmt           *sql.Stmt
	hasNonRevertedTipsetStmt              *sql.Stmt
	updateEventsToRevertedStmt            *sql.Stmt
	updateEventsToNonRevertedStmt         *sql.Stmt
	getMsgIdForMsgCidAndTipsetStmt        *sql.Stmt
	insertEventStmt                       *sql.Stmt
	insertEventEntryStmt                  *sql.Stmt
	getEventIdAndEmitterIdStmt            *sql.Stmt
	getEventEntriesStmt                   *sql.Stmt

	hasNullRoundAtHeightStmt         *sql.Stmt
	getNonRevertedTipsetAtHeightStmt *sql.Stmt
	countTipsetsAtHeightStmt         *sql.Stmt

	getNonRevertedTipsetMessageCountStmt      *sql.Stmt
	getNonRevertedTipsetEventCountStmt        *sql.Stmt
	getNonRevertedTipsetEventEntriesCountStmt *sql.Stmt
	hasRevertedEventsInTipsetStmt             *sql.Stmt
	removeRevertedTipsetsBeforeHeightStmt     *sql.Stmt
}

type SqliteIndexer struct {
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	db *sql.DB
	cs ChainStore

	actorToDelegatedAddresFunc ActorToDelegatedAddressFunc
	executedMessagesLoaderFunc emsLoaderFunc

	stmts *preparedStatements

	gcRetentionEpochs   int64
	reconcileEmptyIndex bool
	maxReconcileTipsets uint64

	mu           sync.Mutex
	updateSubs   map[uint64]*updateSub
	subIdCounter uint64

	started bool

	// ensures writes are serialized so backfilling does not race with index updates
	writerLk sync.Mutex
}

func NewSqliteIndexer(path string, cs ChainStore, gcRetentionEpochs int64, reconcileEmptyIndex bool,
	maxReconcileTipsets uint64) (si *SqliteIndexer, err error) {

	if gcRetentionEpochs != 0 && gcRetentionEpochs < builtin.EpochsInDay {
		return nil, xerrors.Errorf("gc retention epochs must be 0 or greater than %d", builtin.EpochsInDay)
	}

	db, err := sqlite.Open(path)
	if err != nil {
		return nil, xerrors.Errorf("failed to setup message index db: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	defer func() {
		if err != nil {
			_ = db.Close()
			cancel()
		}
	}()

	err = sqlite.InitDb(ctx, "chain index", db, ddls, []sqlite.MigrationFunc{})
	if err != nil {
		return nil, xerrors.Errorf("failed to init chain index db: %w", err)
	}

	si = &SqliteIndexer{
		ctx:                 ctx,
		cancel:              cancel,
		db:                  db,
		cs:                  cs,
		updateSubs:          make(map[uint64]*updateSub),
		subIdCounter:        0,
		gcRetentionEpochs:   gcRetentionEpochs,
		reconcileEmptyIndex: reconcileEmptyIndex,
		maxReconcileTipsets: maxReconcileTipsets,
		stmts:               &preparedStatements{},
	}

	if err = si.initStatements(); err != nil {
		return nil, xerrors.Errorf("failed to prepare statements: %w", err)
	}

	return si, nil
}

func (si *SqliteIndexer) Start() {
	si.wg.Add(1)
	go si.gcLoop()

	si.started = true
}

func (si *SqliteIndexer) SetActorToDelegatedAddresFunc(actorToDelegatedAddresFunc ActorToDelegatedAddressFunc) {
	si.actorToDelegatedAddresFunc = actorToDelegatedAddresFunc
}

func (si *SqliteIndexer) SetRecomputeTipSetStateFunc(f RecomputeTipSetStateFunc) {
	si.buildExecutedMessagesLoader(f)
}

func (si *SqliteIndexer) buildExecutedMessagesLoader(rf RecomputeTipSetStateFunc) {
	si.executedMessagesLoaderFunc = func(ctx context.Context, cs ChainStore, msgTs, rctTs *types.TipSet) ([]executedMessage, error) {
		return loadExecutedMessages(ctx, cs, rf, msgTs, rctTs)
	}
}

func (si *SqliteIndexer) Close() error {
	si.cancel()
	si.wg.Wait()

	// Close is idempotent, it doesn't hurt to call it multiple times.
	if err := si.db.Close(); err != nil {
		return xerrors.Errorf("failed to close db: %w", err)
	}
	return nil
}

func (si *SqliteIndexer) initStatements() error {
	stmtMapping := preparedStatementMapping(si.stmts)
	for stmtPointer, query := range stmtMapping {
		var err error
		*stmtPointer, err = si.db.Prepare(query)
		if err != nil {
			return xerrors.Errorf("prepare statement [%s]: %w", query, err)
		}
	}

	return nil
}

func (si *SqliteIndexer) IndexEthTxHash(ctx context.Context, txHash ethtypes.EthHash, msgCid cid.Cid) error {
	if si.isClosed() {
		return ErrClosed
	}

	return withTx(ctx, si.db, func(tx *sql.Tx) error {
		return si.indexEthTxHash(ctx, tx, txHash, msgCid)
	})
}

func (si *SqliteIndexer) indexEthTxHash(ctx context.Context, tx *sql.Tx, txHash ethtypes.EthHash, msgCid cid.Cid) error {
	insertEthTxHashStmt := tx.Stmt(si.stmts.insertEthTxHashStmt)
	_, err := insertEthTxHashStmt.ExecContext(ctx, txHash.String(), msgCid.Bytes())
	if err != nil {
		return xerrors.Errorf("failed to index eth tx hash: %w", err)
	}

	return nil
}

func (si *SqliteIndexer) IndexSignedMessage(ctx context.Context, msg *types.SignedMessage) error {
	if msg.Signature.Type != crypto.SigTypeDelegated {
		return nil
	}

	if si.isClosed() {
		return ErrClosed
	}

	return withTx(ctx, si.db, func(tx *sql.Tx) error {
		return si.indexSignedMessage(ctx, tx, msg)
	})
}

func (si *SqliteIndexer) indexSignedMessage(ctx context.Context, tx *sql.Tx, msg *types.SignedMessage) error {
	ethTx, err := ethtypes.EthTransactionFromSignedFilecoinMessage(msg)
	if err != nil {
		return xerrors.Errorf("failed to convert filecoin message to eth tx: %w", err)
	}

	txHash, err := ethTx.TxHash()
	if err != nil {
		return xerrors.Errorf("failed to hash transaction: %w", err)
	}

	return si.indexEthTxHash(ctx, tx, txHash, msg.Cid())
}

func (si *SqliteIndexer) Apply(ctx context.Context, from, to *types.TipSet) error {
	if si.isClosed() {
		return ErrClosed
	}

	si.writerLk.Lock()

	// We're moving the chain ahead from the `from` tipset to the `to` tipset
	// Height(to) > Height(from)
	err := withTx(ctx, si.db, func(tx *sql.Tx) error {
		if err := si.indexTipsetWithParentEvents(ctx, tx, from, to); err != nil {
			return xerrors.Errorf("failed to index tipset: %w", err)
		}

		return nil
	})

	if err != nil {
		si.writerLk.Unlock()
		return xerrors.Errorf("failed to apply tipset: %w", err)
	}
	si.writerLk.Unlock()

	si.notifyUpdateSubs()

	return nil
}

func (si *SqliteIndexer) indexTipset(ctx context.Context, tx *sql.Tx, ts *types.TipSet) error {
	tsKeyCidBytes, err := toTipsetKeyCidBytes(ts)
	if err != nil {
		return xerrors.Errorf("failed to compute tipset cid: %w", err)
	}

	if restored, err := si.restoreTipsetIfExists(ctx, tx, tsKeyCidBytes); err != nil {
		return xerrors.Errorf("failed to restore tipset: %w", err)
	} else if restored {
		return nil
	}

	height := ts.Height()

	msgs, err := si.cs.MessagesForTipset(ctx, ts)
	if err != nil {
		return xerrors.Errorf("failed to get messages for tipset: %w", err)
	}

	if len(msgs) == 0 {
		insertTipsetMsgStmt := tx.Stmt(si.stmts.insertTipsetMessageStmt)
		// If there are no messages, just insert the tipset and return
		if _, err := insertTipsetMsgStmt.ExecContext(ctx, tsKeyCidBytes, height, 0, nil, -1); err != nil {
			return xerrors.Errorf("failed to insert empty tipset: %w", err)
		}
		return nil
	}

	for i, msg := range msgs {
		insertTipsetMsgStmt := tx.Stmt(si.stmts.insertTipsetMessageStmt)
		msg := msg
		if _, err := insertTipsetMsgStmt.ExecContext(ctx, tsKeyCidBytes, height, 0, msg.Cid().Bytes(), i); err != nil {
			return xerrors.Errorf("failed to insert tipset message: %w", err)
		}
	}

	for _, blk := range ts.Blocks() {
		_, smsgs, err := si.cs.MessagesForBlock(ctx, blk)
		if err != nil {
			return xerrors.Errorf("failed to get messages for block: %w", err)
		}

		for _, smsg := range smsgs {
			if smsg.Signature.Type != crypto.SigTypeDelegated {
				continue
			}
			if err := si.indexSignedMessage(ctx, tx, smsg); err != nil {
				return xerrors.Errorf("failed to index eth tx hash: %w", err)
			}
		}
	}

	return nil
}

func (si *SqliteIndexer) indexTipsetWithParentEvents(ctx context.Context, tx *sql.Tx, parentTs *types.TipSet, currentTs *types.TipSet) error {
	// Index the parent tipset if it doesn't exist yet.
	// This is necessary to properly index events produced by executing
	// messages included in the parent tipset by the current tipset (deferred execution).
	if err := si.indexTipset(ctx, tx, parentTs); err != nil {
		return xerrors.Errorf("failed to index parent tipset: %w", err)
	}
	if err := si.indexTipset(ctx, tx, currentTs); err != nil {
		return xerrors.Errorf("failed to index tipset: %w", err)
	}

	// Now Index events
	if err := si.indexEvents(ctx, tx, parentTs, currentTs); err != nil {
		return xerrors.Errorf("failed to index events: %w", err)
	}

	return nil
}

func (si *SqliteIndexer) restoreTipsetIfExists(ctx context.Context, tx *sql.Tx, tsKeyCidBytes []byte) (bool, error) {
	// Check if the tipset already exists
	var exists bool
	if err := tx.Stmt(si.stmts.hasTipsetStmt).QueryRowContext(ctx, tsKeyCidBytes).Scan(&exists); err != nil {
		return false, xerrors.Errorf("failed to check if tipset exists: %w", err)
	}
	if exists {
		if _, err := tx.Stmt(si.stmts.updateTipsetToNonRevertedStmt).ExecContext(ctx, tsKeyCidBytes); err != nil {
			return false, xerrors.Errorf("failed to restore tipset: %w", err)
		}
		return true, nil
	}
	return false, nil
}

func (si *SqliteIndexer) Revert(ctx context.Context, from, to *types.TipSet) error {
	if si.isClosed() {
		return ErrClosed
	}

	// We're reverting the chain from the tipset at `from` to the tipset at `to`.
	// Height(to) < Height(from)

	revertTsKeyCid, err := toTipsetKeyCidBytes(from)
	if err != nil {
		return xerrors.Errorf("failed to get tipset key cid: %w", err)
	}

	// Because of deferred execution in Filecoin, events at tipset T are reverted when a tipset T+1 is reverted.
	// However, the tipet `T` itself is not reverted.
	eventTsKeyCid, err := toTipsetKeyCidBytes(to)
	if err != nil {
		return xerrors.Errorf("failed to get tipset key cid: %w", err)
	}

	si.writerLk.Lock()

	err = withTx(ctx, si.db, func(tx *sql.Tx) error {
		// revert the `from` tipset
		if _, err := tx.Stmt(si.stmts.updateTipsetToRevertedStmt).ExecContext(ctx, revertTsKeyCid); err != nil {
			return xerrors.Errorf("failed to mark tipset %s as reverted: %w", revertTsKeyCid, err)
		}

		// index the `to` tipset -> it is idempotent
		if err := si.indexTipset(ctx, tx, to); err != nil {
			return xerrors.Errorf("failed to index tipset: %w", err)
		}

		// events are indexed against the message inclusion tipset, not the message execution tipset.
		// So we need to revert the events for the message inclusion tipset i.e. `to` tipset.
		if _, err := tx.Stmt(si.stmts.updateEventsToRevertedStmt).ExecContext(ctx, eventTsKeyCid); err != nil {
			return xerrors.Errorf("failed to revert events for tipset %s: %w", eventTsKeyCid, err)
		}

		return nil
	})
	if err != nil {
		si.writerLk.Unlock()
		return xerrors.Errorf("failed during revert transaction: %w", err)
	}

	si.writerLk.Unlock()
	si.notifyUpdateSubs()

	return nil
}

func (si *SqliteIndexer) isClosed() bool {
	return si.ctx.Err() != nil
}

func (si *SqliteIndexer) setExecutedMessagesLoaderFunc(f emsLoaderFunc) {
	si.executedMessagesLoaderFunc = f
}
