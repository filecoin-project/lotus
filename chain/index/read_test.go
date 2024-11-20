package index

import (
	"context"
	"errors"
	pseudo "math/rand"
	"sync"
	"testing"
	"time"

	"github.com/ipfs/go-cid"
	mh "github.com/multiformats/go-multihash"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/crypto"

	"github.com/filecoin-project/lotus/chain/actors/adt"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/types/ethtypes"
)

func TestGetCidFromHash(t *testing.T) {
	seed := time.Now().UnixNano()
	t.Logf("seed: %d", seed)
	rng := pseudo.New(pseudo.NewSource(seed))
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
	seed := time.Now().UnixNano()
	t.Logf("seed: %d", seed)
	rng := pseudo.New(pseudo.NewSource(seed))
	s, _, _ := setupWithHeadIndexed(t, 10, rng)

	t.Run("message exists", func(t *testing.T) {
		msgCid := randomCid(t, rng)
		msgCidBytes := msgCid.Bytes()
		tsKeyCid := randomCid(t, rng)

		insertTipsetMessage(t, s, tipsetMessage{
			tipsetKeyCid: tsKeyCid.Bytes(),
			height:       uint64(1),
			reverted:     false,
			messageCid:   msgCidBytes,
			messageIndex: 1,
		})

		mi, err := s.GetMsgInfo(ctx, msgCid)
		require.NoError(t, err)
		require.Equal(t, msgCid, mi.Message)
		require.Equal(t, tsKeyCid, mi.TipSet)
		require.Equal(t, abi.ChainEpoch(1), mi.Epoch)
	})

	t.Run("message not found", func(t *testing.T) {
		nonExistentMsgCid := randomCid(t, rng)
		mi, err := s.GetMsgInfo(ctx, nonExistentMsgCid)
		require.Error(t, err)
		require.ErrorIs(t, err, ErrNotFound)
		require.Nil(t, mi)
	})
}

func setupWithHeadIndexed(t *testing.T, headHeight abi.ChainEpoch, rng *pseudo.Rand) (*SqliteIndexer, *types.TipSet, *dummyChainStore) {
	head := fakeTipSet(t, rng, headHeight, []cid.Cid{})
	d := newDummyChainStore()
	d.SetHeaviestTipSet(head)

	s, err := NewSqliteIndexer(":memory:", d, 0, false, 0)
	require.NoError(t, err)
	insertHead(t, s, head, headHeight)

	return s, head, d
}

func insertHead(t *testing.T, s *SqliteIndexer, head *types.TipSet, height abi.ChainEpoch) {
	headKeyBytes, err := toTipsetKeyCidBytes(head)
	require.NoError(t, err)

	insertTipsetMessage(t, s, tipsetMessage{
		tipsetKeyCid: headKeyBytes,
		height:       uint64(height),
		reverted:     false,
		messageCid:   nil,
		messageIndex: -1,
	})
}

func insertEthTxHash(t *testing.T, s *SqliteIndexer, ethTxHash ethtypes.EthHash, messageCid cid.Cid) {
	msgCidBytes := messageCid.Bytes()

	res, err := s.stmts.insertEthTxHashStmt.Exec(ethTxHash.String(), msgCidBytes)
	require.NoError(t, err)
	rowsAffected, err := res.RowsAffected()
	require.NoError(t, err)
	require.Equal(t, int64(1), rowsAffected)
}

type dummyChainStore struct {
	mu sync.RWMutex

	heightToTipSet    map[abi.ChainEpoch]*types.TipSet
	messagesForTipset map[*types.TipSet][]types.ChainMsg
	keyToTipSet       map[types.TipSetKey]*types.TipSet
	tipsetCidToTipset map[cid.Cid]*types.TipSet

	heaviestTipSet   *types.TipSet
	messagesForBlock func(ctx context.Context, b *types.BlockHeader) ([]*types.Message, []*types.SignedMessage, error)
	actorStore       func(ctx context.Context) adt.Store
}

func newDummyChainStore() *dummyChainStore {
	return &dummyChainStore{
		heightToTipSet:    make(map[abi.ChainEpoch]*types.TipSet),
		messagesForTipset: make(map[*types.TipSet][]types.ChainMsg),
		keyToTipSet:       make(map[types.TipSetKey]*types.TipSet),
		tipsetCidToTipset: make(map[cid.Cid]*types.TipSet),
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

func (d *dummyChainStore) GetTipSetByCid(_ context.Context, tsKeyCid cid.Cid) (*types.TipSet, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if _, ok := d.tipsetCidToTipset[tsKeyCid]; !ok {
		return nil, errors.New("not found")
	}
	return d.tipsetCidToTipset[tsKeyCid], nil
}

func (d *dummyChainStore) SetTipSetByCid(t *testing.T, ts *types.TipSet) {
	d.mu.Lock()
	defer d.mu.Unlock()

	tsKeyCid, err := ts.Key().Cid()
	require.NoError(t, err)
	d.tipsetCidToTipset[tsKeyCid] = ts
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

func (d *dummyChainStore) SetTipsetByHeightAndKey(h abi.ChainEpoch, tsk types.TipSetKey, ts *types.TipSet) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.heightToTipSet[h] = ts
	d.keyToTipSet[tsk] = ts
}

func randomIDAddr(tb testing.TB, rng *pseudo.Rand) address.Address {
	tb.Helper()
	addr, err := address.NewIDAddress(uint64(rng.Int63()))
	require.NoError(tb, err)
	return addr
}

func randomCid(tb testing.TB, rng *pseudo.Rand) cid.Cid {
	tb.Helper()
	cb := cid.V1Builder{Codec: cid.Raw, MhType: mh.IDENTITY}
	c, err := cb.Sum(randomBytes(10, rng))
	require.NoError(tb, err)
	return c
}

func randomBytes(n int, rng *pseudo.Rand) []byte {
	buf := make([]byte, n)
	rng.Read(buf)
	return buf
}

func fakeTipSet(tb testing.TB, rng *pseudo.Rand, h abi.ChainEpoch, parents []cid.Cid) *types.TipSet {
	tb.Helper()
	ts, err := types.NewTipSet([]*types.BlockHeader{
		{
			Height: h,
			Miner:  randomIDAddr(tb, rng),

			Parents: parents,

			Ticket: &types.Ticket{VRFProof: []byte{byte(h % 2)}},

			ParentStateRoot:       randomCid(tb, rng),
			Messages:              randomCid(tb, rng),
			ParentMessageReceipts: randomCid(tb, rng),

			BlockSig:     &crypto.Signature{Type: crypto.SigTypeBLS},
			BLSAggregate: &crypto.Signature{Type: crypto.SigTypeBLS},
		},
		{
			Height: h,
			Miner:  randomIDAddr(tb, rng),

			Parents: parents,

			Ticket: &types.Ticket{VRFProof: []byte{byte((h + 1) % 2)}},

			ParentStateRoot:       randomCid(tb, rng),
			Messages:              randomCid(tb, rng),
			ParentMessageReceipts: randomCid(tb, rng),

			BlockSig:     &crypto.Signature{Type: crypto.SigTypeBLS},
			BLSAggregate: &crypto.Signature{Type: crypto.SigTypeBLS},
		},
	})

	require.NoError(tb, err)

	return ts
}
