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

	"github.com/mitchellh/go-homedir"
	"github.com/urfave/cli/v2"
	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/crypto"
	"github.com/filecoin-project/go-state-types/exitcode"

	lapi "github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/ethhashlookup"
	"github.com/filecoin-project/lotus/chain/events/filter"
	"github.com/filecoin-project/lotus/chain/index"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/types/ethtypes"
	lcli "github.com/filecoin-project/lotus/cli"
)

const (
	// same as in chain/events/index.go
	eventExists                = `SELECT MAX(id) FROM event WHERE height=? AND tipset_key=? AND tipset_key_cid=? AND emitter_addr=? AND event_index=? AND message_cid=? AND message_index=?`
	eventCount                 = `SELECT COUNT(*) FROM event WHERE tipset_key_cid=?`
	entryCount                 = `SELECT COUNT(*) FROM event_entry JOIN event ON event_entry.event_id=event.id WHERE event.tipset_key_cid=?`
	insertEvent                = `INSERT OR IGNORE INTO event(height, tipset_key, tipset_key_cid, emitter_addr, event_index, message_cid, message_index, reverted) VALUES(?, ?, ?, ?, ?, ?, ?, ?)`
	insertEntry                = `INSERT OR IGNORE INTO event_entry(event_id, indexed, flags, key, codec, value) VALUES(?, ?, ?, ?, ?, ?)`
	upsertEventsSeen           = `INSERT INTO events_seen(height, tipset_key_cid, reverted) VALUES(?, ?, false) ON CONFLICT(height, tipset_key_cid) DO UPDATE SET reverted=false`
	tipsetSeen                 = `SELECT height,reverted FROM events_seen WHERE tipset_key_cid=?`
	selectEventIdsFromHeight   = `SELECT id, height FROM event WHERE height < ?`
	selectEventIdsBetweenRange = `SELECT id, height FROM event WHERE height < ? AND height >= ?`
)

// delete queries
const (
	deleteMsgIndexFromStartHeight = `DELETE FROM messages WHERE epoch < ?`
	deleteMsgIndexBetweenRange    = `DELETE FROM messages WHERE epoch < ? AND epoch >= ?`

	deleteTxHashIndexByCID = `DELETE FROM eth_tx_hashes WHERE cid = ?`

	deleteEventsByIDs        = `DELETE FROM event WHERE id IN (%s)`
	deleteEventEntriesByIDs  = `DELETE FROM event_entry WHERE event_id IN (%s)`
	deleteEventSeenByHeights = `DELETE FROM events_seen WHERE height IN (%s)`
)

// database related constants
const (
	databaseType   = "sqlite"
	databaseDriver = "sqlite3"
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
		withCategory("indexes", pruneAllIndexesCmd),
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

				for eventIdx, event := range events {
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
						eventIdx,
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
						eventIdx,             // event_index
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

		processHeight := func(ctx context.Context, ts *types.TipSet, receipts []*types.MessageReceipt) error {
			tsKeyCid, err := ts.Key().Cid()
			if err != nil {
				return fmt.Errorf("failed to get tipset key cid: %w", err)
			}

			var expectEvents int
			var expectEntries int

			for _, receipt := range receipts {
				if receipt.ExitCode != exitcode.Ok || receipt.EventsRoot == nil {
					continue
				}
				events, err := api.ChainGetEvents(ctx, *receipt.EventsRoot)
				if err != nil {
					return fmt.Errorf("failed to load events for tipset %s: %w", currTs, err)
				}
				expectEvents += len(events)
				for _, event := range events {
					expectEntries += len(event.Entries)
				}
			}

			var problems []string

			var seenHeight int
			var seenReverted int
			if err := stmtTipsetSeen.QueryRow(tsKeyCid.Bytes()).Scan(&seenHeight, &seenReverted); err != nil {
				if err == sql.ErrNoRows {
					if expectEvents > 0 {
						problems = append(problems, "not in events_seen table")
					} else {
						problems = append(problems, "zero-event epoch not in events_seen table")
					}
				} else {
					return fmt.Errorf("failed to check if tipset is seen: %w", err)
				}
			} else {
				if seenHeight != int(ts.Height()) {
					problems = append(problems, fmt.Sprintf("events_seen height mismatch (%d)", seenHeight))
				}
				if seenReverted != 0 {
					problems = append(problems, "events_seen marked as reverted")
				}
			}

			var actualEvents int
			if err := stmtEventCount.QueryRow(tsKeyCid.Bytes()).Scan(&actualEvents); err != nil {
				return fmt.Errorf("failed to count events for epoch %d (tsk CID %s): %w", ts.Height(), tsKeyCid, err)
			}
			var actualEntries int
			if err := stmtEntryCount.QueryRow(tsKeyCid.Bytes()).Scan(&actualEntries); err != nil {
				return fmt.Errorf("failed to count entries for epoch %d (tsk CID %s): %w", ts.Height(), tsKeyCid, err)
			}

			if actualEvents != expectEvents {
				problems = append(problems, fmt.Sprintf("expected %d events, got %d", expectEvents, actualEvents))
			}
			if actualEntries != expectEntries {
				problems = append(problems, fmt.Sprintf("expected %d entries, got %d", expectEntries, actualEntries))
			}

			if len(problems) > 0 {
				_, _ = fmt.Fprintf(cctx.App.Writer, "✗ Epoch %d (%s): %s\n", ts.Height(), tsKeyCid, problems)
			} else if logGood {
				_, _ = fmt.Fprintf(cctx.App.Writer, "✓ Epoch %d (%s)\n", ts.Height(), tsKeyCid)
			}

			return nil
		}

		for i := 0; ctx.Err() == nil && i < epochs; i++ {
			// get receipts for the parent of the previous tipset (which will be currTs)
			receipts, err := api.ChainGetParentReceipts(ctx, prevTs.Blocks()[0].Cid())
			if err != nil {
				_, _ = fmt.Fprintf(cctx.App.ErrWriter, "Missing parent receipts for epoch %d (checked %d epochs)", prevTs.Height(), i)
				break
			}

			err = processHeight(ctx, currTs, receipts)
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

		return nil
	},
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

// pruneAllIndexesCmd is a command that prunes all the indexes
var pruneAllIndexesCmd = &cli.Command{
	Name:  "prune-all-indexes",
	Usage: "Prune all the index for a number of epochs starting from a specified height",
	Flags: []cli.Flag{
		&cli.IntFlag{
			Name:  "from",
			Value: 0,
			Usage: "the tipset height to start backfilling from (0 is head of chain)",
		},
		&cli.IntFlag{
			Name:  "epochs",
			Value: 0,
			Usage: "the number of epochs to prune (working backwards, 0 means prune all from the specified height)",
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

		basePath, err := homedir.Expand(cctx.String("repo"))
		if err != nil {
			return err
		}

		startHeight := cctx.Int64("from")
		if startHeight == 0 {
			currTs, err := api.ChainHead(ctx)
			if err != nil {
				return err
			}

			startHeight += int64(currTs.Height())

			if startHeight < 0 || startHeight == 0 {
				return xerrors.Errorf("bogus start height %d", startHeight)
			}
		}

		epochs := cctx.Int64("epochs")

		var g = new(errgroup.Group)

		g.Go(func() error {
			if err := pruneMsgIndex(ctx, basePath, startHeight, epochs); err != nil {
				return xerrors.Errorf("error pruning msgindex: %w", err)
			}

			return nil
		})

		g.Go(func() error {
			if err := pruneTxIndex(ctx, api, basePath, startHeight, epochs); err != nil {
				return xerrors.Errorf("error pruning txindex: %w", err)
			}

			return nil
		})

		g.Go(func() error {
			if err := pruneEventsIndex(ctx, basePath, startHeight, epochs); err != nil {
				return xerrors.Errorf("error pruning events index: %w", err)
			}

			return nil
		})

		return g.Wait()
	},
}

// pruneEventsIndex is a helper function that prunes the events index.db for a number of epochs starting from a specified height
func pruneEventsIndex(_ context.Context, basePath string, startHeight, epochs int64) error {
	dbPath := path.Join(basePath, databaseType, filter.DefaultDbFilename)
	db, err := sql.Open(databaseDriver, dbPath+"?_txlock=immediate")
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

	var rows *sql.Rows
	var res sql.Result
	if epochs == 0 {
		rows, err = tx.Query(selectEventIdsFromHeight, startHeight)
	} else {
		lowerBound := startHeight - epochs
		if lowerBound < 0 {
			lowerBound = 0
		}

		rows, err = tx.Query(selectEventIdsBetweenRange, startHeight, lowerBound)
	}

	if err != nil {
		if err := tx.Rollback(); err != nil {
			fmt.Printf("ERROR: rollback: %s", err)
		}
		return err
	}

	defer func() {
		_ = rows.Close()
	}()

	deleteEventQuery, deleteEventEntriesQuery, deleteEventSeenQuery, err := getEventsDBDeleteQueries(rows)
	if err != nil {
		return xerrors.Errorf("error getting delete query: %w", err)
	}

	res, err = tx.Exec(deleteEventQuery)
	if err != nil {
		if err := tx.Rollback(); err != nil {
			fmt.Printf("ERROR: rollback event: %s", err)
		}
		return err
	}

	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return xerrors.Errorf("error getting event table rows affected: %s", err)
	}

	log.Infof("pruned %s table, rows affected: %d", "event", rowsAffected)

	res, err = tx.Exec(deleteEventEntriesQuery)
	if err != nil {
		if err := tx.Rollback(); err != nil {
			fmt.Printf("ERROR: rollback event_entries: %s", err)
		}
		return err
	}

	rowsAffected, err = res.RowsAffected()
	if err != nil {
		return xerrors.Errorf("error getting event entries table rows affected: %s", err)
	}

	log.Infof("pruned %s table, rows affected: %d", "event_entries", rowsAffected)

	res, err = tx.Exec(deleteEventSeenQuery)
	if err != nil {
		if err := tx.Rollback(); err != nil {
			fmt.Printf("ERROR: rollback event_seen: %s", err)
		}
		return err
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	rowsAffected, err = res.RowsAffected()
	if err != nil {
		return xerrors.Errorf("error getting event seen table rows affected: %s", err)
	}

	log.Infof("pruned %s table, rows affected: %d", "event_seen", rowsAffected)

	return nil
}

func getEventsDBDeleteQueries(rows *sql.Rows) (string, string, string, error) {
	var (
		eventIds []int64
		heights  []int64
	)
	for rows.Next() {
		var (
			id     int64
			height int64
		)
		if err := rows.Scan(&id, &height); err != nil {
			return "", "", "", xerrors.Errorf("error scanning event id: %w", err)
		}
		eventIds = append(eventIds, id)
		heights = append(heights, height)
	}

	if err := rows.Err(); err != nil {
		return "", "", "", xerrors.Errorf("error iterating event ids: %w", err)
	}

	if len(eventIds) == 0 {
		log.Info("No events to delete")
		return "", "", "", nil
	}

	// Delete event entries
	idPlaceholders := strings.Repeat("?,", len(eventIds))
	idPlaceholders = idPlaceholders[:len(idPlaceholders)-1] // Remove trailing comma

	deleteEventTableQuery := fmt.Sprintf(deleteEventsByIDs, idPlaceholders)
	deleteEventEntriesTableQuery := fmt.Sprintf(deleteEventEntriesByIDs, idPlaceholders)

	heightPlaceholders := strings.Repeat("?,", len(heights))
	heightPlaceholders = heightPlaceholders[:len(heightPlaceholders)-1] // Remove trailing comma

	deleteEventSeenTableQuery := fmt.Sprintf(deleteEventSeenByHeights, heightPlaceholders)

	return deleteEventTableQuery, deleteEventEntriesTableQuery, deleteEventSeenTableQuery, nil
}

// pruneTxIndex is a helper function that prunes the txindex.db for a number of epochs starting from a specified height
func pruneTxIndex(ctx context.Context, api lapi.FullNode, basePath string, startHeight, epochs int64) error {
	dbPath := path.Join(basePath, databaseType, ethhashlookup.DefaultDbFilename)
	db, err := sql.Open(databaseDriver, dbPath)
	if err != nil {
		return err
	}

	defer func() {
		err := db.Close()
		if err != nil {
			fmt.Printf("ERROR: closing db: %s", err)
		}
	}()

	// we want to delete the txhash for the previous epoch
	startHeight = startHeight - 1

	deleteStmt, err := db.Prepare(deleteTxHashIndexByCID)
	if err != nil {
		return xerrors.Errorf("failed to prepare tx hash index delete statement: %w", err)
	}

	currTs, err := api.ChainHead(ctx)
	if err != nil {
		return err
	}

	var totalRowsAffected int64 = 0
	for i := 0; i < int(epochs); i++ {
		epoch := abi.ChainEpoch(startHeight - int64(i))

		select {
		case <-ctx.Done():
			fmt.Println("request cancelled")
			return nil
		default:
		}

		currTsk := currTs.Parents()
		execTs, err := api.ChainGetTipSet(ctx, currTsk)
		if err != nil {
			return fmt.Errorf("failed to call ChainGetTipSet for %s: %w", currTsk, err)
		}

		for _, blockheader := range execTs.Blocks() {
			blkMsgs, err := api.ChainGetBlockMessages(ctx, blockheader.Cid())
			if err != nil {
				log.Infof("failed to get block messages at epoch: %d", epoch)
				continue // should we break here?
			}

			for _, smsg := range blkMsgs.SecpkMessages {
				// we are only storing delegated messages.
				// This will reduce unnecessary calls to db.
				if smsg.Signature.Type != crypto.SigTypeDelegated {
					continue
				}

				cid := smsg.Cid().String()
				res, err := deleteStmt.Exec(cid)
				if err != nil {
					return fmt.Errorf("failed to delete tx hash index at cid: %s: %w", cid, err)
				}

				rowsAffected, err := res.RowsAffected()
				if err != nil {
					return fmt.Errorf("failed to get rows affected: %w", err)
				}

				if rowsAffected == 0 {
					log.Infof("No tx hash index found at cid: %s", cid)
				}

				totalRowsAffected += rowsAffected
			}

		}

		currTs = execTs
	}

	log.Infof("Done pruning %s, rows affected: %d", ethhashlookup.DefaultDbFilename, totalRowsAffected)

	return nil
}

// pruneMsgIndex is a helper function that prunes the msgindex.db for a number of epochs starting from a specified height
func pruneMsgIndex(_ context.Context, basePath string, startHeight int64, epochs int64) error {
	dbPath := path.Join(basePath, databaseType, index.DefaultDbFilename)
	db, err := sql.Open(databaseDriver, dbPath)
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

	var res sql.Result
	if epochs == 0 { // if epochs is 0, delete all messages before the start height
		res, err = tx.Exec(deleteMsgIndexFromStartHeight, startHeight)
	} else { // if epochs is not 0, delete messages between the start height and the start height - epochs
		lowerBound := startHeight - epochs
		if lowerBound < 0 { // if lowerBound is negative, set it to 0 (delete all messages from the start height)
			lowerBound = 0
		}

		res, err = tx.Exec(deleteMsgIndexBetweenRange, startHeight, lowerBound)
	}

	if err != nil {
		if err := tx.Rollback(); err != nil {
			fmt.Printf("ERROR: rollback: %s", err)
		}
		return err
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return xerrors.Errorf("error getting rows affected: %s", err)
	}

	log.Infof("Done pruning %s, rows affected: %d", index.DefaultDbFilename, rowsAffected)

	return nil
}
