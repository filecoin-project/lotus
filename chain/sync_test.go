package chain_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	logging "github.com/ipfs/go-log"
	mocknet "github.com/libp2p/go-libp2p/p2p/net/mock"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-lotus/api"
	"github.com/filecoin-project/go-lotus/chain/gen"
	"github.com/filecoin-project/go-lotus/node"
	"github.com/filecoin-project/go-lotus/node/modules"
	"github.com/filecoin-project/go-lotus/node/repo"
)

const source = 0

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

func (tu *syncTestUtil) addSourceNode(gen int) {
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
	tu.nds = append(tu.nds, out) // always at 0
}

func (tu *syncTestUtil) addClientNode() int {
	if tu.genesis == nil {
		tu.t.Fatal("source doesn't exists")
	}

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

func (tu *syncTestUtil) connect(from, to int) {
	toPI, err := tu.nds[to].NetAddrsListen(tu.ctx)
	require.NoError(tu.t, err)

	err = tu.nds[from].NetConnect(tu.ctx, toPI)
	require.NoError(tu.t, err)
}

func (tu *syncTestUtil) checkHeight(name string, n int, h int) {
	b, err := tu.nds[n].ChainHead(tu.ctx)
	require.NoError(tu.t, err)

	require.Equal(tu.t, uint64(h), b.Height())
	fmt.Printf("%s H: %d\n", name, b.Height())
}

func (tu *syncTestUtil) compareSourceState(with int) {
	sourceAccounts, err := tu.nds[source].WalletList(tu.ctx)
	require.NoError(tu.t, err)

	for _, addr := range sourceAccounts {
		sourceBalance, err := tu.nds[source].WalletBalance(tu.ctx, addr)
		require.NoError(tu.t, err)

		actBalance, err := tu.nds[with].WalletBalance(tu.ctx, addr)
		require.NoError(tu.t, err)

		require.Equal(tu.t, sourceBalance, actBalance)
	}
}

func TestSyncSimple(t *testing.T) {
	logging.SetLogLevel("*", "INFO")

	H := 20
	ctx := context.Background()
	tu := &syncTestUtil{
		t:   t,
		ctx: ctx,
		mn:  mocknet.New(ctx),
	}

	tu.addSourceNode(H)
	tu.checkHeight("source", source, H)

	// separate logs
	fmt.Println("\x1b[31m///////////////////////////////////////////////////\x1b[39b")

	client := tu.addClientNode()
	tu.checkHeight("client", client, 0)

	require.NoError(t, tu.mn.LinkAll())
	tu.connect(1, 0)
	time.Sleep(time.Second * 1)

	tu.checkHeight("client", client, H)

	tu.compareSourceState(client)
}
