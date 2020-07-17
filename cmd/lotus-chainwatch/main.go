package main

import (
	"database/sql"
	_ "net/http/pprof"
	"os"

	_ "github.com/lib/pq"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/build"
	lcli "github.com/filecoin-project/lotus/cli"
	logging "github.com/ipfs/go-log/v2"
	"github.com/urfave/cli/v2"

	"github.com/filecoin-project/lotus/cmd/lotus-chainwatch/processor"
	"github.com/filecoin-project/lotus/cmd/lotus-chainwatch/syncer"
)

var log = logging.Logger("chainwatch")

func main() {
	_ = logging.SetLogLevel("*", "INFO")
	if err := logging.SetLogLevel("rpc", "error"); err != nil {
		panic(err)
	}

	log.Info("Starting chainwatch")

	local := []*cli.Command{
		dotCmd,
		runCmd,
	}

	app := &cli.App{
		Name:    "lotus-chainwatch",
		Usage:   "Devnet token distribution utility",
		Version: build.UserVersion(),
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "repo",
				EnvVars: []string{"LOTUS_PATH"},
				Value:   "~/.lotus", // TODO: Consider XDG_DATA_HOME
			},
			&cli.StringFlag{
				Name:    "db",
				EnvVars: []string{"LOTUS_DB"},
				Value:   "",
			},
		},

		Commands: local,
	}

	if err := app.Run(os.Args); err != nil {
		log.Warnf("%+v", err)
		os.Exit(1)
	}
}

var runCmd = &cli.Command{
	Name:  "run",
	Usage: "Start lotus chainwatch",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "front",
			Value: "127.0.0.1:8418",
		},
		&cli.IntFlag{
			Name:  "max-batch",
			Value: 1000,
		},
	},
	Action: func(cctx *cli.Context) error {
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
