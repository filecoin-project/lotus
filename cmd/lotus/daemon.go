// +build !nodaemon

package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"runtime/pprof"
	"strings"

	"github.com/filecoin-project/lotus/chain/types"

	paramfetch "github.com/filecoin-project/go-paramfetch"
	blockstore "github.com/ipfs/go-ipfs-blockstore"
	"github.com/mitchellh/go-homedir"
	"github.com/multiformats/go-multiaddr"
	"go.opencensus.io/stats"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/tag"
	"golang.org/x/xerrors"
	"gopkg.in/urfave/cli.v2"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/vm"
	"github.com/filecoin-project/lotus/lib/peermgr"
	"github.com/filecoin-project/lotus/metrics"
	"github.com/filecoin-project/lotus/node"
	"github.com/filecoin-project/lotus/node/modules"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
	"github.com/filecoin-project/lotus/node/modules/testing"
	"github.com/filecoin-project/lotus/node/repo"
	"github.com/filecoin-project/sector-storage/ffiwrapper"
)

const (
	makeGenFlag     = "lotus-make-genesis"
	preTemplateFlag = "genesis-template"
)

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
			Usage: "on first run, load chain from given file",
		},
		&cli.BoolFlag{
			Name:  "halt-after-import",
			Usage: "halt the process after importing chain from file",
		},
		&cli.StringFlag{
			Name:  "pprof",
			Usage: "specify name of file for writing cpu profile to",
		},
		&cli.StringFlag{
			Name:  "profile",
			Usage: "specify type of node",
		},
	},
	Action: func(cctx *cli.Context) error {
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

		ctx, _ := tag.New(context.Background(), tag.Insert(metrics.Version, build.BuildVersion), tag.Insert(metrics.Commit, build.CurrentCommit))
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

		if err := r.Init(repo.FullNode); err != nil && err != repo.ErrRepoExists {
			return xerrors.Errorf("repo init error: %w", err)
		}

		if err := paramfetch.GetParams(build.ParametersJson(), 0); err != nil {
			return xerrors.Errorf("fetching proof parameters: %w", err)
		}

		var genBytes []byte
		if cctx.String("genesis") != "" {
			genBytes, err = ioutil.ReadFile(cctx.String("genesis"))
			if err != nil {
				return xerrors.Errorf("reading genesis: %w", err)
			}
		} else {
			genBytes = build.MaybeGenesis()
		}

		chainfile := cctx.String("import-chain")
		if chainfile != "" {
			if err := ImportChain(r, chainfile); err != nil {
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

		var api api.FullNode

		stop, err := node.New(ctx,
			node.FullAPI(&api),

			node.Override(new(dtypes.Bootstrapper), isBootstrapper),
			node.Online(),
			node.Repo(r),

			genesis,

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

		// Register all metric views
		if err = view.Register(
			metrics.DefaultViews...,
		); err != nil {
			log.Fatalf("Cannot register the view: %v", err)
		}

		// Set the metric to one so it is published to the exporter
		stats.Record(ctx, metrics.LotusInfo.M(1))

		endpoint, err := r.APIEndpoint()
		if err != nil {
			return xerrors.Errorf("getting api endpoint: %w", err)
		}

		// TODO: properly parse api endpoint (or make it a URL)
		return serveRPC(api, stop, endpoint)
	},
}

func importKey(ctx context.Context, api api.FullNode, f string) error {
	f, err := homedir.Expand(f)
	if err != nil {
		return err
	}

	hexdata, err := ioutil.ReadFile(f)
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

	log.Info("successfully imported key for %s", addr)
	return nil
}

func ImportChain(r repo.Repo, fname string) error {
	fi, err := os.Open(fname)
	if err != nil {
		return err
	}

	lr, err := r.Lock(repo.FullNode)
	if err != nil {
		return err
	}
	defer lr.Close()

	ds, err := lr.Datastore("/blocks")
	if err != nil {
		return err
	}

	mds, err := lr.Datastore("/metadata")
	if err != nil {
		return err
	}

	bs := blockstore.NewBlockstore(ds)

	cst := store.NewChainStore(bs, mds, vm.Syscalls(ffiwrapper.ProofVerifier))

	log.Info("importing chain from file...")
	ts, err := cst.Import(fi)
	if err != nil {
		return xerrors.Errorf("importing chain failed: %w", err)
	}

	stm := stmgr.NewStateManager(cst)

	log.Infof("validating imported chain...")
	if err := stm.ValidateChain(context.TODO(), ts); err != nil {
		return xerrors.Errorf("chain validation failed: %w", err)
	}

	log.Info("accepting %s as new head", ts.Cids())
	if err := cst.SetHead(ts); err != nil {
		return err
	}

	return nil
}
