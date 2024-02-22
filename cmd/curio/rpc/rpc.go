// Package rpc provides all direct access to this node.
package rpc

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net"
	"net/http"
	"time"

	"github.com/gbrlsnchs/jwt/v3"
	"github.com/gorilla/mux"
	logging "github.com/ipfs/go-log/v2"
	"go.opencensus.io/tag"
	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-jsonrpc"
	"github.com/filecoin-project/go-jsonrpc/auth"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/cmd/curio/deps"
	"github.com/filecoin-project/lotus/curiosrc/web"
	"github.com/filecoin-project/lotus/lib/rpcenc"
	"github.com/filecoin-project/lotus/metrics"
	"github.com/filecoin-project/lotus/metrics/proxy"
	"github.com/filecoin-project/lotus/storage/paths"
)

var log = logging.Logger("curio/rpc")

func CurioHandler(
	authv func(ctx context.Context, token string) ([]auth.Permission, error),
	remote http.HandlerFunc,
	a api.Curio,
	permissioned bool) http.Handler {
	mux := mux.NewRouter()
	readerHandler, readerServerOpt := rpcenc.ReaderParamDecoder()
	rpcServer := jsonrpc.NewServer(jsonrpc.WithServerErrors(api.RPCErrors), readerServerOpt)

	wapi := proxy.MetricedAPI[api.Curio, api.CurioStruct](a)
	if permissioned {
		wapi = api.PermissionedAPI[api.Curio, api.CurioStruct](wapi)
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

type CurioAPI struct {
	*deps.Deps
	ShutdownChan chan struct{}
}

func (p *CurioAPI) Version(context.Context) (api.Version, error) {
	return api.CurioAPIVersion0, nil
}

// Trigger shutdown
func (p *CurioAPI) Shutdown(context.Context) error {
	close(p.ShutdownChan)
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
	// local APIs
	{
		// debugging
		mux := mux.NewRouter()
		mux.PathPrefix("/").Handler(http.DefaultServeMux) // pprof
		mux.PathPrefix("/remote").HandlerFunc(remoteHandler)
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
		Handler: CurioHandler(
			authVerify,
			remoteHandler,
			&CurioAPI{dependencies, shutdownChan},
			true),
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
		web, err := web.GetSrv(ctx, dependencies)
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
