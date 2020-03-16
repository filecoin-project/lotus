package main

import (
	"context"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/gorilla/mux"
	logging "github.com/ipfs/go-log/v2"
	"golang.org/x/xerrors"
	"gopkg.in/urfave/cli.v2"

	paramfetch "github.com/filecoin-project/go-paramfetch"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/api/apistruct"
	"github.com/filecoin-project/lotus/build"
	lcli "github.com/filecoin-project/lotus/cli"
	"github.com/filecoin-project/lotus/lib/auth"
	"github.com/filecoin-project/lotus/lib/jsonrpc"
	"github.com/filecoin-project/lotus/lib/lotuslog"
	"github.com/filecoin-project/lotus/node/repo"
	"github.com/filecoin-project/lotus/storage/sealmgr/advmgr"
	"github.com/filecoin-project/lotus/storage/sealmgr/stores"
)

var log = logging.Logger("main")

const FlagStorageRepo = "workerrepo"

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
				Name:    "workerrepo",
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
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "address",
			Usage: "Locally reachable address",
		},
	},
	Action: func(cctx *cli.Context) error {
		if !cctx.Bool("enable-gpu-proving") {
			os.Setenv("BELLMAN_NO_GPU", "true")
		}

		if cctx.String("address") == "" {
			return xerrors.Errorf("--address flag is required")
		}

		// Connect to storage-miner

		nodeApi, closer, err := lcli.GetStorageMinerAPI(cctx)
		if err != nil {
			return xerrors.Errorf("getting miner api: %w", err)
		}
		defer closer()
		ctx := lcli.ReqContext(cctx)

		v, err := nodeApi.Version(ctx)
		if err != nil {
			return err
		}
		if v.APIVersion != build.APIVersion {
			return xerrors.Errorf("lotus-storage-miner API version doesn't match: local: ", api.Version{APIVersion: build.APIVersion})
		}
		log.Infof("Remote version %s", v)

		go func() {
			<-ctx.Done()
			log.Warn("Shutting down..")
		}()

		// Check params

		act, err := nodeApi.ActorAddress(ctx)
		if err != nil {
			return err
		}
		ssize, err := nodeApi.ActorSectorSize(ctx, act)
		if err != nil {
			return err
		}

		if err := paramfetch.GetParams(build.ParametersJson(), uint64(ssize)); err != nil {
			return xerrors.Errorf("get params: %w", err)
		}

		// Open repo

		repoPath := cctx.String(FlagStorageRepo)
		r, err := repo.NewFS(repoPath)
		if err != nil {
			return err
		}

		ok, err := r.Exists()
		if err != nil {
			return err
		}
		if !ok {
			return xerrors.Errorf("repo at '%s' is not initialized, run 'lotus-seal-worker init' to set it up", repoPath)
		}

		lr, err := r.Lock(repo.Worker)
		if err != nil {
			return err
		}

		localStore, err := stores.NewLocal(lr)
		if err != nil {
			return err
		}

		if err := stores.DeclareLocalStorage(
			ctx,
			nodeApi,
			localStore,
			[]string{"http://" + cctx.String("address") + "/remote"}, // TODO: Less hardcoded
			1); err != nil {
			return err
		}
		// Setup remote sector store
		_, spt, err := api.ProofTypeFromSectorSize(ssize)
		if err != nil {
			return xerrors.Errorf("getting proof type: %w", err)
		}

		sminfo, err := lcli.GetAPIInfo(cctx, repo.StorageMiner)
		if err != nil {
			return xerrors.Errorf("could not get api info: %w", err)
		}

		remote := stores.NewRemote(localStore, nodeApi, sminfo.AuthHeader())

		// Create / expose the worker

		workerApi := &worker{
			LocalWorker: advmgr.NewLocalWorker(act, spt, remote, localStore, stores.NewIndex()),
		}

		mux := mux.NewRouter()

		rpcServer := jsonrpc.NewServer()
		rpcServer.Register("Filecoin", apistruct.PermissionedWorkerAPI(workerApi))

		mux.Handle("/rpc/v0", rpcServer)
		mux.PathPrefix("/remote").HandlerFunc((&stores.FetchHandler{Store: localStore}).ServeHTTP)
		mux.PathPrefix("/").Handler(http.DefaultServeMux) // pprof

		ah := &auth.Handler{
			Verify: nodeApi.AuthVerify,
			Next:   mux.ServeHTTP,
		}

		srv := &http.Server{Handler: ah}

		sigChan := make(chan os.Signal, 2)
		go func() {
			<-sigChan
			log.Warn("Shutting down..")
			if err := srv.Shutdown(context.TODO()); err != nil {
				log.Errorf("shutting down RPC server failed: %s", err)
			}
			log.Warn("Graceful shutdown successful")
		}()

		signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGINT)

		nl, err := net.Listen("tcp4", cctx.String("address"))
		if err != nil {
			return err
		}

		return srv.Serve(nl)
	},
}
