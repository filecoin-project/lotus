// Package rpc provides all direct access to this node.
package rpc

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/gbrlsnchs/jwt/v3"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
	logging "github.com/ipfs/go-log/v2"
	"github.com/mitchellh/go-homedir"
	"github.com/urfave/cli/v2"
	"go.opencensus.io/tag"
	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-jsonrpc"
	"github.com/filecoin-project/go-jsonrpc/auth"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/api/client"
	cliutil "github.com/filecoin-project/lotus/cli/util"
	"github.com/filecoin-project/lotus/cmd/curio/deps"
	"github.com/filecoin-project/lotus/curiosrc/market"
	"github.com/filecoin-project/lotus/curiosrc/web"
	"github.com/filecoin-project/lotus/lib/rpcenc"
	"github.com/filecoin-project/lotus/metrics"
	"github.com/filecoin-project/lotus/metrics/proxy"
	"github.com/filecoin-project/lotus/node/repo"
	"github.com/filecoin-project/lotus/storage/paths"
	"github.com/filecoin-project/lotus/storage/pipeline/piece"
	"github.com/filecoin-project/lotus/storage/sealer/fsutil"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
)

const metaFile = "sectorstore.json"

var log = logging.Logger("curio/rpc")
var permissioned = os.Getenv("LOTUS_DISABLE_AUTH_PERMISSIONED") != "1"

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
	paths.SectorIndex
	ShutdownChan chan struct{}
}

func (p *CurioAPI) Version(context.Context) (api.Version, error) {
	return api.CurioAPIVersion0, nil
}
func (p *CurioAPI) StorageDetachLocal(ctx context.Context, path string) error {
	path, err := homedir.Expand(path)
	if err != nil {
		return xerrors.Errorf("expanding local path: %w", err)
	}

	// check that we have the path opened
	lps, err := p.LocalStore.Local(ctx)
	if err != nil {
		return xerrors.Errorf("getting local path list: %w", err)
	}

	var localPath *storiface.StoragePath
	for _, lp := range lps {
		if lp.LocalPath == path {
			lp := lp // copy to make the linter happy
			localPath = &lp
			break
		}
	}
	if localPath == nil {
		return xerrors.Errorf("no local paths match '%s'", path)
	}

	// drop from the persisted storage.json
	var found bool
	if err := p.LocalPaths.SetStorage(func(sc *storiface.StorageConfig) {
		out := make([]storiface.LocalPath, 0, len(sc.StoragePaths))
		for _, storagePath := range sc.StoragePaths {
			if storagePath.Path != path {
				out = append(out, storagePath)
				continue
			}
			found = true
		}
		sc.StoragePaths = out
	}); err != nil {
		return xerrors.Errorf("set storage config: %w", err)
	}
	if !found {
		// maybe this is fine?
		return xerrors.Errorf("path not found in storage.json")
	}

	// unregister locally, drop from sector index
	return p.LocalStore.ClosePath(ctx, localPath.ID)
}

func (p *CurioAPI) StorageLocal(ctx context.Context) (map[storiface.ID]string, error) {
	ps, err := p.LocalStore.Local(ctx)
	if err != nil {
		return nil, err
	}

	var out = make(map[storiface.ID]string)
	for _, path := range ps {
		out[path.ID] = path.LocalPath
	}

	return out, nil
}

func (p *CurioAPI) StorageStat(ctx context.Context, id storiface.ID) (fsutil.FsStat, error) {
	return p.Stor.FsStat(ctx, id)
}

func (p *CurioAPI) AllocatePieceToSector(ctx context.Context, maddr address.Address, piece piece.PieceDealInfo, rawSize int64, source url.URL, header http.Header) (api.SectorOffset, error) {
	di, err := market.NewPieceIngester(ctx, p.Deps.DB, p.Deps.Full, maddr, true, time.Minute)
	if err != nil {
		return api.SectorOffset{}, xerrors.Errorf("failed to create a piece ingestor")
	}

	sector, err := di.AllocatePieceToSector(ctx, maddr, piece, rawSize, source, header)
	if err != nil {
		return api.SectorOffset{}, xerrors.Errorf("failed to add piece to a sector")
	}

	err = di.Seal()
	if err != nil {
		return api.SectorOffset{}, xerrors.Errorf("failed to start sealing the sector %d for actor %s", sector.Sector, maddr)
	}

	return sector, nil
}

// Trigger shutdown
func (p *CurioAPI) Shutdown(context.Context) error {
	close(p.ShutdownChan)
	return nil
}

func (p *CurioAPI) StorageInit(ctx context.Context, path string, opts storiface.LocalStorageMeta) error {
	path, err := homedir.Expand(path)
	if err != nil {
		return xerrors.Errorf("expanding local path: %w", err)
	}

	if err := os.MkdirAll(path, 0755); err != nil {
		if !os.IsExist(err) {
			return err
		}
	}
	_, err = os.Stat(filepath.Join(path, metaFile))
	if !os.IsNotExist(err) {
		if err == nil {
			return xerrors.Errorf("path is already initialized")
		}
		return err
	}
	if opts.ID == "" {
		opts.ID = storiface.ID(uuid.New().String())
	}
	if !(opts.CanStore || opts.CanSeal) {
		return xerrors.Errorf("must specify at least one of --store or --seal")
	}
	b, err := json.MarshalIndent(opts, "", "  ")
	if err != nil {
		return xerrors.Errorf("marshaling storage config: %w", err)
	}
	if err := os.WriteFile(filepath.Join(path, metaFile), b, 0644); err != nil {
		return xerrors.Errorf("persisting storage metadata (%s): %w", filepath.Join(path, metaFile), err)
	}
	return nil
}

func (p *CurioAPI) StorageAddLocal(ctx context.Context, path string) error {
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

func (p *CurioAPI) LogList(ctx context.Context) ([]string, error) {
	return logging.GetSubsystems(), nil
}

func (p *CurioAPI) LogSetLevel(ctx context.Context, subsystem, level string) error {
	return logging.SetLogLevel(subsystem, level)
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
		Handler: CurioHandler(
			authVerify,
			remoteHandler,
			&CurioAPI{dependencies, dependencies.Si, shutdownChan},
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

		uiAddress := dependencies.Cfg.Subsystems.GuiAddress
		if uiAddress == "" || uiAddress[0] == ':' {
			uiAddress = "localhost" + uiAddress
		}
		log.Infof("GUI:  http://%s", uiAddress)
		eg.Go(web.ListenAndServe)
	}
	return eg.Wait()
}

func GetCurioAPI(ctx *cli.Context) (api.Curio, jsonrpc.ClientCloser, error) {
	addr, headers, err := cliutil.GetRawAPI(ctx, repo.Curio, "v0")
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

	return client.NewCurioRpc(ctx.Context, addr, headers)
}
