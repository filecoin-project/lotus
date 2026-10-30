package debughttp

import (
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

var ok = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})

func TestLocalOnly(t *testing.T) {
	h := LocalOnly(ok)
	for _, tc := range []struct {
		name    string
		remote  string
		host    string
		headers map[string]string
		code    int
	}{
		{name: "ipv4 loopback", remote: "127.0.0.1:5000", host: "127.0.0.1:1234", code: http.StatusOK},
		{name: "other ipv4 loopback", remote: "127.8.9.10:5000", host: "127.0.0.1:1234", code: http.StatusOK},
		{name: "ipv6 loopback", remote: "[::1]:5000", host: "[::1]:1234", code: http.StatusOK},
		{name: "localhost host", remote: "127.0.0.1:5000", host: "LocalHost:1234", code: http.StatusOK},
		{name: "host without port", remote: "127.0.0.1:5000", host: "localhost", code: http.StatusOK},
		{name: "fqdn localhost", remote: "127.0.0.1:5000", host: "localhost.:1234", code: http.StatusOK},
		{name: "mapped ipv4 loopback", remote: "[::ffff:127.0.0.1]:5000", host: "[::ffff:127.0.0.1]:1234", code: http.StatusOK},
		{name: "localhost lookalike", remote: "127.0.0.1:5000", host: "localhost.evil.example:1234", code: http.StatusForbidden},
		{name: "localhost suffix lookalike", remote: "127.0.0.1:5000", host: "evillocalhost:1234", code: http.StatusForbidden},
		{name: "lan client", remote: "192.168.1.5:5000", host: "192.168.1.2:1234", code: http.StatusForbidden},
		{name: "ipv6 client", remote: "[2001:db8::1]:5000", host: "[::1]:1234", code: http.StatusForbidden},
		{name: "mapped ipv4 client", remote: "[::ffff:10.0.0.1]:5000", host: "127.0.0.1:1234", code: http.StatusForbidden},
		{name: "no remote", remote: "", host: "127.0.0.1:1234", code: http.StatusForbidden},
		{name: "unparseable remote", remote: "@", host: "127.0.0.1:1234", code: http.StatusForbidden},
		{name: "rebound hostname", remote: "127.0.0.1:5000", host: "evil.example:1234", code: http.StatusForbidden},
		{name: "lan address as host", remote: "127.0.0.1:5000", host: "192.168.1.2:1234", code: http.StatusForbidden},
		{name: "address bar", remote: "127.0.0.1:5000", host: "127.0.0.1:1234",
			headers: map[string]string{"Sec-Fetch-Site": "none"}, code: http.StatusOK},
		{name: "same origin link", remote: "127.0.0.1:5000", host: "127.0.0.1:1234",
			headers: map[string]string{"Sec-Fetch-Site": "same-origin", "Origin": "http://127.0.0.1:1234"}, code: http.StatusOK},
		{name: "cross-site fetch", remote: "127.0.0.1:5000", host: "127.0.0.1:1234",
			headers: map[string]string{"Sec-Fetch-Site": "cross-site"}, code: http.StatusForbidden},
		{name: "same-site fetch", remote: "127.0.0.1:5000", host: "127.0.0.1:1234",
			headers: map[string]string{"Sec-Fetch-Site": "same-site"}, code: http.StatusForbidden},
		{name: "foreign origin", remote: "127.0.0.1:5000", host: "127.0.0.1:1234",
			headers: map[string]string{"Origin": "http://evil.example"}, code: http.StatusForbidden},
		{name: "opaque origin", remote: "127.0.0.1:5000", host: "127.0.0.1:1234",
			headers: map[string]string{"Origin": "null"}, code: http.StatusForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/debug/pprof-set/mutex", nil)
			req.RemoteAddr = tc.remote
			req.Host = tc.host
			for k, v := range tc.headers {
				req.Header.Set(k, v)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			require.Equal(t, tc.code, rec.Code)
		})
	}
}

func TestLocalOnlyAllowRemote(t *testing.T) {
	t.Setenv(AllowRemoteEnv, "1")
	req := httptest.NewRequest(http.MethodGet, "/debug/pprof/", nil)
	req.RemoteAddr = "192.168.1.5:5000"
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	rec := httptest.NewRecorder()
	LocalOnly(ok).ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
}

func TestLocalOnlyUnixSocket(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "api.sock")
	lst, err := net.Listen("unix", sock)
	require.NoError(t, err)
	srv := &http.Server{Handler: LocalOnly(ok)}
	go func() { _ = srv.Serve(lst) }()
	t.Cleanup(func() { _ = srv.Close() })

	client := &http.Client{Transport: &http.Transport{
		Dial: func(_, _ string) (net.Conn, error) { return net.Dial("unix", sock) },
	}}
	resp, err := client.Get("http://unix/debug/pprof/")
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, http.StatusOK, resp.StatusCode)
}
