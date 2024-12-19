package sub

import (
	"context"
	"testing"

	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/go-address"

	"github.com/filecoin-project/lotus/chain/types"
)

type getter struct {
	msgs []*types.Message
}

func (g *getter) GetBlock(ctx context.Context, c cid.Cid) (blocks.Block, error) { panic("NYI") }

func (g *getter) GetBlocks(ctx context.Context, ks []cid.Cid) <-chan blocks.Block {
	ch := make(chan blocks.Block, len(g.msgs))
	for _, m := range g.msgs {
		by, err := m.Serialize()
		if err != nil {
			panic(err)
		}
		b, err := blocks.NewBlockWithCid(by, m.Cid())
		if err != nil {
			panic(err)
		}
		ch <- b
	}
	close(ch)
	return ch
}

func TestFetchCidsWithDedup(t *testing.T) {
	msgs := []*types.Message{}
	for i := 0; i < 10; i++ {
		msgs = append(msgs, &types.Message{
			From: address.TestAddress,
			To:   address.TestAddress,

			Nonce: uint64(i),
		})
	}
	cids := []cid.Cid{}
	for _, m := range msgs {
		cids = append(cids, m.Cid())
	}
	g := &getter{msgs}

	// the cids have a duplicate
	res, err := FetchMessagesByCids(context.TODO(), g, append(cids, cids[0]))

	t.Logf("err: %+v", err)
	t.Logf("res: %+v", res)
	if err == nil {
		t.Errorf("there should be an error")
	}
	if err == nil && (res[0] == nil || res[len(res)-1] == nil) {
		t.Fatalf("there is a nil message: first %p, last %p", res[0], res[len(res)-1])
	}
}
