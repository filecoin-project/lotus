package node

import (
	"context"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-jsonrpc/auth"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/api/v2api"
)

// TestFullNodeHandlerPprofSetPermissions checks that the handlers which change
// runtime profiling rates are only reachable with an admin token.
func TestFullNodeHandlerPprofSetPermissions(t *testing.T) {
	const adminToken = "admin-token"

	var v1 api.FullNodeStruct
	v1.CommonStruct.Internal.AuthVerify = func(_ context.Context, token string) ([]auth.Permission, error) {
		if token != adminToken {
			return nil, xerrors.New("unknown token")
		}
		return api.AllPermissions, nil
	}
	var v2 v2api.FullNodeStruct

	handler, err := FullNodeHandler(&v1, &v2, true)
	require.NoError(t, err)

	initialFraction := runtime.SetMutexProfileFraction(-1)
	t.Cleanup(func() { runtime.SetMutexProfileFraction(initialFraction) })

	post := func(token string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/debug/pprof-set/mutex", strings.NewReader("x=7"))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}

	require.Equal(t, http.StatusUnauthorized, post("").Code, "no token")
	require.Equal(t, initialFraction, runtime.SetMutexProfileFraction(-1), "mutex profiling changed without a token")

	require.Equal(t, http.StatusUnauthorized, post("not-"+adminToken).Code, "bad token")
	require.Equal(t, initialFraction, runtime.SetMutexProfileFraction(-1), "mutex profiling changed with a bad token")

	require.Equal(t, http.StatusOK, post(adminToken).Code, "admin token")
	require.Equal(t, 7, runtime.SetMutexProfileFraction(-1), "admin token did not set mutex profiling")
}
