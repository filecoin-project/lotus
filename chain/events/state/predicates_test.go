package state

import (
	"context"
	"testing"

	"github.com/filecoin-project/specs-actors/actors/crypto"
	"github.com/ipfs/go-hamt-ipld"

	"github.com/filecoin-project/go-amt-ipld/v2"
	"github.com/filecoin-project/specs-actors/actors/builtin/market"
	ds "github.com/ipfs/go-datastore"
	ds_sync "github.com/ipfs/go-datastore/sync"
	bstore "github.com/ipfs/go-ipfs-blockstore"
	cbornode "github.com/ipfs/go-ipld-cbor"
	"golang.org/x/xerrors"

	"github.com/ipfs/go-cid"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/specs-actors/actors/abi"
)

var dummyCid cid.Cid

func init() {
	dummyCid, _ = cid.Parse("bafkqaaa")
}

type mockAPI struct {
	ts map[types.TipSetKey]*types.Actor
	bs bstore.Blockstore
}

func newMockAPI(bs bstore.Blockstore) *mockAPI {
	return &mockAPI{
		bs: bs,
		ts: make(map[types.TipSetKey]*types.Actor),
	}
}

func (m mockAPI) ChainHasObj(ctx context.Context, c cid.Cid) (bool, error) {
	return m.bs.Has(c)
}

func (m mockAPI) ChainReadObj(ctx context.Context, c cid.Cid) ([]byte, error) {
	blk, err := m.bs.Get(c)
	if err != nil {
		return nil, xerrors.Errorf("blockstore get: %w", err)
	}

	return blk.RawData(), nil
}

func (m mockAPI) StateGetActor(ctx context.Context, actor address.Address, tsk types.TipSetKey) (*types.Actor, error) {
	return m.ts[tsk], nil
}

func (m mockAPI) setActor(tsk types.TipSetKey, act *types.Actor) {
	m.ts[tsk] = act
}

func TestPredicates(t *testing.T) {
	ctx := context.Background()
	bs := bstore.NewBlockstore(ds_sync.MutexWrap(ds.NewMapDatastore()))
	store := cbornode.NewCborStore(bs)

	oldDeals := map[abi.DealID]*market.DealState{
		abi.DealID(1): {
			SectorStartEpoch: 1,
			LastUpdatedEpoch: 2,
			SlashEpoch:       0,
		},
		abi.DealID(2): {
			SectorStartEpoch: 4,
			LastUpdatedEpoch: 5,
			SlashEpoch:       0,
		},
	}
	oldStateC := createMarketState(ctx, t, store, oldDeals)

	newDeals := map[abi.DealID]*market.DealState{
		abi.DealID(1): {
			SectorStartEpoch: 1,
			LastUpdatedEpoch: 3,
			SlashEpoch:       0,
		},
		abi.DealID(2): {
			SectorStartEpoch: 4,
			LastUpdatedEpoch: 6,
			SlashEpoch:       6,
		},
	}
	newStateC := createMarketState(ctx, t, store, newDeals)

	miner, err := address.NewFromString("t00")
	require.NoError(t, err)
	oldState, err := mockTipset(miner, 1)
	require.NoError(t, err)
	newState, err := mockTipset(miner, 2)
	require.NoError(t, err)

	api := newMockAPI(bs)
	api.setActor(oldState.Key(), &types.Actor{Head: oldStateC})
	api.setActor(newState.Key(), &types.Actor{Head: newStateC})

	preds := NewStatePredicates(api)

	dealIds := []abi.DealID{abi.DealID(1), abi.DealID(2)}
	diffFn := preds.OnStorageMarketActorChanged(preds.OnDealStateChanged(preds.DealStateChangedForIDs(dealIds)))

	// Diff a state against itself: expect no change
	changed, _, err := diffFn(ctx, oldState, oldState)
	require.NoError(t, err)
	require.False(t, changed)

	// Diff old state against new state
	changed, val, err := diffFn(ctx, oldState, newState)
	require.NoError(t, err)
	require.True(t, changed)

	changedDeals, ok := val.(ChangedDeals)
	require.True(t, ok)
	require.Len(t, changedDeals, 2)
	require.Contains(t, changedDeals, abi.DealID(1))
	require.Contains(t, changedDeals, abi.DealID(2))
	deal1 := changedDeals[abi.DealID(1)]
	if deal1.From.LastUpdatedEpoch != 2 || deal1.To.LastUpdatedEpoch != 3 {
		t.Fatal("Unexpected change to LastUpdatedEpoch")
	}
	deal2 := changedDeals[abi.DealID(2)]
	if deal2.From.SlashEpoch != 0 || deal2.To.SlashEpoch != 6 {
		t.Fatal("Unexpected change to SlashEpoch")
	}

	// Test that OnActorStateChanged does not call the callback if the state has not changed
	mockAddr, err := address.NewFromString("t01")
	require.NoError(t, err)
	actorDiffFn := preds.OnActorStateChanged(mockAddr, func(context.Context, cid.Cid, cid.Cid) (bool, UserData, error) {
		t.Fatal("No state change so this should not be called")
		return false, nil, nil
	})
	changed, _, err = actorDiffFn(ctx, oldState, oldState)
	require.NoError(t, err)
	require.False(t, changed)

	// Test that OnDealStateChanged does not call the callback if the state has not changed
	diffDealStateFn := preds.OnDealStateChanged(func(context.Context, *amt.Root, *amt.Root) (bool, UserData, error) {
		t.Fatal("No state change so this should not be called")
		return false, nil, nil
	})
	marketState := createEmptyMarketState(t, store)
	changed, _, err = diffDealStateFn(ctx, marketState, marketState)
	require.NoError(t, err)
	require.False(t, changed)
}

func mockTipset(miner address.Address, timestamp uint64) (*types.TipSet, error) {
	return types.NewTipSet([]*types.BlockHeader{{
		Miner:                 miner,
		Height:                5,
		ParentStateRoot:       dummyCid,
		Messages:              dummyCid,
		ParentMessageReceipts: dummyCid,
		BlockSig:              &crypto.Signature{Type: crypto.SigTypeBLS},
		BLSAggregate:          &crypto.Signature{Type: crypto.SigTypeBLS},
		Timestamp:             timestamp,
	}})
}

func createMarketState(ctx context.Context, t *testing.T, store *cbornode.BasicIpldStore, deals map[abi.DealID]*market.DealState) cid.Cid {
	rootCid := createAMT(ctx, t, store, deals)

	state := createEmptyMarketState(t, store)
	state.States = rootCid

	stateC, err := store.Put(ctx, state)
	require.NoError(t, err)
	return stateC
}

func createEmptyMarketState(t *testing.T, store *cbornode.BasicIpldStore) *market.State {
	emptyArrayCid, err := amt.NewAMT(store).Flush(context.TODO())
	require.NoError(t, err)
	emptyMap, err := store.Put(context.TODO(), hamt.NewNode(store, hamt.UseTreeBitWidth(5)))
	require.NoError(t, err)
	return market.ConstructState(emptyArrayCid, emptyMap, emptyMap)
}

func createAMT(ctx context.Context, t *testing.T, store *cbornode.BasicIpldStore, deals map[abi.DealID]*market.DealState) cid.Cid {
	root := amt.NewAMT(store)
	for dealID, dealState := range deals {
		err := root.Set(ctx, uint64(dealID), dealState)
		require.NoError(t, err)
	}
	rootCid, err := root.Flush(ctx)
	require.NoError(t, err)
	return rootCid
}
