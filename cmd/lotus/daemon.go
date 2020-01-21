// +build !nodaemon

package main

import (
	"context"
	"io/ioutil"
	"os"

	paramfetch "github.com/filecoin-project/go-paramfetch"
	"github.com/filecoin-project/go-sectorbuilder"
	blockstore "github.com/ipfs/go-ipfs-blockstore"
	"github.com/multiformats/go-multiaddr"
	"golang.org/x/xerrors"
	"gopkg.in/urfave/cli.v2"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/vm"
	"github.com/filecoin-project/lotus/node"
	"github.com/filecoin-project/lotus/node/modules"
	"github.com/filecoin-project/lotus/node/modules/testing"
	"github.com/filecoin-project/lotus/node/repo"
	"github.com/filecoin-project/lotus/peermgr"
)

const (
	makeGenFlag          = "lotus-make-random-genesis"
	preSealedSectorsFlag = "genesis-presealed-sectors"
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
			Name:   preSealedSectorsFlag,
			Hidden: true,
		},
		&cli.StringFlag{
			Name:  "genesis",
			Usage: "genesis file to use for first node run",
		},
		&cli.StringFlag{
			Name:   "genesis-timestamp",
			Hidden: true,
			Usage:  "set the timestamp for the genesis block that will be created",
		},
		&cli.BoolFlag{
			Name:  "bootstrap",
			Value: true,
		},
		&cli.StringFlag{
			Name:  "import-chain",
			Usage: "on first run, load chain from given file",
		},
	},
	Action: func(cctx *cli.Context) error {
		ctx := context.Background()
		r, err := repo.NewFS(cctx.String("repo"))
		if err != nil {
			return xerrors.Errorf("opening fs repo: %w", err)
		}

		if err := r.Init(repo.FullNode); err != nil && err != repo.ErrRepoExists {
			return xerrors.Errorf("repo init error: %w", err)
		}

		if err := paramfetch.GetParams(build.ParametersJson, 0); err != nil {
			return xerrors.Errorf("fetching proof parameters: %w", err)
		}

		genBytes := build.MaybeGenesis()

		if cctx.String("genesis") != "" {
			genBytes, err = ioutil.ReadFile(cctx.String("genesis"))
			if err != nil {
				return xerrors.Errorf("reading genesis: %w", err)
			}
		}

		chainfile := cctx.String("import-chain")
		if chainfile != "" {
			if err := ImportChain(r, chainfile); err != nil {
				return err
			}
		}

		genesis := node.Options()
		if len(genBytes) > 0 {
			genesis = node.Override(new(modules.Genesis), modules.LoadGenesis(genBytes))
		}
		if cctx.String(makeGenFlag) != "" {
			if cctx.String(preSealedSectorsFlag) == "" {
				return xerrors.Errorf("must also pass file with miner preseal info to `--%s`", preSealedSectorsFlag)
			}
			genesis = node.Override(new(modules.Genesis), testing.MakeGenesis(cctx.String(makeGenFlag), cctx.String(preSealedSectorsFlag), cctx.String("genesis-timestamp")))
		}

		var api api.FullNode

		stop, err := node.New(ctx,
			node.FullAPI(&api),

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

		endpoint, err := r.APIEndpoint()
		if err != nil {
			return xerrors.Errorf("getting api endpoint: %w", err)
		}

		// TODO: properly parse api endpoint (or make it a URL)
		return serveRPC(api, stop, endpoint)
	},
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

	cst := store.NewChainStore(bs, mds, vm.Syscalls(sectorbuilder.ProofVerifier))

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
