package node

import (
	"context"
	"net"
	"net/http"
	_ "net/http/pprof"
	"runtime"
	"strconv"
	"time"

	"github.com/gorilla/mux"
	logging "github.com/ipfs/go-log/v2"
	"github.com/multiformats/go-multiaddr"
	manet "github.com/multiformats/go-multiaddr/net"
	"go.opencensus.io/tag"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-jsonrpc"
	"github.com/filecoin-project/go-jsonrpc/auth"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/api/v0api"
	"github.com/filecoin-project/lotus/api/v1api"
	"github.com/filecoin-project/lotus/api/v2api"
	"github.com/filecoin-project/lotus/lib/rpcenc"
	"github.com/filecoin-project/lotus/metrics"
	"github.com/filecoin-project/lotus/metrics/proxy"
	"github.com/filecoin-project/lotus/node/impl"
)

var rpclog = logging.Logger("rpc")

// ServeRPC serves an HTTP handler over the supplied listen multiaddr.
//
// This function spawns a goroutine to run the server, and returns immediately.
// It returns the stop function to be called to terminate the endpoint.
//
// The supplied ID is used in tracing, by inserting a tag in the context.
func ServeRPC(h http.Handler, id string, addr multiaddr.Multiaddr) (StopFunc, error) {
	// Start listening to the addr; if invalid or occupied, we will fail early.
	lst, err := manet.Listen(addr)
	if err != nil {
		return nil, xerrors.Errorf("could not listen: %w", err)
	}

	// Instantiate the server and start listening.
	srv := &http.Server{
		Handler:           h,
		ReadHeaderTimeout: 30 * time.Second,
		BaseContext: func(listener net.Listener) context.Context {
			ctx := context.Background()
			ctx = metrics.AddNetworkTag(ctx)
			ctx, _ = tag.New(ctx, tag.Upsert(metrics.APIInterface, id))
			return ctx
		},
	}

	go func() {
		err = srv.Serve(manet.NetListener(lst))
		if err != http.ErrServerClosed {
			rpclog.Warnf("rpc server failed: %s", err)
		}
	}()

	return srv.Shutdown, err
}

// FullNodeHandler returns a full node handler, to be mounted as-is on the server.
func FullNodeHandler(v1 v1api.FullNode, v2 v2api.FullNode, permissioned bool, opts ...jsonrpc.ServerOption) (http.Handler, error) {
	m := mux.NewRouter()

	serveRpc := func(path string, hnd interface{}) {
		rpcServer := jsonrpc.NewServer(append(opts, jsonrpc.WithReverseClient[api.EthSubscriberMethods]("Filecoin"), jsonrpc.WithServerErrors(api.RPCErrors))...)
		rpcServer.Register("Filecoin", hnd)
		rpcServer.AliasMethod("rpc.discover", "Filecoin.Discover")

		api.CreateEthRPCAliases(rpcServer)

		var handler http.Handler = rpcServer
		if permissioned {
			handler = &auth.Handler{Verify: v1.AuthVerify, Next: rpcServer.ServeHTTP}
		}

		m.Handle(path, handler)
	}

	v1Proxy := proxy.MetricedFullAPI(v1)
	v2Proxy := proxy.MetricedFullV2API(v2)
	if permissioned {
		v1Proxy = api.PermissionedFullAPI(v1Proxy)
		v2Proxy = v2api.PermissionedFullAPI(v2Proxy)
	}
	v0Proxy := &v0api.WrapperV1Full{FullNode: v1Proxy}

	serveRpc("/rpc/v1", v1Proxy)
	serveRpc("/rpc/v2", v2Proxy)
	serveRpc("/rpc/v0", v0Proxy)

	// debugging
	m.Handle("/debug/metrics", metrics.Exporter())
	m.Handle("/debug/pprof-set/block", handleFractionOpt("BlockProfileRate", runtime.SetBlockProfileRate))
	m.Handle("/debug/pprof-set/mutex", handleFractionOpt("MutexProfileFraction", setMutexProfileFraction))
	m.Handle("/health/livez", NewLiveHandler(v1))
	m.Handle("/health/readyz", NewReadyHandler(v1))
	m.PathPrefix("/").Handler(http.DefaultServeMux) // pprof

	return m, nil
}

func setMutexProfileFraction(to int) {
	from := runtime.SetMutexProfileFraction(to)
	log.Debugw("Mutex profile fraction rate is set", "from", from, "to", to)
}

// MinerHandler returns a miner handler, to be mounted as-is on the server.
func MinerHandler(a api.StorageMiner, permissioned bool) (http.Handler, error) {
	mapi := proxy.MetricedStorMinerAPI(a)
	if permissioned {
		mapi = api.PermissionedStorMinerAPI(mapi)
	}

	readerHandler, readerServerOpt := rpcenc.ReaderParamDecoder()
	rpcServer := jsonrpc.NewServer(jsonrpc.WithServerErrors(api.RPCErrors), readerServerOpt)
	rpcServer.Register("Filecoin", mapi)
	rpcServer.AliasMethod("rpc.discover", "Filecoin.Discover")

	rootMux := mux.NewRouter()

	// remote storage
	if _, realImpl := a.(*impl.StorageMinerAPI); realImpl {
		m := mux.NewRouter()
		m.PathPrefix("/remote").HandlerFunc(a.(*impl.StorageMinerAPI).ServeRemote(permissioned))

		var hnd http.Handler = m
		if permissioned {
			hnd = &auth.Handler{
				Verify: a.StorageAuthVerify,
				Next:   m.ServeHTTP,
			}
		}

		rootMux.PathPrefix("/remote").Handler(hnd)
	}

	// local APIs
	{
		m := mux.NewRouter()
		m.Handle("/rpc/v0", rpcServer)
		m.Handle("/rpc/streams/v0/push/{uuid}", readerHandler)
		// debugging
		m.Handle("/debug/metrics", metrics.Exporter())
		m.PathPrefix("/").Handler(http.DefaultServeMux) // pprof

		var hnd http.Handler = m
		if permissioned {
			hnd = &auth.Handler{
				Verify: a.AuthVerify,
				Next:   m.ServeHTTP,
			}
		}

		rootMux.PathPrefix("/").Handler(hnd)
	}

	return rootMux, nil
}

func handleFractionOpt(name string, setter func(int)) http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(rw, "only POST allowed", http.StatusMethodNotAllowed)
			return
		}
		if err := r.ParseForm(); err != nil {
			http.Error(rw, err.Error(), http.StatusBadRequest)
			return
		}

		asfr := r.Form.Get("x")
		if len(asfr) == 0 {
			http.Error(rw, "parameter 'x' must be set", http.StatusBadRequest)
			return
		}

		fr, err := strconv.Atoi(asfr)
		if err != nil {
			http.Error(rw, err.Error(), http.StatusBadRequest)
			return
		}
		rpclog.Infof("setting %s to %d", name, fr)
		setter(fr)
	}
}
