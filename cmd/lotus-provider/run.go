package main

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-contrib/pprof"
	"github.com/gin-gonic/gin"
	"github.com/ipfs/go-datastore/namespace"
	"github.com/pkg/errors"
	"github.com/urfave/cli/v2"
	"go.opencensus.io/stats"
	"go.opencensus.io/tag"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-jsonrpc/auth"
	"github.com/filecoin-project/go-statestore"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	lcli "github.com/filecoin-project/lotus/cli"
	cliutil "github.com/filecoin-project/lotus/cli/util"
	"github.com/filecoin-project/lotus/journal"
	"github.com/filecoin-project/lotus/journal/fsjournal"
	"github.com/filecoin-project/lotus/lib/harmony/harmonydb"
	"github.com/filecoin-project/lotus/lib/harmony/harmonytask"
	"github.com/filecoin-project/lotus/lib/ulimit"
	"github.com/filecoin-project/lotus/metrics"
	"github.com/filecoin-project/lotus/node"
	"github.com/filecoin-project/lotus/node/config"
	"github.com/filecoin-project/lotus/node/modules"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
	"github.com/filecoin-project/lotus/node/repo"
	"github.com/filecoin-project/lotus/storage/paths"
	"github.com/filecoin-project/lotus/storage/sealer"
	"github.com/filecoin-project/lotus/storage/sealer/ffiwrapper"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
)

type stackTracer interface {
	StackTrace() errors.StackTrace
}

var runCmd = &cli.Command{
	Name:  "run",
	Usage: "Start a lotus provider process",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "listen",
			Usage:   "host address and port the worker api will listen on",
			Value:   "0.0.0.0:12300",
			EnvVars: []string{"LOTUS_WORKER_LISTEN"},
		},
		&cli.BoolFlag{
			Name:  "nosync",
			Usage: "don't check full-node sync status",
		},
		&cli.BoolFlag{
			Name:   "halt-after-init",
			Usage:  "only run init, then return",
			Hidden: true,
		},
		&cli.BoolFlag{
			Name:  "manage-fdlimit",
			Usage: "manage open file limit",
			Value: true,
		},
		&cli.StringSliceFlag{
			Name:  "layers",
			Usage: "list of layers to be interpreted (atop defaults). Default: base",
			Value: cli.NewStringSlice("base"),
		},
		&cli.StringFlag{
			Name:  "storage-json",
			Usage: "path to json file containing storage config",
			Value: "~/.lotus/storage.json",
		},
	},
	Action: func(cctx *cli.Context) (err error) {
		defer func() {
			if err != nil {
				if err, ok := err.(stackTracer); ok {
					for _, f := range err.StackTrace() {
						fmt.Printf("%+s:%d\n", f, f)
					}
				}
			}
		}()
		if !cctx.Bool("enable-gpu-proving") {
			err := os.Setenv("BELLMAN_NO_GPU", "true")
			if err != nil {
				return err
			}
		}

		ctx, _ := tag.New(lcli.DaemonContext(cctx),
			tag.Insert(metrics.Version, build.BuildVersion),
			tag.Insert(metrics.Commit, build.CurrentCommit),
			tag.Insert(metrics.NodeType, "provider"),
		)
		// Register all metric views
		/*
			if err := view.Register(
				metrics.MinerNodeViews...,
			); err != nil {
				log.Fatalf("Cannot register the view: %v", err)
			}
		*/
		// Set the metric to one so it is published to the exporter
		stats.Record(ctx, metrics.LotusInfo.M(1))

		if cctx.Bool("manage-fdlimit") {
			if _, _, err := ulimit.ManageFdLimit(); err != nil {
				log.Errorf("setting file descriptor limit: %s", err)
			}
		}

		// Open repo

		repoPath := cctx.String(FlagRepoPath)
		fmt.Println("repopath", repoPath)
		r, err := repo.NewFS(repoPath)
		if err != nil {
			return err
		}

		ok, err := r.Exists()
		if err != nil {
			return err
		}
		if !ok {
			if err := r.Init(repo.Provider); err != nil {
				return err
			}
			/*
				lr, err := r.Lock(repo.Provider)
				if err != nil {
					return err
				}

				var localPaths []storiface.LocalPath

				if err := lr.SetStorage(func(sc *storiface.StorageConfig) {
					sc.StoragePaths = append(sc.StoragePaths, localPaths...)
				}); err != nil {
					return fmt.Errorf("set storage config: %w", err)
				}

				{
					// init datastore for r.Exists
					_, err := lr.Datastore(context.Background(), "/metadata")
					if err != nil {
						return err
					}
				}
				if err := lr.Close(); err != nil {
					return fmt.Errorf("close repo: %w", err)
				}
			*/
		}

		db, err := makeDB(cctx)
		if err != nil {
			return err
		}
		shutdownChan := make(chan struct{})

		/* defaults break lockedRepo (below)
		stop, err := node.New(ctx,
			node.Override(new(dtypes.ShutdownChan), shutdownChan),
			node.Provider(r),
		)
		if err != nil {
			return fmt.Errorf("creating node: %w", err)
		}
		*/

		const unspecifiedAddress = "0.0.0.0"
		listenAddr := cctx.String("listen")
		addressSlice := strings.Split(listenAddr, ":")
		if ip := net.ParseIP(addressSlice[0]); ip != nil {
			if ip.String() == unspecifiedAddress {
				rip, err := db.GetRoutableIP()
				if err != nil {
					return err
				}
				listenAddr = rip + ":" + addressSlice[1]
			}
		}

		lr, err := r.Lock(repo.Provider)
		if err != nil {
			return err
		}
		defer func() {
			if err := lr.Close(); err != nil {
				log.Error("closing repo", err)
			}
		}()
		if err := lr.SetAPIToken([]byte(listenAddr)); err != nil { // our assigned listen address is our unique token
			return xerrors.Errorf("setting api token: %w", err)
		}
		localStore, err := paths.NewLocal(ctx, &paths.BasicLocalStorage{
			PathToJSON: cctx.String("storage-json"),
		}, nil, []string{"http://" + listenAddr + "/remote"})
		if err != nil {
			return err
		}
		cfg, err := getConfig(cctx, db)
		if err != nil {
			return err
		}
		// The config feeds into task runners & their helpers

		var activeTasks []harmonytask.TaskInterface

		var verif storiface.Verifier = ffiwrapper.ProofVerifier

		as, err := modules.LotusProvderAddressSelector(&cfg.Addresses)()
		if err != nil {
			return err
		}

		de, err := journal.ParseDisabledEvents(cfg.Journal.DisabledEvents)
		if err != nil {
			return err
		}
		j, err := fsjournal.OpenFSJournal(lr, de)
		if err != nil {
			return err
		}
		defer j.Close()

		si := paths.NewIndexProxy( /*TODO Alerting*/ nil, db, true)

		lstor, err := paths.NewLocal(ctx, lr, si, nil /*TODO URLs*/)
		if err != nil {
			return err
		}
		full, fullCloser, err := cliutil.GetFullNodeAPIV1LotusProvider(cctx, cfg.Apis.FULLNODE_API_INFO) // TODO switch this into DB entries.
		if err != nil {
			return err
		}
		defer fullCloser()

		sa, err := modules.StorageAuth(ctx, full)
		if err != nil {
			return err
		}

		stor := paths.NewRemote(lstor, si, http.Header(sa), 10, &paths.DefaultPartialFileHandler{})
		mds, err := lr.Datastore(ctx, "/metadata")
		if err != nil {
			return err
		}

		wsts := statestore.New(namespace.Wrap(mds, modules.WorkerCallsPrefix))
		smsts := statestore.New(namespace.Wrap(mds, modules.ManagerWorkPrefix))
		sealer, err := sealer.New(ctx, lstor, stor, lr, si, cfg.SealerConfig, config.ProvingConfig{}, wsts, smsts)
		if err != nil {
			return err
		}

		//ds, dsCloser, err := modules.DatastoreV2(ctx, false, lr)
		if err != nil {
			return err
		}
		//defer dsCloser()

		var maddrs []dtypes.MinerAddress
		for _, s := range cfg.Addresses.MinerAddresses {
			addr, err := address.NewFromString(s)
			if err != nil {
				return err
			}
			maddrs = append(maddrs, dtypes.MinerAddress(addr))
		}

		if cfg.Subsystems.EnableWindowPost {
			wdPostTask, err := modules.WindowPostSchedulerV2(ctx, cfg.Fees, cfg.Proving, full, sealer, verif, j,
				as, maddrs, db, cfg.Subsystems.WindowPostMaxTasks)
			if err != nil {
				return err
			}
			activeTasks = append(activeTasks, wdPostTask)
		}
		taskEngine, err := harmonytask.New(db, activeTasks, listenAddr)
		if err != nil {
			return err
		}

		handler := gin.New()

		taskEngine.ApplyHttpHandlers(handler.Group("/"))
		defer taskEngine.GracefullyTerminate(time.Hour)

		fh := &paths.FetchHandler{Local: localStore, PfHandler: &paths.DefaultPartialFileHandler{}}
		handler.NoRoute(gin.HandlerFunc(func(c *gin.Context) {
			if !auth.HasPerm(c, nil, api.PermAdmin) {
				c.JSON(401, struct{ Error string }{"unauthorized: missing admin permission"})
				return
			}

			fh.ServeHTTP(c.Writer, c.Request)
		}))
		// local APIs
		{
			// debugging
			handler.GET("/debug/metrics", gin.WrapH(metrics.Exporter()))
			pprof.Register(handler)
		}

		// Serve the RPC.
		/*
			endpoint, err := r.APIEndpoint()
			fmt.Println("Endpoint: ", endpoint)
			if err != nil {
				return fmt.Errorf("getting API endpoint: %w", err)
			}
			rpcStopper, err := node.ServeRPC(handler, "lotus-provider", endpoint)
			if err != nil {
				return fmt.Errorf("failed to start json-rpc endpoint: %s", err)
			}
		*/

		// Monitor for shutdown.
		finishCh := node.MonitorShutdown(shutdownChan) //node.ShutdownHandler{Component: "rpc server", StopFunc: rpcStopper},
		//node.ShutdownHandler{Component: "provider", StopFunc: stop},

		<-finishCh

		return nil
	},
}

func makeDB(cctx *cli.Context) (*harmonydb.DB, error) {
	dbConfig := config.HarmonyDB{
		Username: cctx.String("db-user"),
		Password: cctx.String("db-password"),
		Hosts:    strings.Split(cctx.String("db-host"), ","),
		Database: cctx.String("db-name"),
		Port:     cctx.String("db-port"),
	}
	return harmonydb.NewFromConfig(dbConfig)

}
