package modules

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/ipfs/go-cid"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/crypto"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/index"
	"github.com/filecoin-project/lotus/chain/types"
)

// blockingIndexer holds every IndexSignedMessage call until release closes.
type blockingIndexer struct {
	index.Indexer
	release chan struct{}

	lk      sync.Mutex
	indexed []cid.Cid
}

func (b *blockingIndexer) IndexSignedMessage(ctx context.Context, msg *types.SignedMessage) error {
	select {
	case <-b.release:
	case <-ctx.Done():
		return ctx.Err()
	}
	b.lk.Lock()
	defer b.lk.Unlock()
	b.indexed = append(b.indexed, msg.Cid())
	return nil
}

func (b *blockingIndexer) indexedCids() []cid.Cid {
	b.lk.Lock()
	defer b.lk.Unlock()
	return append([]cid.Cid(nil), b.indexed...)
}

func TestWaitForMpoolUpdatesDrainsWhileIndexerBlocks(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	idx := &blockingIndexer{release: make(chan struct{})}
	ch := make(chan api.MpoolUpdate)
	done := make(chan struct{})
	go func() {
		WaitForMpoolUpdates(ctx, ch, idx)
		close(done)
	}()

	msg := func(nonce uint64, sigType crypto.SigType) *types.SignedMessage {
		return &types.SignedMessage{
			Message:   types.Message{To: address.TestAddress, From: address.TestAddress, Nonce: nonce},
			Signature: crypto.Signature{Type: sigType},
		}
	}
	send := func(u api.MpoolUpdate) {
		t.Helper()
		select {
		case ch <- u:
		case <-time.After(time.Second):
			t.Fatal("WaitForMpoolUpdates stopped taking updates")
		}
	}

	// Sent while the queue has room, so they would be indexed if not skipped.
	removed := msg(1<<32, crypto.SigTypeDelegated)
	secp := msg(1<<33, crypto.SigTypeSecp256k1)
	send(api.MpoolUpdate{Type: api.MpoolRemove, Message: removed})
	send(api.MpoolUpdate{Type: api.MpoolAdd, Message: secp})

	// With the indexer blocked, every update is still taken; adds beyond the
	// queue are dropped.
	for i := range mpoolIndexQueueSize + 10 {
		send(api.MpoolUpdate{Type: api.MpoolAdd, Message: msg(uint64(i), crypto.SigTypeDelegated)})
	}

	close(idx.release)
	require.Eventually(t, func() bool { return len(idx.indexedCids()) >= mpoolIndexQueueSize }, 5*time.Second, 10*time.Millisecond)
	time.Sleep(50 * time.Millisecond)
	indexed := idx.indexedCids()
	// The queue plus the one message the indexing goroutine holds.
	require.LessOrEqual(t, len(indexed), mpoolIndexQueueSize+1)
	require.NotContains(t, indexed, removed.Cid())
	require.NotContains(t, indexed, secp.Cid())

	close(ch)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("WaitForMpoolUpdates did not return after its update channel closed")
	}
}
