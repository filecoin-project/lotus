package gateway

import (
	"net/http"

	"contrib.go.opencensus.io/exporter/prometheus"
	"github.com/filecoin-project/go-jsonrpc"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/api/v0api"
	"github.com/filecoin-project/lotus/api/v1api"
	"github.com/filecoin-project/lotus/metrics/proxy"
	"github.com/gorilla/mux"
	promclient "github.com/prometheus/client_golang/prometheus"
)

// Handler returns a gateway http.Handler, to be mounted as-is on the server.
func Handler(a api.Gateway, opts ...jsonrpc.ServerOption) (http.Handler, error) {
	m := mux.NewRouter()

	serveRpc := func(path string, hnd interface{}) {
		rpcServer := jsonrpc.NewServer(opts...)
		rpcServer.Register("Filecoin", hnd)
		m.Handle(path, rpcServer)
	}

	ma := proxy.MetricedGatewayAPI(a)

	serveRpc("/rpc/v1", ma)
	serveRpc("/rpc/v0", api.Wrap(new(v1api.FullNodeStruct), new(v0api.WrapperV1Full), ma))

	registry := promclient.DefaultRegisterer.(*promclient.Registry)
	exporter, err := prometheus.NewExporter(prometheus.Options{
		Registry:  registry,
		Namespace: "lotus_gw",
	})
	if err != nil {
		return nil, err
	}
	m.Handle("/debug/metrics", exporter)
	m.PathPrefix("/").Handler(http.DefaultServeMux)

	/*ah := &auth.Handler{
		Verify: nodeApi.AuthVerify,
		Next:   mux.ServeHTTP,
	}*/

	return m, nil
}
