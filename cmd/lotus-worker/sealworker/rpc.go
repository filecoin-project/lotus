package sealworker

import (
	"context"
	"net/http"
	"sync/atomic"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	logging "github.com/ipfs/go-log/v2"
	"github.com/mitchellh/go-homedir"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-jsonrpc"
	"github.com/filecoin-project/go-jsonrpc/auth"

	"github.com/filecoin-project/lotus/api"
	apitypes "github.com/filecoin-project/lotus/api/types"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/lib/rpcenc"
	"github.com/filecoin-project/lotus/metrics/proxy"
	"github.com/filecoin-project/lotus/storage/paths"
	"github.com/filecoin-project/lotus/storage/sealer"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
)

var log = logging.Logger("sealworker")

func WorkerHandler(
	authv func(ctx context.Context, token string) ([]auth.Permission, error),
	remote http.HandlerFunc,
	a api.Worker,
	permissioned bool) http.Handler {
	mux := mux.NewRouter()
	readerHandler, readerServerOpt := rpcenc.ReaderParamDecoder()
	rpcServer := jsonrpc.NewServer(jsonrpc.WithServerErrors(api.RPCErrors), readerServerOpt)

	wapi := proxy.MetricedWorkerAPI(a)
	if permissioned {
		wapi = api.PermissionedWorkerAPI(wapi)
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

type Worker struct {
	*sealer.LocalWorker

	LocalStore *paths.Local
	Storage    paths.LocalStorage

	disabled int64
}

func (w *Worker) Version(context.Context) (api.Version, error) {
	return api.WorkerAPIVersion0, nil
}

func (w *Worker) StorageLocal(ctx context.Context) (map[storiface.ID]string, error) {
	l, err := w.LocalStore.Local(ctx)
	if err != nil {
		return nil, err
	}

	out := map[storiface.ID]string{}
	for _, st := range l {
		out[st.ID] = st.LocalPath
	}

	return out, nil
}

func (w *Worker) StorageAddLocal(ctx context.Context, path string) error {
	path, err := homedir.Expand(path)
	if err != nil {
		return xerrors.Errorf("expanding local path: %w", err)
	}

	if err := w.LocalStore.OpenPath(ctx, path); err != nil {
		return xerrors.Errorf("opening local path: %w", err)
	}

	if err := w.Storage.SetStorage(func(sc *storiface.StorageConfig) {
		sc.StoragePaths = append(sc.StoragePaths, storiface.LocalPath{Path: path})
	}); err != nil {
		return xerrors.Errorf("get storage config: %w", err)
	}

	return nil
}

func (w *Worker) StorageDetachLocal(ctx context.Context, path string) error {
	path, err := homedir.Expand(path)
	if err != nil {
		return xerrors.Errorf("expanding local path: %w", err)
	}

	// check that we have the path opened
	lps, err := w.LocalStore.Local(ctx)
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
	if err := w.Storage.SetStorage(func(sc *storiface.StorageConfig) {
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
	return w.LocalStore.ClosePath(ctx, localPath.ID)
}

func (w *Worker) StorageDetachAll(ctx context.Context) error {

	lps, err := w.LocalStore.Local(ctx)
	if err != nil {
		return xerrors.Errorf("getting local path list: %w", err)
	}

	for _, lp := range lps {
		err = w.LocalStore.ClosePath(ctx, lp.ID)
		if err != nil {
			log.Warnf("unable to close path: %w", err)
		}
	}

	return nil
}

func (w *Worker) StorageRedeclareLocal(ctx context.Context, id *storiface.ID, dropMissing bool) error {
	return w.LocalStore.Redeclare(ctx, id, dropMissing)
}

func (w *Worker) SetEnabled(ctx context.Context, enabled bool) error {
	disabled := int64(1)
	if enabled {
		disabled = 0
	}
	atomic.StoreInt64(&w.disabled, disabled)
	return nil
}

func (w *Worker) Enabled(ctx context.Context) (bool, error) {
	return atomic.LoadInt64(&w.disabled) == 0, nil
}

func (w *Worker) WaitQuiet(ctx context.Context) error {
	w.LocalWorker.WaitQuiet() // uses WaitGroup under the hood so no ctx :/
	return nil
}

func (w *Worker) ProcessSession(ctx context.Context) (uuid.UUID, error) {
	return w.LocalWorker.Session(ctx)
}

func (w *Worker) Session(ctx context.Context) (uuid.UUID, error) {
	if atomic.LoadInt64(&w.disabled) == 1 {
		return uuid.UUID{}, xerrors.Errorf("worker disabled")
	}

	return w.LocalWorker.Session(ctx)
}

func (w *Worker) Discover(ctx context.Context) (apitypes.OpenRPCDocument, error) {
	return build.OpenRPCDiscoverJSON_Worker(), nil
}

func (w *Worker) Shutdown(ctx context.Context) error {
	return w.LocalWorker.Close()
}

var _ storiface.WorkerCalls = &Worker{}
var _ api.Worker = &Worker{}
