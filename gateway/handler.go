package gateway

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"strings"
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
	statefulCallTrackerKeyV1         filterTrackerKeyType               = "statefulCallTrackerV1"
	statefulCallTrackerKeyV2         filterTrackerKeyType               = "statefulCallTrackerV2"
	connectionLimiterCleanupInterval                                    = 30 * time.Second
)

// ShutdownHandler is an http.Handler that can be gracefully shutdown.
type ShutdownHandler interface {
	http.Handler

	Shutdown(ctx context.Context) error
}

var _ ShutdownHandler = (*statefulCallHandler)(nil)
var _ ShutdownHandler = (*RateLimitHandler)(nil)
var _ ShutdownHandler = (*CORSHandler)(nil)
var _ ShutdownHandler = (*LoggingHandler)(nil)

// handlerOptions holds the options for the Handler function.
type handlerOptions struct {
	perConnectionAPIRateLimit   int
	perHostConnectionsPerMinute int
	jsonrpcServerOptions        []jsonrpc.ServerOption
	enableCORS                  bool
	enableRequestLogging        bool
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

// WithCORS sets whether to enable CORS headers to allow cross-origin requests from web browsers.
func WithCORS(enable bool) HandlerOption {
	return func(opts *handlerOptions) {
		opts.enableCORS = enable
	}
}

// WithRequestLogging sets whether to enable request logging.
func WithRequestLogging(enable bool) HandlerOption {
	return func(opts *handlerOptions) {
		opts.enableRequestLogging = enable
	}
}

// Handler returns a gateway http.Handler, to be mounted as-is on the server. The handler is
// returned as a ShutdownHandler which allows for graceful shutdown of the handler via its
// Shutdown method.
func Handler(gateway *Node, options ...HandlerOption) (ShutdownHandler, error) {
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

	v2Gateway := proxy.MetricedGatewayV2API(gateway.V2ReverseProxy())
	v1Gateway := proxy.MetricedGatewayAPI(gateway.V1ReverseProxy())
	v0Gateway := lapi.Wrap(new(v1api.FullNodeStruct), new(v0api.WrapperV1Full), v1Gateway)
	serveRpc("/rpc/v2", v2Gateway)
	serveRpc("/rpc/v1", v1Gateway)
	serveRpc("/rpc/v0", v0Gateway)

	registry := promclient.DefaultRegisterer.(*promclient.Registry)
	exporter, err := prometheus.NewExporter(prometheus.Options{
		Registry:  registry,
		Namespace: "lotus_gw",
	})
	if err != nil {
		return nil, err
	}
	m.Handle("/debug/metrics", exporter)
	m.Handle("/health/livez", node.NewLiveHandler(gateway.v1Proxy.server))
	m.Handle("/health/readyz", node.NewReadyHandler(gateway.v1Proxy.server))
	m.PathPrefix("/").Handler(http.DefaultServeMux)

	var handler http.Handler = &statefulCallHandler{m}

	// Apply logging middleware if enabled
	if opts.enableRequestLogging {
		handler = NewLoggingHandler(handler)
	}

	// Apply CORS wrapper if enabled
	if opts.enableCORS {
		handler = NewCORSHandler(handler)
	}

	// Apply rate limiting wrapper if enabled
	if opts.perConnectionAPIRateLimit > 0 || opts.perHostConnectionsPerMinute > 0 {
		handler = NewRateLimitHandler(
			handler,
			opts.perConnectionAPIRateLimit,
			opts.perHostConnectionsPerMinute,
			connectionLimiterCleanupInterval,
		)
	}

	return handler.(ShutdownHandler), nil
}

type statefulCallHandler struct {
	next http.Handler
}

func (h statefulCallHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	tracker := newStatefulCallTracker()
	defer func() {
		go tracker.cleanup()
	}()
	if strings.HasPrefix(r.URL.Path, "/rpc/v2") {
		// Scope v2 handling of stateful calls separately to avoid any accidental
		// cross-contamination in request handling between the two.
		r = r.WithContext(context.WithValue(r.Context(), statefulCallTrackerKeyV2, tracker))
	} else {
		r = r.WithContext(context.WithValue(r.Context(), statefulCallTrackerKeyV1, tracker))
	}
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

// CORSHandler handles CORS headers for cross-origin requests.
type CORSHandler struct {
	next http.Handler
}

// NewCORSHandler creates a new CORSHandler that wraps the provided handler
// and adds appropriate CORS headers to allow cross-origin requests from web browsers.
func NewCORSHandler(next http.Handler) *CORSHandler {
	return &CORSHandler{next: next}
}

func (h *CORSHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Set CORS headers
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, Content-Length, Accept-Encoding, Authorization, X-Requested-With")
	w.Header().Set("Access-Control-Max-Age", "86400") // 24 hours

	// Handle preflight OPTIONS requests
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	h.next.ServeHTTP(w, r)
}

func (h *CORSHandler) Shutdown(ctx context.Context) error {
	return shutdown(ctx, h.next)
}

// LoggingHandler logs incoming HTTP requests with details
type LoggingHandler struct {
	next http.Handler
}

// NewLoggingHandler creates a new LoggingHandler that logs request details
func NewLoggingHandler(next http.Handler) *LoggingHandler {
	return &LoggingHandler{next: next}
}

func (h *LoggingHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Prepare log fields
	logFields := []interface{}{
		"remote_ip", getRemoteIP(r),
		"method", r.Method,
		"url", r.URL.String(),
	}

	// For POST requests, try to read and log up to maxLogBodyBytes of the body
	const maxLogBodyBytes = 1024
	if r.Method == http.MethodPost {
		limited := &io.LimitedReader{R: r.Body, N: maxLogBodyBytes + 1}
		buf, err := io.ReadAll(limited)
		if err == nil {
			var bodyStr string
			if int64(len(buf)) > maxLogBodyBytes {
				bodyStr = string(buf[:maxLogBodyBytes]) + "...[truncated]"
			} else {
				bodyStr = string(buf)
			}
			logFields = append(logFields, "body", bodyStr)
			// Reconstruct the body for downstream handlers: combine what we read and the rest
			rest := io.MultiReader(bytes.NewReader(buf), r.Body)
			r.Body = io.NopCloser(rest)
		}
	}

	log.Infow("request", logFields...)

	h.next.ServeHTTP(w, r)
}

func (h *LoggingHandler) Shutdown(ctx context.Context) error {
	return shutdown(ctx, h.next)
}

// getRemoteIP returns the remote IP address from the request.
func getRemoteIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func shutdown(ctx context.Context, handler http.Handler) error {
	if sh, ok := handler.(ShutdownHandler); ok {
		return sh.Shutdown(ctx)
	}
	return nil
}
