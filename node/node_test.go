package node_test

import (
	"bytes"
	"context"
	"net/http/httptest"
	"testing"

	mocknet "github.com/libp2p/go-libp2p/p2p/net/mock"

	"github.com/filecoin-project/go-lotus/node"
	"github.com/filecoin-project/go-lotus/node/modules"
	modtest "github.com/filecoin-project/go-lotus/node/modules/testing"
	"github.com/filecoin-project/go-lotus/node/repo"

	"github.com/filecoin-project/go-lotus/api"
	"github.com/filecoin-project/go-lotus/api/client"
	"github.com/filecoin-project/go-lotus/api/test"
	"github.com/filecoin-project/go-lotus/lib/jsonrpc"
)

func builder(t *testing.T, n int) []api.FullNode {
	ctx := context.Background()
	mn := mocknet.New(ctx)

	out := make([]api.FullNode, n)

	var genbuf bytes.Buffer

	for i := 0; i < n; i++ {
		var genesis node.Option
		if i == 0 {
			genesis = node.Override(new(modules.Genesis), modtest.MakeGenesisMem(&genbuf))
		} else {
			genesis = node.Override(new(modules.Genesis), modules.LoadGenesis(genbuf.Bytes()))
		}

		var err error
		err = node.New(ctx,
			node.FullAPI(&out[i]),
			node.Online(),
			node.Repo(repo.NewMemory(nil)),
			MockHost(mn),

			genesis,
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

func rpcBuilder(t *testing.T, n int) []api.FullNode {
	nodeApis := builder(t, n)
	out := make([]api.FullNode, n)

	for i, a := range nodeApis {
		rpcServer := jsonrpc.NewServer()
		rpcServer.Register("Filecoin", a)
		testServ := httptest.NewServer(rpcServer) //  todo: close

		var err error
		out[i], err = client.NewFullNodeRPC("ws://"+testServ.Listener.Addr().String(), nil)
		if err != nil {
			t.Fatal(err)
		}
	}
	return out
}

func TestAPIRPC(t *testing.T) {
	test.TestApis(t, rpcBuilder)
}
