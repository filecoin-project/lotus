package main

import (
	"database/sql"
	"fmt"
	"path"
	"path/filepath"
	"strings"

	"github.com/mitchellh/go-homedir"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/crypto"

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
