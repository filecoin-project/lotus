package kit2

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/filecoin-project/lotus/api/client"
	"github.com/filecoin-project/lotus/node"
	"github.com/multiformats/go-multiaddr"
	manet "github.com/multiformats/go-multiaddr/net"
	"github.com/stretchr/testify/require"
)

func CreateRPCServer(t *testing.T, handler http.Handler) (*httptest.Server, multiaddr.Multiaddr) {
	testServ := httptest.NewServer(handler)
	t.Cleanup(testServ.Close)
	t.Cleanup(testServ.CloseClientConnections)

	addr := testServ.Listener.Addr()
	maddr, err := manet.FromNetAddr(addr)
	require.NoError(t, err)
	return testServ, maddr
}

func fullRpc(t *testing.T, f *TestFullNode) *TestFullNode {
	handler, err := node.FullNodeHandler(f.FullNode, false)
	require.NoError(t, err)

	srv, maddr := CreateRPCServer(t, handler)

	cl, stop, err := client.NewFullNodeRPCV1(context.Background(), "ws://"+srv.Listener.Addr().String()+"/rpc/v1", nil)
	require.NoError(t, err)
	t.Cleanup(stop)
	f.ListenAddr, f.FullNode = maddr, cl

	return f
}

func minerRpc(t *testing.T, m *TestMiner) *TestMiner {
	handler, err := node.MinerHandler(m.StorageMiner, false)
	require.NoError(t, err)

	srv, maddr := CreateRPCServer(t, handler)

	cl, stop, err := client.NewStorageMinerRPCV0(context.Background(), "ws://"+srv.Listener.Addr().String()+"/rpc/v0", nil)
	require.NoError(t, err)
	t.Cleanup(stop)

	m.ListenAddr, m.StorageMiner = maddr, cl
	return m
}
