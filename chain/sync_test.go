package chain_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	mocknet "github.com/libp2p/go-libp2p/p2p/net/mock"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-lotus/api"
	"github.com/filecoin-project/go-lotus/chain/gen"
	"github.com/filecoin-project/go-lotus/node"
	"github.com/filecoin-project/go-lotus/node/modules"
	"github.com/filecoin-project/go-lotus/node/repo"
)

func repoWithChain(t *testing.T, h int) (repo.Repo, []byte) {
	g, err := gen.NewGenerator()
	if err != nil {
		t.Fatal(err)
	}

	for i := 0; i < h; i++ {
		b, err := g.NextBlock()
		require.NoError(t, err)
		require.Equal(t, uint64(i+1), b.Header.Height, "wrong height")
	}

	r, err := g.YieldRepo()
	require.NoError(t, err)

	genb, err := g.GenesisCar()
	require.NoError(t, err)

	return r, genb
}

type syncTestUtil struct {
	t *testing.T

	ctx context.Context
	mn  mocknet.Mocknet

	genesis []byte

	nds []api.FullNode
}

func (tu *syncTestUtil) addSourceNode(gen int) int {
	if tu.genesis != nil {
		tu.t.Fatal("source node already exists")
	}

	sourceRepo, genesis := repoWithChain(tu.t, gen)
	var out api.FullNode

	err := node.New(tu.ctx,
		node.FullAPI(&out),
		node.Online(),
		node.Repo(sourceRepo),
		node.MockHost(tu.mn),

		node.Override(new(modules.Genesis), modules.LoadGenesis(genesis)),
	)
	require.NoError(tu.t, err)

	tu.genesis = genesis
	tu.nds = append(tu.nds, out)
	return len(tu.nds) - 1
}

func (tu *syncTestUtil) addClientNode() int {
	var out api.FullNode

	err := node.New(tu.ctx,
		node.FullAPI(&out),
		node.Online(),
		node.Repo(repo.NewMemory(nil)),
		node.MockHost(tu.mn),

		node.Override(new(modules.Genesis), modules.LoadGenesis(tu.genesis)),
	)
	require.NoError(tu.t, err)

	tu.nds = append(tu.nds, out)
	return len(tu.nds) - 1
}

func TestSyncSimple(t *testing.T) {
	H := 3

	ctx := context.Background()

	mn := mocknet.New(ctx)

	tu := &syncTestUtil{
		t:   t,
		ctx: ctx,
		mn:  mn,
	}

	source := tu.addSourceNode(H)

	b, err := tu.nds[source].ChainHead(ctx)
	require.NoError(t, err)

	require.Equal(t, uint64(H), b.Height())
	fmt.Printf("source H: %d\n", b.Height())

	// separate logs
	fmt.Println("///////////////////////////////////////////////////")

	client := tu.addClientNode()

	require.NoError(t, mn.LinkAll())

	cb, err := tu.nds[client].ChainHead(ctx)
	require.NoError(t, err)
	require.Equal(t, uint64(0), cb.Height())
	fmt.Printf("client H: %d\n", cb.Height())

	sourcePI, err := tu.nds[source].NetAddrsListen(ctx)
	require.NoError(t, err)

	err = tu.nds[client].NetConnect(ctx, sourcePI)
	require.NoError(t, err)

	time.Sleep(time.Second * 1)

	cb, err = tu.nds[client].ChainHead(ctx)
	require.NoError(t, err)
	require.Equal(t, uint64(H), cb.Height())
	fmt.Printf("client H: %d\n", cb.Height())
}
