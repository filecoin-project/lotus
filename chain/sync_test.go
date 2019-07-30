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

func TestSyncSimple(t *testing.T) {
	ctx := context.Background()

	var source api.FullNode
	var client api.FullNode

	mn := mocknet.New(ctx)

	sourceRepo, genesis := repoWithChain(t, 20)

	err := node.New(ctx,
		node.FullAPI(&source),
		node.Online(),
		node.Repo(sourceRepo),
		node.MockHost(mn),

		node.Override(new(modules.Genesis), modules.LoadGenesis(genesis)),
	)
	require.NoError(t, err)

	b, err := source.ChainHead(ctx)
	require.NoError(t, err)

	require.Equal(t, uint64(20), b.Height())
	fmt.Printf("source H: %d\n", b.Height())

	err = node.New(ctx,
		node.FullAPI(&client),
		node.Online(),
		node.Repo(repo.NewMemory(nil)),
		node.MockHost(mn),

		node.Override(new(modules.Genesis), modules.LoadGenesis(genesis)),
	)
	require.NoError(t, err)

	require.NoError(t, mn.LinkAll())

	cb, err := client.ChainHead(ctx)
	require.NoError(t, err)
	require.Equal(t, uint64(0), cb.Height())
	fmt.Printf("client H: %d\n", cb.Height())

	sourcePI, err := source.NetAddrsListen(ctx)
	require.NoError(t, err)

	err = client.NetConnect(ctx, sourcePI)
	require.NoError(t, err)

	time.Sleep(time.Second)

	cb, err = client.ChainHead(ctx)
	require.NoError(t, err)
	require.Equal(t, uint64(20), cb.Height())
	fmt.Printf("client H: %d\n", cb.Height())
}
