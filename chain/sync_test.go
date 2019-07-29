package chain_test

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	mocknet "github.com/libp2p/go-libp2p/p2p/net/mock"

	"github.com/filecoin-project/go-lotus/api"
	"github.com/filecoin-project/go-lotus/chain/gen"
	"github.com/filecoin-project/go-lotus/node"
	"github.com/filecoin-project/go-lotus/node/modules"
	modtest "github.com/filecoin-project/go-lotus/node/modules/testing"
	"github.com/filecoin-project/go-lotus/node/repo"
)

func repoWithChain(t *testing.T, h int) repo.Repo {
	g, err := gen.NewGenerator()
	if err != nil {
		t.Fatal(err)
	}

	for i := 0; i < h; i++ {
		b, err := g.NextBlock()
		if err != nil {
			t.Fatalf("error at H:%d, %s", i, err)
		}
		if b.Header.Height != uint64(i+1) {
			t.Fatal("wrong height")
		}
	}

	r, err := g.YieldRepo()
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestSyncSimple(t *testing.T) {
	ctx := context.Background()

	var genbuf bytes.Buffer
	var source api.FullNode
	var client api.FullNode

	mn := mocknet.New(ctx)

	err := node.New(ctx,
		node.FullAPI(&source),
		node.Online(),
		node.Repo(repoWithChain(t, 20)),
		node.MockHost(mn),

		node.Override(new(modules.Genesis), modtest.MakeGenesisMem(&genbuf)),
	)
	if err != nil {
		t.Fatal(err)
	}

	b, err := source.ChainHead(ctx)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println(b.Height())

	err = node.New(ctx,
		node.FullAPI(&client),
		node.Online(),
		node.Repo(repo.NewMemory(nil)),
		node.MockHost(mn),

		node.Override(new(modules.Genesis), modules.LoadGenesis(genbuf.Bytes())),
	)
	if err != nil {
		t.Fatal(err)
	}

}
