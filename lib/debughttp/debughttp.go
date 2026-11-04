// Package debughttp guards the process debug endpoints served alongside the
// Lotus APIs: net/http/pprof, expvar and x/net/trace on http.DefaultServeMux,
// and the pprof-set handlers. None of these take a token, so by default they
// only answer local clients that are not acting for another web page.
package debughttp

import (
	"net"
	"net/http"
	_ "net/http/pprof"
	"net/netip"
	"net/url"
	"os"
	"strings"
)

// AllowRemoteEnv, set to "1", serves the debug endpoints to every client.
const AllowRemoteEnv = "LOTUS_DEBUG_ALLOW_REMOTE"

// Handler serves http.DefaultServeMux, where the pprof, expvar and x/net/trace
// handlers register themselves, to local clients only.
func Handler() http.Handler {
	return LocalOnly(http.DefaultServeMux)
}

// LocalOnly serves next to clients connecting over a unix socket, or over
// loopback with a loopback Host header and no sign of a cross-origin browser
// request. The Host check defeats DNS rebinding, which otherwise lets a web
// page read these endpoints through a loopback connection. When
// AllowRemoteEnv is "1" every request is served.
//
// Behind a reverse proxy on the same host every request arrives over
// loopback, so such a proxy must not forward the debug paths.
func LocalOnly(next http.Handler) http.Handler {
	if os.Getenv(AllowRemoteEnv) == "1" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isLocal(r) {
			http.Error(w, "debug endpoints are only served to local clients; set "+AllowRemoteEnv+"=1 to allow all clients", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func isLocal(r *http.Request) bool {
	if _, ok := r.Context().Value(http.LocalAddrContextKey).(*net.UnixAddr); ok {
		return true
	}
	remote, err := netip.ParseAddrPort(r.RemoteAddr)
	if err != nil || !remote.Addr().Unmap().IsLoopback() {
		return false
	}
	return isLoopbackHost(hostname(r.Host)) && !isCrossOrigin(r)
}

// isLoopbackHost accepts loopback IP literals and "localhost". It cannot
// resolve names instead: DNS rebinding works by resolving a foreign name to a
// loopback address.
func isLoopbackHost(host string) bool {
	if addr, err := netip.ParseAddr(host); err == nil {
		return addr.Unmap().IsLoopback()
	}
	return strings.EqualFold(strings.TrimSuffix(host, "."), "localhost")
}

// hostname strips the port, and the brackets of an IPv6 literal, from a Host
// header value.
func hostname(hostport string) string {
	if host, _, err := net.SplitHostPort(hostport); err == nil {
		return host
	}
	return strings.TrimSuffix(strings.TrimPrefix(hostport, "["), "]")
}

// isCrossOrigin reports a browser request made on behalf of another origin.
// Sec-Fetch-Site is sent by all current browsers, Origin covers older ones;
// curl, go tool pprof and other non-browser clients send neither.
func isCrossOrigin(r *http.Request) bool {
	switch r.Header.Get("Sec-Fetch-Site") {
	case "", "same-origin", "none":
	default:
		return true
	}
	origin := r.Header.Get("Origin")
	if origin == "" {
		return false
	}
	u, err := url.Parse(origin)
	return err != nil || u.Host != r.Host
}
