// Package rpc provides all direct access to this node.
package rpc

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"github.com/filecoin-project/lotus/api/client"
	cliutil "github.com/filecoin-project/lotus/cli/util"
	"github.com/filecoin-project/lotus/node/repo"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
	"github.com/mitchellh/go-homedir"
	"github.com/urfave/cli/v2"
	"net"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/gbrlsnchs/jwt/v3"
	"github.com/gorilla/mux"
	logging "github.com/ipfs/go-log/v2"
	"go.opencensus.io/tag"
	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-jsonrpc"
	"github.com/filecoin-project/go-jsonrpc/auth"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/cmd/lotus-provider/deps"
	"github.com/filecoin-project/lotus/lib/rpcenc"
	"github.com/filecoin-project/lotus/metrics"
	"github.com/filecoin-project/lotus/metrics/proxy"
	"github.com/filecoin-project/lotus/provider/lpmarket"
	"github.com/filecoin-project/lotus/provider/lpweb"
	"github.com/filecoin-project/lotus/storage/paths"
)

var log = logging.Logger("lp/rpc")

var permissioned = os.Getenv("LOTUS_DISABLE_AUTH_PERMISSIONED") != "1"

func LotusProviderHandler(
	authv func(ctx context.Context, token string) ([]auth.Permission, error),
	remote http.HandlerFunc,
	a api.LotusProvider,
	permissioned bool) http.Handler {
	mux := mux.NewRouter()
	readerHandler, readerServerOpt := rpcenc.ReaderParamDecoder()
	rpcServer := jsonrpc.NewServer(jsonrpc.WithServerErrors(api.RPCErrors), readerServerOpt)

	wapi := proxy.MetricedAPI[api.LotusProvider, api.LotusProviderStruct](a)
	if permissioned {
		wapi = api.PermissionedAPI[api.LotusProvider, api.LotusProviderStruct](wapi)
	}

	rpcServer.Register("Filecoin", wapi)
	rpcServer.AliasMethod("rpc.discover", "Filecoin.Discover")

	mux.Handle("/rpc/v0", rpcServer)
	mux.Handle("/rpc/streams/v0/push/{uuid}", readerHandler)
	mux.PathPrefix("/remote").HandlerFunc(remote)
	mux.PathPrefix("/").Handler(http.DefaultServeMux) // pprof

	if !permissioned {
		return mux
	}

	ah := &auth.Handler{
		Verify: authv,
		Next:   mux.ServeHTTP,
	}
	return ah
}

type ProviderAPI struct {
	*deps.Deps
	ShutdownChan chan struct{}
}

func (p *ProviderAPI) Version(context.Context) (api.Version, error) {
	return api.ProviderAPIVersion0, nil
}

func (p *ProviderAPI) AllocatePieceToSector(ctx context.Context, maddr address.Address, piece api.PieceDealInfo, rawSize int64, source url.URL, header http.Header) (api.SectorOffset, error) {
	di := lpmarket.NewPieceIngester(p.Deps.DB, p.Deps.Full)

	return di.AllocatePieceToSector(ctx, maddr, piece, rawSize, source, header)
}

// Trigger shutdown
func (p *ProviderAPI) Shutdown(context.Context) error {
	close(p.ShutdownChan)
	return nil
}

func (p *ProviderAPI) StorageAddLocal(ctx context.Context, path string) error {
	path, err := homedir.Expand(path)
	if err != nil {
		return xerrors.Errorf("expanding local path: %w", err)
	}

	if err := p.LocalStore.OpenPath(ctx, path); err != nil {
		return xerrors.Errorf("opening local path: %w", err)
	}

	if err := p.LocalPaths.SetStorage(func(sc *storiface.StorageConfig) {
		sc.StoragePaths = append(sc.StoragePaths, storiface.LocalPath{Path: path})
	}); err != nil {
		return xerrors.Errorf("get storage config: %w", err)
	}

	return nil
}

func ListenAndServe(ctx context.Context, dependencies *deps.Deps, shutdownChan chan struct{}) error {
	fh := &paths.FetchHandler{Local: dependencies.LocalStore, PfHandler: &paths.DefaultPartialFileHandler{}}
	remoteHandler := func(w http.ResponseWriter, r *http.Request) {
		if !auth.HasPerm(r.Context(), nil, api.PermAdmin) {
			w.WriteHeader(401)
			_ = json.NewEncoder(w).Encode(struct{ Error string }{"unauthorized: missing admin permission"})
			return
		}

		fh.ServeHTTP(w, r)
	}

	var authVerify func(context.Context, string) ([]auth.Permission, error)
	{
		privateKey, err := base64.StdEncoding.DecodeString(dependencies.Cfg.Apis.StorageRPCSecret)
		if err != nil {
			return xerrors.Errorf("decoding storage rpc secret: %w", err)
		}
		authVerify = func(ctx context.Context, token string) ([]auth.Permission, error) {
			var payload deps.JwtPayload
			if _, err := jwt.Verify([]byte(token), jwt.NewHS256(privateKey), &payload); err != nil {
				return nil, xerrors.Errorf("JWT Verification failed: %w", err)
			}

			return payload.Allow, nil
		}
	}
	// Serve the RPC.
	srv := &http.Server{
		Handler: LotusProviderHandler(
			authVerify,
			remoteHandler,
			&ProviderAPI{dependencies, shutdownChan},
			permissioned),
		ReadHeaderTimeout: time.Minute * 3,
		BaseContext: func(listener net.Listener) context.Context {
			ctx, _ := tag.New(context.Background(), tag.Upsert(metrics.APIInterface, "lotus-worker"))
			return ctx
		},
		Addr: dependencies.ListenAddr,
	}

	log.Infof("Setting up RPC server at %s", dependencies.ListenAddr)
	eg := errgroup.Group{}
	eg.Go(srv.ListenAndServe)

	if dependencies.Cfg.Subsystems.EnableWebGui {
		web, err := lpweb.GetSrv(ctx, dependencies)
		if err != nil {
			return err
		}

		go func() {
			<-ctx.Done()
			log.Warn("Shutting down...")
			if err := srv.Shutdown(context.TODO()); err != nil {
				log.Errorf("shutting down RPC server failed: %s", err)
			}
			if err := web.Shutdown(context.Background()); err != nil {
				log.Errorf("shutting down web server failed: %s", err)
			}
			log.Warn("Graceful shutdown successful")
		}()
		log.Infof("Setting up web server at %s", dependencies.Cfg.Subsystems.GuiAddress)
		eg.Go(web.ListenAndServe)
	}
	return eg.Wait()
}

func GetProviderAPI(ctx *cli.Context) (api.LotusProvider, jsonrpc.ClientCloser, error) {
	addr, headers, err := cliutil.GetRawAPI(ctx, repo.Provider, "v0")
	if err != nil {
		return nil, nil, err
	}

	u, err := url.Parse(addr)
	if err != nil {
		return nil, nil, xerrors.Errorf("parsing miner api URL: %w", err)
	}

	switch u.Scheme {
	case "ws":
		u.Scheme = "http"
	case "wss":
		u.Scheme = "https"
	}

	addr = u.String()

	return client.NewProviderRpc(ctx.Context, addr, headers)
}
