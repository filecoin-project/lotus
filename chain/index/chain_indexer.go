package index

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	cbg "github.com/whyrusleeping/cbor-gen"

	"github.com/filecoin-project/go-address"
	amt4 "github.com/filecoin-project/go-amt-ipld/v4"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/crypto"
	blockadt "github.com/filecoin-project/specs-actors/actors/util/adt"

	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/types/ethtypes"
	"github.com/filecoin-project/lotus/lib/sqlite"

	// TODO: Solve this import cycle
	"github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log/v2"
	"golang.org/x/xerrors"
)

var logger = logging.Logger("chain/index")

type ChainIndexer struct {
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	db *sql.DB
	cs *store.ChainStore

	stmts *preparedStatements

	resolver func(ctx context.Context, emitter abi.ActorID, ts *types.TipSet) (address.Address, bool)

	mu           sync.Mutex
	updateSubs   map[uint64]*updateSub
	subIdCounter uint64
}

type updateSub struct {
	ctx    context.Context
	cancel context.CancelFunc
	ch     chan ChainIndexUpdated
}

type ChainIndexUpdated struct{}

func (ci *ChainIndexer) SubscribeUpdates(ctx context.Context) (chan ChainIndexUpdated, func()) {
	subCtx, subCancel := context.WithCancel(ctx)
	ch := make(chan ChainIndexUpdated)

	ci.mu.Lock()
	subId := ci.subIdCounter
	ci.subIdCounter++
	ci.updateSubs[subId] = &updateSub{
		ctx:    subCtx,
		cancel: subCancel,
		ch:     ch,
	}
	ci.mu.Unlock()

	unSubscribeF := func() {
		ci.mu.Lock()
		if sub, ok := ci.updateSubs[subId]; ok {
			sub.cancel()
			delete(ci.updateSubs, subId)
		}
		ci.mu.Unlock()
	}

	return ch, unSubscribeF
}

func NewChainIndexer(ctx context.Context, path string, chainStore *store.ChainStore, resolver func(ctx context.Context, emitter abi.ActorID, ts *types.TipSet) (address.Address, bool)) (*ChainIndexer, error) {
	cctx, cancel := context.WithCancel(ctx)

	db, _, err := sqlite.Open(path)
	if err != nil {
		return nil, xerrors.Errorf("failed to setup event index db: %w", err)
	}

	err = sqlite.InitDb(ctx, "chain index", db, ddls, nil)
	if err != nil {
		_ = db.Close()
		return nil, xerrors.Errorf("failed to setup event index db: %w", err)
	}

	chainIndexer := ChainIndexer{
		ctx:          cctx,
		cancel:       cancel,
		db:           db,
		cs:           chainStore,
		resolver:     resolver,
		stmts:        &preparedStatements{},
		updateSubs:   make(map[uint64]*updateSub),
		subIdCounter: 0,
	}

	if err = chainIndexer.initStatements(); err != nil {
		_ = db.Close()
		return nil, xerrors.Errorf("error preparing eventIndex database statements: %w", err)
	}

	chainIndexer.wg.Add(1)
	go chainIndexer.cleanupReorgsLoop(cctx)

	return &chainIndexer, nil
}

func (ci *ChainIndexer) initStatements() error {
	stmtMapping := preparedStatementMapping(ci.stmts)
	for stmtPointer, query := range stmtMapping {
		var err error
		*stmtPointer, err = ci.db.Prepare(query)
		if err != nil {
			return xerrors.Errorf("prepare statement [%s]: %w", query, err)
		}
	}

	return nil
}

func (ci *ChainIndexer) Close() error {
	if ci.db == nil {
		return nil
	}
	ci.cancel()
	ci.wg.Wait()
	return ci.db.Close()
}

func (ci *ChainIndexer) notifyUpdateSubs() {
	ci.mu.Lock()
	tSubs := make([]*updateSub, 0, len(ci.updateSubs))
	for _, tSub := range ci.updateSubs {
		tSubs = append(tSubs, tSub)
	}
	ci.mu.Unlock()

	for _, tSub := range tSubs {
		tSub := tSub
		select {
		case tSub.ch <- ChainIndexUpdated{}:
		case <-tSub.ctx.Done():
			// subscription was cancelled, ignore
		case <-ci.ctx.Done():
			return
		}
	}
}

func (ci *ChainIndexer) Apply(ctx context.Context, from, to *types.TipSet) error {
	// We're moving the chain ahead from the `from` tipset to the `to` tipset
	// Height(to) > Height(from)
	tx, err := ci.db.BeginTx(ctx, nil)
	if err != nil {
		return xerrors.Errorf("beginning transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	// index the `to` tipset first as we only need to index the tipsets and messages for it
	if err := ci.indexTipset(ctx, tx, to); err != nil {
		return xerrors.Errorf("error indexing tipset: %w", err)
	}

	// index the `from` tipset in case we've not seen it before
	if err := ci.indexTipset(ctx, tx, from); err != nil {
		return xerrors.Errorf("error indexing tipset messages: %w", err)
	}

	// insert events for `from` tipset as it's messages have now been executed in `to` tipset
	if err := ci.indexEvents(ctx, tx, from, to); err != nil {
		return xerrors.Errorf("error indexing events: %w", err)
	}

	fromTsKeyCidBytes, err := toTipsetKeyCidBytes(from)
	if err != nil {
		return xerrors.Errorf("error getting tipset key cid: %w", err)
	}

	// events processed for `from` tipset
	if _, err := tx.Stmt(ci.stmts.stmtMarkTipsetEventsProcessed).ExecContext(ctx, fromTsKeyCidBytes); err != nil {
		return xerrors.Errorf("error marking tipset events processed: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return xerrors.Errorf("error committing transaction: %w", err)
	}
	ci.notifyUpdateSubs()
	return nil
}

func (ci *ChainIndexer) indexEvents(ctx context.Context, tx *sql.Tx, msgTs *types.TipSet, executionTs *types.TipSet) error {
	// check if we have an event indexed for any message in the `msgTs` tipset -> if so, there's nothig to do here
	msgTsKeyCidBytes, err := toTipsetKeyCidBytes(msgTs)
	if err != nil {
		return xerrors.Errorf("error getting tipset key cid: %w", err)
	}

	// if we've already indexed events for this tipset, mark them as unreverted and return
	res, err := tx.Stmt(ci.stmts.stmtEventsUnRevert).ExecContext(ctx, msgTsKeyCidBytes)
	if err != nil {
		return xerrors.Errorf("error unreverting events for tipset: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return xerrors.Errorf("error unreverting events for tipset: %w", err)
	}
	if rows > 0 {
		return nil
	}

	ems, err := ci.loadExecutedMessages(ctx, msgTs, executionTs)
	if err != nil {
		return xerrors.Errorf("error loading executed messages: %w", err)
	}
	eventCount := 0
	addressLookups := make(map[abi.ActorID]address.Address)

	for _, em := range ems {
		msgCidBytes := em.msg.Cid().Bytes()

		// read message id for this message cid and tipset key cid
		var messageID int64
		if err := tx.Stmt(ci.stmts.selectMsgIdForMsgCidAndTipset).QueryRow(msgCidBytes, msgTsKeyCidBytes).Scan(&messageID); err != nil {
			return xerrors.Errorf("error getting message id for message cid and tipset key cid: %w", err)
		}

		// Insert events for this message
		for _, event := range em.evs {
			event := event

			addr, found := addressLookups[event.Emitter]
			if !found {
				var ok bool
				addr, ok = ci.resolver(ctx, event.Emitter, executionTs)
				if !ok {
					// not an address we will be able to match against
					continue
				}
				addressLookups[event.Emitter] = addr
			}

			// Insert event into events table
			eventResult, err := tx.Stmt(ci.stmts.stmtInsertEvent).Exec(messageID, eventCount, addr.Bytes(), 0)
			if err != nil {
				return xerrors.Errorf("error inserting event: %w", err)
			}

			// Get the event_id of the inserted event
			eventID, err := eventResult.LastInsertId()
			if err != nil {
				return xerrors.Errorf("error getting last insert id for event: %w", err)
			}

			// Insert event entries
			for _, entry := range event.Entries {
				_, err := tx.Stmt(ci.stmts.stmtInsertEventEntry).Exec(
					eventID,
					isIndexedValue(entry.Flags),
					[]byte{entry.Flags},
					entry.Key,
					entry.Codec,
					entry.Value,
				)
				if err != nil {
					return xerrors.Errorf("error inserting event entry: %w", err)
				}
			}
			eventCount++
		}
	}

	return nil
}

func (ci *ChainIndexer) indexTipset(ctx context.Context, tx *sql.Tx, ts *types.TipSet) error {
	tsKeyCidBytes, err := toTipsetKeyCidBytes(ts)
	if err != nil {
		return xerrors.Errorf("error computing tipset cid: %w", err)
	}

	restored, err := ci.restoreTipsetIfExists(ctx, tx, tsKeyCidBytes)
	if err != nil {
		return xerrors.Errorf("error restoring tipset: %w", err)
	}
	if restored {
		return nil
	}

	height := ts.Height()
	insertTipsetMsgStmt := tx.Stmt(ci.stmts.stmtInsertTipsetMessage)

	msgs, err := ci.cs.MessagesForTipset(ctx, ts)
	if err != nil {
		return xerrors.Errorf("error getting messages for tipset: %w", err)
	}

	for i, msg := range msgs {
		msg := msg
		if _, err := insertTipsetMsgStmt.ExecContext(ctx, tsKeyCidBytes, height, 0, msg.Cid().Bytes(), i, 0); err != nil {
			return xerrors.Errorf("error inserting tipset message: %w", err)
		}

		if err := ci.indexEthTxHash(ctx, tx, msg); err != nil {
			return xerrors.Errorf("error indexing eth tx hash: %w", err)
		}
	}

	return nil
}

func (ci *ChainIndexer) indexEthTxHash(ctx context.Context, tx *sql.Tx, msg types.ChainMsg) error {
	smsg, ok := msg.(*types.SignedMessage)
	if !ok || smsg.Signature.Type != crypto.SigTypeDelegated {
		return nil
	}
	hash, err := ethTxHashFromSignedMessage(smsg)
	if err != nil {
		return err
	}
	if _, err := tx.Stmt(ci.stmts.stmtInsertEthTxHash).Exec(hash.String(), msg.Cid().Bytes()); err != nil {
		return xerrors.Errorf("error inserting eth tx hash: %w", err)
	}
	return nil
}

func (ci *ChainIndexer) restoreTipsetIfExists(ctx context.Context, tx *sql.Tx, tsKeyCid []byte) (bool, error) {
	// Check if the tipset already exists
	var exists bool
	if err := tx.Stmt(ci.stmts.stmtTipsetExists).QueryRowContext(ctx, tsKeyCid).Scan(&exists); err != nil {
		return false, xerrors.Errorf("error checking if tipset exists: %w", err)
	}
	if exists {
		if _, err := tx.Stmt(ci.stmts.stmtTipsetUnRevert).ExecContext(ctx, tsKeyCid); err != nil {
			return false, xerrors.Errorf("error restoring tipset: %w", err)
		}
		return true, nil
	}
	return false, nil
}

func (ci *ChainIndexer) Revert(ctx context.Context, from *types.TipSet, to *types.TipSet) error {
	// We're reverting the chain from the tipset at `from` to the tipset at `to`.
	// Height(to) < Height(from)

	tx, err := ci.db.BeginTx(ctx, nil)
	if err != nil {
		return xerrors.Errorf("error beginning transaction: %w", err)
	}
	// rollback the transaction (a no-op if the transaction was already committed)
	defer func() { _ = tx.Rollback() }()

	revertTsKeyCid, err := toTipsetKeyCidBytes(from)
	if err != nil {
		return xerrors.Errorf("error getting tipset key cid: %w", err)
	}

	if _, err := tx.Stmt(ci.stmts.stmtRevertTipset).Exec(revertTsKeyCid); err != nil {
		return xerrors.Errorf("error marking tipset as reverted: %w", err)
	}

	// events in `to` have also been reverted as the corresponding execution tipset `from` has been reverted
	eventTsKeyCid, err := toTipsetKeyCidBytes(to)
	if err != nil {
		return xerrors.Errorf("error getting tipset key cid: %w", err)
	}

	if _, err := tx.Stmt(ci.stmts.stmtRevertEvents).Exec(eventTsKeyCid); err != nil {
		return xerrors.Errorf("error marking events as reverted: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return xerrors.Errorf("error committing transaction: %w", err)
	}
	ci.notifyUpdateSubs()
	return nil
}

func toTipsetKeyCidBytes(ts *types.TipSet) ([]byte, error) {
	tsKeyCid, err := ts.Key().Cid()
	if err != nil {
		return nil, xerrors.Errorf("error getting tipset key cid: %w", err)
	}
	return tsKeyCid.Bytes(), nil
}

func isIndexedValue(b uint8) bool {
	// currently we mark the full entry as indexed if either the key
	// or the value are indexed; in the future we will need finer-grained
	// management of indices
	return b&(types.EventFlagIndexedKey|types.EventFlagIndexedValue) > 0
}

func ethTxHashFromSignedMessage(smsg *types.SignedMessage) (ethtypes.EthHash, error) {
	if smsg.Signature.Type == crypto.SigTypeDelegated {
		tx, err := ethtypes.EthTransactionFromSignedFilecoinMessage(smsg)
		if err != nil {
			return ethtypes.EthHash{}, xerrors.Errorf("failed to convert from signed message: %w", err)
		}

		return tx.TxHash()
	} else if smsg.Signature.Type == crypto.SigTypeSecp256k1 {
		return ethtypes.EthHashFromCid(smsg.Cid())
	}
	// else BLS message
	return ethtypes.EthHashFromCid(smsg.Message.Cid())
}

type executedMessage struct {
	msg types.ChainMsg
	rct *types.MessageReceipt
	// events extracted from receipt
	evs []*types.Event
}

func (ci *ChainIndexer) loadExecutedMessages(ctx context.Context, msgTs, rctTs *types.TipSet) ([]executedMessage, error) {
	msgs, err := ci.cs.MessagesForTipset(ctx, msgTs)
	if err != nil {
		return nil, xerrors.Errorf("read messages: %w", err)
	}

	st := ci.cs.ActorStore(ctx)

	arr, err := blockadt.AsArray(st, rctTs.Blocks()[0].ParentMessageReceipts)
	if err != nil {
		return nil, xerrors.Errorf("load receipts amt: %w", err)
	}

	if uint64(len(msgs)) != arr.Length() {
		return nil, xerrors.Errorf("mismatching message and receipt counts (%d msgs, %d rcts)", len(msgs), arr.Length())
	}

	ems := make([]executedMessage, len(msgs))

	for i := 0; i < len(msgs); i++ {
		ems[i].msg = msgs[i]

		var rct types.MessageReceipt
		found, err := arr.Get(uint64(i), &rct)
		if err != nil {
			return nil, xerrors.Errorf("load receipt: %w", err)
		}
		if !found {
			return nil, xerrors.Errorf("receipt %d not found", i)
		}
		ems[i].rct = &rct

		if rct.EventsRoot == nil {
			continue
		}

		evtArr, err := amt4.LoadAMT(ctx, st, *rct.EventsRoot, amt4.UseTreeBitWidth(types.EventAMTBitwidth))
		if err != nil {
			return nil, xerrors.Errorf("load events amt: %w", err)
		}

		ems[i].evs = make([]*types.Event, evtArr.Len())
		var evt types.Event
		err = evtArr.ForEach(ctx, func(u uint64, deferred *cbg.Deferred) error {
			if u > math.MaxInt {
				return xerrors.Errorf("too many events")
			}
			if err := evt.UnmarshalCBOR(bytes.NewReader(deferred.Raw)); err != nil {
				return err
			}

			cpy := evt
			ems[i].evs[int(u)] = &cpy //nolint:scopelint
			return nil
		})

		if err != nil {
			return nil, xerrors.Errorf("read events: %w", err)
		}

	}

	return ems, nil
}

func (ci *ChainIndexer) GetMsgInfo(ctx context.Context, msg_cid cid.Cid) (MsgInfo, error) {
	msgInfo, err := ci.lookupMsg(ctx, msg_cid)
	if err == nil {
		return msgInfo, nil
	}

	if err == sql.ErrNoRows {
		// wait till head is processed and retry
		if err := ci.waitTillHeadIndexed(ctx); err != nil {
			return MsgInfo{}, err
		}
		msgInfo, err := ci.lookupMsg(ctx, msg_cid)
		if err == sql.ErrNoRows {
			return MsgInfo{}, ErrNotFound
		}
		return msgInfo, err
	}
	return MsgInfo{}, err
}

func (ci *ChainIndexer) lookupMsg(ctx context.Context, msg_cid cid.Cid) (MsgInfo, error) {
	var tipsetKeyCidBytes []byte
	var height uint64

	row := ci.stmts.stmtSelectMsg.QueryRowContext(ctx, msg_cid.Bytes())
	err := row.Scan(&tipsetKeyCidBytes, &height)
	if err != nil {
		return MsgInfo{}, err
	}

	tsKeyCid, err := cid.Cast(tipsetKeyCidBytes)
	if err != nil {
		return MsgInfo{}, xerrors.Errorf("error casting tipset key cid: %w", err)
	}

	return MsgInfo{
		Message: msg_cid,
		Epoch:   abi.ChainEpoch(height),
		TipSet:  tsKeyCid,
	}, nil
}

func (ci *ChainIndexer) waitTillHeadIndexed(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	head := ci.cs.GetHeaviestTipSet()
	headTsKeyCidBytes, err := toTipsetKeyCidBytes(head)
	if err != nil {
		return xerrors.Errorf("error getting tipset key cid: %w", err)
	}

	// is it already indexed?
	if exists, err := ci.isTipsetIndexed(ctx, headTsKeyCidBytes); err != nil {
		return xerrors.Errorf("error checking if tipset exists: %w", err)
	} else if exists {
		return nil
	}

	// wait till it is indexed
	subCh, unsubFn := ci.SubscribeUpdates(ctx)
	defer unsubFn()

	// is it already indexed?
	if exists, err := ci.isTipsetIndexed(ctx, headTsKeyCidBytes); err != nil {
		return xerrors.Errorf("error checking if tipset exists: %w", err)
	} else if exists {
		return nil
	}

	for {
		select {
		case <-subCh:
			// is it already indexed?
			if exists, err := ci.isTipsetIndexed(ctx, headTsKeyCidBytes); err != nil {
				return xerrors.Errorf("error checking if tipset exists: %w", err)
			} else if exists {
				return nil
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (ci *ChainIndexer) isTipsetIndexed(ctx context.Context, tsKeyCid []byte) (bool, error) {
	var exists bool
	err := ci.stmts.stmtTipsetExists.QueryRowContext(ctx, tsKeyCid).Scan(&exists)
	if err != nil {
		return false, xerrors.Errorf("error checking if tipset exists: %w", err)
	}
	return exists, nil
}

func (ci *ChainIndexer) GetMsgCidFromEthTxHash(ctx context.Context, txHash ethtypes.EthHash) (cid.Cid, error) {
	c, err := ci.lookupEthTxHash(ctx, txHash)
	if err == nil {
		return c, nil
	}
	if err == sql.ErrNoRows {
		// wait till head is processed and retry
		if err := ci.waitTillHeadIndexed(ctx); err != nil {
			return cid.Undef, err
		}
		c, err = ci.lookupEthTxHash(ctx, txHash)
		if err == sql.ErrNoRows {
			return cid.Undef, ErrNotFound
		}
		return c, nil
	}
	return cid.Undef, err
}

func (ci *ChainIndexer) lookupEthTxHash(ctx context.Context, txHash ethtypes.EthHash) (cid.Cid, error) {
	row := ci.stmts.stmtGetMsgCidFromEthHash.QueryRowContext(ctx, txHash.String())
	var c []byte
	err := row.Scan(&c)
	if err != nil {
		return cid.Undef, err
	}
	return cid.Cast(c)
}

// TODO: Finish this
func (ci *ChainIndexer) GetEvents(ctx context.Context, f *eventFilter) ([]*types.Event, error) {
	// always wait for head to be indexed here
	if err := ci.waitTillHeadIndexed(ctx); err != nil {
		return nil, err
	}
	return nil, nil
}

// TODO: Fix and this this
type eventFilter struct {
	minHeight abi.ChainEpoch // minimum epoch to apply filter or -1 if no minimum
	maxHeight abi.ChainEpoch // maximum epoch to apply filter or -1 if no maximum
	tipsetCid cid.Cid
	addresses []address.Address // list of actor addresses that are extpected to emit the event

	keysWithCodec map[string][]types.ActorEventBlock // map of key names to a list of alternate values that may match
}

func makePrefillFilterQuery(f *eventFilter, excludeReverted bool) ([]any, string) {
	clauses := []string{}
	values := []any{}
	joins := []string{}

	if f.tipsetCid != cid.Undef {
		clauses = append(clauses, "tm.tipset_key_cid=?")
		values = append(values, f.tipsetCid.Bytes())
	} else {
		if f.minHeight >= 0 && f.minHeight == f.maxHeight {
			clauses = append(clauses, "tm.height=?")
			values = append(values, f.minHeight)
		} else {
			if f.maxHeight >= 0 && f.minHeight >= 0 {
				clauses = append(clauses, "tm.height BETWEEN ? AND ?")
				values = append(values, f.minHeight, f.maxHeight)
			} else if f.minHeight >= 0 {
				clauses = append(clauses, "tm.height >= ?")
				values = append(values, f.minHeight)
			} else if f.maxHeight >= 0 {
				clauses = append(clauses, "tm.height <= ?")
				values = append(values, f.maxHeight)
			}
		}
	}

	if excludeReverted {
		clauses = append(clauses, "tm.reverted=?")
		values = append(values, false)
		clauses = append(clauses, "et.reverted=?")
		values = append(values, false)
	}

	if len(f.addresses) > 0 {
		for _, addr := range f.addresses {
			values = append(values, addr.Bytes())
		}
		clauses = append(clauses, "e.emitter_addr IN ("+strings.Repeat("?,", len(f.addresses)-1)+"?)")
	}

	if len(f.keysWithCodec) > 0 {
		join := 0
		for key, vals := range f.keysWithCodec {
			if len(vals) > 0 {
				join++
				joinAlias := fmt.Sprintf("ee%d", join)
				joins = append(joins, fmt.Sprintf("event_entry %s ON e.event_id=%[1]s.event_id", joinAlias))
				clauses = append(clauses, fmt.Sprintf("%s.indexed=1 AND %[1]s.key=?", joinAlias))
				values = append(values, key)
				subclauses := make([]string, 0, len(vals))
				for _, val := range vals {
					subclauses = append(subclauses, fmt.Sprintf("(%s.value=? AND %[1]s.codec=?)", joinAlias))
					values = append(values, val.Value, val.Codec)
				}
				clauses = append(clauses, "("+strings.Join(subclauses, " OR ")+")")
			}
		}
	}

	s := `SELECT
				e.event_id,
				tm.height,
				tm.tipset_key_cid,
				e.emitter_addr,
				e.event_index,
				tm.message_cid,
				e.reverted,
				ee.flags,
				ee.key,
				ee.codec,
				ee.value
			FROM tipset_messages tm
			JOIN events e ON tm.message_id=e.message_id
			JOIN event_entry ee ON e.event_id=ee.event_id
			`

	if len(joins) > 0 {
		s = s + ", " + strings.Join(joins, ", ")
	}

	if len(clauses) > 0 {
		s = s + " WHERE " + strings.Join(clauses, " AND ")
	}

	// retain insertion order of event_entry rows with the implicit _rowid_ column
	s += " ORDER BY tm.height DESC, ee._rowid_ ASC"
	return values, s
}
