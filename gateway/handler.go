package gateway

import (
	"context"
	"net"
	"net/http"
	"sync"
	"time"

	"contrib.go.opencensus.io/exporter/prometheus"
	"github.com/gorilla/mux"
	promclient "github.com/prometheus/client_golang/prometheus"
	"golang.org/x/time/rate"

	"github.com/filecoin-project/go-jsonrpc"

	lapi "github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/api/v0api"
	"github.com/filecoin-project/lotus/api/v1api"
	"github.com/filecoin-project/lotus/metrics/proxy"
	"github.com/filecoin-project/lotus/node"
)

type perConnLimiterKeyType string

const perConnLimiterKey perConnLimiterKeyType = "limiter"

type filterTrackerKeyType string

const statefulCallTrackerKey filterTrackerKeyType = "statefulCallTracker"

// Handler returns a gateway http.Handler, to be mounted as-is on the server.
func Handler(gwapi lapi.Gateway, api lapi.FullNode, rateLimit int64, connPerMinute int64, opts ...jsonrpc.ServerOption) (http.Handler, error) {
	m := mux.NewRouter()

	serveRpc := func(path string, hnd interface{}) {
		rpcServer := jsonrpc.NewServer(append(opts, jsonrpc.WithReverseClient[lapi.EthSubscriberMethods]("Filecoin"), jsonrpc.WithServerErrors(lapi.RPCErrors))...)
		rpcServer.Register("Filecoin", hnd)
		rpcServer.AliasMethod("rpc.discover", "Filecoin.Discover")

		lapi.CreateEthRPCAliases(rpcServer)

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

	rlh := NewRateLimiterHandler(m, rateLimit)
	clh := NewConnectionRateLimiterHandler(rlh, connPerMinute)
	return clh, nil
}

func NewRateLimiterHandler(handler http.Handler, rateLimit int64) *RateLimiterHandler {
	limiter := limiterFromRateLimit(rateLimit)

	return &RateLimiterHandler{
		handler: handler,
		limiter: limiter,
	}
}

// Adds a rate limiter to the request context for per-connection rate limiting
type RateLimiterHandler struct {
	handler http.Handler
	limiter *rate.Limiter
}

func (h RateLimiterHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	r = r.WithContext(context.WithValue(r.Context(), perConnLimiterKey, h.limiter))

	// also add a filter tracker to the context
	r = r.WithContext(context.WithValue(r.Context(), statefulCallTrackerKey, newStatefulCallTracker()))

	h.handler.ServeHTTP(w, r)
}

// this blocks new connections if there have already been too many.
func NewConnectionRateLimiterHandler(handler http.Handler, connPerMinute int64) *ConnectionRateLimiterHandler {
	ipmap := make(map[string]int64)
	return &ConnectionRateLimiterHandler{
		ipmap:         ipmap,
		connPerMinute: connPerMinute,
		handler:       handler,
	}
}

type ConnectionRateLimiterHandler struct {
	mu            sync.Mutex
	ipmap         map[string]int64
	connPerMinute int64
	handler       http.Handler
}

func (h *ConnectionRateLimiterHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.connPerMinute == 0 {
		h.handler.ServeHTTP(w, r)
		return
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	h.mu.Lock()
	seen, ok := h.ipmap[host]
	if !ok {
		h.ipmap[host] = 1
		h.mu.Unlock()
		h.handler.ServeHTTP(w, r)
		return
	}
	// rate limited
	if seen > h.connPerMinute {
		h.mu.Unlock()
		w.WriteHeader(http.StatusTooManyRequests)
		return
	}
	h.ipmap[host] = seen + 1
	h.mu.Unlock()
	go func() {
		select {
		case <-time.After(time.Minute):
			h.mu.Lock()
			defer h.mu.Unlock()
			h.ipmap[host] = h.ipmap[host] - 1
			if h.ipmap[host] <= 0 {
				delete(h.ipmap, host)
			}
		}
	}()
	h.handler.ServeHTTP(w, r)
}

func limiterFromRateLimit(rateLimit int64) *rate.Limiter {
	var limit rate.Limit
	if rateLimit == 0 {
		limit = rate.Inf
	} else {
		limit = rate.Every(time.Second / time.Duration(rateLimit))
	}
	return rate.NewLimiter(limit, stateRateLimitTokens)
}
