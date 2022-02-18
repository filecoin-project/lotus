package kit

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/filecoin-project/lotus/api/client"
	"github.com/filecoin-project/lotus/node"
	"github.com/multiformats/go-multiaddr"
	manet "github.com/multiformats/go-multiaddr/net"
	"github.com/stretchr/testify/require"
)

func CreateRPCServer(t *testing.T, handler http.Handler, listener net.Listener) (*httptest.Server, multiaddr.Multiaddr) {
	testServ := &httptest.Server{
		Listener: listener,
		Config:   &http.Server{Handler: handler},
	}
	testServ.Start()

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

	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	srv, maddr := CreateRPCServer(t, handler, l)
	fmt.Printf("FULLNODE RPC ENV FOR CLI DEBUGGING `export FULLNODE_API_INFO=%s`\n", "ws://"+srv.Listener.Addr().String())

	cl, stop, err := client.NewFullNodeRPCV1(context.Background(), "ws://"+srv.Listener.Addr().String()+"/rpc/v1", nil)
	require.NoError(t, err)
	t.Cleanup(stop)
	f.ListenAddr, f.FullNode = maddr, cl

	return f
}

func minerRpc(t *testing.T, m *TestMiner) *TestMiner {
	handler, err := node.MinerHandler(m.StorageMiner, false)
	require.NoError(t, err)

	srv, maddr := CreateRPCServer(t, handler, m.RemoteListener)

	fmt.Printf("creating RPC server for %s at %s\n", m.ActorAddr, srv.Listener.Addr().String())
	fmt.Printf("SP RPC ENV FOR CLI DEBUGGING `export MINER_API_INFO=%s`\n", "ws://"+srv.Listener.Addr().String())

	url := "ws://" + srv.Listener.Addr().String() + "/rpc/v0"
	cl, stop, err := client.NewStorageMinerRPCV0(context.Background(), url, nil)
	require.NoError(t, err)
	t.Cleanup(stop)

	m.ListenAddr, m.StorageMiner = maddr, cl
	return m
}
