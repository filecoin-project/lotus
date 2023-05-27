package index

import (
	"context"
	"errors"
	"math/rand"
	"os"
	"testing"
	"time"

	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"

	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/types/mock"
)

func TestBasicMsgIndex(t *testing.T) {
	// the most basic of tests:
	// 1. Create an index with mock chain store
	// 2. Advance the chain for a few tipsets
	// 3. Verify that the index contains all messages with the correct tipset/epoch
	cs := newMockChainStore()
	cs.genesis()

	tmp := t.TempDir()
	t.Cleanup(func() { _ = os.RemoveAll(tmp) })

	msgIndex, err := NewMsgIndex(context.Background(), tmp, cs)
	require.NoError(t, err)

	defer msgIndex.Close() //nolint

	for i := 0; i < 10; i++ {
		t.Logf("advance to epoch %d", i+1)
		err := cs.advance()
		require.NoError(t, err)
	}

	waitForCoalescerAfterLastEvent()

	t.Log("verifying index")
	verifyIndex(t, cs, msgIndex)
}

func TestReorgMsgIndex(t *testing.T) {
	// slightly more nuanced test that includes reorgs
	// 1. Create an index with mock chain store
	// 2. Advance/Reorg the chain for a few tipsets
	// 3. Verify that the index contains all messages with the correct tipset/epoch
	cs := newMockChainStore()
	cs.genesis()

	tmp := t.TempDir()
	t.Cleanup(func() { _ = os.RemoveAll(tmp) })

	msgIndex, err := NewMsgIndex(context.Background(), tmp, cs)
	require.NoError(t, err)

	defer msgIndex.Close() //nolint

	for i := 0; i < 10; i++ {
		t.Logf("advance to epoch %d", i+1)
		err := cs.advance()
		require.NoError(t, err)
	}

	waitForCoalescerAfterLastEvent()

	// a simple reorg
	t.Log("doing reorg")
	reorgme := cs.curTs
	reorgmeParent, err := cs.GetTipSetFromKey(context.Background(), reorgme.Parents())
	require.NoError(t, err)
	cs.setHead(reorgmeParent)
	reorgmeChild := cs.makeBlk()
	err = cs.reorg([]*types.TipSet{reorgme}, []*types.TipSet{reorgmeChild})
	require.NoError(t, err)

	waitForCoalescerAfterLastEvent()

	t.Log("verifying index")
	verifyIndex(t, cs, msgIndex)

	t.Log("verifying that reorged messages are not present")
	verifyMissing(t, cs, msgIndex, reorgme)
}

func TestReconcileMsgIndex(t *testing.T) {
	// test that exercises the reconciliation code paths
	// 1. Create  and populate a basic msgindex, similar to TestBasicMsgIndex.
	// 2. Close  it
	// 3. Reorg the mock chain store
	// 4. Reopen the index to trigger reconciliation
	// 5. Enxure that only the stable messages remain.
	cs := newMockChainStore()
	cs.genesis()

	tmp := t.TempDir()
	t.Cleanup(func() { _ = os.RemoveAll(tmp) })

	msgIndex, err := NewMsgIndex(context.Background(), tmp, cs)
	require.NoError(t, err)

	for i := 0; i < 10; i++ {
		t.Logf("advance to epoch %d", i+1)
		err := cs.advance()
		require.NoError(t, err)
	}

	waitForCoalescerAfterLastEvent()

	// Close it and reorg
	err = msgIndex.Close()
	require.NoError(t, err)
	cs.notify = nil

	// a simple reorg
	t.Log("doing reorg")
	reorgme := cs.curTs
	reorgmeParent, err := cs.GetTipSetFromKey(context.Background(), reorgme.Parents())
	require.NoError(t, err)
	cs.setHead(reorgmeParent)
	reorgmeChild := cs.makeBlk()
	err = cs.reorg([]*types.TipSet{reorgme}, []*types.TipSet{reorgmeChild})
	require.NoError(t, err)

	// reopen to reconcile
	msgIndex, err = NewMsgIndex(context.Background(), tmp, cs)
	require.NoError(t, err)

	defer msgIndex.Close() //nolint

	t.Log("verifying index")
	// need to step one up because the last tipset is not known by the index
	cs.setHead(reorgmeParent)
	verifyIndex(t, cs, msgIndex)

	t.Log("verifying that reorged and unknown messages are not present")
	verifyMissing(t, cs, msgIndex, reorgme, reorgmeChild)
}

func verifyIndex(t *testing.T, cs *mockChainStore, msgIndex MsgIndex) {
	for ts := cs.curTs; ts.Height() > 0; {
		t.Logf("verify at height %d", ts.Height())
		blks := ts.Blocks()
		if len(blks) == 0 {
			break
		}

		tsCid, err := ts.Key().Cid()
		require.NoError(t, err)

		msgs, err := cs.MessagesForTipset(context.Background(), ts)
		require.NoError(t, err)
		for _, m := range msgs {
			minfo, err := msgIndex.GetMsgInfo(context.Background(), m.Cid())
			require.NoError(t, err)
			require.Equal(t, tsCid, minfo.TipSet)
			require.Equal(t, ts.Height(), minfo.Epoch)
		}

		parents := ts.Parents()
		ts, err = cs.GetTipSetFromKey(context.Background(), parents)
		require.NoError(t, err)
	}
}

func verifyMissing(t *testing.T, cs *mockChainStore, msgIndex MsgIndex, missing ...*types.TipSet) {
	for _, ts := range missing {
		msgs, err := cs.MessagesForTipset(context.Background(), ts)
		require.NoError(t, err)
		for _, m := range msgs {
			_, err := msgIndex.GetMsgInfo(context.Background(), m.Cid())
			require.Equal(t, ErrNotFound, err)
		}
	}
}

type mockChainStore struct {
	notify store.ReorgNotifee

	curTs   *types.TipSet
	tipsets map[types.TipSetKey]*types.TipSet
	msgs    map[types.TipSetKey][]types.ChainMsg

	nonce uint64
}

var _ ChainStore = (*mockChainStore)(nil)

var systemAddr address.Address
var rng *rand.Rand

func init() {
	systemAddr, _ = address.NewIDAddress(0)
	rng = rand.New(rand.NewSource(314159))

	// adjust those to make tests snappy
	CoalesceMinDelay = 100 * time.Millisecond
	CoalesceMaxDelay = time.Second
	CoalesceMergeInterval = 100 * time.Millisecond
}

func newMockChainStore() *mockChainStore {
	return &mockChainStore{
		tipsets: make(map[types.TipSetKey]*types.TipSet),
		msgs:    make(map[types.TipSetKey][]types.ChainMsg),
	}
}

func (cs *mockChainStore) genesis() {
	genBlock := mock.MkBlock(nil, 0, 0)
	genTs := mock.TipSet(genBlock)
	cs.msgs[genTs.Key()] = nil
	cs.setHead(genTs)
}

func (cs *mockChainStore) setHead(ts *types.TipSet) {
	cs.curTs = ts
	cs.tipsets[ts.Key()] = ts
}

func (cs *mockChainStore) advance() error {
	ts := cs.makeBlk()
	return cs.reorg(nil, []*types.TipSet{ts})
}

func (cs *mockChainStore) reorg(rev, app []*types.TipSet) error {
	for _, ts := range rev {
		parents := ts.Parents()
		cs.curTs = cs.tipsets[parents]
	}

	for _, ts := range app {
		cs.tipsets[ts.Key()] = ts
		cs.curTs = ts
	}

	if cs.notify != nil {
		return cs.notify(rev, app)
	}

	return nil
}

func (cs *mockChainStore) makeBlk() *types.TipSet {
	height := cs.curTs.Height() + 1

	blk := mock.MkBlock(cs.curTs, uint64(height), uint64(height))
	blk.Messages = cs.makeGarbageCid()

	ts := mock.TipSet(blk)
	msg1 := cs.makeMsg()
	msg2 := cs.makeMsg()
	cs.msgs[ts.Key()] = []types.ChainMsg{msg1, msg2}

	return ts
}

func (cs *mockChainStore) makeMsg() *types.Message {
	nonce := cs.nonce
	cs.nonce++
	return &types.Message{To: systemAddr, From: systemAddr, Nonce: nonce}
}

func (cs *mockChainStore) makeGarbageCid() cid.Cid {
	garbage := blocks.NewBlock([]byte{byte(rng.Intn(256)), byte(rng.Intn(256)), byte(rng.Intn(256))})
	return garbage.Cid()
}

func (cs *mockChainStore) SubscribeHeadChanges(f store.ReorgNotifee) {
	cs.notify = f
}

func (cs *mockChainStore) MessagesForTipset(ctx context.Context, ts *types.TipSet) ([]types.ChainMsg, error) {
	msgs, ok := cs.msgs[ts.Key()]
	if !ok {
		return nil, errors.New("unknown tipset")
	}

	return msgs, nil
}

func (cs *mockChainStore) GetHeaviestTipSet() *types.TipSet {
	return cs.curTs
}

func (cs *mockChainStore) GetTipSetFromKey(ctx context.Context, tsk types.TipSetKey) (*types.TipSet, error) {
	ts, ok := cs.tipsets[tsk]
	if !ok {
		return nil, errors.New("unknown tipset")
	}
	return ts, nil
}

func waitForCoalescerAfterLastEvent() {
	// It can take up to CoalesceMinDelay for the coalescer timer to fire after the last event.
	// When the timer fires, it can wait up to CoalesceMinDelay again for more events.
	// Therefore the total wait is 2 * CoalesceMinDelay.
	// Then we wait another second for the listener (the index) to actually process events.
	time.Sleep(2*CoalesceMinDelay + time.Second)
}
