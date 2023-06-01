package main

import (
	"database/sql"
	"fmt"
	"path"

	_ "github.com/mattn/go-sqlite3"
	"github.com/mitchellh/go-homedir"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-state-types/abi"

	lcli "github.com/filecoin-project/lotus/cli"
)

var msgindexCmd = &cli.Command{
	Name:  "msgindex",
	Usage: "Tools for managing the message index",
	Subcommands: []*cli.Command{
		msgindexBackfillCmd,
		msgindexPruneCmd,
	},
}

var msgindexBackfillCmd = &cli.Command{
	Name:  "backfill",
	Usage: "Backfill the message index for a number of epochs starting from a specified height",
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
		&cli.StringFlag{
			Name:  "repo",
			Value: "~/.lotus",
			Usage: "path to the repo",
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

var msgindexPruneCmd = &cli.Command{
	Name:  "prune",
	Usage: "Prune the message index for messages included before a given epoch",
	Flags: []cli.Flag{
		&cli.IntFlag{
			Name:  "from",
			Usage: "height to start the prune; if negative it indicates epochs from current head",
		},
		&cli.StringFlag{
			Name:  "repo",
			Value: "~/.lotus",
			Usage: "path to the repo",
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
