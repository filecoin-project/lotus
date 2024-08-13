package index

import (
	"context"
	"database/sql"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/chain/types"
)

func (ci *ChainIndexer) Apply(ctx context.Context, from, to *types.TipSet) error {
	return ci.indexFromTipset(ctx, from, to)
}

func (ci *ChainIndexer) indexFromTipset(ctx context.Context, from, to *types.TipSet) error {
	// We index messages and events by their inclusion tipset, NOT by their execution tipset
	// so let's index all the messages and events in the `from`tipset here as that is the inclusion tipset for
	// all the messages in `from`.
	tsKeyCid, err := toTipsetKeyCidBytes(from)
	if err != nil {
		return xerrors.Errorf("error computing tipset cid: %w", err)
	}
	tx, err := ci.db.BeginTx(ctx, nil)
	if err != nil {
		return xerrors.Errorf("beginning transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	// If this tipset already exists in the tipsets table, it means that we're seeing an apply for a tipset
	// that was previously reverted. However, the set of messages and the events produced by those messages
	// should remain unchanged given that the `tipset_cid` is the same as `tipset_cid` is a unique identifier for a tipset
	// that also identifies the execution context for the tipset (height, parent state etc).
	// So, we'll mark this tipset as "non-reverted" and skip re-indexing its messages and events.
	// TODO: Is this a valid assumption ? (looks like it but confirm)
	restored, err := ci.restoreTipsetIfExists(ctx, tx, tsKeyCid)
	if err != nil {
		return xerrors.Errorf("error restoring tipset: %w", err)
	}
	if restored {
		if err := tx.Commit(); err != nil {
			return xerrors.Errorf("error committing transaction: %w", err)
		}
		return nil
	}

	// Index tipset
	// Insert new tipset
	if _, err := tx.Stmt(ci.stmts.insertTipset).Exec(tsKeyCid, from.Height(), false); err != nil {
		return xerrors.Errorf("error inserting tipset: %w", err)
	}

	// prepare statements here instead of preparing them in the loop to avoid repeated preparations
	insertTipsetMsgStmt := tx.Stmt(ci.stmts.insertTipsetMsg)
	insertEventStmt := tx.Stmt(ci.stmts.insertEvent)
	insertEventEntryStmt := tx.Stmt(ci.stmts.insertEventEntry)

	ems, err := ci.loadExecutedMessages(ctx, from, to)
	if err != nil {
		return xerrors.Errorf("error loading executed messages: %w", err)
	}

	eventCount := 0
	addressLookups := make(map[abi.ActorID]address.Address)

	// Index messages and events in the tipset
	for i, em := range ems {
		// Insert message into tipset_messages table
		ethTxHash, err := toEthTxHashParam(em.msg)
		if err != nil {
			return xerrors.Errorf("computing eth tx hash: %w", err)
		}

		// Execute the statement with the hash parameter
		msgResult, err := insertTipsetMsgStmt.Exec(tsKeyCid, em.msg.Cid().Bytes(), i, ethTxHash)
		if err != nil {
			return xerrors.Errorf("error inserting tipset message: %w", err)
		}

		// Get the message_id of the inserted message
		messageID, err := msgResult.LastInsertId()
		if err != nil {
			return xerrors.Errorf("error getting last insert id for message: %w", err)
		}

		// Insert events for this message
		for _, event := range em.evs {
			event := event

			addr, found := addressLookups[event.Emitter]
			if !found {
				var ok bool
				addr, ok = ci.resolver(ctx, event.Emitter, to)
				if !ok {
					// not an address we will be able to match against
					continue
				}
				addressLookups[event.Emitter] = addr
			}

			// Insert event into events table
			eventResult, err := insertEventStmt.Exec(messageID, eventCount, addr.Bytes())
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
				_, err := insertEventEntryStmt.Exec(
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

	// Commit the transaction
	if err := tx.Commit(); err != nil {
		return xerrors.Errorf("error committing transaction: %w", err)
	}

	return nil
}

func (ci *ChainIndexer) restoreTipsetIfExists(ctx context.Context, tx *sql.Tx, tsKeyCid []byte) (bool, error) {
	// Check if the tipset already exists
	var exists bool
	if err := tx.Stmt(ci.stmts.tipsetExists).QueryRow(tsKeyCid).Scan(&exists); err != nil {
		return false, xerrors.Errorf("error checking if tipset exists: %w", err)
	}
	if exists {
		if _, err := tx.Stmt(ci.stmts.restoreTipset).Exec(tsKeyCid); err != nil {
			return false, xerrors.Errorf("error restoring tipset: %w", err)
		}
		return true, nil
	}
	return false, nil
}

func (ci *ChainIndexer) Revert(ctx context.Context, _ *types.TipSet, to *types.TipSet) error {
	// We're reverting the chain from the tipset at `from` to the tipset at `to`.
	// This means that the msg execution and associated events for msgs in `to` have been reverted
	// (because of deferred execution).
	// This is because we index messages and events by their inclusion tipset, NOT by their execution tipset
	// but the indexing is done only when we have the execution tipset so that we can access the execution
	// receipts and events.

	tx, err := ci.db.BeginTx(ctx, nil)
	if err != nil {
		return xerrors.Errorf("error beginning transaction: %w", err)
	}
	// rollback the transaction (a no-op if the transaction was already committed)
	defer func() { _ = tx.Rollback() }()

	tsKeyCid, err := toTipsetKeyCidBytes(to)
	if err != nil {
		return xerrors.Errorf("error getting tipset key cid: %w", err)
	}

	// mark tipset `to` as reverted
	if _, err := tx.Stmt(ci.stmts.revertTipset).Exec(tsKeyCid); err != nil {
		return xerrors.Errorf("error marking tipset as reverted: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return xerrors.Errorf("error committing transaction: %w", err)
	}

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
