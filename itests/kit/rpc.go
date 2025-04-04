package kit

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/multiformats/go-multiaddr"
	manet "github.com/multiformats/go-multiaddr/net"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-jsonrpc"

	"github.com/filecoin-project/lotus/api/client"
	"github.com/filecoin-project/lotus/cmd/lotus-worker/sealworker"
	"github.com/filecoin-project/lotus/node"
)

type Closer func()

func CreateRPCServer(t *testing.T, handler http.Handler, listener net.Listener) (*httptest.Server, multiaddr.Multiaddr, Closer) {
	testServ := &httptest.Server{
		Listener: listener,
		Config: &http.Server{
			Handler:           handler,
			ReadHeaderTimeout: 30 * time.Second,
		},
	}
	testServ.Start()

	addr := testServ.Listener.Addr()
	maddr, err := manet.FromNetAddr(addr)
	require.NoError(t, err)
	closer := func() {
		testServ.CloseClientConnections()
		testServ.Close()
	}

	return testServ, maddr, closer
}

func fullRpc(t *testing.T, f *TestFullNode) (*TestFullNode, Closer) {
	handler, err := node.FullNodeHandler(f.FullNode, f.V2, false)
	require.NoError(t, err)

	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	srv, maddr, rpcCloser := CreateRPCServer(t, handler, l)
	fmt.Printf("FULLNODE RPC ENV FOR CLI DEBUGGING `export FULLNODE_API_INFO=%s`\n", "ws://"+srv.Listener.Addr().String())
	sendItestdNotif("FULLNODE_API_INFO", t.Name(), "ws://"+srv.Listener.Addr().String())

	rpcOpts := []jsonrpc.Option{
		jsonrpc.WithClientHandler("Filecoin", f.EthSubRouter),
		jsonrpc.WithClientHandlerAlias("eth_subscription", "Filecoin.EthSubscription"),
	}

	cl, stop, err := client.NewFullNodeRPCV1(context.Background(), "ws://"+srv.Listener.Addr().String()+"/rpc/v1", nil, rpcOpts...)
	require.NoError(t, err)
	f.ListenAddr, f.ListenURL, f.FullNode = maddr, srv.URL, cl

	return f, func() { stop(); rpcCloser() }
}

func minerRpc(t *testing.T, m *TestMiner) *TestMiner {
	handler, err := node.MinerHandler(m.StorageMiner, false)
	require.NoError(t, err)

	srv, maddr, _ := CreateRPCServer(t, handler, m.RemoteListener)

	fmt.Printf("creating RPC server for %s at %s\n", m.ActorAddr, srv.Listener.Addr().String())
	fmt.Printf("SP RPC ENV FOR CLI DEBUGGING `export MINER_API_INFO=%s`\n", "ws://"+srv.Listener.Addr().String())
	sendItestdNotif("MINER_API_INFO", t.Name(), "ws://"+srv.Listener.Addr().String())

	url := "ws://" + srv.Listener.Addr().String() + "/rpc/v0"
	cl, stop, err := client.NewStorageMinerRPCV0(context.Background(), url, nil)
	require.NoError(t, err)
	t.Cleanup(stop)

	m.ListenAddr, m.StorageMiner = maddr, cl
	return m
}

func workerRpc(t *testing.T, m *TestWorker) *TestWorker {
	handler := sealworker.WorkerHandler(m.MinerNode.AuthVerify, m.FetchHandler, m.Worker, false)

	srv, maddr, _ := CreateRPCServer(t, handler, m.RemoteListener)

	fmt.Println("creating RPC server for a worker at: ", srv.Listener.Addr().String())
	url := "ws://" + srv.Listener.Addr().String() + "/rpc/v0"
	cl, stop, err := client.NewWorkerRPCV0(context.Background(), url, nil)
	require.NoError(t, err)
	t.Cleanup(stop)

	m.Stop = func(ctx context.Context) error {
		srv.Close()
		srv.CloseClientConnections()
		return nil
	}

	m.ListenAddr, m.Worker = maddr, cl
	return m
}

func (full *TestFullNode) DoRawRPCRequest(t *testing.T, version int, body string) (int, []byte) {
	t.Helper()
	require.NotEmpty(t, full.ListenURL, "not listening for rpc, turn on with `kit.ThroughRPC()`")

	endpoint := fmt.Sprintf("%s/rpc/v%d", full.ListenURL, version)
	request, err := http.NewRequest("POST", endpoint, bytes.NewReader([]byte(body)))
	require.NoError(t, err)
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	require.NoError(t, err)
	defer func() { require.NoError(t, response.Body.Close()) }()
	respBody, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	return response.StatusCode, respBody
}
