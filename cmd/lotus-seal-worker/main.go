package main

import (
	"github.com/filecoin-project/lotus/lib/jsonrpc"
	logging "github.com/ipfs/go-log/v2"
	"github.com/mitchellh/go-homedir"
	"golang.org/x/xerrors"
	"gopkg.in/urfave/cli.v2"
	"os"
	"time"

	"github.com/filecoin-project/lotus/api"
	lapi "github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	lcli "github.com/filecoin-project/lotus/cli"
	"github.com/filecoin-project/lotus/lib/lotuslog"
	"github.com/filecoin-project/lotus/node/repo"
	manet "github.com/multiformats/go-multiaddr-net"
)

var log = logging.Logger("main")


var nodeApi lapi.StorageMiner

var closer jsonrpc.ClientCloser

var gcctx cli.Context



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
				//Value:   "~/.lotusworker", // TODO: Consider XDG_DATA_HOME
				Value:   "~/.lotusstorage", // TODO: Consider XDG_DATA_HOME
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

const (checkHealthSleepDuration = 30)
const (startErrorSleepDuration = 10)


var runCmd = &cli.Command{
	Name:  "run",
	Usage: "Start lotus worker",
	Action: func(cctx *cli.Context) error {

		gcctx = *cctx

		ctx := lcli.ReqContext(cctx)

		quit := make(chan int)

		start(cctx, quit)

		time.Sleep(time.Second * checkHealthSleepDuration)
	loop:
		for {
			select {
			case <-ctx.Done():
				quit <- 1
				log.Warn("Shutting down..")

				if closer != nil{
					closer()
				}
				break loop
			default:

				_, err := nodeApi.NetPeers(ctx)

				//_, lcloser, err := lcli.GetStorageMinerAPI(cctx)

				if err != nil {
					restart(cctx)
					time.Sleep(time.Second * checkHealthSleepDuration)
					continue;
				}

				/*defer lcloser()

				_, err = lcli.GetAPIInfo(cctx, repo.StorageMiner)
				if err != nil {
					restart(cctx)
					time.Sleep(time.Second * checkHealthSleepDuration)
					continue;
				}*/

				log.Infof("the health check is ok")

				time.Sleep(time.Second * checkHealthSleepDuration)
			}
		}
		return nil
	},
}

func startCMD(cctx *cli.Context, ch chan int)error{

	err := register(cctx)

	if err != nil{
		return err
	}

	ctx := lcli.ReqContext(cctx)

	return	acceptJobs(ctx, ch)
}

func start(cctx *cli.Context, ch chan int) {
	go func() {
		for {
			err := startCMD(cctx, ch)
			if (err != nil) {
				time.Sleep(time.Second * startErrorSleepDuration)
				continue
			} else {
				break
			}
		}
	}()
}


func restart(cctx *cli.Context) {
	go func() {
		for {
			err := register(cctx)
			if (err != nil) {
				time.Sleep(time.Second * startErrorSleepDuration)
				continue
			} else {
				break
			}
		}
	}()
}

func register(cctx *cli.Context) error{

	log.Infof("the miner isn't online, waiting...")

	if !cctx.Bool("enable-gpu-proving") {
		os.Setenv("BELLMAN_NO_GPU", "true")
	}

	lnodeApi, lcloser, err := lcli.GetStorageMinerAPI(cctx)

	if err != nil {
		return xerrors.Errorf("getting miner api: %w", err)
	}

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

	v, err := lnodeApi.Version(ctx)
	if err != nil {
		return err
	}
	if v.APIVersion != build.APIVersion {
		return xerrors.Errorf("lotus-storage-miner API version doesn't match: local: ", api.Version{APIVersion: build.APIVersion})
	}

	nodeApi = lnodeApi

	closer = lcloser;

	return  registerToMiner(ctx, "http://"+storageAddr, ainfo.AuthHeader(), r, cctx.Bool("no-precommit"), cctx.Bool("no-commit"))
}





