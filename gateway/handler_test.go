package gateway_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/lotus/gateway"
)

func TestRequestRateLimiterHandler(t *testing.T) {
	var callCount int
	h := gateway.NewRateLimitHandler(
		http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
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
