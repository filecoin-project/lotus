package gateway

import (
	"net/http"

	"contrib.go.opencensus.io/exporter/prometheus"
	"github.com/filecoin-project/go-jsonrpc"
	lapi "github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/api/v0api"
	"github.com/filecoin-project/lotus/api/v1api"
	"github.com/filecoin-project/lotus/metrics/proxy"
	"github.com/filecoin-project/lotus/node"
	"github.com/gorilla/mux"
	promclient "github.com/prometheus/client_golang/prometheus"
)

// Handler returns a gateway http.Handler, to be mounted as-is on the server.
func Handler(gwapi lapi.Gateway, api lapi.FullNode, opts ...jsonrpc.ServerOption) (http.Handler, error) {
	m := mux.NewRouter()

	serveRpc := func(path string, hnd interface{}) {
		rpcServer := jsonrpc.NewServer(opts...)
		rpcServer.Register("Filecoin", hnd)
		rpcServer.AliasMethod("rpc.discover", "Filecoin.Discover")

		m.Handle(path, rpcServer)
	}

	ma := proxy.MetricedGatewayAPI(gwapi)

	serveRpc("/rpc/v1", ma)
	serveRpc("/rpc/v0", lapi.Wrap(new(v1api.FullNodeStruct), new(v0api.WrapperV1Full), ma))

	registry := promclient.DefaultRegisterer.(*promclient.Registry)
	exporter, err := prometheus.NewExporter(prometheus.Options{
		Registry:  registry,
		Namespace: "lotus_gw",
	})
	if err != nil {
		return nil, err
	}
	m.Handle("/debug/metrics", exporter)
	m.Handle("/health/livez", node.NewLiveHandler(api))
	m.Handle("/health/readyz", node.NewReadyHandler(api))
	m.PathPrefix("/").Handler(http.DefaultServeMux)

	/*ah := &auth.Handler{
		Verify: nodeApi.AuthVerify,
		Next:   mux.ServeHTTP,
	}*/

	return m, nil
}
