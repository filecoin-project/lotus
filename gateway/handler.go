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

type perConnectionAPIRateLimiterKeyType string
type filterTrackerKeyType string

const (
	perConnectionAPIRateLimiterKey   perConnectionAPIRateLimiterKeyType = "limiter"
	statefulCallTrackerKey           filterTrackerKeyType               = "statefulCallTracker"
	connectionLimiterCleanupInterval                                    = 30 * time.Second
)

// ShutdownHandler is an http.Handler that can be gracefully shutdown.
type ShutdownHandler interface {
	http.Handler

	Shutdown(ctx context.Context) error
}

var _ ShutdownHandler = &statefulCallHandler{}
var _ ShutdownHandler = &RateLimitHandler{}

// Handler returns a gateway http.Handler, to be mounted as-is on the server. The handler is
// returned as a ShutdownHandler which allows for graceful shutdown of the handler via its
// Shutdown method.
//
// The handler will limit the number of API calls per minute within a single WebSocket connection
// (where API calls are weighted by their relative expense), and the number of connections per
// minute from a single host.
//
// Connection limiting is a hard limit that will reject requests with a 429 status code if the limit
// is exceeded. API call limiting is a soft limit that will delay requests if the limit is exceeded.
func Handler(
	gwapi lapi.Gateway,
	api lapi.FullNode,
	perConnectionAPIRateLimit int,
	perHostConnectionsPerMinute int,
	opts ...jsonrpc.ServerOption,
) (ShutdownHandler, error) {

	m := mux.NewRouter()

	opts = append(opts, jsonrpc.WithReverseClient[lapi.EthSubscriberMethods]("Filecoin"), jsonrpc.WithServerErrors(lapi.RPCErrors))
	serveRpc := func(path string, hnd interface{}) {
		rpcServer := jsonrpc.NewServer(opts...)
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

	handler := &statefulCallHandler{m}
	if perConnectionAPIRateLimit > 0 && perHostConnectionsPerMinute > 0 {
		return NewRateLimitHandler(
			handler,
			perConnectionAPIRateLimit,
			perHostConnectionsPerMinute,
			connectionLimiterCleanupInterval,
		), nil
	}
	return handler, nil
}

type statefulCallHandler struct {
	next http.Handler
}

func (h statefulCallHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	r = r.WithContext(context.WithValue(r.Context(), statefulCallTrackerKey, newStatefulCallTracker()))
	h.next.ServeHTTP(w, r)
}

func (h statefulCallHandler) Shutdown(ctx context.Context) error {
	return shutdown(ctx, h.next)
}

type hostLimiter struct {
	limiter    *rate.Limiter
	lastAccess time.Time
}

type RateLimitHandler struct {
	cancelFunc                  context.CancelFunc
	mu                          sync.Mutex
	limiters                    map[string]*hostLimiter
	perConnectionAPILimit       rate.Limit
	perHostConnectionsPerMinute int
	next                        http.Handler
	cleanupInterval             time.Duration
	expiryDuration              time.Duration
}

// NewRateLimitHandler creates a new RateLimitHandler that wraps the
// provided handler and limits the number of API calls per minute within a single WebSocket
// connection (where API calls are weighted by their relative expense), and the number of
// connections per minute from a single host.
// The cleanupInterval determines how often the handler will check for unused limiters to clean up.
func NewRateLimitHandler(
	next http.Handler,
	perConnectionAPIRateLimit int,
	perHostConnectionsPerMinute int,
	cleanupInterval time.Duration,
) *RateLimitHandler {

	ctx, cancel := context.WithCancel(context.Background())
	h := &RateLimitHandler{
		cancelFunc:                  cancel,
		limiters:                    make(map[string]*hostLimiter),
		perConnectionAPILimit:       rate.Inf,
		perHostConnectionsPerMinute: perHostConnectionsPerMinute,
		next:                        next,
		cleanupInterval:             cleanupInterval,
		expiryDuration:              5 * cleanupInterval,
	}
	if perConnectionAPIRateLimit > 0 {
		h.perConnectionAPILimit = rate.Every(time.Second / time.Duration(perConnectionAPIRateLimit))
	}
	go h.cleanupExpiredLimiters(ctx)
	return h
}

func (h *RateLimitHandler) getLimits(host string) *hostLimiter {
	h.mu.Lock()
	defer h.mu.Unlock()

	entry, exists := h.limiters[host]
	if !exists {
		var limiter *rate.Limiter
		if h.perHostConnectionsPerMinute > 0 {
			requestLimit := rate.Every(time.Minute / time.Duration(h.perHostConnectionsPerMinute))
			limiter = rate.NewLimiter(requestLimit, h.perHostConnectionsPerMinute)
		}
		entry = &hostLimiter{
			limiter:    limiter,
			lastAccess: time.Now(),
		}
		h.limiters[host] = entry
	} else {
		entry.lastAccess = time.Now()
	}

	return entry
}

func (h *RateLimitHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	limits := h.getLimits(host)
	if limits.limiter != nil && !limits.limiter.Allow() {
		w.WriteHeader(http.StatusTooManyRequests)
		return
	}

	if h.perConnectionAPILimit != rate.Inf {
		// new rate limiter for each connection, to throttle a single WebSockets connection;
		// allow for a burst of MaxRateLimitTokens
		apiLimiter := rate.NewLimiter(h.perConnectionAPILimit, MaxRateLimitTokens)
		r = r.WithContext(context.WithValue(r.Context(), perConnectionAPIRateLimiterKey, apiLimiter))
	}

	h.next.ServeHTTP(w, r)
}

func (h *RateLimitHandler) cleanupExpiredLimiters(ctx context.Context) {
	if h.cleanupInterval == 0 {
		return
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(h.cleanupInterval):
			h.mu.Lock()
			now := time.Now()
			for host, entry := range h.limiters {
				if now.Sub(entry.lastAccess) > h.expiryDuration {
					delete(h.limiters, host)
				}
			}
			h.mu.Unlock()
		}
	}
}

func (h *RateLimitHandler) Shutdown(ctx context.Context) error {
	h.cancelFunc()
	return shutdown(ctx, h.next)
}

func shutdown(ctx context.Context, handler http.Handler) error {
	if sh, ok := handler.(ShutdownHandler); ok {
		return sh.Shutdown(ctx)
	}
	return nil
}
