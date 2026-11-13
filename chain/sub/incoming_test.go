package sub

import (
	"context"
	"testing"
	"time"

	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/peer"
	mocknet "github.com/libp2p/go-libp2p/p2p/net/mock"
	"github.com/stretchr/testify/require"

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

func TestHandleIncomingBlocksSurvivesWrongValidatorData(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mn := mocknet.New()
	defer mn.Close() //nolint:errcheck

	h, err := mn.GenPeer()
	require.NoError(t, err)

	ps, err := pubsub.NewGossipSub(ctx, h)
	require.NoError(t, err)

	const topicName = "/test/blocks"
	require.NoError(t, ps.RegisterTopicValidator(topicName, func(ctx context.Context, _ peer.ID, msg *pubsub.Message) pubsub.ValidationResult {
		msg.ValidatorData = "not a block"
		return pubsub.ValidationAccept
	}))

	topic, err := ps.Join(topicName)
	require.NoError(t, err)

	bsub, err := topic.Subscribe()
	require.NoError(t, err)
	observer, err := topic.Subscribe()
	require.NoError(t, err)

	done := make(chan struct{})
	go func() {
		defer close(done)
		HandleIncomingBlocks(ctx, bsub, nil, nil, nil)
	}()

	require.NoError(t, topic.Publish(ctx, []byte("bad block")))

	// messages are delivered to all local subscriptions together, so once the observer has it
	// the handler has been handed it too
	_, err = observer.Next(ctx)
	require.NoError(t, err)

	select {
	case <-done:
		t.Fatal("HandleIncomingBlocks exited after a message with unexpected ValidatorData")
	case <-time.After(time.Second):
	}

	cancel()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("HandleIncomingBlocks did not exit after context cancellation")
	}
}
