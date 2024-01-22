//go:build !nodaemon
// +build !nodaemon

package main

import (
	"bufio"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime/pprof"
	"strings"

	"github.com/DataDog/zstd"
	metricsprom "github.com/ipfs/go-metrics-prometheus"
	"github.com/mitchellh/go-homedir"
	"github.com/multiformats/go-multiaddr"
	"github.com/urfave/cli/v2"
	"go.opencensus.io/plugin/runmetrics"
	"go.opencensus.io/stats"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/tag"
	"golang.org/x/xerrors"
	"gopkg.in/cheggaaa/pb.v1"

	"github.com/filecoin-project/go-jsonrpc"
	"github.com/filecoin-project/go-paramfetch"

	lapi "github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/beacon/drand"
	"github.com/filecoin-project/lotus/chain/consensus"
	"github.com/filecoin-project/lotus/chain/consensus/filcns"
	"github.com/filecoin-project/lotus/chain/index"
	proofsffi "github.com/filecoin-project/lotus/chain/proofs/ffi"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/vm"
	lcli "github.com/filecoin-project/lotus/cli"
	"github.com/filecoin-project/lotus/journal"
	"github.com/filecoin-project/lotus/journal/fsjournal"
	"github.com/filecoin-project/lotus/lib/httpreader"
	"github.com/filecoin-project/lotus/lib/peermgr"
	"github.com/filecoin-project/lotus/lib/ulimit"
	"github.com/filecoin-project/lotus/metrics"
	"github.com/filecoin-project/lotus/node"
	"github.com/filecoin-project/lotus/node/config"
	"github.com/filecoin-project/lotus/node/modules"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
	"github.com/filecoin-project/lotus/node/modules/testing"
	"github.com/filecoin-project/lotus/node/repo"
)

const (
	makeGenFlag     = "lotus-make-genesis"
	preTemplateFlag = "genesis-template"
)

var daemonStopCmd = &cli.Command{
	Name:  "stop",
	Usage: "Stop a running lotus daemon",
	Flags: []cli.Flag{},
	Action: func(cctx *cli.Context) error {
		api, closer, err := lcli.GetAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		err = api.Shutdown(lcli.ReqContext(cctx))
		if err != nil {
			return err
		}

		return nil
	},
}

// DaemonCmd is the `go-lotus daemon` command
var DaemonCmd = &cli.Command{
	Name:  "daemon",
	Usage: "Start a lotus daemon process",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "api",
			Value: "1234",
		},
		&cli.StringFlag{
			Name:   makeGenFlag,
			Value:  "",
			Hidden: true,
		},
		&cli.StringFlag{
			Name:   preTemplateFlag,
			Hidden: true,
		},
		&cli.StringFlag{
			Name:   "import-key",
			Usage:  "on first run, import a default key from a given file",
			Hidden: true,
		},
		&cli.StringFlag{
			Name:  "genesis",
			Usage: "genesis file to use for first node run",
		},
		&cli.BoolFlag{
			Name:  "bootstrap",
			Value: true,
		},
		&cli.StringFlag{
			Name:  "import-chain",
			Usage: "on first run, load chain from given file or url and validate",
		},
		&cli.StringFlag{
			Name:  "import-snapshot",
			Usage: "import chain state from a given chain export file or url",
		},
		&cli.BoolFlag{
			Name:  "remove-existing-chain",
			Usage: "remove existing chain and splitstore data on a snapshot-import",
		},
		&cli.BoolFlag{
			Name:  "halt-after-import",
			Usage: "halt the process after importing chain from file",
		},
		&cli.BoolFlag{
			Name:  "lite",
			Usage: "start lotus in lite mode",
		},
		&cli.StringFlag{
			Name:  "pprof",
			Usage: "specify name of file for writing cpu profile to",
		},
		&cli.StringFlag{
			Name:  "profile",
			Usage: "specify type of node",
		},
		&cli.BoolFlag{
			Name:  "manage-fdlimit",
			Usage: "manage open file limit",
			Value: true,
		},
		&cli.StringFlag{
			Name:  "config",
			Usage: "specify path of config file to use",
		},
		// FIXME: This is not the correct place to put this configuration
		//  option. Ideally it would be part of `config.toml` but at the
		//  moment that only applies to the node configuration and not outside
		//  components like the RPC server.
		&cli.IntFlag{
			Name:  "api-max-req-size",
			Usage: "maximum API request size accepted by the JSON RPC server",
		},
		&cli.PathFlag{
			Name:  "restore",
			Usage: "restore from backup file",
		},
		&cli.PathFlag{
			Name:  "restore-config",
			Usage: "config file to use when restoring from backup",
		},
	},
	Action: func(cctx *cli.Context) error {
		isLite := cctx.Bool("lite")

		err := runmetrics.Enable(runmetrics.RunMetricOptions{
			EnableCPU:    true,
			EnableMemory: true,
		})
		if err != nil {
			return xerrors.Errorf("enabling runtime metrics: %w", err)
		}

		interactive := cctx.Bool("interactive")

		if cctx.Bool("manage-fdlimit") {
			if _, _, err := ulimit.ManageFdLimit(); err != nil {
				log.Errorf("setting file descriptor limit: %s", err)
			}
		}

		if prof := cctx.String("pprof"); prof != "" {
			profile, err := os.Create(prof)
			if err != nil {
				return err
			}

			if err := pprof.StartCPUProfile(profile); err != nil {
				return err
			}
			defer pprof.StopCPUProfile()
		}

		var isBootstrapper dtypes.Bootstrapper
		switch profile := cctx.String("profile"); profile {
		case "bootstrapper":
			isBootstrapper = true
		case "":
			// do nothing
		default:
			return fmt.Errorf("unrecognized profile type: %q", profile)
		}

		ctx, _ := tag.New(context.Background(),
			tag.Insert(metrics.Version, build.NodeBuildVersion),
			tag.Insert(metrics.Commit, build.CurrentCommit),
			tag.Insert(metrics.NodeType, "chain"),
		)
		// Register all metric views
		if err = view.Register(
			metrics.ChainNodeViews...,
		); err != nil {
			log.Fatalf("Cannot register the view: %v", err)
		}
		// Set the metric to one so it is published to the exporter
		stats.Record(ctx, metrics.LotusInfo.M(1))

		{
			dir, err := homedir.Expand(cctx.String("repo"))
			if err != nil {
				log.Warnw("could not expand repo location", "error", err)
			} else {
				log.Infof("lotus repo: %s", dir)
			}
		}

		r, err := repo.NewFS(cctx.String("repo"))
		if err != nil {
			return xerrors.Errorf("opening fs repo: %w", err)
		}

		if cctx.String("config") != "" {
			r.SetConfigPath(cctx.String("config"))
		}

		err = r.Init(repo.FullNode)
		if err != nil && err != repo.ErrRepoExists {
			return xerrors.Errorf("repo init error: %w", err)
		}
		freshRepo := err != repo.ErrRepoExists

		if !isLite {
			if err := paramfetch.GetParams(lcli.ReqContext(cctx), build.ParametersJSON(), build.SrsJSON(), 0); err != nil {
				return xerrors.Errorf("fetching proof parameters: %w", err)
			}
		}

		var genBytes []byte
		if cctx.String("genesis") != "" {
			genBytes, err = os.ReadFile(cctx.String("genesis"))
			if err != nil {
				return xerrors.Errorf("reading genesis: %w", err)
			}
		} else {
			genBytes = build.MaybeGenesis()
		}

		if cctx.IsSet("restore") {
			if !freshRepo {
				return xerrors.Errorf("restoring from backup is only possible with a fresh repo!")
			}
			if err := restore(cctx, r); err != nil {
				return xerrors.Errorf("restoring from backup: %w", err)
			}
		}

		if cctx.Bool("remove-existing-chain") {
			lr, err := repo.NewFS(cctx.String("repo"))
			if err != nil {
				return xerrors.Errorf("error opening fs repo: %w", err)
			}

			exists, err := lr.Exists()
			if err != nil {
				return err
			}
			if !exists {
				return xerrors.Errorf("lotus repo doesn't exist")
			}

			err = removeExistingChain(cctx, lr)
			if err != nil {
				return err
			}
		}

		chainfile := cctx.String("import-chain")
		snapshot := cctx.String("import-snapshot")
		willImportChain := false
		if chainfile != "" || snapshot != "" {
			if chainfile != "" && snapshot != "" {
				return fmt.Errorf("cannot specify both 'import-snapshot' and 'import-chain'")
			}
			willImportChain = true
		}

		var willRemoveChain bool
		if interactive && willImportChain && !cctx.IsSet("remove-existing-chain") {
			// Confirm with the user about the intention to remove chain data.
			reader := bufio.NewReader(os.Stdin)
			fmt.Print("Importing chain or snapshot will by default delete existing local chain data. Do you want to proceed and delete? (yes/no): ")
			userInput, err := reader.ReadString('\n')
			if err != nil {
				return xerrors.Errorf("reading user input: %w", err)
			}
			userInput = strings.ToLower(strings.TrimSpace(userInput))
			switch userInput {
			case "yes":
				willRemoveChain = true
			case "no":
				willRemoveChain = false
			default:
				return fmt.Errorf("invalid input. please answer with 'yes' or 'no'")
			}
		} else {
			willRemoveChain = cctx.Bool("remove-existing-chain")
		}

		if willRemoveChain {
			lr, err := repo.NewFS(cctx.String("repo"))
			if err != nil {
				return xerrors.Errorf("error opening fs repo: %w", err)
			}

			exists, err := lr.Exists()
			if err != nil {
				return err
			}
			if !exists {
				return xerrors.Errorf("lotus repo doesn't exist")
			}

			err = removeExistingChain(cctx, lr)
			if err != nil {
				return err
			}
		}

		if willImportChain {
			var issnapshot bool
			if chainfile == "" {
				chainfile = snapshot
				issnapshot = true
			}

			if err := ImportChain(ctx, r, chainfile, issnapshot); err != nil {
				return err
			}
			if cctx.Bool("halt-after-import") {
				fmt.Println("Chain import complete, halting as requested...")
				return nil
			}
		}

		genesis := node.Options()
		if len(genBytes) > 0 {
			genesis = node.Override(new(modules.Genesis), modules.LoadGenesis(genBytes))
		}
		if cctx.String(makeGenFlag) != "" {
			if cctx.String(preTemplateFlag) == "" {
				return xerrors.Errorf("must also pass file with genesis template to `--%s`", preTemplateFlag)
			}
			genesis = node.Override(new(modules.Genesis), testing.MakeGenesis(cctx.String(makeGenFlag), cctx.String(preTemplateFlag)))
		}

		shutdownChan := make(chan struct{})

		// If the daemon is started in "lite mode", provide a  Gateway
		// for RPC calls
		liteModeDeps := node.Options()
		if isLite {
			gapi, closer, err := lcli.GetGatewayAPI(cctx)
			if err != nil {
				return err
			}

			defer closer()
			liteModeDeps = node.Override(new(lapi.Gateway), gapi)
		}

		// some libraries like ipfs/go-ds-measure and ipfs/go-ipfs-blockstore
		// use ipfs/go-metrics-interface. This injects a Prometheus exporter
		// for those. Metrics are exported to the default registry.
		if err := metricsprom.Inject(); err != nil {
			log.Warnf("unable to inject prometheus ipfs/go-metrics exporter; some metrics will be unavailable; err: %s", err)
		}

		var api lapi.FullNode
		stop, err := node.New(ctx,
			node.FullAPI(&api, node.Lite(isLite)),

			node.Base(),
			node.Repo(r),

			node.Override(new(dtypes.Bootstrapper), isBootstrapper),
			node.Override(new(dtypes.ShutdownChan), shutdownChan),

			genesis,
			liteModeDeps,

			node.ApplyIf(func(s *node.Settings) bool { return cctx.IsSet("api") },
				node.Override(node.SetApiEndpointKey, func(lr repo.LockedRepo) error {
					apima, err := multiaddr.NewMultiaddr("/ip4/127.0.0.1/tcp/" +
						cctx.String("api"))
					if err != nil {
						return err
					}
					return lr.SetAPIEndpoint(apima)
				})),
			node.ApplyIf(func(s *node.Settings) bool { return !cctx.Bool("bootstrap") },
				node.Unset(node.RunPeerMgrKey),
				node.Unset(new(*peermgr.PeerMgr)),
			),
		)
		if err != nil {
			return xerrors.Errorf("initializing node: %w", err)
		}

		if cctx.String("import-key") != "" {
			if err := importKey(ctx, api, cctx.String("import-key")); err != nil {
				log.Errorf("importing key failed: %+v", err)
			}
		}

		endpoint, err := r.APIEndpoint()
		if err != nil {
			return xerrors.Errorf("getting api endpoint: %w", err)
		}

		//
		// Instantiate JSON-RPC endpoint.
		// ----

		// Populate JSON-RPC options.
		serverOptions := []jsonrpc.ServerOption{jsonrpc.WithServerErrors(lapi.RPCErrors)}
		if maxRequestSize := cctx.Int("api-max-req-size"); maxRequestSize != 0 {
			serverOptions = append(serverOptions, jsonrpc.WithMaxRequestSize(int64(maxRequestSize)))
		}

		// Instantiate the full node handler.
		h, err := node.FullNodeHandler(api, true, serverOptions...)
		if err != nil {
			return fmt.Errorf("failed to instantiate rpc handler: %s", err)
		}

		// Serve the RPC.
		rpcStopper, err := node.ServeRPC(h, "lotus-daemon", endpoint)
		if err != nil {
			return fmt.Errorf("failed to start json-rpc endpoint: %s", err)
		}
		// Monitor for shutdown.
		finishCh := node.MonitorShutdown(shutdownChan,
			node.ShutdownHandler{Component: "rpc server", StopFunc: rpcStopper},
			node.ShutdownHandler{Component: "node", StopFunc: stop},
		)
		<-finishCh // fires when shutdown is complete.

		// TODO: properly parse api endpoint (or make it a URL)
		return nil
	},
	Subcommands: []*cli.Command{
		daemonStopCmd,
	},
}

func importKey(ctx context.Context, api lapi.FullNode, f string) error {
	f, err := homedir.Expand(f)
	if err != nil {
		return err
	}

	hexdata, err := os.ReadFile(f)
	if err != nil {
		return err
	}

	data, err := hex.DecodeString(strings.TrimSpace(string(hexdata)))
	if err != nil {
		return err
	}

	var ki types.KeyInfo
	if err := json.Unmarshal(data, &ki); err != nil {
		return err
	}

	addr, err := api.WalletImport(ctx, &ki)
	if err != nil {
		return err
	}

	if err := api.WalletSetDefault(ctx, addr); err != nil {
		return err
	}

	log.Infof("successfully imported key for %s", addr)
	return nil
}

func ImportChain(ctx context.Context, r repo.Repo, fname string, snapshot bool) (err error) {
	var rd io.Reader
	var l int64
	if strings.HasPrefix(fname, "http://") || strings.HasPrefix(fname, "https://") {
		rrd, err := httpreader.NewResumableReader(ctx, fname)
		if err != nil {
			return xerrors.Errorf("fetching chain CAR failed: setting up resumable reader: %w", err)
		}

		rd = rrd
		l = rrd.ContentLength()
	} else {
		fname, err = homedir.Expand(fname)
		if err != nil {
			return err
		}

		fi, err := os.Open(fname)
		if err != nil {
			return err
		}
		defer fi.Close() //nolint:errcheck

		st, err := os.Stat(fname)
		if err != nil {
			return err
		}

		rd = fi
		l = st.Size()
	}

	lr, err := r.Lock(repo.FullNode)
	if err != nil {
		return err
	}
	defer lr.Close() //nolint:errcheck

	bs, err := lr.Blockstore(ctx, repo.UniversalBlockstore)
	if err != nil {
		return xerrors.Errorf("failed to open blockstore: %w", err)
	}

	mds, err := lr.Datastore(ctx, "/metadata")
	if err != nil {
		return err
	}

	j, err := fsjournal.OpenFSJournal(lr, journal.EnvDisabledEvents())
	if err != nil {
		return xerrors.Errorf("failed to open journal: %w", err)
	}

	cst := store.NewChainStore(bs, bs, mds, filcns.Weight, j)
	defer cst.Close() //nolint:errcheck

	log.Infof("importing chain from %s...", fname)

	bufr := bufio.NewReaderSize(rd, 1<<20)

	header, err := bufr.Peek(4)
	if err != nil {
		return xerrors.Errorf("peek header: %w", err)
	}

	bar := pb.New64(l)
	br := bar.NewProxyReader(bufr)
	bar.ShowTimeLeft = true
	bar.ShowPercent = true
	bar.ShowSpeed = true
	bar.Units = pb.U_BYTES

	var ir io.Reader = br

	if string(header[1:]) == "\xB5\x2F\xFD" { // zstd
		zr := zstd.NewReader(br)
		defer func() {
			if err := zr.Close(); err != nil {
				log.Errorw("closing zstd reader", "error", err)
			}
		}()
		ir = zr
	}

	bar.Start()
	ts, gen, err := cst.Import(ctx, ir)
	bar.Finish()

	if err != nil {
		return xerrors.Errorf("importing chain failed: %w", err)
	}

	if err := cst.FlushValidationCache(ctx); err != nil {
		return xerrors.Errorf("flushing validation cache failed: %w", err)
	}

	log.Infof("setting genesis")
	err = cst.SetGenesis(ctx, gen)
	if err != nil {
		return err
	}

	if !snapshot {
		shd, err := drand.BeaconScheduleFromDrandSchedule(buildconstants.DrandConfigSchedule(), gen.Timestamp, nil)
		if err != nil {
			return xerrors.Errorf("failed to construct beacon schedule: %w", err)
		}

		stm, err := stmgr.NewStateManager(cst, consensus.NewTipSetExecutor(filcns.RewardFunc), vm.Syscalls(proofsffi.ProofVerifier), filcns.DefaultUpgradeSchedule(), shd, mds, index.DummyMsgIndex)
		if err != nil {
			return err
		}

		log.Infof("validating imported chain...")
		if err := stm.ValidateChain(ctx, ts); err != nil {
			return xerrors.Errorf("chain validation failed: %w", err)
		}
	}

	log.Infof("accepting %s as new head", ts.Cids())
	if err := cst.ForceHeadSilent(ctx, ts); err != nil {
		return err
	}

	// populate the message index if user has EnableMsgIndex enabled
	//
	c, err := lr.Config()
	if err != nil {
		return err
	}
	cfg, ok := c.(*config.FullNode)
	if !ok {
		return xerrors.Errorf("invalid config for repo, got: %T", c)
	}
	if cfg.Index.EnableMsgIndex {
		log.Info("populating message index...")
		basePath, err := lr.SqlitePath()
		if err != nil {
			return err
		}
		if err := index.PopulateAfterSnapshot(ctx, filepath.Join(basePath, index.DefaultDbFilename), cst); err != nil {
			return err
		}
		log.Info("populating message index done")
	}

	return nil
}

func removeExistingChain(cctx *cli.Context, lr repo.Repo) error {
	lockedRepo, err := lr.Lock(repo.FullNode)
	if err != nil {
		return xerrors.Errorf("error locking repo: %w", err)
	}
	// Ensure that lockedRepo is closed when this function exits
	defer func() {
		if closeErr := lockedRepo.Close(); closeErr != nil {
			log.Errorf("Error closing the lockedRepo: %v", closeErr)
		}
	}()

	log.Info("removing splitstore directory...")
	err = deleteSplitstoreDir(lockedRepo)
	if err != nil {
		return xerrors.Errorf("error removing splitstore directory: %w", err)
	}

	// Get the base repo path
	repoPath := lockedRepo.Path()

	// Construct the path to the chain directory
	chainPath := filepath.Join(repoPath, "datastore", "chain")

	log.Info("removing chain directory:", chainPath)

	err = os.RemoveAll(chainPath)
	if err != nil {
		return xerrors.Errorf("error removing chain directory: %w", err)
	}

	log.Info("chain and splitstore data have been removed")
	return nil
}

func deleteSplitstoreDir(lr repo.LockedRepo) error {
	path, err := lr.SplitstorePath()
	if err != nil {
		return xerrors.Errorf("error getting splitstore path: %w", err)
	}

	return os.RemoveAll(path)
}
