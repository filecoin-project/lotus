package main

import (
	"context"
	"database/sql"
	"fmt"
	"path"
	"path/filepath"
	"strings"

	"github.com/mitchellh/go-homedir"
	"github.com/multiformats/go-varint"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	builtintypes "github.com/filecoin-project/go-state-types/builtin"
	"github.com/filecoin-project/go-state-types/crypto"
	"github.com/filecoin-project/go-state-types/exitcode"

	lapi "github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/types/ethtypes"
	lcli "github.com/filecoin-project/lotus/cli"
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
	},
}

var backfillEventsCmd = &cli.Command{
	Name:  "backfill-events",
	Usage: "Backfill the events.db for a number of epochs starting from a specified height",
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
			currTs, err = api.ChainGetTipSetByHeight(ctx, abi.ChainEpoch(cctx.Int("from")+1), currTs.Key())
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

		dbPath := path.Join(basePath, "sqlite", "events.db")
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

		addressLookups := make(map[abi.ActorID]address.Address)

		resolveFn := func(ctx context.Context, emitter abi.ActorID, ts *types.TipSet) (address.Address, bool) {
			// we only want to match using f4 addresses
			idAddr, err := address.NewIDAddress(uint64(emitter))
			if err != nil {
				return address.Undef, false
			}

			actor, err := api.StateGetActor(ctx, idAddr, ts.Key())
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

		isIndexedValue := func(b uint8) bool {
			// currently we mark the full entry as indexed if either the key
			// or the value are indexed; in the future we will need finer-grained
			// management of indices
			return b&(types.EventFlagIndexedKey|types.EventFlagIndexedValue) > 0
		}

		var totalEventsAffected int64
		var totalEntriesAffected int64

		processHeight := func(ctx context.Context, cnt int, msgs []lapi.Message, receipts []*types.MessageReceipt) error {
			tx, err := db.BeginTx(ctx, nil)
			if err != nil {
				return fmt.Errorf("failed to start transaction: %w", err)
			}
			defer tx.Rollback() //nolint:errcheck

			stmtSelectEvent, err := tx.Prepare("SELECT MAX(id) from event WHERE height=? AND tipset_key=? and tipset_key_cid=? and emitter_addr=? and event_index=? and message_cid=? and message_index=? and reverted=false")
			if err != nil {
				return err
			}
			stmtEvent, err := tx.Prepare("INSERT INTO event (height, tipset_key, tipset_key_cid, emitter_addr, event_index, message_cid, message_index, reverted) VALUES(?, ?, ?, ?, ?, ?, ?, ?)")
			if err != nil {
				return err
			}
			stmtEntry, err := tx.Prepare("INSERT INTO event_entry(event_id, indexed, flags, key, codec, value) VALUES(?, ?, ?, ?, ?, ?)")
			if err != nil {
				return err
			}

			var eventsAffected int64
			var entriesAffected int64

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

					tsKeyCid, err := currTs.Key().Cid()
					if err != nil {
						return fmt.Errorf("failed to get tipset key cid: %w", err)
					}

					// select the highest event id that exists in database, or null if none exists
					var entryID sql.NullInt64
					err = stmtSelectEvent.QueryRow(
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
					res, err := tx.Stmt(stmtEvent).Exec(
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
						_, err := tx.Stmt(stmtEntry).Exec(
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

					tx, err := ethtypes.EthTxFromSignedEthMessage(smsg)
					if err != nil {
						return fmt.Errorf("failed to convert from signed message: %w at epoch: %d", err, epoch)
					}

					tx.Hash, err = tx.TxHash()
					if err != nil {
						return fmt.Errorf("failed to calculate hash for ethTx: %w at epoch: %d", err, epoch)
					}

					res, err := insertStmt.Exec(tx.Hash.String(), smsg.Cid().String())
					if err != nil {
						return fmt.Errorf("error inserting tx mapping to db: %s at epoch: %d", err, epoch)
					}

					rowsAffected, err := res.RowsAffected()
					if err != nil {
						return fmt.Errorf("error getting rows affected: %s at epoch: %d", err, epoch)
					}

					if rowsAffected > 0 {
						log.Debugf("Inserted txhash %s, cid: %s at epoch: %d", tx.Hash.String(), smsg.Cid().String(), epoch)
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
