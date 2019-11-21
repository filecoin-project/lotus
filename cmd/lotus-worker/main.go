package main

import (
	"github.com/mitchellh/go-homedir"
	"os"

	logging "github.com/ipfs/go-log"
	"golang.org/x/xerrors"
	"gopkg.in/urfave/cli.v2"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	lcli "github.com/filecoin-project/lotus/cli"
)

var log = logging.Logger("main")

func main() {
	logging.SetLogLevel("*", "INFO")

	log.Info("Starting lotus worker")

	local := []*cli.Command{
		runCmd,
	}

	app := &cli.App{
		Name:    "lotus-worker",
		Usage:   "Remote storage miner worker",
		Version: build.Version,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "repo",
				EnvVars: []string{"WORKER_PATH"},
				Value:   "~/.lotusworker", // TODO: Consider XDG_DATA_HOME
			},
			&cli.StringFlag{
				Name:    "storagerepo",
				EnvVars: []string{"LOTUS_STORAGE_PATH"},
				Value:   "~/.lotusstorage", // TODO: Consider XDG_DATA_HOME
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
	Usage: "Start lotus worker",
	Action: func(cctx *cli.Context) error {
		nodeApi, closer, err := lcli.GetStorageMinerAPI(cctx)
		if err != nil {
			return xerrors.Errorf("getting miner api: %w", err)
		}
		defer closer()
		ctx := lcli.ReqContext(cctx)

		_, auth, err := lcli.GetRawAPI(cctx, "storagerepo")

		_, storageAddr, err := lcli.RepoInfo(cctx, "storagerepo")
		if err != nil {
			return xerrors.Errorf("getting miner repo: %w", err)
		}

		r, err := homedir.Expand(cctx.String("repo"))
		if err != nil {
			return err
		}

		v, err := nodeApi.Version(ctx)
		if err != nil {
			return err
		}
		if v.APIVersion != build.APIVersion {
			return xerrors.Errorf("lotus-storage-miner API version doesn't match: local: ", api.Version{APIVersion: build.APIVersion})
		}

		go func() {
			<-ctx.Done()
			os.Exit(0)
		}()

		return acceptJobs(ctx, nodeApi, "http://"+storageAddr, auth, r)
	},
}
