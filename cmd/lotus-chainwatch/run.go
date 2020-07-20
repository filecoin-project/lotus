package main

import (
	"database/sql"
	"os"

	_ "github.com/lib/pq"

	lcli "github.com/filecoin-project/lotus/cli"
	logging "github.com/ipfs/go-log/v2"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/cmd/lotus-chainwatch/processor"
	"github.com/filecoin-project/lotus/cmd/lotus-chainwatch/syncer"
)

var runCmd = &cli.Command{
	Name:  "run",
	Usage: "Start lotus chainwatch",
	Flags: []cli.Flag{
		&cli.IntFlag{
			Name:  "max-batch",
			Value: 1000,
		},
	},
	Action: func(cctx *cli.Context) error {
		ll := cctx.String("log-level")
		if err := logging.SetLogLevel("*", ll); err != nil {
			return err
		}
		if err := logging.SetLogLevel("rpc", "error"); err != nil {
			return err
		}

		api, closer, err := lcli.GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()
		ctx := lcli.ReqContext(cctx)

		v, err := api.Version(ctx)
		if err != nil {
			return err
		}

		log.Infof("Remote version: %s", v.Version)

		maxBatch := cctx.Int("max-batch")

		db, err := sql.Open("postgres", cctx.String("db"))
		if err != nil {
			return err
		}
		defer func() {
			if err := db.Close(); err != nil {
				log.Errorw("Failed to close database", "error", err)
			}
		}()

		if err := db.Ping(); err != nil {
			return xerrors.Errorf("Database failed to respond to ping (is it online?): %w", err)
		}
		db.SetMaxOpenConns(1350)

		sync := syncer.NewSyncer(db, api)
		sync.Start(ctx)

		proc := processor.NewProcessor(db, api, maxBatch)
		proc.Start(ctx)

		<-ctx.Done()
		os.Exit(0)
		return nil
	},
}
