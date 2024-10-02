package index

import (
	"context"
	"errors"
	pseudo "math/rand"
	"sync"
	"testing"
	"time"

	"github.com/ipfs/go-cid"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/chain/actors/adt"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/types/ethtypes"
)

func TestGetCidFromHash(t *testing.T) {
	rng := pseudo.New(pseudo.NewSource(time.Now().UnixNano()))
	ctx := context.Background()

	s, _, _ := setupWithHeadIndexed(t, 10, rng)

	ethTxHash := ethtypes.EthHash([32]byte{1})
	msgCid := randomCid(t, rng)

	// read from empty db -> ErrNotFound
	c, err := s.GetCidFromHash(ctx, ethTxHash)
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrNotFound))
	require.EqualValues(t, cid.Undef, c)

	// insert and read
	insertEthTxHash(t, s, ethTxHash, msgCid)
	c, err = s.GetCidFromHash(ctx, ethTxHash)
	require.NoError(t, err)
	require.EqualValues(t, msgCid, c)

	// look up some other hash -> fails
	c, err = s.GetCidFromHash(ctx, ethtypes.EthHash([32]byte{2}))
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrNotFound))
	require.EqualValues(t, cid.Undef, c)
}

func TestGetMsgInfo(t *testing.T) {
	ctx := context.Background()
	rng := pseudo.New(pseudo.NewSource(time.Now().UnixNano()))
	s, _, _ := setupWithHeadIndexed(t, 10, rng)

	msgCid := randomCid(t, rng)

	// read from empty db -> ErrNotFound
	mi, err := s.GetMsgInfo(ctx, msgCid)
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrNotFound))
	require.Nil(t, mi)

	msgCidBytes := msgCid.Bytes()
	tsKeyCid := randomCid(t, rng)
	// insert and read
	insertTipsetMessage(t, s, tipsetMessage{
		tipsetKeyCid: tsKeyCid.Bytes(),
		height:       uint64(1),
		reverted:     false,
		messageCid:   msgCidBytes,
		messageIndex: 1,
	})
	mi, err = s.GetMsgInfo(ctx, msgCid)
	require.NoError(t, err)
	require.Equal(t, msgCid, mi.Message)
	require.Equal(t, tsKeyCid, mi.TipSet)
	require.Equal(t, abi.ChainEpoch(1), mi.Epoch)
}

type dummyChainStore struct {
	mu sync.RWMutex

	heightToTipSet    map[abi.ChainEpoch]*types.TipSet
	messagesForTipset map[*types.TipSet][]types.ChainMsg
	keyToTipSet       map[types.TipSetKey]*types.TipSet

	heaviestTipSet   *types.TipSet
	tipSetByCid      func(ctx context.Context, tsKeyCid cid.Cid) (*types.TipSet, error)
	messagesForBlock func(ctx context.Context, b *types.BlockHeader) ([]*types.Message, []*types.SignedMessage, error)
	actorStore       func(ctx context.Context) adt.Store
}

func newDummyChainStore() *dummyChainStore {
	return &dummyChainStore{
		heightToTipSet:    make(map[abi.ChainEpoch]*types.TipSet),
		messagesForTipset: make(map[*types.TipSet][]types.ChainMsg),
		keyToTipSet:       make(map[types.TipSetKey]*types.TipSet),
	}
}

func (d *dummyChainStore) MessagesForTipset(ctx context.Context, ts *types.TipSet) ([]types.ChainMsg, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	msgs, ok := d.messagesForTipset[ts]
	if !ok {
		return nil, nil
	}
	return msgs, nil
}

func (d *dummyChainStore) GetHeaviestTipSet() *types.TipSet {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.heaviestTipSet
}

func (d *dummyChainStore) GetTipSetByCid(ctx context.Context, tsKeyCid cid.Cid) (*types.TipSet, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.tipSetByCid != nil {
		return d.tipSetByCid(ctx, tsKeyCid)
	}
	return nil, nil
}

func (d *dummyChainStore) GetTipSetFromKey(ctx context.Context, tsk types.TipSetKey) (*types.TipSet, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	return d.keyToTipSet[tsk], nil
}

func (d *dummyChainStore) MessagesForBlock(ctx context.Context, b *types.BlockHeader) ([]*types.Message, []*types.SignedMessage, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.messagesForBlock != nil {
		return d.messagesForBlock(ctx, b)
	}
	return nil, nil, nil
}

func (d *dummyChainStore) ActorStore(ctx context.Context) adt.Store {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.actorStore != nil {
		return d.actorStore(ctx)
	}
	return nil
}

func (d *dummyChainStore) GetTipsetByHeight(ctx context.Context, h abi.ChainEpoch, _ *types.TipSet, prev bool) (*types.TipSet, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	ts, ok := d.heightToTipSet[h]
	if !ok {
		return nil, errors.New("tipset not found")
	}
	return ts, nil
}

func (d *dummyChainStore) IsStoringEvents() bool {
	return true
}

// Setter methods to configure the mock

func (d *dummyChainStore) SetMessagesForTipset(ts *types.TipSet, msgs []types.ChainMsg) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.messagesForTipset[ts] = msgs
}

func (d *dummyChainStore) SetHeaviestTipSet(ts *types.TipSet) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.heaviestTipSet = ts
}

func (d *dummyChainStore) SetTipSetByCid(f func(ctx context.Context, tsKeyCid cid.Cid) (*types.TipSet, error)) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.tipSetByCid = f
}

func (d *dummyChainStore) SetMessagesForBlock(f func(ctx context.Context, b *types.BlockHeader) ([]*types.Message, []*types.SignedMessage, error)) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.messagesForBlock = f
}

func (d *dummyChainStore) SetActorStore(f func(ctx context.Context) adt.Store) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.actorStore = f
}

func (d *dummyChainStore) SetTipsetByHeightAndKey(h abi.ChainEpoch, tsk types.TipSetKey, ts *types.TipSet) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.heightToTipSet[h] = ts
	d.keyToTipSet[tsk] = ts
}
