package main

import (
	"os"

	"github.com/mitchellh/go-homedir"

	logging "github.com/ipfs/go-log/v2"
	"golang.org/x/xerrors"
	"gopkg.in/urfave/cli.v2"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	lcli "github.com/filecoin-project/lotus/cli"
	"github.com/filecoin-project/lotus/lib/lotuslog"
	"github.com/filecoin-project/lotus/node/repo"
	manet "github.com/multiformats/go-multiaddr-net"
)

var log = logging.Logger("main")

func main() {
	lotuslog.SetupLogLevels()

	log.Info("Starting lotus worker")

	local := []*cli.Command{
		runCmd,
	}

	app := &cli.App{
		Name:    "lotus-seal-worker",
		Usage:   "Remote storage miner worker",
		Version: build.UserVersion,
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
			&cli.BoolFlag{
				Name:  "enable-gpu-proving",
				Usage: "enable use of GPU for mining operations",
				Value: true,
			},
			&cli.BoolFlag{
				Name: "no-precommit",
			},
			&cli.BoolFlag{
				Name: "no-commit",
			},
		},

		Commands: local,
	}
	app.Setup()
	app.Metadata["repoType"] = repo.StorageMiner

	if err := app.Run(os.Args); err != nil {
		log.Warnf("%+v", err)
		return
	}
}

var runCmd = &cli.Command{
	Name:  "run",
	Usage: "Start lotus worker",
	Action: func(cctx *cli.Context) error {
		if !cctx.Bool("enable-gpu-proving") {
			os.Setenv("BELLMAN_NO_GPU", "true")
		}

		nodeApi, closer, err := lcli.GetStorageMinerAPI(cctx)
		if err != nil {
			return xerrors.Errorf("getting miner api: %w", err)
		}
		defer closer()
		ctx := lcli.ReqContext(cctx)

		ainfo, err := lcli.GetAPIInfo(cctx, repo.StorageMiner)
		if err != nil {
			return xerrors.Errorf("could not get api info: %w", err)
		}
		_, storageAddr, err := manet.DialArgs(ainfo.Addr)

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
			log.Warn("Shutting down..")
		}()

		return acceptJobs(ctx, nodeApi, "http://"+storageAddr, ainfo.AuthHeader(), r, cctx.Bool("no-precommit"), cctx.Bool("no-commit"))
	},
}
