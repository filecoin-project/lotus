package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/gbrlsnchs/jwt/v3"
	"github.com/gorilla/mux"
	logging "github.com/ipfs/go-log/v2"
	"github.com/urfave/cli/v2"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/tag"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-jsonrpc"
	"github.com/filecoin-project/go-jsonrpc/auth"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/api/v0api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/wallet"
	ledgerwallet "github.com/filecoin-project/lotus/chain/wallet/ledger"
	lcli "github.com/filecoin-project/lotus/cli"
	"github.com/filecoin-project/lotus/lib/lotuslog"
	"github.com/filecoin-project/lotus/metrics"
	"github.com/filecoin-project/lotus/metrics/proxy"
	"github.com/filecoin-project/lotus/node/modules"
	"github.com/filecoin-project/lotus/node/repo"
)

var log = logging.Logger("main")

const FlagWalletRepo = "wallet-repo"

type jwtPayload struct {
	Allow []auth.Permission
}

func main() {
	lotuslog.SetupLogLevels()

	local := []*cli.Command{
		runCmd,
		getApiKeyCmd,
	}

	app := &cli.App{
		Name:    "lotus-wallet",
		Usage:   "Basic external wallet",
		Version: string(build.NodeUserVersion()),
		Description: `
lotus-wallet provides a remote wallet service for lotus.

To configure your lotus node to use a remote wallet:
* Run 'lotus-wallet get-api-key' to generate API key
* Start lotus-wallet using 'lotus-wallet run' (see --help for additional flags)
* Edit lotus config (~/.lotus/config.toml)
  * Find the '[Wallet]' section
  * Set 'RemoteBackend' to '[api key]:http://[wallet ip]:[wallet port]'
    (the default port is 1777)
* Start (or restart) the lotus daemon`,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    FlagWalletRepo,
				EnvVars: []string{"WALLET_PATH"},
				Value:   "~/.lotuswallet", // TODO: Consider XDG_DATA_HOME
			},
			&cli.StringFlag{
				Name:    "repo",
				EnvVars: []string{"LOTUS_PATH"},
				Hidden:  true,
				Value:   "~/.lotus",
			},
		},

		Commands: local,
	}
	app.Setup()

	if err := app.Run(os.Args); err != nil {
		log.Warnf("%+v", err)
		return
	}
}

var getApiKeyCmd = &cli.Command{
	Name:  "get-api-key",
	Usage: "Generate API Key",
	Action: func(cctx *cli.Context) error {
		lr, ks, err := openRepo(cctx)
		if err != nil {
			return err
		}
		defer lr.Close() // nolint

		p := jwtPayload{
			Allow: []auth.Permission{api.PermAdmin},
		}

		authKey, err := modules.APISecret(ks, lr)
		if err != nil {
			return xerrors.Errorf("setting up api secret: %w", err)
		}

		k, err := jwt.Sign(&p, (*jwt.HMACSHA)(authKey))
		if err != nil {
			return xerrors.Errorf("jwt sign: %w", err)
		}

		fmt.Println(string(k))
		return nil
	},
}

var runCmd = &cli.Command{
	Name:  "run",
	Usage: "Start lotus wallet",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "listen",
			Usage: "host address and port the wallet api will listen on",
			Value: "0.0.0.0:1777",
		},
		&cli.BoolFlag{
			Name:  "ledger",
			Usage: "use a ledger device instead of an on-disk wallet",
		},
		&cli.BoolFlag{
			Name:  "interactive",
			Usage: "prompt before performing actions (DO NOT USE FOR MINER WORKER ADDRESS)",
		},
		&cli.BoolFlag{
			Name:  "offline",
			Usage: "don't query chain state in interactive mode",
		},
		&cli.BoolFlag{
			Name:   "disable-auth",
			Usage:  "(insecure) disable api auth",
			Hidden: true,
		},
		&cli.StringFlag{
			Name:  "http-server-timeout",
			Value: "30s",
		},
	},
	Description: "Needs FULLNODE_API_INFO env-var to be set before running (see lotus-wallet --help for setup instructions)",
	Action: func(cctx *cli.Context) error {
		log.Info("Starting lotus wallet")

		ctx := lcli.ReqContext(cctx)
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		// Register all metric views
		if err := view.Register(
			metrics.DefaultViews...,
		); err != nil {
			log.Fatalf("Cannot register the view: %v", err)
		}

		lr, ks, err := openRepo(cctx)
		if err != nil {
			return err
		}
		defer lr.Close() // nolint

		lw, err := wallet.NewWallet(ks)
		if err != nil {
			return err
		}

		var w api.Wallet = lw
		if cctx.Bool("ledger") {
			ds, err := lr.Datastore(context.Background(), "/metadata")
			if err != nil {
				return err
			}

			w = wallet.MultiWallet{
				Local:  lw,
				Ledger: ledgerwallet.NewWallet(ds),
			}
		}

		address := cctx.String("listen")
		mux := mux.NewRouter()

		log.Info("Setting up API endpoint at " + address)

		if cctx.Bool("interactive") {
			var ag func() (v0api.FullNode, jsonrpc.ClientCloser, error)

			if !cctx.Bool("offline") {
				ag = func() (v0api.FullNode, jsonrpc.ClientCloser, error) {
					return lcli.GetFullNodeAPI(cctx)
				}
			}

			w = &InteractiveWallet{
				under:     w,
				apiGetter: ag,
			}
		} else {
			w = &LoggedWallet{under: w}
		}

		rpcApi := proxy.MetricedWalletAPI(w)
		if !cctx.Bool("disable-auth") {
			rpcApi = api.PermissionedWalletAPI(rpcApi)
		}

		rpcServer := jsonrpc.NewServer(jsonrpc.WithServerErrors(api.RPCErrors))
		rpcServer.Register("Filecoin", rpcApi)

		mux.Handle("/rpc/v0", rpcServer)
		mux.PathPrefix("/").Handler(http.DefaultServeMux) // pprof

		var handler http.Handler = mux

		if !cctx.Bool("disable-auth") {
			authKey, err := modules.APISecret(ks, lr)
			if err != nil {
				return xerrors.Errorf("setting up api secret: %w", err)
			}

			authVerify := func(ctx context.Context, token string) ([]auth.Permission, error) {
				var payload jwtPayload
				if _, err := jwt.Verify([]byte(token), (*jwt.HMACSHA)(authKey), &payload); err != nil {
					return nil, xerrors.Errorf("JWT Verification failed: %w", err)
				}

				return payload.Allow, nil
			}

			log.Info("API auth enabled, use 'lotus-wallet get-api-key' to get API key")
			handler = &auth.Handler{
				Verify: authVerify,
				Next:   mux.ServeHTTP,
			}
		}

		timeout, err := time.ParseDuration(cctx.String("http-server-timeout"))
		if err != nil {
			return xerrors.Errorf("invalid time string %s: %x", cctx.String("http-server-timeout"), err)
		}

		srv := &http.Server{
			Handler:           handler,
			ReadHeaderTimeout: timeout,
			BaseContext: func(listener net.Listener) context.Context {
				ctx, _ := tag.New(context.Background(), tag.Upsert(metrics.APIInterface, "lotus-wallet"))
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

		return srv.Serve(nl)
	},
}

func openRepo(cctx *cli.Context) (repo.LockedRepo, types.KeyStore, error) {
	repoPath := cctx.String(FlagWalletRepo)
	r, err := repo.NewFS(repoPath)
	if err != nil {
		return nil, nil, err
	}

	ok, err := r.Exists()
	if err != nil {
		return nil, nil, err
	}
	if !ok {
		if err := r.Init(repo.Wallet); err != nil {
			return nil, nil, err
		}
	}

	lr, err := r.Lock(repo.Wallet)
	if err != nil {
		return nil, nil, err
	}

	ks, err := lr.KeyStore()
	if err != nil {
		return nil, nil, err
	}

	return lr, ks, nil
}
