package node

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/require"
)

// TestServeRPCTimeouts checks that the http.Server mutators handed to ServeRPC
// reach the server that is actually serving: with a short ReadTimeout, a client
// that announces a body and then never sends it fails the server-side read
// instead of hanging on to the connection.
func TestServeRPCTimeouts(t *testing.T) {
	// Grab a free port and hand it back; ServeRPC does its own listening.
	probe, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := probe.Addr().(*net.TCPAddr).Port
	require.NoError(t, probe.Close())

	maddr, err := multiaddr.NewMultiaddr(fmt.Sprintf("/ip4/127.0.0.1/tcp/%d", port))
	require.NoError(t, err)

	readErr := make(chan error, 1)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := io.ReadAll(r.Body)
		readErr <- err
	})
	stop, err := ServeRPC(handler, "test", maddr, func(srv *http.Server) {
		srv.ReadTimeout = 100 * time.Millisecond
	})
	require.NoError(t, err)
	defer func() { _ = stop(t.Context()) }()

	conn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()

	// Announce a body, then never send it.
	_, err = fmt.Fprint(conn, "POST / HTTP/1.1\r\nHost: localhost\r\nContent-Length: 100\r\n\r\n")
	require.NoError(t, err)

	select {
	case err := <-readErr:
		require.ErrorIs(t, err, os.ErrDeadlineExceeded)
	case <-time.After(10 * time.Second):
		t.Fatal("server kept reading the request body past ReadTimeout")
	}
}
