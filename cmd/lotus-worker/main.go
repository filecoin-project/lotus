package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/ipfs/go-datastore/namespace"
	logging "github.com/ipfs/go-log/v2"
	manet "github.com/multiformats/go-multiaddr/net"
	"github.com/urfave/cli/v2"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/tag"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-jsonrpc/auth"
	"github.com/filecoin-project/go-paramfetch"
	"github.com/filecoin-project/go-statestore"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	lcli "github.com/filecoin-project/lotus/cli"
	cliutil "github.com/filecoin-project/lotus/cli/util"
	"github.com/filecoin-project/lotus/cmd/lotus-worker/sealworker"
	"github.com/filecoin-project/lotus/lib/lotuslog"
	"github.com/filecoin-project/lotus/lib/ulimit"
	"github.com/filecoin-project/lotus/metrics"
	"github.com/filecoin-project/lotus/node/modules"
	"github.com/filecoin-project/lotus/node/repo"
	"github.com/filecoin-project/lotus/storage/paths"
	"github.com/filecoin-project/lotus/storage/sealer"
	"github.com/filecoin-project/lotus/storage/sealer/sealtasks"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
)

var log = logging.Logger("main")

const FlagWorkerRepo = "worker-repo"

// TODO remove after deprecation period
const FlagWorkerRepoDeprecation = "workerrepo"

func main() {
	api.RunningNodeType = api.NodeWorker

	lotuslog.SetupLogLevels()

	local := []*cli.Command{
		runCmd,
		stopCmd,
		infoCmd,
		storageCmd,
		setCmd,
		waitQuietCmd,
		resourcesCmd,
		tasksCmd,
	}

	app := &cli.App{
		Name:                 "lotus-worker",
		Usage:                "Remote miner worker",
		Version:              build.UserVersion(),
		EnableBashCompletion: true,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    FlagWorkerRepo,
				Aliases: []string{FlagWorkerRepoDeprecation},
				EnvVars: []string{"LOTUS_WORKER_PATH", "WORKER_PATH"},
				Value:   "~/.lotusworker", // TODO: Consider XDG_DATA_HOME
				Usage:   fmt.Sprintf("Specify worker repo path. flag %s and env WORKER_PATH are DEPRECATION, will REMOVE SOON", FlagWorkerRepoDeprecation),
			},
			&cli.StringFlag{
				Name:    "panic-reports",
				EnvVars: []string{"LOTUS_PANIC_REPORT_PATH"},
				Hidden:  true,
				Value:   "~/.lotusworker", // should follow --repo default
			},
			&cli.StringFlag{
				Name:    "miner-repo",
				Aliases: []string{"storagerepo"},
				EnvVars: []string{"LOTUS_MINER_PATH", "LOTUS_STORAGE_PATH"},
				Value:   "~/.lotusminer", // TODO: Consider XDG_DATA_HOME
				Usage:   fmt.Sprintf("Specify miner repo path. flag storagerepo and env LOTUS_STORAGE_PATH are DEPRECATION, will REMOVE SOON"),
			},
			&cli.BoolFlag{
				Name:    "enable-gpu-proving",
				Usage:   "enable use of GPU for mining operations",
				Value:   true,
				EnvVars: []string{"LOTUS_WORKER_ENABLE_GPU_PROVING"},
			},
		},

		After: func(c *cli.Context) error {
			if r := recover(); r != nil {
				// Generate report in LOTUS_PANIC_REPORT_PATH and re-raise panic
				build.GeneratePanicReport(c.String("panic-reports"), c.String(FlagWorkerRepo), c.App.Name)
				panic(r)
			}
			return nil
		},
		Commands: local,
	}
	app.Setup()
	app.Metadata["repoType"] = repo.Worker

	if err := app.Run(os.Args); err != nil {
		log.Warnf("%+v", err)
		return
	}
}

var stopCmd = &cli.Command{
	Name:  "stop",
	Usage: "Stop a running lotus worker",
	Flags: []cli.Flag{},
	Action: func(cctx *cli.Context) error {
		api, closer, err := lcli.GetWorkerAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		ctx := lcli.ReqContext(cctx)

		// Detach any storage associated with this worker
		err = api.StorageDetachAll(ctx)
		if err != nil {
			return err
		}

		err = api.Shutdown(ctx)
		if err != nil {
			return err
		}

		return nil
	},
}

var runCmd = &cli.Command{
	Name:  "run",
	Usage: "Start lotus worker",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "listen",
			Usage:   "host address and port the worker api will listen on",
			Value:   "0.0.0.0:3456",
			EnvVars: []string{"LOTUS_WORKER_LISTEN"},
		},
		&cli.StringFlag{
			Name:   "address",
			Hidden: true,
		},
		&cli.BoolFlag{
			Name:    "no-local-storage",
			Usage:   "don't use storageminer repo for sector storage",
			EnvVars: []string{"LOTUS_WORKER_NO_LOCAL_STORAGE"},
		},
		&cli.BoolFlag{
			Name:    "no-swap",
			Usage:   "don't use swap",
			Value:   false,
			EnvVars: []string{"LOTUS_WORKER_NO_SWAP"},
		},
		&cli.StringFlag{
			Name:        "name",
			Usage:       "custom worker name",
			EnvVars:     []string{"LOTUS_WORKER_NAME"},
			DefaultText: "hostname",
		},
		&cli.BoolFlag{
			Name:    "addpiece",
			Usage:   "enable addpiece",
			Value:   true,
			EnvVars: []string{"LOTUS_WORKER_ADDPIECE"},
		},
		&cli.BoolFlag{
			Name:    "precommit1",
			Usage:   "enable precommit1",
			Value:   true,
			EnvVars: []string{"LOTUS_WORKER_PRECOMMIT1"},
		},
		&cli.BoolFlag{
			Name:    "unseal",
			Usage:   "enable unsealing",
			Value:   true,
			EnvVars: []string{"LOTUS_WORKER_UNSEAL"},
		},
		&cli.BoolFlag{
			Name:    "precommit2",
			Usage:   "enable precommit2",
			Value:   true,
			EnvVars: []string{"LOTUS_WORKER_PRECOMMIT2"},
		},
		&cli.BoolFlag{
			Name:    "commit",
			Usage:   "enable commit",
			Value:   true,
			EnvVars: []string{"LOTUS_WORKER_COMMIT"},
		},
		&cli.BoolFlag{
			Name:    "replica-update",
			Usage:   "enable replica update",
			Value:   true,
			EnvVars: []string{"LOTUS_WORKER_REPLICA_UPDATE"},
		},
		&cli.BoolFlag{
			Name:    "prove-replica-update2",
			Usage:   "enable prove replica update 2",
			Value:   true,
			EnvVars: []string{"LOTUS_WORKER_PROVE_REPLICA_UPDATE2"},
		},
		&cli.BoolFlag{
			Name:    "regen-sector-key",
			Usage:   "enable regen sector key",
			Value:   true,
			EnvVars: []string{"LOTUS_WORKER_REGEN_SECTOR_KEY"},
		},
		&cli.BoolFlag{
			Name:    "sector-download",
			Usage:   "enable external sector data download",
			Value:   false,
			EnvVars: []string{"LOTUS_WORKER_SECTOR_DOWNLOAD"},
		},
		&cli.BoolFlag{
			Name:    "windowpost",
			Usage:   "enable window post",
			Value:   false,
			EnvVars: []string{"LOTUS_WORKER_WINDOWPOST"},
		},
		&cli.BoolFlag{
			Name:    "winningpost",
			Usage:   "enable winning post",
			Value:   false,
			EnvVars: []string{"LOTUS_WORKER_WINNINGPOST"},
		},
		&cli.BoolFlag{
			Name:    "no-default",
			Usage:   "disable all default compute tasks, use the worker for storage/fetching only",
			Value:   false,
			EnvVars: []string{"LOTUS_WORKER_NO_DEFAULT"},
		},
		&cli.IntFlag{
			Name:    "parallel-fetch-limit",
			Usage:   "maximum fetch operations to run in parallel",
			Value:   5,
			EnvVars: []string{"LOTUS_WORKER_PARALLEL_FETCH_LIMIT"},
		},
		&cli.IntFlag{
			Name:    "post-parallel-reads",
			Usage:   "maximum number of parallel challenge reads (0 = no limit)",
			Value:   32,
			EnvVars: []string{"LOTUS_WORKER_POST_PARALLEL_READS"},
		},
		&cli.DurationFlag{
			Name:    "post-read-timeout",
			Usage:   "time limit for reading PoSt challenges (0 = no limit)",
			Value:   0,
			EnvVars: []string{"LOTUS_WORKER_POST_READ_TIMEOUT"},
		},
		&cli.StringFlag{
			Name:    "timeout",
			Usage:   "used when 'listen' is unspecified. must be a valid duration recognized by golang's time.ParseDuration function",
			Value:   "30m",
			EnvVars: []string{"LOTUS_WORKER_TIMEOUT"},
		},
		&cli.StringFlag{
			Name:  "http-server-timeout",
			Value: "30s",
		},
	},
	Before: func(cctx *cli.Context) error {
		if cctx.IsSet("address") {
			log.Warnf("The '--address' flag is deprecated, it has been replaced by '--listen'")
			if err := cctx.Set("listen", cctx.String("address")); err != nil {
				return err
			}
		}

		return nil
	},
	Action: func(cctx *cli.Context) error {
		log.Info("Starting lotus worker")

		if !cctx.Bool("enable-gpu-proving") {
			if err := os.Setenv("BELLMAN_NO_GPU", "true"); err != nil {
				return xerrors.Errorf("could not set no-gpu env: %+v", err)
			}
		}

		limit, _, err := ulimit.GetLimit()
		switch {
		case err == ulimit.ErrUnsupported:
			log.Errorw("checking file descriptor limit failed", "error", err)
		case err != nil:
			return xerrors.Errorf("checking fd limit: %w", err)
		default:
			if limit < build.MinerFDLimit {
				return xerrors.Errorf("soft file descriptor limit (ulimit -n) too low, want %d, current %d", build.MinerFDLimit, limit)
			}
		}

		// Connect to storage-miner
		ctx := lcli.ReqContext(cctx)

		var nodeApi api.StorageMiner
		var closer func()
		for {
			nodeApi, closer, err = lcli.GetStorageMinerAPI(cctx, cliutil.StorageMinerUseHttp)
			if err == nil {
				_, err = nodeApi.Version(ctx)
				if err == nil {
					break
				}
			}
			fmt.Printf("\r\x1b[0KConnecting to miner API... (%s)", err)
			time.Sleep(time.Second)
			continue
		}

		defer closer()
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		// Register all metric views
		if err := view.Register(
			metrics.DefaultViews...,
		); err != nil {
			log.Fatalf("Cannot register the view: %v", err)
		}

		v, err := nodeApi.Version(ctx)
		if err != nil {
			return err
		}
		if v.APIVersion != api.MinerAPIVersion0 {
			return xerrors.Errorf("lotus-miner API version doesn't match: expected: %s", api.APIVersion{APIVersion: api.MinerAPIVersion0})
		}
		log.Infof("Remote version %s", v)

		// Check params

		act, err := nodeApi.ActorAddress(ctx)
		if err != nil {
			return err
		}
		ssize, err := nodeApi.ActorSectorSize(ctx, act)
		if err != nil {
			return err
		}

		var taskTypes []sealtasks.TaskType
		var workerType string
		var needParams bool

		if cctx.Bool("windowpost") {
			needParams = true
			workerType = sealtasks.WorkerWindowPoSt
			taskTypes = append(taskTypes, sealtasks.TTGenerateWindowPoSt)
		}
		if cctx.Bool("winningpost") {
			needParams = true
			workerType = sealtasks.WorkerWinningPoSt
			taskTypes = append(taskTypes, sealtasks.TTGenerateWinningPoSt)
		}

		if workerType == "" {
			taskTypes = append(taskTypes, sealtasks.TTFetch, sealtasks.TTCommit1, sealtasks.TTProveReplicaUpdate1, sealtasks.TTFinalize, sealtasks.TTFinalizeUnsealed, sealtasks.TTFinalizeReplicaUpdate)

			if !cctx.Bool("no-default") {
				workerType = sealtasks.WorkerSealing
			}
		}

		if (workerType == sealtasks.WorkerSealing || cctx.IsSet("addpiece")) && cctx.Bool("addpiece") {
			taskTypes = append(taskTypes, sealtasks.TTAddPiece, sealtasks.TTDataCid)
		}
		if (workerType == sealtasks.WorkerSealing || cctx.IsSet("sector-download")) && cctx.Bool("sector-download") {
			taskTypes = append(taskTypes, sealtasks.TTDownloadSector)
		}
		if (workerType == sealtasks.WorkerSealing || cctx.IsSet("precommit1")) && cctx.Bool("precommit1") {
			taskTypes = append(taskTypes, sealtasks.TTPreCommit1)
		}
		if (workerType == sealtasks.WorkerSealing || cctx.IsSet("unseal")) && cctx.Bool("unseal") {
			taskTypes = append(taskTypes, sealtasks.TTUnseal)
		}
		if (workerType == sealtasks.WorkerSealing || cctx.IsSet("precommit2")) && cctx.Bool("precommit2") {
			taskTypes = append(taskTypes, sealtasks.TTPreCommit2)
		}
		if (workerType == sealtasks.WorkerSealing || cctx.IsSet("commit")) && cctx.Bool("commit") {
			needParams = true
			taskTypes = append(taskTypes, sealtasks.TTCommit2)
		}
		if (workerType == sealtasks.WorkerSealing || cctx.IsSet("replica-update")) && cctx.Bool("replica-update") {
			taskTypes = append(taskTypes, sealtasks.TTReplicaUpdate)
		}
		if (workerType == sealtasks.WorkerSealing || cctx.IsSet("prove-replica-update2")) && cctx.Bool("prove-replica-update2") {
			needParams = true
			taskTypes = append(taskTypes, sealtasks.TTProveReplicaUpdate2)
		}
		if (workerType == sealtasks.WorkerSealing || cctx.IsSet("regen-sector-key")) && cctx.Bool("regen-sector-key") {
			taskTypes = append(taskTypes, sealtasks.TTRegenSectorKey)
		}

		if cctx.Bool("no-default") && workerType == "" {
			workerType = sealtasks.WorkerSealing
		}

		if len(taskTypes) == 0 {
			return xerrors.Errorf("no task types specified")
		}
		for _, taskType := range taskTypes {
			if taskType.WorkerType() != workerType {
				return xerrors.Errorf("expected all task types to be for %s worker, but task %s is for %s worker", workerType, taskType, taskType.WorkerType())
			}
		}

		if needParams {
			if err := paramfetch.GetParams(ctx, build.ParametersJSON(), build.SrsJSON(), uint64(ssize)); err != nil {
				return xerrors.Errorf("get params: %w", err)
			}
		}

		// Open repo

		repoPath := cctx.String(FlagWorkerRepo)
		r, err := repo.NewFS(repoPath)
		if err != nil {
			return err
		}

		ok, err := r.Exists()
		if err != nil {
			return err
		}
		if !ok {
			if err := r.Init(repo.Worker); err != nil {
				return err
			}

			lr, err := r.Lock(repo.Worker)
			if err != nil {
				return err
			}

			var localPaths []storiface.LocalPath

			if !cctx.Bool("no-local-storage") {
				b, err := json.MarshalIndent(&storiface.LocalStorageMeta{
					ID:       storiface.ID(uuid.New().String()),
					Weight:   10,
					CanSeal:  true,
					CanStore: false,
				}, "", "  ")
				if err != nil {
					return xerrors.Errorf("marshaling storage config: %w", err)
				}

				if err := ioutil.WriteFile(filepath.Join(lr.Path(), "sectorstore.json"), b, 0644); err != nil {
					return xerrors.Errorf("persisting storage metadata (%s): %w", filepath.Join(lr.Path(), "sectorstore.json"), err)
				}

				localPaths = append(localPaths, storiface.LocalPath{
					Path: lr.Path(),
				})
			}

			if err := lr.SetStorage(func(sc *storiface.StorageConfig) {
				sc.StoragePaths = append(sc.StoragePaths, localPaths...)
			}); err != nil {
				return xerrors.Errorf("set storage config: %w", err)
			}

			{
				// init datastore for r.Exists
				_, err := lr.Datastore(context.Background(), "/metadata")
				if err != nil {
					return err
				}
			}
			if err := lr.Close(); err != nil {
				return xerrors.Errorf("close repo: %w", err)
			}
		}

		lr, err := r.Lock(repo.Worker)
		if err != nil {
			return err
		}
		defer func() {
			if err := lr.Close(); err != nil {
				log.Error("closing repo", err)
			}
		}()
		ds, err := lr.Datastore(context.Background(), "/metadata")
		if err != nil {
			return err
		}

		log.Info("Opening local storage; connecting to master")
		const unspecifiedAddress = "0.0.0.0"
		address := cctx.String("listen")
		addressSlice := strings.Split(address, ":")
		if ip := net.ParseIP(addressSlice[0]); ip != nil {
			if ip.String() == unspecifiedAddress {
				timeout, err := time.ParseDuration(cctx.String("timeout"))
				if err != nil {
					return err
				}
				rip, err := extractRoutableIP(timeout)
				if err != nil {
					return err
				}
				address = rip + ":" + addressSlice[1]
			}
		}

		localStore, err := paths.NewLocal(ctx, lr, nodeApi, []string{"http://" + address + "/remote"})
		if err != nil {
			return err
		}

		// Setup remote sector store
		sminfo, err := lcli.GetAPIInfo(cctx, repo.StorageMiner)
		if err != nil {
			return xerrors.Errorf("could not get api info: %w", err)
		}

		remote := paths.NewRemote(localStore, nodeApi, sminfo.AuthHeader(), cctx.Int("parallel-fetch-limit"),
			&paths.DefaultPartialFileHandler{})

		fh := &paths.FetchHandler{Local: localStore, PfHandler: &paths.DefaultPartialFileHandler{}}
		remoteHandler := func(w http.ResponseWriter, r *http.Request) {
			if !auth.HasPerm(r.Context(), nil, api.PermAdmin) {
				w.WriteHeader(401)
				_ = json.NewEncoder(w).Encode(struct{ Error string }{"unauthorized: missing admin permission"})
				return
			}

			fh.ServeHTTP(w, r)
		}

		// Create / expose the worker

		wsts := statestore.New(namespace.Wrap(ds, modules.WorkerCallsPrefix))

		workerApi := &sealworker.Worker{
			LocalWorker: sealer.NewLocalWorker(sealer.WorkerConfig{
				TaskTypes:                 taskTypes,
				NoSwap:                    cctx.Bool("no-swap"),
				MaxParallelChallengeReads: cctx.Int("post-parallel-reads"),
				ChallengeReadTimeout:      cctx.Duration("post-read-timeout"),
				Name:                      cctx.String("name"),
			}, remote, localStore, nodeApi, nodeApi, wsts),
			LocalStore: localStore,
			Storage:    lr,
		}

		log.Info("Setting up control endpoint at " + address)

		timeout, err := time.ParseDuration(cctx.String("http-server-timeout"))
		if err != nil {
			return xerrors.Errorf("invalid time string %s: %x", cctx.String("http-server-timeout"), err)
		}

		srv := &http.Server{
			Handler:           sealworker.WorkerHandler(nodeApi.AuthVerify, remoteHandler, workerApi, true),
			ReadHeaderTimeout: timeout,
			BaseContext: func(listener net.Listener) context.Context {
				ctx, _ := tag.New(context.Background(), tag.Upsert(metrics.APIInterface, "lotus-worker"))
				return ctx
			},
		}

		go func() {
			<-ctx.Done()
			log.Warn("Shutting down...")
			if err := srv.Shutdown(context.TODO()); err != nil {
				log.Errorf("shutting down RPC server failed: %s", err)
			}
			log.Warn("Graceful shutdown successful")
		}()

		nl, err := net.Listen("tcp", address)
		if err != nil {
			return err
		}

		{
			a, err := net.ResolveTCPAddr("tcp", address)
			if err != nil {
				return xerrors.Errorf("parsing address: %w", err)
			}

			ma, err := manet.FromNetAddr(a)
			if err != nil {
				return xerrors.Errorf("creating api multiaddress: %w", err)
			}

			if err := lr.SetAPIEndpoint(ma); err != nil {
				return xerrors.Errorf("setting api endpoint: %w", err)
			}

			ainfo, err := lcli.GetAPIInfo(cctx, repo.StorageMiner)
			if err != nil {
				return xerrors.Errorf("could not get miner API info: %w", err)
			}

			// TODO: ideally this would be a token with some permissions dropped
			if err := lr.SetAPIToken(ainfo.Token); err != nil {
				return xerrors.Errorf("setting api token: %w", err)
			}
		}

		minerSession, err := nodeApi.Session(ctx)
		if err != nil {
			return xerrors.Errorf("getting miner session: %w", err)
		}

		waitQuietCh := func() chan struct{} {
			out := make(chan struct{})
			go func() {
				workerApi.LocalWorker.WaitQuiet()
				close(out)
			}()
			return out
		}

		go func() {
			heartbeats := time.NewTicker(paths.HeartbeatInterval)
			defer heartbeats.Stop()

			var redeclareStorage bool
			var readyCh chan struct{}
			for {
				// If we're reconnecting, redeclare storage first
				if redeclareStorage {
					log.Info("Redeclaring local storage")

					if err := localStore.Redeclare(ctx, nil, false); err != nil {
						log.Errorf("Redeclaring local storage failed: %+v", err)

						select {
						case <-ctx.Done():
							return // graceful shutdown
						case <-heartbeats.C:
						}
						continue
					}
				}

				// TODO: we could get rid of this, but that requires tracking resources for restarted tasks correctly
				if readyCh == nil {
					log.Info("Making sure no local tasks are running")
					readyCh = waitQuietCh()
				}

				for {
					curSession, err := nodeApi.Session(ctx)
					if err != nil {
						log.Errorf("heartbeat: checking remote session failed: %+v", err)
					} else {
						if curSession != minerSession {
							minerSession = curSession
							break
						}
					}

					select {
					case <-readyCh:
						if err := nodeApi.WorkerConnect(ctx, "http://"+address+"/rpc/v0"); err != nil {
							log.Errorf("Registering worker failed: %+v", err)
							cancel()
							return
						}

						log.Info("Worker registered successfully, waiting for tasks")

						readyCh = nil
					case <-heartbeats.C:
					case <-ctx.Done():
						return // graceful shutdown
					}
				}

				log.Errorf("LOTUS-MINER CONNECTION LOST")

				redeclareStorage = true
			}
		}()

		go func() {
			<-workerApi.Done()
			// Wait 20s to allow the miner to unregister the worker on next heartbeat
			time.Sleep(20 * time.Second)
			log.Warn("Shutting down...")
			if err := srv.Shutdown(context.TODO()); err != nil {
				log.Errorf("shutting down RPC server failed: %s", err)
			}
			log.Warn("Graceful shutdown successful")
		}()

		return srv.Serve(nl)
	},
}

func extractRoutableIP(timeout time.Duration) (string, error) {
	minerMultiAddrKey := "MINER_API_INFO"
	deprecatedMinerMultiAddrKey := "STORAGE_API_INFO"
	env, ok := os.LookupEnv(minerMultiAddrKey)
	if !ok {
		// TODO remove after deprecation period
		_, ok = os.LookupEnv(deprecatedMinerMultiAddrKey)
		if ok {
			log.Warnf("Using a deprecated env(%s) value, please use env(%s) instead.", deprecatedMinerMultiAddrKey, minerMultiAddrKey)
		}
		return "", xerrors.New("MINER_API_INFO environment variable required to extract IP")
	}
	minerAddr := strings.Split(env, "/")
	conn, err := net.DialTimeout("tcp", minerAddr[2]+":"+minerAddr[4], timeout)
	if err != nil {
		return "", err
	}
	defer conn.Close() //nolint:errcheck

	localAddr := conn.LocalAddr().(*net.TCPAddr)

	return strings.Split(localAddr.IP.String(), ":")[0], nil
}
