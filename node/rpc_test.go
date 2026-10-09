package node

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-jsonrpc"
)

// freeAddr picks a free port and releases it; ServeRPC listens itself.
func freeAddr(t *testing.T) (multiaddr.Multiaddr, string) {
	t.Helper()
	probe, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := probe.Addr().(*net.TCPAddr).Port
	require.NoError(t, probe.Close())
	maddr, err := multiaddr.NewMultiaddr(fmt.Sprintf("/ip4/127.0.0.1/tcp/%d", port))
	require.NoError(t, err)
	return maddr, fmt.Sprintf("127.0.0.1:%d", port)
}

// Timeouts must reach the serving server: a body that never arrives fails the read.
func TestServeRPCTimeouts(t *testing.T) {
	maddr, hostport := freeAddr(t)

	readErr := make(chan error, 1)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := io.ReadAll(r.Body)
		readErr <- err
	})
	stop, err := ServeRPC(handler, "test", maddr, WithTimeouts(Timeouts{Read: 100 * time.Millisecond}))
	require.NoError(t, err)
	defer func() { _ = stop(t.Context()) }()

	conn, err := net.Dial("tcp", hostport)
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()

	// Announce a body, then never send it.
	_, err = fmt.Fprint(conn, "POST / HTTP/1.1\r\nHost: localhost\r\nContent-Length: 100\r\n\r\n")
	require.NoError(t, err)

	select {
	case err := <-readErr:
		require.ErrorIs(t, err, os.ErrDeadlineExceeded)
	case <-time.After(10 * time.Second):
		t.Fatal("server kept reading the request body past the read timeout")
	}
}

// The read timeout bounds reading the request, not how long a handler may take.
func TestServeRPCSlowHandler(t *testing.T) {
	maddr, hostport := freeAddr(t)

	const readTimeout = 300 * time.Millisecond
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(3 * readTimeout)
		_, _ = w.Write([]byte("slow but fine"))
	})
	stop, err := ServeRPC(handler, "test", maddr, WithTimeouts(Timeouts{
		ReadHeader: readTimeout,
		Read:       readTimeout,
		Idle:       readTimeout,
	}))
	require.NoError(t, err)
	defer func() { _ = stop(t.Context()) }()

	resp, err := http.Post("http://"+hostport, "application/json", strings.NewReader("{}"))
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, "slow but fine", string(body))
}

// The request cap bounds the memory one request can make the server hold.
func TestServeRPCMaxRequestSize(t *testing.T) {
	maddr, hostport := freeAddr(t)

	const cap = 1 << 20
	rpcServer := jsonrpc.NewServer(jsonrpc.WithMaxRequestSize(cap))
	rpcServer.Register("Filecoin", &echoAPI{})
	stop, err := ServeRPC(rpcServer, "test", maddr)
	require.NoError(t, err)
	defer func() { _ = stop(t.Context()) }()

	post := func(payloadSize int) string {
		req := fmt.Sprintf(`{"jsonrpc":"2.0","method":"Filecoin.Echo","params":["%s"],"id":1}`,
			strings.Repeat("x", payloadSize))
		resp, err := http.Post("http://"+hostport, "application/json", strings.NewReader(req))
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		return string(body)
	}

	require.Contains(t, post(cap/2), `"result"`)
	require.Contains(t, post(cap*2), "request bigger than maximum")
}

type echoAPI struct{}

func (*echoAPI) Echo(s string) (int, error) { return len(s), nil }

// Callers that pass no options, the daemon and miner, still serve a slow body.
func TestServeRPCNoOptions(t *testing.T) {
	maddr, hostport := freeAddr(t)

	done := make(chan string, 1)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		done <- string(body)
	})
	stop, err := ServeRPC(handler, "test", maddr)
	require.NoError(t, err)
	defer func() { _ = stop(t.Context()) }()

	conn, err := net.Dial("tcp", hostport)
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()

	_, err = fmt.Fprint(conn, "POST / HTTP/1.1\r\nHost: localhost\r\nContent-Length: 4\r\n\r\n")
	require.NoError(t, err)
	for _, b := range []string{"s", "l", "o", "w"} {
		time.Sleep(300 * time.Millisecond)
		_, err = fmt.Fprint(conn, b)
		require.NoError(t, err)
	}

	select {
	case body := <-done:
		require.Equal(t, "slow", body)
	case <-time.After(10 * time.Second):
		t.Fatal("a trickled request body was cut off with no options set")
	}
}
