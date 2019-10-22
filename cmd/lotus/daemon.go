// +build !nodaemon

package main

import (
	"context"
	"io/ioutil"

	"github.com/filecoin-project/lotus/peermgr"

	"github.com/multiformats/go-multiaddr"
	"golang.org/x/xerrors"
	"gopkg.in/urfave/cli.v2"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/node"
	"github.com/filecoin-project/lotus/node/modules"
	"github.com/filecoin-project/lotus/node/modules/testing"
	"github.com/filecoin-project/lotus/node/repo"
)

const (
	makeGenFlag = "lotus-make-random-genesis"
)

// DaemonCmd is the `go-lotus daemon` command
var DaemonCmd = &cli.Command{
	Name:  "daemon",
	Usage: "Start a lotus daemon process",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:   makeGenFlag,
			Value:  "",
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
	},
	Action: func(cctx *cli.Context) error {
		ctx := context.Background()
		r, err := repo.NewFS(cctx.String("repo"))
		if err != nil {
			return err
		}

		if err := r.Init(repo.FullNode); err != nil && err != repo.ErrRepoExists {
			return err
		}

		if err := build.GetParams(false); err != nil {
			return xerrors.Errorf("fetching proof parameters: %w", err)
		}

		genBytes := build.MaybeGenesis()

		if cctx.String("genesis") != "" {
			genBytes, err = ioutil.ReadFile(cctx.String("genesis"))
			if err != nil {
				return err
			}

		}

		genesis := node.Options()
		if len(genBytes) > 0 {
			genesis = node.Override(new(modules.Genesis), modules.LoadGenesis(genBytes))
		}
		if cctx.String(makeGenFlag) != "" {
			genesis = node.Override(new(modules.Genesis), testing.MakeGenesis(cctx.String(makeGenFlag)))
		}

		var api api.FullNode
		var endpoint multiaddr.Multiaddr

		stop, err := node.New(ctx,
			node.FullAPI(&api),

			node.Online(),
			node.Repo(r),

			genesis,

			node.Override(node.SetApiEndpointKey, func(lr repo.LockedRepo) error {
				cfg, err := lr.Config()
				if err != nil {
					return xerrors.Errorf("could not get config: %w", err)
				}

				apima, err := multiaddr.NewMultiaddr(cfg.API.ListenAddress)
				if err != nil {
					return err
				}
				endpoint = apima
				return lr.SetAPIEndpoint(apima)
			}),

			node.ApplyIf(func(s *node.Settings) bool { return !cctx.Bool("bootstrap") },
				node.Unset(node.RunPeerMgrKey),
				node.Unset(new(*peermgr.PeerMgr)),
			),
		)
		if err != nil {
			return err
		}

		// TODO: properly parse api endpoint (or make it a URL)
		return serveRPC(api, stop, endpoint)
	},
}
