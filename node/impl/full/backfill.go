package full

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"

	"github.com/multiformats/go-varint"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	builtintypes "github.com/filecoin-project/go-state-types/builtin"
	"github.com/filecoin-project/go-state-types/exitcode"

	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/types"
)

func isIndexedValue(b uint8) bool {
	// currently we mark the full entry as indexed if either the key
	// or the value are indexed; in the future we will need finer-grained
	// management of indices
	return b&(types.EventFlagIndexedKey|types.EventFlagIndexedValue) > 0
}

// BackfillEvents walks up the chain from the current head and backfills all actor events that were not stored in
// the event.db database.
func BackfillEvents(ctx context.Context, chainapi ChainAPI, stateApi StateAPI, stateManager *stmgr.StateManager) error {
	resolveFn := func(ctx context.Context, emitter abi.ActorID, ts *types.TipSet) (address.Address, bool) {
		// we only want to match using f4 addresses
		idAddr, err := address.NewIDAddress(uint64(emitter))
		if err != nil {
			return address.Undef, false
		}

		actor, err := stateManager.LoadActor(ctx, idAddr, ts)
		if err != nil || actor.Address == nil {
			return address.Undef, false
		}

		// if robust address is not f4 then we won't match against it so bail early
		if actor.Address.Protocol() != address.Delegated {
			return address.Undef, false
		}
		// we have an f4 address, make sure it's assigned by the EAM
		if namespace, _, err := varint.FromUvarint(actor.Address.Payload()); err != nil || namespace != builtintypes.EthereumAddressManagerActorID {
			return address.Undef, false
		}
		return *actor.Address, true
	}

	dbPath := filepath.Join(chainapi.Repo.Path(), "sqlite", "events.db")
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return err
	}

	defer func() {
		err := db.Close()
		if err != nil {
			fmt.Printf("ERROR: closing db: %s", err)
		}
	}()

	stmtSelectEvent, err := db.Prepare("SELECT MAX(id) from event WHERE height=? AND tipset_key=? and tipset_key_cid=? and emitter_addr=? and event_index=? and message_cid=? and message_index=? and reverted=false")
	if err != nil {
		return err
	}
	stmtSelectEntry, err := db.Prepare("SELECT EXISTS(SELECT 1 from event_entry WHERE event_id=? and indexed=? and flags=? and key=? and codec=? and value=?)")
	if err != nil {
		return err
	}
	stmtEvent, err := db.Prepare("INSERT INTO event (height, tipset_key, tipset_key_cid, emitter_addr, event_index, message_cid, message_index, reverted) VALUES(?, ?, ?, ?, ?, ?, ?, ?)")
	if err != nil {
		return err
	}
	stmtEntry, err := db.Prepare("INSERT INTO event_entry(event_id, indexed, flags, key, codec, value) VALUES(?, ?, ?, ?, ?, ?)")
	if err != nil {
		return err
	}

	checkTs := chainapi.Chain.GetHeaviestTipSet()

	// look into the event_backfilled table to see where we left off
	var height sql.NullInt32
	err = db.QueryRow("SELECT MIN(height) FROM event_backfilled").Scan(&height)
	if err != nil {
		return err
	}
	if !height.Valid {
		// no backfilling has been done yet, so we need to start from the beginning (current head)
		_, err := db.Exec("INSERT INTO event_backfilled(height) VALUES(?)", checkTs.Height())
		if err != nil {
			return err
		}
		log.Infof("Starting backfill from current head %d", checkTs.Height())
	} else {
		checkTs, err = chainapi.ChainGetTipSetByHeight(ctx, abi.ChainEpoch(height.Int32), checkTs.Key())
		if err != nil {
			return err
		}
		log.Infof("Starting backfill from epoch %d", checkTs.Height())
	}

	addressLookups := make(map[abi.ActorID]address.Address)

	var eventsAffected int64
	var entriesAffected int64
	for i := 0; i < int(checkTs.Height()); i++ {
		select {
		case <-ctx.Done():
			log.Infoln("request cancelled")
			return nil
		default:
		}

		execTsk := checkTs.Parents()
		execTs, err := chainapi.Chain.GetTipSetFromKey(ctx, execTsk)
		if err != nil {
			return fmt.Errorf("failed to load tipset %s: %w", execTsk, err)
		}

		if i%100 == 0 {
			log.Infof("[%d] backfilling actor events epoch:%d, eventsAffected:%d, entriesAffected:%d", i, execTs.Height(), eventsAffected, entriesAffected)
		}

		_, rcptRoot, err := stateManager.TipSetState(ctx, execTs)
		if err != nil {
			return fmt.Errorf("failed to load tipset state %s: %w", execTsk, err)
		}

		msgs, err := chainapi.Chain.MessagesForTipset(ctx, execTs)
		if err != nil {
			return fmt.Errorf("failed to load messages for tipset %s: %w", execTsk, err)
		}

		rcpts, err := chainapi.Chain.ReadReceipts(ctx, rcptRoot)
		if err != nil {
			return fmt.Errorf("failed to load receipts for tipset %s: %w", execTsk, err)
		}

		if len(msgs) != len(rcpts) {
			return fmt.Errorf("mismatched message and receipt count %d vs %d", len(msgs), len(rcpts))
		}

		for idx, rcpt := range rcpts {
			msg := msgs[idx]

			if rcpt.ExitCode != exitcode.Ok {
				continue
			}
			if rcpt.EventsRoot == nil {
				continue
			}

			events, err := chainapi.ChainGetEvents(ctx, *rcpt.EventsRoot)
			if err != nil {
				return fmt.Errorf("failed to load events for tipset %s: %w", execTsk, err)
			}

			for eventIdx, event := range events {
				addr, found := addressLookups[event.Emitter]
				if !found {
					var ok bool
					addr, ok = resolveFn(ctx, event.Emitter, execTs)
					if !ok {
						// not an address we will be able to match against
						continue
					}
					addressLookups[event.Emitter] = addr
				}

				//log.Infof("[BackfillEvents] %d %d: event:%+v", eventIdx, execTs.Height(), event)
				tsKeyCid, err := execTs.Key().Cid()
				if err != nil {
					return fmt.Errorf("failed to get tipset key cid: %w", err)
				}

				// select the highest event id that exists in database, or null if none exists
				var entryID sql.NullInt64
				err = stmtSelectEvent.QueryRow(
					execTs.Height(),
					execTs.Key().Bytes(),
					tsKeyCid.Bytes(),
					addr.Bytes(),
					eventIdx,
					msg.Cid().Bytes(),
					idx,
				).Scan(&entryID)
				if err != nil {
					return fmt.Errorf("error checking if event exists: %w", err)
				}

				if !entryID.Valid {
					// event does not exist, lets backfill it
					res, err := stmtEvent.Exec(
						execTs.Height(),      // height
						execTs.Key().Bytes(), // tipset_key
						tsKeyCid.Bytes(),     // tipset_key_cid
						addr.Bytes(),         // emitter_addr
						eventIdx,             // event_index
						msg.Cid().Bytes(),    // message_cid
						idx,                  // message_index
						false,                // reverted
					)

					if err != nil {
						return fmt.Errorf("error inserting event: %w", err)
					}

					entryID.Int64, err = res.LastInsertId()
					if err != nil {
						return fmt.Errorf("could not get last insert id: %w", err)
					}

					rowsAffected, err := res.RowsAffected()
					if err != nil {
						return fmt.Errorf("error getting rows affected: %s", err)
					}

					eventsAffected += rowsAffected
				}

				//log.Infof("[BackfillEvents] entryID:%d", entryID.Int64)
				for _, entry := range event.Entries {
					//log.Infof("    [BackfillEvents] entry:%+v", entry)

					// check if entry exists
					var exists bool
					err = stmtSelectEntry.QueryRow(
						entryID.Int64,
						isIndexedValue(entry.Flags),
						[]byte{entry.Flags},
						entry.Key,
						entry.Codec,
						entry.Value,
					).Scan(&exists)
					if err != nil {
						return fmt.Errorf("error checking if entry exists: %w", err)
					}

					if !exists {
						// entry does not exist, lets backfill it
						res, err := stmtEntry.Exec(
							entryID.Int64,               // event_id
							isIndexedValue(entry.Flags), // indexed
							[]byte{entry.Flags},         // flags
							entry.Key,                   // key
							entry.Codec,                 // codec
							entry.Value,                 // value
						)
						if err != nil {
							return fmt.Errorf("error inserting entry: %w", err)
						}

						rowsAffected, err := res.RowsAffected()
						if err != nil {
							return fmt.Errorf("error getting rows affected: %s", err)
						}
						entriesAffected += rowsAffected
					}
				}
			}
		}

		_, err = db.Exec("UPDATE event_backfilled set height =?", execTs.Height())
		if err != nil {
			return fmt.Errorf("error updating backfill at height %d: %w", execTs.Height(), err)
		}

		checkTs = execTs
	}

	log.Infof("backfilling events complete, eventsAffected:%d, entriesAffected:%d", eventsAffected, entriesAffected)

	return nil
}
