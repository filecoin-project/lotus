package node

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/api/v2api"
)

func TestFullNodeHandlerDebugEndpointsLocalOnly(t *testing.T) {
	handler, err := FullNodeHandler(&api.FullNodeStruct{}, &v2api.FullNodeStruct{}, true)
	require.NoError(t, err)

	serve := func(method, path, remote string) int {
		req := httptest.NewRequest(method, path, nil)
		req.Host = "127.0.0.1:1234"
		req.RemoteAddr = remote
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec.Code
	}

	for _, tc := range []struct {
		method, path string
	}{
		{http.MethodGet, "/debug/pprof/"},
		{http.MethodGet, "/debug/pprof/cmdline"},
		{http.MethodGet, "/debug/vars"},
		{http.MethodPost, "/debug/pprof-set/block"},
		{http.MethodPost, "/debug/pprof-set/mutex"},
	} {
		require.Equal(t, http.StatusForbidden, serve(tc.method, tc.path, "192.168.1.5:5000"), "%s %s", tc.method, tc.path)
		require.NotEqual(t, http.StatusForbidden, serve(tc.method, tc.path, "127.0.0.1:5000"), "%s %s", tc.method, tc.path)
	}

	require.Equal(t, http.StatusOK, serve(http.MethodGet, "/debug/metrics", "192.168.1.5:5000"))
}
