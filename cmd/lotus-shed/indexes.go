package main

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"
	"github.com/mitchellh/go-homedir"
	"github.com/urfave/cli/v2"
	cbg "github.com/whyrusleeping/cbor-gen"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	amt4 "github.com/filecoin-project/go-amt-ipld/v4"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/crypto"
	"github.com/filecoin-project/go-state-types/exitcode"

	lapi "github.com/filecoin-project/lotus/api"
	bstore "github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/types/ethtypes"
	lcli "github.com/filecoin-project/lotus/cli"
)

const (
	// same as in chain/events/index.go
	eventExists      = `SELECT MAX(id) FROM event WHERE height=? AND tipset_key=? AND tipset_key_cid=? AND emitter_addr=? AND event_index=? AND message_cid=? AND message_index=?`
	eventCount       = `SELECT COUNT(*) FROM event WHERE tipset_key_cid=?`
	entryCount       = `SELECT COUNT(*) FROM event_entry JOIN event ON event_entry.event_id=event.id WHERE event.tipset_key_cid=?`
	insertEvent      = `INSERT OR IGNORE INTO event(height, tipset_key, tipset_key_cid, emitter_addr, event_index, message_cid, message_index, reverted) VALUES(?, ?, ?, ?, ?, ?, ?, ?)`
	insertEntry      = `INSERT OR IGNORE INTO event_entry(event_id, indexed, flags, key, codec, value) VALUES(?, ?, ?, ?, ?, ?)`
	upsertEventsSeen = `INSERT INTO events_seen(height, tipset_key_cid, reverted) VALUES(?, ?, false) ON CONFLICT(height, tipset_key_cid) DO UPDATE SET reverted=false`
	tipsetSeen       = `SELECT height,reverted FROM events_seen WHERE tipset_key_cid=?`

	// these queries are used to extract just the information used to reconstruct the event AMT from the database
	selectEventIdAndEmitter = `SELECT id, emitter_addr FROM event WHERE tipset_key_cid=? and message_cid=? ORDER BY event_index ASC`
	selectEventEntries      = `SELECT flags, key, codec, value FROM event_entry WHERE event_id=? ORDER BY _rowid_ ASC`
)

func withCategory(cat string, cmd *cli.Command) *cli.Command {
	cmd.Category = strings.ToUpper(cat)
	return cmd
}

var indexesCmd = &cli.Command{
	Name:            "indexes",
	Usage:           "Commands related to managing sqlite indexes",
	HideHelpCommand: true,
	Subcommands: []*cli.Command{
		withCategory("msgindex", backfillMsgIndexCmd),
		withCategory("msgindex", pruneMsgIndexCmd),
		withCategory("txhash", backfillTxHashCmd),
		withCategory("events", backfillEventsCmd),
		withCategory("events", inspectEventsCmd),
	},
}

var backfillEventsCmd = &cli.Command{
	Name:  "backfill-events",
	Usage: "Backfill the events.db for a number of epochs starting from a specified height and working backward",
	Flags: []cli.Flag{
		&cli.UintFlag{
			Name:  "from",
			Value: 0,
			Usage: "the tipset height to start backfilling from (0 is head of chain)",
		},
		&cli.IntFlag{
			Name:  "epochs",
			Value: 2000,
			Usage: "the number of epochs to backfill (working backwards)",
		},
		&cli.BoolFlag{
			Name:  "temporary-index",
			Value: false,
			Usage: "use a temporary index to speed up the backfill process",
		},
		&cli.BoolFlag{
			Name:  "vacuum",
			Value: false,
			Usage: "run VACUUM on the database after backfilling is complete; this will reclaim space from deleted rows, but may take a long time",
		},
	},
	Action: func(cctx *cli.Context) error {
		srv, err := lcli.GetFullNodeServices(cctx)
		if err != nil {
			return err
		}
		defer srv.Close() //nolint:errcheck

		api := srv.FullNodeAPI()
		ctx := lcli.ReqContext(cctx)

		// currTs will be the tipset where we start backfilling from
		currTs, err := api.ChainHead(ctx)
		if err != nil {
			return err
		}
		if cctx.IsSet("from") {
			// we need to fetch the tipset after the epoch being specified since we will need to advance currTs
			currTs, err = api.ChainGetTipSetAfterHeight(ctx, abi.ChainEpoch(cctx.Int("from")+1), currTs.Key())
			if err != nil {
				return err
			}
		}

		// advance currTs by one epoch and maintain prevTs as the previous tipset (this allows us to easily use the ChainGetParentMessages/Receipt API)
		prevTs := currTs
		currTs, err = api.ChainGetTipSet(ctx, currTs.Parents())
		if err != nil {
			return fmt.Errorf("failed to load tipset %s: %w", prevTs.Parents(), err)
		}

		epochs := cctx.Int("epochs")

		basePath, err := homedir.Expand(cctx.String("repo"))
		if err != nil {
			return err
		}

		log.Infof(
			"WARNING: If this command is run against a node that is currently collecting events with DisableHistoricFilterAPI=false, " +
				"it may cause the node to fail to record recent events due to the need to obtain an exclusive lock on the database for writes.")

		dbPath := path.Join(basePath, "sqlite", "events.db")
		db, err := sql.Open("sqlite3", dbPath+"?_txlock=immediate")
		if err != nil {
			return err
		}

		defer func() {
			err := db.Close()
			if err != nil {
				fmt.Printf("ERROR: closing db: %s", err)
			}
		}()

		if cctx.Bool("temporary-index") {
			log.Info("creating temporary index (tmp_event_backfill_index) on event table to speed up backfill")
			_, err := db.Exec("CREATE INDEX IF NOT EXISTS tmp_event_backfill_index ON event (height, tipset_key, tipset_key_cid, emitter_addr, event_index, message_cid, message_index, reverted);")
			if err != nil {
				return err
			}
		}

		addressLookups := make(map[abi.ActorID]address.Address)

		// TODO: We don't need this address resolution anymore once https://github.com/filecoin-project/lotus/issues/11594 lands
		resolveFn := func(ctx context.Context, emitter abi.ActorID, ts *types.TipSet) (address.Address, bool) {
			// we only want to match using f4 addresses
			idAddr, err := address.NewIDAddress(uint64(emitter))
			if err != nil {
				return address.Undef, false
			}

			actor, err := api.StateGetActor(ctx, idAddr, ts.Key())
			if err != nil || actor.DelegatedAddress == nil {
				return idAddr, true
			}

			return *actor.DelegatedAddress, true
		}

		isIndexedValue := func(b uint8) bool {
			// currently we mark the full entry as indexed if either the key
			// or the value are indexed; in the future we will need finer-grained
			// management of indices
			return b&(types.EventFlagIndexedKey|types.EventFlagIndexedValue) > 0
		}

		var totalEventsAffected int64
		var totalEntriesAffected int64

		stmtEventExists, err := db.Prepare(eventExists)
		if err != nil {
			return err
		}
		stmtInsertEvent, err := db.Prepare(insertEvent)
		if err != nil {
			return err
		}
		stmtInsertEntry, err := db.Prepare(insertEntry)
		if err != nil {
			return err
		}

		stmtUpsertEventSeen, err := db.Prepare(upsertEventsSeen)
		if err != nil {
			return err
		}

		processHeight := func(ctx context.Context, cnt int, msgs []lapi.Message, receipts []*types.MessageReceipt) error {
			var tx *sql.Tx
			for {
				var err error
				tx, err = db.BeginTx(ctx, nil)
				if err != nil {
					if err.Error() == "database is locked" {
						log.Warnf("database is locked, retrying in 200ms")
						time.Sleep(200 * time.Millisecond)
						continue
					}
					return err
				}
				break
			}
			defer tx.Rollback() //nolint:errcheck

			var eventsAffected int64
			var entriesAffected int64

			tsKeyCid, err := currTs.Key().Cid()
			if err != nil {
				return fmt.Errorf("failed to get tipset key cid: %w", err)
			}

			eventCount := 0
			// loop over each message receipt and backfill the events
			for idx, receipt := range receipts {
				msg := msgs[idx]

				if receipt.ExitCode != exitcode.Ok {
					continue
				}

				if receipt.EventsRoot == nil {
					continue
				}

				events, err := api.ChainGetEvents(ctx, *receipt.EventsRoot)
				if err != nil {
					return fmt.Errorf("failed to load events for tipset %s: %w", currTs, err)
				}

				for _, event := range events {
					addr, found := addressLookups[event.Emitter]
					if !found {
						var ok bool
						addr, ok = resolveFn(ctx, event.Emitter, currTs)
						if !ok {
							// not an address we will be able to match against
							continue
						}
						addressLookups[event.Emitter] = addr
					}

					// select the highest event id that exists in database, or null if none exists
					var entryID sql.NullInt64
					err = tx.Stmt(stmtEventExists).QueryRow(
						currTs.Height(),
						currTs.Key().Bytes(),
						tsKeyCid.Bytes(),
						addr.Bytes(),
						eventCount,
						msg.Cid.Bytes(),
						idx,
					).Scan(&entryID)
					if err != nil {
						return fmt.Errorf("error checking if event exists: %w", err)
					}

					// we already have this event
					if entryID.Valid {
						continue
					}

					// event does not exist, lets backfill it
					res, err := tx.Stmt(stmtInsertEvent).Exec(
						currTs.Height(),      // height
						currTs.Key().Bytes(), // tipset_key
						tsKeyCid.Bytes(),     // tipset_key_cid
						addr.Bytes(),         // emitter_addr
						eventCount,           // event_index
						msg.Cid.Bytes(),      // message_cid
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
						return fmt.Errorf("could not get rows affected: %w", err)
					}
					eventsAffected += rowsAffected

					// backfill the event entries
					for _, entry := range event.Entries {
						_, err := tx.Stmt(stmtInsertEntry).Exec(
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
							return fmt.Errorf("could not get rows affected: %w", err)
						}
						entriesAffected += rowsAffected
					}
					eventCount++
				}
			}

			// mark the tipset as processed
			_, err = tx.Stmt(stmtUpsertEventSeen).Exec(
				currTs.Height(),
				tsKeyCid.Bytes(),
			)
			if err != nil {
				return xerrors.Errorf("exec upsert events seen: %w", err)
			}

			err = tx.Commit()
			if err != nil {
				return fmt.Errorf("failed to commit transaction: %w", err)
			}

			log.Infof("[%d] backfilling actor events epoch:%d, eventsAffected:%d, entriesAffected:%d", cnt, currTs.Height(), eventsAffected, entriesAffected)

			totalEventsAffected += eventsAffected
			totalEntriesAffected += entriesAffected

			return nil
		}

		for i := 0; i < epochs; i++ {
			select {
			case <-ctx.Done():
				return nil
			default:
			}

			blockCid := prevTs.Blocks()[0].Cid()

			// get messages for the parent of the previous tipset (which will be currTs)
			msgs, err := api.ChainGetParentMessages(ctx, blockCid)
			if err != nil {
				return fmt.Errorf("failed to get parent messages for block %s: %w", blockCid, err)
			}

			// get receipts for the parent of the previous tipset (which will be currTs)
			receipts, err := api.ChainGetParentReceipts(ctx, blockCid)
			if err != nil {
				return fmt.Errorf("failed to get parent receipts for block %s: %w", blockCid, err)
			}

			if len(msgs) != len(receipts) {
				return fmt.Errorf("mismatched in message and receipt count: %d != %d", len(msgs), len(receipts))
			}

			err = processHeight(ctx, i, msgs, receipts)
			if err != nil {
				return err
			}

			// advance prevTs and currTs up the chain
			prevTs = currTs
			currTs, err = api.ChainGetTipSet(ctx, currTs.Parents())
			if err != nil {
				return fmt.Errorf("failed to load tipset %s: %w", currTs, err)
			}
		}

		log.Infof("backfilling events complete, totalEventsAffected:%d, totalEntriesAffected:%d", totalEventsAffected, totalEntriesAffected)

		if cctx.Bool("temporary-index") {
			log.Info("dropping temporary index (tmp_event_backfill_index) on event table")
			_, err := db.Exec("DROP INDEX IF EXISTS tmp_event_backfill_index;")
			if err != nil {
				fmt.Printf("ERROR: dropping index: %s", err)
			}
		}

		if cctx.Bool("vacuum") {
			log.Info("running VACUUM on the database")
			_, err := db.Exec("VACUUM;")
			if err != nil {
				return fmt.Errorf("failed to run VACUUM on the database: %w", err)
			}
		}

		return nil
	},
}

var inspectEventsCmd = &cli.Command{
	Name: "inspect-events",
	Usage: "Perform a health-check on the events.db for a number of epochs starting from a specified height and working backward. " +
		"Logs tipsets with problems and optionally logs tipsets without problems. Without specifying a fixed number of epochs, " +
		"the command will continue until it reaches a tipset that is not in the blockstore.",
	Flags: []cli.Flag{
		&cli.UintFlag{
			Name:  "from",
			Value: 0,
			Usage: "the tipset height to start inspecting from (0 is head of chain)",
		},
		&cli.IntFlag{
			Name:  "epochs",
			Value: 0,
			Usage: "the number of epochs to inspect (working backwards) [0 = until we reach a block we don't have]",
		},
		&cli.BoolFlag{
			Name:  "log-good",
			Usage: "log tipsets that have no detected problems",
			Value: false,
		},
	},
	Action: func(cctx *cli.Context) error {
		srv, err := lcli.GetFullNodeServices(cctx)
		if err != nil {
			return err
		}
		defer srv.Close() //nolint:errcheck

		api := srv.FullNodeAPI()
		ctx := lcli.ReqContext(cctx)

		// currTs will be the tipset where we start backfilling from
		currTs, err := api.ChainHead(ctx)
		if err != nil {
			return err
		}
		if cctx.IsSet("from") {
			// we need to fetch the tipset after the epoch being specified since we will need to advance currTs
			currTs, err = api.ChainGetTipSetAfterHeight(ctx, abi.ChainEpoch(cctx.Int("from")+1), currTs.Key())
			if err != nil {
				return err
			}
		}

		logGood := cctx.Bool("log-good")

		// advance currTs by one epoch and maintain prevTs as the previous tipset (this allows us to easily use the ChainGetParentMessages/Receipt API)
		prevTs := currTs
		currTs, err = api.ChainGetTipSet(ctx, currTs.Parents())
		if err != nil {
			return fmt.Errorf("failed to load tipset %s: %w", prevTs.Parents(), err)
		}

		epochs := cctx.Int("epochs")
		if epochs <= 0 {
			epochs = math.MaxInt32
		}

		basePath, err := homedir.Expand(cctx.String("repo"))
		if err != nil {
			return err
		}

		dbPath := path.Join(basePath, "sqlite", "events.db")
		db, err := sql.Open("sqlite3", dbPath+"?mode=ro")
		if err != nil {
			return err
		}

		defer func() {
			err := db.Close()
			if err != nil {
				fmt.Printf("ERROR: closing db: %s", err)
			}
		}()

		stmtEventCount, err := db.Prepare(eventCount)
		if err != nil {
			return err
		}
		stmtEntryCount, err := db.Prepare(entryCount)
		if err != nil {
			return err
		}
		stmtTipsetSeen, err := db.Prepare(tipsetSeen)
		if err != nil {
			return err
		}
		stmtSelectEventIdAndEmitter, err := db.Prepare(selectEventIdAndEmitter)
		if err != nil {
			return err
		}
		stmtSelectEventEntries, err := db.Prepare(selectEventEntries)
		if err != nil {
			return err
		}

		processHeight := func(ctx context.Context, messages []lapi.Message, receipts []*types.MessageReceipt) error {
			tsKeyCid, err := currTs.Key().Cid()
			if err != nil {
				return xerrors.Errorf("failed to get tipset key cid: %w", err)
			}

			var problems []string

			checkEventAndEntryCounts := func() error {
				// compare by counting events, using ChainGetEvents to load the events from the chain
				expectEvents, expectEntries, err := chainEventAndEntryCountsAt(ctx, currTs, receipts, api)
				if err != nil {
					return err
				}

				actualEvents, actualEntries, err := dbEventAndEntryCountsAt(currTs, stmtEventCount, stmtEntryCount)
				if err != nil {
					return err
				}

				if actualEvents != expectEvents {
					problems = append(problems, fmt.Sprintf("expected %d events, got %d", expectEvents, actualEvents))
				}
				if actualEntries != expectEntries {
					problems = append(problems, fmt.Sprintf("expected %d entries, got %d", expectEntries, actualEntries))
				}

				return nil
			}

			// Compare the AMT roots: we reconstruct the event AMT from the database data we have and
			// compare it with the on-chain AMT root from the receipt. If it's the same CID then we have
			// exactly the same event data. Any variation, in number of events, and even a single byte
			// in event data, will be considered a mismatch.

			// cache for address -> actorID because it's typical for tipsets to generate many events for
			// the same actors so we can try and avoid too many StateLookupID calls
			addrIdCache := make(map[address.Address]abi.ActorID)

			eventIndex := 0
			var hasEvents bool
			for msgIndex, receipt := range receipts {
				if receipt.EventsRoot == nil {
					continue
				}

				amtRoot, has, problem, err := amtRootForEvents(
					ctx,
					api,
					tsKeyCid,
					prevTs.Key(),
					stmtSelectEventIdAndEmitter,
					stmtSelectEventEntries,
					messages[msgIndex],
					addrIdCache,
				)
				if err != nil {
					return err
				}
				if has && !hasEvents {
					hasEvents = true
				}

				if problem != "" {
					problems = append(problems, problem)
				} else if amtRoot != *receipt.EventsRoot {
					problems = append(problems, fmt.Sprintf("events root mismatch for message %s", messages[msgIndex].Cid))
					// also provide more information about the mismatch
					if err := checkEventAndEntryCounts(); err != nil {
						return err
					}
				}

				eventIndex++
			}

			var seenHeight int
			var seenReverted int
			if err := stmtTipsetSeen.QueryRow(tsKeyCid.Bytes()).Scan(&seenHeight, &seenReverted); err != nil {
				if err == sql.ErrNoRows {
					if hasEvents {
						problems = append(problems, "not in events_seen table")
					} else {
						problems = append(problems, "zero-event epoch not in events_seen table")
					}
				} else {
					return xerrors.Errorf("failed to check if tipset is seen: %w", err)
				}
			} else {
				if seenHeight != int(currTs.Height()) {
					problems = append(problems, fmt.Sprintf("events_seen height mismatch (%d)", seenHeight))
				}
				if seenReverted != 0 {
					problems = append(problems, "events_seen marked as reverted")
				}
			}

			if len(problems) > 0 {
				_, _ = fmt.Fprintf(cctx.App.Writer, "✗ Epoch %d (%s): %s\n", currTs.Height(), tsKeyCid, strings.Join(problems, ", "))
			} else if logGood {
				_, _ = fmt.Fprintf(cctx.App.Writer, "✓ Epoch %d (%s)\n", currTs.Height(), tsKeyCid)
			}

			return nil
		}

		for i := 0; ctx.Err() == nil && i < epochs; i++ {
			// get receipts and messages for the parent of the previous tipset (which will be currTs)

			blockCid := prevTs.Blocks()[0].Cid()

			messages, err := api.ChainGetParentMessages(ctx, blockCid)
			if err != nil {
				_, _ = fmt.Fprintf(cctx.App.ErrWriter, "Missing parent messages for epoch %d (checked %d epochs)", prevTs.Height(), i)
				break
			}
			receipts, err := api.ChainGetParentReceipts(ctx, blockCid)
			if err != nil {
				_, _ = fmt.Fprintf(cctx.App.ErrWriter, "Missing parent receipts for epoch %d (checked %d epochs)", prevTs.Height(), i)
				break
			}

			if len(messages) != len(receipts) {
				return fmt.Errorf("mismatched in message and receipt count: %d != %d", len(messages), len(receipts))
			}

			err = processHeight(ctx, messages, receipts)
			if err != nil {
				return err
			}

			// advance prevTs and currTs up the chain
			prevTs = currTs
			currTs, err = api.ChainGetTipSet(ctx, currTs.Parents())
			if err != nil {
				return xerrors.Errorf("failed to load tipset %s: %w", currTs, err)
			}
		}

		return nil
	},
}

// amtRootForEvents generates the events AMT root CID for a given message's events, and returns
// whether the message has events, a string describing any non-fatal problem encountered,
// and a fatal error if one occurred.
func amtRootForEvents(
	ctx context.Context,
	api lapi.FullNode,
	tsKeyCid cid.Cid,
	prevTsKey types.TipSetKey,
	stmtSelectEventIdAndEmitter, stmtSelectEventEntries *sql.Stmt,
	message lapi.Message,
	addrIdCache map[address.Address]abi.ActorID,
) (cid.Cid, bool, string, error) {

	events := make([]cbg.CBORMarshaler, 0)

	rows, err := stmtSelectEventIdAndEmitter.Query(tsKeyCid.Bytes(), message.Cid.Bytes())
	if err != nil {
		return cid.Undef, false, "", xerrors.Errorf("failed to query events: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	for rows.Next() {
		var eventId int
		var emitterAddr []byte
		if err := rows.Scan(&eventId, &emitterAddr); err != nil {
			return cid.Undef, false, "", xerrors.Errorf("failed to scan row: %w", err)
		}

		addr, err := address.NewFromBytes(emitterAddr)
		if err != nil {
			return cid.Undef, false, "", xerrors.Errorf("failed to parse address: %w", err)
		}
		var actorId abi.ActorID
		if id, ok := addrIdCache[addr]; ok {
			actorId = id
		} else {
			if addr.Protocol() != address.ID {
				// use the previous tipset (height+1) to do an address lookup because the actor
				// may have been created in the current tipset (i.e. deferred execution means the
				// changed state isn't available until the next epoch)
				idAddr, err := api.StateLookupID(ctx, addr, prevTsKey)
				if err != nil {
					// TODO: fix this? we should be able to resolve all addresses
					return cid.Undef, false, fmt.Sprintf("failed to resolve address (%s), could not compare amt", addr.String()), nil
				}
				addr = idAddr
			}
			id, err := address.IDFromAddress(addr)
			if err != nil {
				return cid.Undef, false, "", xerrors.Errorf("failed to get ID from address: %w", err)
			}
			actorId = abi.ActorID(id)
			addrIdCache[addr] = actorId
		}

		event := types.Event{
			Emitter: actorId,
			Entries: make([]types.EventEntry, 0),
		}

		rows2, err := stmtSelectEventEntries.Query(eventId)
		if err != nil {
			return cid.Undef, false, "", xerrors.Errorf("failed to query event entries: %w", err)
		}
		defer func() {
			_ = rows2.Close()
		}()

		for rows2.Next() {
			var flags []byte
			var key string
			var codec uint64
			var value []byte
			if err := rows2.Scan(&flags, &key, &codec, &value); err != nil {
				return cid.Undef, false, "", xerrors.Errorf("failed to scan row: %w", err)
			}
			entry := types.EventEntry{
				Flags: flags[0],
				Key:   key,
				Codec: codec,
				Value: value,
			}
			event.Entries = append(event.Entries, entry)
		}

		events = append(events, &event)
	}

	// construct the AMT from our slice to an in-memory IPLD store just so we can get the root,
	// we don't need the blocks themselves
	root, err := amt4.FromArray(ctx, cbor.NewCborStore(bstore.NewMemory()), events, amt4.UseTreeBitWidth(types.EventAMTBitwidth))
	if err != nil {
		return cid.Undef, false, "", xerrors.Errorf("failed to create AMT: %w", err)
	}
	return root, len(events) > 0, "", nil
}

func chainEventAndEntryCountsAt(ctx context.Context, ts *types.TipSet, receipts []*types.MessageReceipt, api lapi.FullNode) (int, int, error) {
	var expectEvents int
	var expectEntries int
	for _, receipt := range receipts {
		if receipt.ExitCode != exitcode.Ok || receipt.EventsRoot == nil {
			continue
		}
		events, err := api.ChainGetEvents(ctx, *receipt.EventsRoot)
		if err != nil {
			return 0, 0, xerrors.Errorf("failed to load events for tipset %s: %w", ts, err)
		}
		expectEvents += len(events)
		for _, event := range events {
			expectEntries += len(event.Entries)
		}
	}
	return expectEvents, expectEntries, nil
}

func dbEventAndEntryCountsAt(ts *types.TipSet, stmtEventCount, stmtEntryCount *sql.Stmt) (int, int, error) {
	tsKeyCid, err := ts.Key().Cid()
	if err != nil {
		return 0, 0, xerrors.Errorf("failed to get tipset key cid: %w", err)
	}
	var actualEvents int
	if err := stmtEventCount.QueryRow(tsKeyCid.Bytes()).Scan(&actualEvents); err != nil {
		return 0, 0, xerrors.Errorf("failed to count events for epoch %d (tsk CID %s): %w", ts.Height(), tsKeyCid, err)
	}
	var actualEntries int
	if err := stmtEntryCount.QueryRow(tsKeyCid.Bytes()).Scan(&actualEntries); err != nil {
		return 0, 0, xerrors.Errorf("failed to count entries for epoch %d (tsk CID %s): %w", ts.Height(), tsKeyCid, err)
	}
	return actualEvents, actualEntries, nil
}

var backfillMsgIndexCmd = &cli.Command{
	Name:  "backfill-msgindex",
	Usage: "Backfill the msgindex.db for a number of epochs starting from a specified height",
	Flags: []cli.Flag{
		&cli.IntFlag{
			Name:  "from",
			Value: 0,
			Usage: "height to start the backfill; uses the current head if omitted",
		},
		&cli.IntFlag{
			Name:  "epochs",
			Value: 1800,
			Usage: "number of epochs to backfill; defaults to 1800 (2 finalities)",
		},
	},
	Action: func(cctx *cli.Context) error {
		api, closer, err := lcli.GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}

		defer closer()
		ctx := lcli.ReqContext(cctx)

		curTs, err := api.ChainHead(ctx)
		if err != nil {
			return err
		}

		startHeight := int64(cctx.Int("from"))
		if startHeight == 0 {
			startHeight = int64(curTs.Height()) - 1
		}
		epochs := cctx.Int("epochs")

		basePath, err := homedir.Expand(cctx.String("repo"))
		if err != nil {
			return err
		}

		dbPath := path.Join(basePath, "sqlite", "msgindex.db")
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

		insertStmt, err := db.Prepare("INSERT OR IGNORE INTO messages (cid, tipset_cid, epoch) VALUES (?, ?, ?)")
		if err != nil {
			return err
		}

		var nrRowsAffected int64
		for i := 0; i < epochs; i++ {
			epoch := abi.ChainEpoch(startHeight - int64(i))

			if i%100 == 0 {
				log.Infof("%d/%d processing epoch:%d, nrRowsAffected:%d", i, epochs, epoch, nrRowsAffected)
			}

			ts, err := api.ChainGetTipSetByHeight(ctx, epoch, curTs.Key())
			if err != nil {
				return fmt.Errorf("failed to get tipset at epoch %d: %w", epoch, err)
			}

			tsCid, err := ts.Key().Cid()
			if err != nil {
				return fmt.Errorf("failed to get tipset cid at epoch %d: %w", epoch, err)
			}

			msgs, err := api.ChainGetMessagesInTipset(ctx, ts.Key())
			if err != nil {
				return fmt.Errorf("failed to get messages in tipset at epoch %d: %w", epoch, err)
			}

			for _, msg := range msgs {
				key := msg.Cid.String()
				tskey := tsCid.String()
				res, err := insertStmt.Exec(key, tskey, int64(epoch))
				if err != nil {
					return fmt.Errorf("failed to insert message cid %s in tipset %s at epoch %d: %w", key, tskey, epoch, err)
				}
				rowsAffected, err := res.RowsAffected()
				if err != nil {
					return fmt.Errorf("failed to get rows affected for message cid %s in tipset %s at epoch %d: %w", key, tskey, epoch, err)
				}
				nrRowsAffected += rowsAffected
			}
		}

		log.Infof("Done backfilling, nrRowsAffected:%d", nrRowsAffected)

		return nil
	},
}

var pruneMsgIndexCmd = &cli.Command{
	Name:  "prune-msgindex",
	Usage: "Prune the msgindex.db for messages included before a given epoch",
	Flags: []cli.Flag{
		&cli.IntFlag{
			Name:  "from",
			Usage: "height to start the prune; if negative it indicates epochs from current head",
		},
	},
	Action: func(cctx *cli.Context) error {
		api, closer, err := lcli.GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}

		defer closer()
		ctx := lcli.ReqContext(cctx)

		startHeight := int64(cctx.Int("from"))
		if startHeight < 0 {
			curTs, err := api.ChainHead(ctx)
			if err != nil {
				return err
			}

			startHeight += int64(curTs.Height())

			if startHeight < 0 {
				return xerrors.Errorf("bogus start height %d", startHeight)
			}
		}

		basePath, err := homedir.Expand(cctx.String("repo"))
		if err != nil {
			return err
		}

		dbPath := path.Join(basePath, "sqlite", "msgindex.db")
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

		tx, err := db.Begin()
		if err != nil {
			return err
		}

		if _, err := tx.Exec("DELETE FROM messages WHERE epoch < ?", startHeight); err != nil {
			if err := tx.Rollback(); err != nil {
				fmt.Printf("ERROR: rollback: %s", err)
			}
			return err
		}

		if err := tx.Commit(); err != nil {
			return err
		}

		return nil
	},
}

var backfillTxHashCmd = &cli.Command{
	Name:  "backfill-txhash",
	Usage: "Backfills the txhash.db for a number of epochs starting from a specified height",
	Flags: []cli.Flag{
		&cli.UintFlag{
			Name:  "from",
			Value: 0,
			Usage: "the tipset height to start backfilling from (0 is head of chain)",
		},
		&cli.IntFlag{
			Name:  "epochs",
			Value: 2000,
			Usage: "the number of epochs to backfill",
		},
	},
	Action: func(cctx *cli.Context) error {
		api, closer, err := lcli.GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		ctx := lcli.ReqContext(cctx)

		curTs, err := api.ChainHead(ctx)
		if err != nil {
			return err
		}

		startHeight := int64(cctx.Int("from"))
		if startHeight == 0 {
			startHeight = int64(curTs.Height()) - 1
		}

		epochs := cctx.Int("epochs")

		basePath, err := homedir.Expand(cctx.String("repo"))
		if err != nil {
			return err
		}

		dbPath := filepath.Join(basePath, "sqlite", "txhash.db")
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

		insertStmt, err := db.Prepare("INSERT OR IGNORE INTO eth_tx_hashes(hash, cid) VALUES(?, ?)")
		if err != nil {
			return err
		}

		var totalRowsAffected int64 = 0
		for i := 0; i < epochs; i++ {
			epoch := abi.ChainEpoch(startHeight - int64(i))

			select {
			case <-cctx.Done():
				fmt.Println("request cancelled")
				return nil
			default:
			}

			curTsk := curTs.Parents()
			execTs, err := api.ChainGetTipSet(ctx, curTsk)
			if err != nil {
				return fmt.Errorf("failed to call ChainGetTipSet for %s: %w", curTsk, err)
			}

			if i%100 == 0 {
				log.Infof("%d/%d processing epoch:%d", i, epochs, epoch)
			}

			for _, blockheader := range execTs.Blocks() {
				blkMsgs, err := api.ChainGetBlockMessages(ctx, blockheader.Cid())
				if err != nil {
					log.Infof("Could not get block messages at epoch: %d, stopping walking up the chain", epoch)
					epochs = i
					break
				}

				for _, smsg := range blkMsgs.SecpkMessages {
					if smsg.Signature.Type != crypto.SigTypeDelegated {
						continue
					}

					tx, err := ethtypes.EthTransactionFromSignedFilecoinMessage(smsg)
					if err != nil {
						return fmt.Errorf("failed to convert from signed message: %w at epoch: %d", err, epoch)
					}

					hash, err := tx.TxHash()
					if err != nil {
						return fmt.Errorf("failed to calculate hash for ethTx: %w at epoch: %d", err, epoch)
					}

					res, err := insertStmt.Exec(hash.String(), smsg.Cid().String())
					if err != nil {
						return fmt.Errorf("error inserting tx mapping to db: %s at epoch: %d", err, epoch)
					}

					rowsAffected, err := res.RowsAffected()
					if err != nil {
						return fmt.Errorf("error getting rows affected: %s at epoch: %d", err, epoch)
					}

					if rowsAffected > 0 {
						log.Debugf("Inserted txhash %s, cid: %s at epoch: %d", hash.String(), smsg.Cid().String(), epoch)
					}

					totalRowsAffected += rowsAffected
				}
			}

			curTs = execTs
		}

		log.Infof("Done, inserted %d missing txhashes", totalRowsAffected)

		return nil
	},
}
