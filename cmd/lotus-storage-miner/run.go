package main

import (
	"net/http"
	"os"

	"github.com/multiformats/go-multiaddr"
	"golang.org/x/xerrors"
	"gopkg.in/urfave/cli.v2"

	"github.com/filecoin-project/go-lotus/api"
	lcli "github.com/filecoin-project/go-lotus/cli"
	"github.com/filecoin-project/go-lotus/lib/auth"
	"github.com/filecoin-project/go-lotus/lib/jsonrpc"
	"github.com/filecoin-project/go-lotus/lib/sectorbuilder"
	"github.com/filecoin-project/go-lotus/node"
	"github.com/filecoin-project/go-lotus/node/modules"
	"github.com/filecoin-project/go-lotus/node/repo"
)

var runCmd = &cli.Command{
	Name:  "run",
	Usage: "Start a lotus storage miner process",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "api",
			Value: "2345",
		},
	},
	Action: func(cctx *cli.Context) error {
		nodeApi, err := lcli.GetAPI(cctx)
		if err != nil {
			return err
		}
		ctx := lcli.ReqContext(cctx)

		go func() {
			// a hack for now to handle sigint

			<-ctx.Done()
			os.Exit(0)
		}()

		v, err := nodeApi.Version(ctx)
		if err != nil {
			return err
		}

		storageRepoPath := cctx.String(FlagStorageRepo)
		r, err := repo.NewFS(storageRepoPath)
		if err != nil {
			return err
		}

		ok, err := r.Exists()
		if err != nil {
			return err
		}
		if !ok {
			return xerrors.Errorf("repo at '%s' is not initialized, run 'lotus-storage-miner init' to set it up", storageRepoPath)
		}

		var minerapi api.StorageMiner
		err = node.New(ctx,
			node.StorageMiner(&minerapi),
			node.Online(),
			node.Repo(r),

			node.Override(node.SetApiEndpointKey, func(lr repo.LockedRepo) error {
				apima, err := multiaddr.NewMultiaddr("/ip4/127.0.0.1/tcp/" + cctx.String("api"))
				if err != nil {
					return err
				}
				return lr.SetAPIEndpoint(apima)
			}),
			node.Override(new(*sectorbuilder.SectorBuilderConfig), modules.SectorBuilderConfig(storageRepoPath)),
		)
		if err != nil {
			return err
		}

		// TODO: libp2p node

		log.Infof("Remote version %s", v)

		rpcServer := jsonrpc.NewServer()
		rpcServer.Register("Filecoin", api.PermissionedStorMinerAPI(minerapi))

		ah := &auth.Handler{
			Verify: minerapi.AuthVerify,
			Next:   rpcServer.ServeHTTP,
		}

		http.Handle("/rpc/v0", ah)
		return http.ListenAndServe("127.0.0.1:"+cctx.String("api"), http.DefaultServeMux)
	},
}
