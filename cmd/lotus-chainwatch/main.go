package main

import (
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"os"

	logging "github.com/ipfs/go-log/v2"
	"golang.org/x/xerrors"
	"gopkg.in/urfave/cli.v2"

	"github.com/filecoin-project/lotus/build"
	lcli "github.com/filecoin-project/lotus/cli"
)

var log = logging.Logger("chainwatch")

func main() {
	logging.SetLogLevel("*", "INFO")

	log.Info("Starting chainwatch")

	local := []*cli.Command{
		runCmd,
		dotCmd,
	}

	app := &cli.App{
		Name:    "lotus-chainwatch",
		Usage:   "Devnet token distribution utility",
		Version: build.UserVersion,
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
		return
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

		log.Info("Remote version: %s", v.Version)

		maxBatch := cctx.Int("max-batch")

		st, err := openStorage(cctx.String("db"))
		if err != nil {
			return err
		}
		defer st.close()

		runSyncer(ctx, api, st, maxBatch)

		h, err := newHandler(api, st)
		if err != nil {
			return xerrors.Errorf("handler setup: %w", err)
		}

		http.Handle("/", h)

		fmt.Printf("Open http://%s\n", cctx.String("front"))

		go func() {
			<-ctx.Done()
			os.Exit(0)
		}()

		return http.ListenAndServe(cctx.String("front"), nil)
	},
}
