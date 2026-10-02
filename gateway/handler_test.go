package gateway_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/api/v2api"
	"github.com/filecoin-project/lotus/gateway"
)

func TestRequestRateLimiterHandler(t *testing.T) {
	var callCount int
	h := gateway.NewRateLimitHandler(
		http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			callCount++
		}),
		0, // api rate
		2, // request rate (per minute)
		0, // cleanup interval
	)

	runRequest := func(host string, expectedStatus, expectedCallCount int) {
		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = host + ":1234"
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)

		require.Equal(t, expectedStatus, w.Code, "expected status %v, got %v", expectedStatus, w.Code)
		require.Equal(t, expectedCallCount, callCount, "expected callCount to be %v, got %v", expectedCallCount, callCount)
	}

	// Test that the handler allows up to 2 requests per minute per host.
	runRequest("boop", http.StatusOK, 1)
	runRequest("boop", http.StatusOK, 2)
	runRequest("beep", http.StatusOK, 3)
	runRequest("boop", http.StatusTooManyRequests, 3)
	runRequest("beep", http.StatusOK, 4)
	runRequest("boop", http.StatusTooManyRequests, 4)
	runRequest("beep", http.StatusTooManyRequests, 4)
}

func TestHandlerDoesNotServeGlobalDebugEndpoints(t *testing.T) {
	h, err := gateway.Handler(gateway.NewNode(&api.FullNodeStub{}, &v2api.FullNodeStub{}))
	require.NoError(t, err)

	status := func(path string) int {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		return w.Code
	}

	// net/http/pprof and expvar register these on http.DefaultServeMux from
	// package init, so they must not be reachable through the gateway.
	for _, path := range []string{
		"/debug/pprof/",
		"/debug/pprof/cmdline",
		"/debug/pprof/heap",
		"/debug/pprof/goroutine",
		"/debug/pprof/symbol",
		"/debug/vars",
	} {
		require.Equal(t, http.StatusNotFound, status(path), "%s must not be served by the gateway", path)
	}

	// The paths the gateway registers itself are still routed.
	for _, path := range []string{"/debug/metrics", "/health/livez", "/health/readyz"} {
		require.NotEqual(t, http.StatusNotFound, status(path), "%s must still be routed", path)
	}
}
