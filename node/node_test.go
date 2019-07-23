package node_test

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/filecoin-project/go-lotus/node"

	"github.com/filecoin-project/go-lotus/api"
	"github.com/filecoin-project/go-lotus/api/client"
	"github.com/filecoin-project/go-lotus/api/test"
	"github.com/filecoin-project/go-lotus/lib/jsonrpc"

	"github.com/filecoin-project/go-lotus/node/repo"
	mocknet "github.com/libp2p/go-libp2p/p2p/net/mock"
)

func builder(t *testing.T, n int) []api.API {
	ctx := context.Background()
	mn := mocknet.New(ctx)

	out := make([]api.API, n)

	for i := 0; i < n; i++ {
		var err error
		err = node.New(ctx,
			node.FullAPI(&out[i]),
			node.Online(),
			node.Repo(repo.NewMemory(nil)),
			MockHost(mn),
		)
		if err != nil {
			t.Fatal(err)
		}
	}

	if err := mn.LinkAll(); err != nil {
		t.Fatal(err)
	}

	return out
}

func TestAPI(t *testing.T) {
	test.TestApis(t, builder)
}

var nextApi int

func rpcBuilder(t *testing.T, n int) []api.API {
	nodeApis := builder(t, n)
	out := make([]api.API, n)

	for i, a := range nodeApis {
		rpcServer := jsonrpc.NewServer()
		rpcServer.Register("Filecoin", a)
		testServ := httptest.NewServer(rpcServer) //  todo: close

		var err error
		out[i], err = client.NewRPC("ws://"+testServ.Listener.Addr().String(), nil)
		if err != nil {
			t.Fatal(err)
		}
	}
	return out
}

func TestAPIRPC(t *testing.T) {
	test.TestApis(t, rpcBuilder)
}
