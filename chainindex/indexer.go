package chainindex

import (
	"context"
	"database/sql"
	"sync"

	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/crypto"

	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/types/ethtypes"
	"github.com/filecoin-project/lotus/lib/sqlite"
)

var _ Indexer = (*SqliteIndexer)(nil)

type SqliteIndexer struct {
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	db *sql.DB
	cs ChainStore

	insertEthTxHashStmt                   *sql.Stmt
	getNonRevertedMsgInfoStmt             *sql.Stmt
	getMsgCidFromEthHashStmt              *sql.Stmt
	insertTipsetMessageStmt               *sql.Stmt
	revertTipsetStmt                      *sql.Stmt
	getMaxNonRevertedTipsetStmt           *sql.Stmt
	tipsetExistsStmt                      *sql.Stmt
	tipsetUnRevertStmt                    *sql.Stmt
	removeRevertedTipsetsBeforeHeightStmt *sql.Stmt
	removeTipsetsBeforeHeightStmt         *sql.Stmt
	deleteEthHashesOlderThanStmt          *sql.Stmt

	gcRetentionEpochs int64

	mu           sync.Mutex
	updateSubs   map[uint64]*updateSub
	subIdCounter uint64
}

func NewSqliteIndexer(path string, cs ChainStore, gcRetentionEpochs int64) (si *SqliteIndexer, err error) {
	db, _, err := sqlite.Open(path)
	if err != nil {
		return nil, xerrors.Errorf("failed to setup message index db: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	defer func() {
		if err != nil {
			cancel()
			_ = db.Close()
		}
	}()

	err = sqlite.InitDb(ctx, "chain index", db, ddls, []sqlite.MigrationFunc{})
	if err != nil {
		return nil, xerrors.Errorf("failed to init message index db: %w", err)
	}

	si = &SqliteIndexer{
		ctx:               ctx,
		cancel:            cancel,
		db:                db,
		cs:                cs,
		updateSubs:        make(map[uint64]*updateSub),
		subIdCounter:      0,
		gcRetentionEpochs: gcRetentionEpochs,
	}
	if err = si.prepareStatements(); err != nil {
		return nil, xerrors.Errorf("failed to prepare statements: %w", err)
	}

	si.wg.Add(1)
	go si.gcLoop()

	return si, nil
}

func (si *SqliteIndexer) Close() error {
	if si.db == nil {
		return nil
	}
	si.cancel()
	si.wg.Wait()

	if err := si.db.Close(); err != nil {
		return xerrors.Errorf("failed to close db: %w", err)
	}
	return nil
}

func (si *SqliteIndexer) prepareStatements() error {
	var err error

	si.insertEthTxHashStmt, err = si.db.Prepare(stmtInsertEthTxHash)
	if err != nil {
		return xerrors.Errorf("prepare %s: %w", "insertEthTxHashStmt", err)
	}
	si.getNonRevertedMsgInfoStmt, err = si.db.Prepare(stmtGetNonRevertedMessageInfo)
	if err != nil {
		return xerrors.Errorf("prepare %s: %w", "getNonRevertedMsgInfoStmt", err)
	}
	si.getMsgCidFromEthHashStmt, err = si.db.Prepare(stmtGetMsgCidFromEthHash)
	if err != nil {
		return xerrors.Errorf("prepare %s: %w", "getMsgCidFromEthHashStmt", err)
	}
	si.insertTipsetMessageStmt, err = si.db.Prepare(stmtInsertTipsetMessage)
	if err != nil {
		return xerrors.Errorf("prepare %s: %w", "insertTipsetMessageStmt", err)
	}
	si.tipsetExistsStmt, err = si.db.Prepare(stmtTipsetExists)
	if err != nil {
		return xerrors.Errorf("prepare %s: %w", "tipsetExistsStmt", err)
	}
	si.tipsetUnRevertStmt, err = si.db.Prepare(stmtTipsetUnRevert)
	if err != nil {
		return xerrors.Errorf("prepare %s: %w", "tipsetUnRevertStmt", err)
	}
	si.revertTipsetStmt, err = si.db.Prepare(stmtRevertTipset)
	if err != nil {
		return xerrors.Errorf("prepare %s: %w", "revertTipsetStmt", err)
	}
	si.getMaxNonRevertedTipsetStmt, err = si.db.Prepare(stmtGetMaxNonRevertedTipset)
	if err != nil {
		return xerrors.Errorf("prepare %s: %w", "getMaxNonRevertedTipsetStmt", err)
	}
	si.removeRevertedTipsetsBeforeHeightStmt, err = si.db.Prepare(stmtRemoveRevertedTipsetsBeforeHeight)
	if err != nil {
		return xerrors.Errorf("prepare %s: %w", "removeRevertedTipsetsBeforeHeightStmt", err)
	}
	si.removeTipsetsBeforeHeightStmt, err = si.db.Prepare(stmtRemoveTipsetsBeforeHeight)
	if err != nil {
		return xerrors.Errorf("prepare %s: %w", "removeTipsetsBeforeHeightStmt", err)
	}
	si.deleteEthHashesOlderThanStmt, err = si.db.Prepare(stmtDeleteEthHashesOlderThan)
	if err != nil {
		return xerrors.Errorf("prepare %s: %w", "deleteEthHashesOlderThanStmt", err)
	}

	return nil
}

func (si *SqliteIndexer) IndexEthTxHash(ctx context.Context, txHash ethtypes.EthHash, msgCid cid.Cid) error {
	return withTx(ctx, si.db, func(tx *sql.Tx) error {
		return si.indexEthTxHash(ctx, tx, txHash, msgCid)
	})
}

func (si *SqliteIndexer) IndexSignedMessage(ctx context.Context, msg *types.SignedMessage) error {
	return withTx(ctx, si.db, func(tx *sql.Tx) error {
		return si.indexSignedMessage(ctx, tx, msg)
	})
}

func (si *SqliteIndexer) indexSignedMessage(ctx context.Context, tx *sql.Tx, msg *types.SignedMessage) error {
	if msg.Signature.Type != crypto.SigTypeDelegated {
		return nil
	}

	ethTx, err := ethtypes.EthTransactionFromSignedFilecoinMessage(msg)
	if err != nil {
		return xerrors.Errorf("error converting filecoin message to eth tx: %w", err)
	}

	txHash, err := ethTx.TxHash()
	if err != nil {
		return xerrors.Errorf("error hashing transaction: %w", err)
	}

	return si.indexEthTxHash(ctx, tx, txHash, msg.Cid())
}

func (si *SqliteIndexer) indexEthTxHash(ctx context.Context, tx *sql.Tx, txHash ethtypes.EthHash, msgCid cid.Cid) error {
	insertEthTxHashStmt := tx.Stmt(si.insertEthTxHashStmt)
	_, err := insertEthTxHashStmt.ExecContext(ctx, txHash.String(), msgCid.Bytes())
	if err != nil {
		return xerrors.Errorf("failed to index eth tx hash: %w", err)
	}

	return nil
}

func (si *SqliteIndexer) Apply(ctx context.Context, from, to *types.TipSet) error {
	// We're moving the chain ahead from the `from` tipset to the `to` tipset
	// Height(to) > Height(from)
	err := withTx(ctx, si.db, func(tx *sql.Tx) error {
		// index the `to` tipset first as we only need to index the tipsets and messages for it
		if err := si.indexTipset(ctx, tx, to); err != nil {
			return xerrors.Errorf("error indexing tipset: %w", err)
		}

		// index the `from` tipset just in case it's not indexed
		if err := si.indexTipset(ctx, tx, from); err != nil {
			return xerrors.Errorf("error indexing tipset: %w", err)
		}

		return nil
	})

	if err != nil {
		return xerrors.Errorf("error applying tipset: %w", err)
	}

	si.notifyUpdateSubs()

	return nil
}

func (si *SqliteIndexer) Revert(ctx context.Context, from, to *types.TipSet) error {
	// We're reverting the chain from the tipset at `from` to the tipset at `to`.
	// Height(to) < Height(from)

	revertTsKeyCid, err := toTipsetKeyCidBytes(from)
	if err != nil {
		return xerrors.Errorf("error getting tipset key cid: %w", err)
	}

	err = withTx(ctx, si.db, func(tx *sql.Tx) error {
		if _, err := tx.Stmt(si.revertTipsetStmt).ExecContext(ctx, revertTsKeyCid); err != nil {
			return xerrors.Errorf("error marking tipset as reverted: %w", err)
		}

		// index the `to` tipset as it has now been applied -> simply for redundancy
		if err := si.indexTipset(ctx, tx, to); err != nil {
			return xerrors.Errorf("error indexing tipset: %w", err)
		}

		return nil
	})
	if err != nil {
		return xerrors.Errorf("error during revert transaction: %w", err)
	}

	si.notifyUpdateSubs()

	return nil
}

func (si *SqliteIndexer) indexTipset(ctx context.Context, tx *sql.Tx, ts *types.TipSet) error {
	tsKeyCidBytes, err := toTipsetKeyCidBytes(ts)
	if err != nil {
		return xerrors.Errorf("error computing tipset cid: %w", err)
	}

	restored, err := si.restoreTipsetIfExists(ctx, tx, tsKeyCidBytes)
	if err != nil {
		return xerrors.Errorf("error restoring tipset: %w", err)
	}
	if restored {
		return nil
	}

	height := ts.Height()
	insertTipsetMsgStmt := tx.Stmt(si.insertTipsetMessageStmt)

	msgs, err := si.cs.MessagesForTipset(ctx, ts)
	if err != nil {
		return xerrors.Errorf("error getting messages for tipset: %w", err)
	}

	for i, msg := range msgs {
		msg := msg
		if _, err := insertTipsetMsgStmt.ExecContext(ctx, tsKeyCidBytes, height, 0, msg.Cid().Bytes(), i); err != nil {
			return xerrors.Errorf("error inserting tipset message: %w", err)
		}

		smsg, ok := msg.(*types.SignedMessage)
		if !ok {
			continue
		}

		if err := si.indexSignedMessage(ctx, tx, smsg); err != nil {
			return xerrors.Errorf("error indexing eth tx hash: %w", err)
		}
	}

	return nil
}

func (si *SqliteIndexer) restoreTipsetIfExists(ctx context.Context, tx *sql.Tx, tsKeyCidBytes []byte) (bool, error) {
	// Check if the tipset already exists
	var exists bool
	if err := tx.Stmt(si.tipsetExistsStmt).QueryRowContext(ctx, tsKeyCidBytes).Scan(&exists); err != nil {
		return false, xerrors.Errorf("error checking if tipset exists: %w", err)
	}
	if exists {
		if _, err := tx.Stmt(si.tipsetUnRevertStmt).ExecContext(ctx, tsKeyCidBytes); err != nil {
			return false, xerrors.Errorf("error restoring tipset: %w", err)
		}
		return true, nil
	}
	return false, nil
}
