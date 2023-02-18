package node

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	_ "net/http/pprof"
	"runtime"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/ipfs/go-cid"
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
	bstore "github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/lib/rpcenc"
	"github.com/filecoin-project/lotus/metrics"
	"github.com/filecoin-project/lotus/metrics/proxy"
	"github.com/filecoin-project/lotus/node/impl"
	"github.com/filecoin-project/lotus/node/impl/client"
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
			ctx, _ := tag.New(context.Background(), tag.Upsert(metrics.APIInterface, id))
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
func FullNodeHandler(a v1api.FullNode, permissioned bool, opts ...jsonrpc.ServerOption) (http.Handler, error) {
	m := mux.NewRouter()

	serveRpc := func(path string, hnd interface{}) {
		rpcServer := jsonrpc.NewServer(append(opts, jsonrpc.WithReverseClient[api.EthSubscriberMethods]("Filecoin"), jsonrpc.WithServerErrors(api.RPCErrors))...)
		rpcServer.Register("Filecoin", hnd)
		rpcServer.AliasMethod("rpc.discover", "Filecoin.Discover")

		api.CreateEthRPCAliases(rpcServer)

		var handler http.Handler = rpcServer
		if permissioned {
			handler = &auth.Handler{Verify: a.AuthVerify, Next: rpcServer.ServeHTTP}
		}

		m.Handle(path, handler)
	}

	fnapi := proxy.MetricedFullAPI(a)
	if permissioned {
		fnapi = api.PermissionedFullAPI(fnapi)
	}

	var v0 v0api.FullNode = &(struct{ v0api.FullNode }{&v0api.WrapperV1Full{FullNode: fnapi}})
	serveRpc("/rpc/v1", fnapi)
	serveRpc("/rpc/v0", v0)

	// Import handler
	handleImportFunc := handleImport(a.(*impl.FullNodeAPI))
	handleExportFunc := handleExport(a.(*impl.FullNodeAPI))
	handleRemoteStoreFunc := handleRemoteStore(a.(*impl.FullNodeAPI))
	if permissioned {
		importAH := &auth.Handler{
			Verify: a.AuthVerify,
			Next:   handleImportFunc,
		}
		m.Handle("/rest/v0/import", importAH)
		exportAH := &auth.Handler{
			Verify: a.AuthVerify,
			Next:   handleExportFunc,
		}
		m.Handle("/rest/v0/export", exportAH)

		storeAH := &auth.Handler{
			Verify: a.AuthVerify,
			Next:   handleRemoteStoreFunc,
		}
		m.Handle("/rest/v0/store/{uuid}", storeAH)
	} else {
		m.HandleFunc("/rest/v0/import", handleImportFunc)
		m.HandleFunc("/rest/v0/export", handleExportFunc)
		m.HandleFunc("/rest/v0/store/{uuid}", handleRemoteStoreFunc)
	}

	// debugging
	m.Handle("/debug/metrics", metrics.Exporter())
	m.Handle("/debug/pprof-set/block", handleFractionOpt("BlockProfileRate", runtime.SetBlockProfileRate))
	m.Handle("/debug/pprof-set/mutex", handleFractionOpt("MutexProfileFraction", func(x int) {
		runtime.SetMutexProfileFraction(x)
	}))
	m.Handle("/health/livez", NewLiveHandler(a))
	m.Handle("/health/readyz", NewReadyHandler(a))
	m.PathPrefix("/").Handler(http.DefaultServeMux) // pprof

	return m, nil
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
	{
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

func handleImport(a *impl.FullNodeAPI) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "PUT" {
			w.WriteHeader(404)
			return
		}
		if !auth.HasPerm(r.Context(), nil, api.PermWrite) {
			w.WriteHeader(401)
			_ = json.NewEncoder(w).Encode(struct{ Error string }{"unauthorized: missing write permission"})
			return
		}

		c, err := a.ClientImportLocal(r.Context(), r.Body)
		if err != nil {
			w.WriteHeader(500)
			_ = json.NewEncoder(w).Encode(struct{ Error string }{err.Error()})
			return
		}
		w.WriteHeader(200)
		err = json.NewEncoder(w).Encode(struct{ Cid cid.Cid }{c})
		if err != nil {
			rpclog.Errorf("/rest/v0/import: Writing response failed: %+v", err)
			return
		}
	}
}

func handleExport(a *impl.FullNodeAPI) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			w.WriteHeader(404)
			return
		}
		if !auth.HasPerm(r.Context(), nil, api.PermWrite) {
			w.WriteHeader(401)
			_ = json.NewEncoder(w).Encode(struct{ Error string }{"unauthorized: missing write permission"})
			return
		}

		var eref api.ExportRef
		if err := json.Unmarshal([]byte(r.FormValue("export")), &eref); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		car := r.FormValue("car") == "true"

		err := a.ClientExportInto(r.Context(), eref, car, client.ExportDest{Writer: w})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
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

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

func handleRemoteStore(a *impl.FullNodeAPI) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		id, err := uuid.Parse(vars["uuid"])
		if err != nil {
			http.Error(w, fmt.Sprintf("parse uuid: %s", err), http.StatusBadRequest)
			return
		}

		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Error(err)
			w.WriteHeader(500)
			return
		}

		nstore := bstore.NewNetworkStoreWS(c)
		if err := a.ApiBlockstoreAccessor.RegisterApiStore(id, nstore); err != nil {
			log.Errorw("registering api bstore", "error", err)
			_ = c.Close()
			return
		}
	}
}
