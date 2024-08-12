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

var _ ShutdownHandler = (*statefulCallHandler)(nil)
var _ ShutdownHandler = (*RateLimitHandler)(nil)

// handlerOptions holds the options for the Handler function.
type handlerOptions struct {
	perConnectionAPIRateLimit   int
	perHostConnectionsPerMinute int
	jsonrpcServerOptions        []jsonrpc.ServerOption
}

// HandlerOption is a functional option for configuring the Handler.
type HandlerOption func(*handlerOptions)

// WithPerConnectionAPIRateLimit sets the per connection API rate limit.
//
// The handler will limit the number of API calls per minute within a single WebSocket connection
// (where API calls are weighted by their relative expense), and the number of connections per
// minute from a single host.
func WithPerConnectionAPIRateLimit(limit int) HandlerOption {
	return func(opts *handlerOptions) {
		opts.perConnectionAPIRateLimit = limit
	}
}

// WithPerHostConnectionsPerMinute sets the per host connections per minute limit.
//
// Connection limiting is a hard limit that will reject requests with a http.StatusTooManyRequests
// status code if the limit is exceeded. API call limiting is a soft limit that will delay requests
// if the limit is exceeded.
func WithPerHostConnectionsPerMinute(limit int) HandlerOption {
	return func(opts *handlerOptions) {
		opts.perHostConnectionsPerMinute = limit
	}
}

// WithJsonrpcServerOptions sets the JSON-RPC server options.
func WithJsonrpcServerOptions(options ...jsonrpc.ServerOption) HandlerOption {
	return func(opts *handlerOptions) {
		opts.jsonrpcServerOptions = options
	}
}

// Handler returns a gateway http.Handler, to be mounted as-is on the server. The handler is
// returned as a ShutdownHandler which allows for graceful shutdown of the handler via its
// Shutdown method.
func Handler(gwapi lapi.Gateway, api lapi.FullNode, options ...HandlerOption) (ShutdownHandler, error) {
	opts := &handlerOptions{}
	for _, option := range options {
		option(opts)
	}

	m := mux.NewRouter()

	rpcopts := append(opts.jsonrpcServerOptions, jsonrpc.WithReverseClient[lapi.EthSubscriberMethods]("Filecoin"), jsonrpc.WithServerErrors(lapi.RPCErrors))
	serveRpc := func(path string, hnd interface{}) {
		rpcServer := jsonrpc.NewServer(rpcopts...)
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
	if opts.perConnectionAPIRateLimit > 0 || opts.perHostConnectionsPerMinute > 0 {
		return NewRateLimitHandler(
			handler,
			opts.perConnectionAPIRateLimit,
			opts.perHostConnectionsPerMinute,
			connectionLimiterCleanupInterval,
		), nil
	}
	return handler, nil
}

type statefulCallHandler struct {
	next http.Handler
}

func (h statefulCallHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	tracker := newStatefulCallTracker()
	defer func() {
		go tracker.cleanup()
	}()
	r = r.WithContext(context.WithValue(r.Context(), statefulCallTrackerKey, tracker))
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
	cancelFunc                   context.CancelFunc
	limiters                     map[string]*hostLimiter
	limitersLk                   sync.Mutex
	perConnectionAPILimit        rate.Limit
	perHostConnectionsLimit      rate.Limit
	perHostConnectionsLimitBurst int
	next                         http.Handler
	cleanupInterval              time.Duration
	expiryDuration               time.Duration
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
		cancelFunc:              cancel,
		limiters:                make(map[string]*hostLimiter),
		perConnectionAPILimit:   rate.Inf,
		perHostConnectionsLimit: rate.Inf,
		next:                    next,
		cleanupInterval:         cleanupInterval,
		expiryDuration:          5 * cleanupInterval,
	}
	if perConnectionAPIRateLimit > 0 {
		h.perConnectionAPILimit = rate.Every(time.Second / time.Duration(perConnectionAPIRateLimit))
	}
	if perHostConnectionsPerMinute > 0 {
		h.perHostConnectionsLimit = rate.Every(time.Minute / time.Duration(perHostConnectionsPerMinute))
		h.perHostConnectionsLimitBurst = perHostConnectionsPerMinute
	}
	go h.cleanupExpiredLimiters(ctx)
	return h
}

func (h *RateLimitHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.perHostConnectionsLimit != rate.Inf {
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		h.limitersLk.Lock()
		entry, exists := h.limiters[host]
		if !exists {
			entry = &hostLimiter{
				limiter:    rate.NewLimiter(h.perHostConnectionsLimit, h.perHostConnectionsLimitBurst),
				lastAccess: time.Now(),
			}
			h.limiters[host] = entry
		} else {
			entry.lastAccess = time.Now()
		}
		h.limitersLk.Unlock()

		if !entry.limiter.Allow() {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
	}

	if h.perConnectionAPILimit != rate.Inf {
		// new rate limiter for each connection, to throttle a single WebSockets connection;
		// allow for a burst of MaxRateLimitTokens
		apiLimiter := rate.NewLimiter(h.perConnectionAPILimit, MaxRateLimitTokens)
		r = r.WithContext(setPerConnectionAPIRateLimiter(r.Context(), apiLimiter))
	}

	h.next.ServeHTTP(w, r)
}

// setPerConnectionAPIRateLimiter sets the rate limiter in the context.
func setPerConnectionAPIRateLimiter(ctx context.Context, limiter *rate.Limiter) context.Context {
	return context.WithValue(ctx, perConnectionAPIRateLimiterKey, limiter)
}

// getPerConnectionAPIRateLimiter retrieves the rate limiter from the context.
func getPerConnectionAPIRateLimiter(ctx context.Context) (*rate.Limiter, bool) {
	limiter, ok := ctx.Value(perConnectionAPIRateLimiterKey).(*rate.Limiter)
	return limiter, ok
}

// cleanupExpiredLimiters periodically checks for limiters that have expired and removes them.
func (h *RateLimitHandler) cleanupExpiredLimiters(ctx context.Context) {
	if h.cleanupInterval == 0 {
		return
	}

	ticker := time.NewTicker(h.cleanupInterval)
	defer ticker.Stop()

	for ctx.Err() == nil {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			h.limitersLk.Lock()
			now := time.Now()
			for host, entry := range h.limiters {
				if now.Sub(entry.lastAccess) > h.expiryDuration {
					delete(h.limiters, host)
				}
			}
			h.limitersLk.Unlock()
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
