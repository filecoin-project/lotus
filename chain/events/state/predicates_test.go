package state

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/ipfs/go-cid"
	ds "github.com/ipfs/go-datastore"
	ds_sync "github.com/ipfs/go-datastore/sync"
	"github.com/ipfs/go-hamt-ipld"
	bstore "github.com/ipfs/go-ipfs-blockstore"
	cbornode "github.com/ipfs/go-ipld-cbor"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-amt-ipld/v2"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/abi/big"
	"github.com/filecoin-project/specs-actors/actors/builtin/market"
	"github.com/filecoin-project/specs-actors/actors/builtin/miner"
	"github.com/filecoin-project/specs-actors/actors/crypto"
	"github.com/filecoin-project/specs-actors/actors/util/adt"
	tutils "github.com/filecoin-project/specs-actors/support/testing"

	"github.com/filecoin-project/lotus/chain/types"
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

func TestMarketPredicates(t *testing.T) {
	ctx := context.Background()
	bs := bstore.NewBlockstore(ds_sync.MutexWrap(ds.NewMapDatastore()))
	store := cbornode.NewCborStore(bs)

	oldDeal1 := &market.DealState{
		SectorStartEpoch: 1,
		LastUpdatedEpoch: 2,
		SlashEpoch:       0,
	}
	oldDeal2 := &market.DealState{
		SectorStartEpoch: 4,
		LastUpdatedEpoch: 5,
		SlashEpoch:       0,
	}
	oldDeals := map[abi.DealID]*market.DealState{
		abi.DealID(1): oldDeal1,
		abi.DealID(2): oldDeal2,
	}

	oldProp1 := &market.DealProposal{
		PieceCID:             dummyCid,
		PieceSize:            0,
		VerifiedDeal:         false,
		Client:               tutils.NewIDAddr(t, 1),
		Provider:             tutils.NewIDAddr(t, 1),
		StartEpoch:           1,
		EndEpoch:             2,
		StoragePricePerEpoch: big.Zero(),
		ProviderCollateral:   big.Zero(),
		ClientCollateral:     big.Zero(),
	}
	oldProp2 := &market.DealProposal{
		PieceCID:             dummyCid,
		PieceSize:            0,
		VerifiedDeal:         false,
		Client:               tutils.NewIDAddr(t, 1),
		Provider:             tutils.NewIDAddr(t, 1),
		StartEpoch:           2,
		EndEpoch:             3,
		StoragePricePerEpoch: big.Zero(),
		ProviderCollateral:   big.Zero(),
		ClientCollateral:     big.Zero(),
	}
	oldProps := map[abi.DealID]*market.DealProposal{
		abi.DealID(1): oldProp1,
		abi.DealID(2): oldProp2,
	}

	oldStateC := createMarketState(ctx, t, store, oldDeals, oldProps)

	newDeal1 := &market.DealState{
		SectorStartEpoch: 1,
		LastUpdatedEpoch: 3,
		SlashEpoch:       0,
	}
	newDeal2 := &market.DealState{
		SectorStartEpoch: 4,
		LastUpdatedEpoch: 6,
		SlashEpoch:       6,
	}
	// added
	newDeal3 := &market.DealState{
		SectorStartEpoch: 1,
		LastUpdatedEpoch: 2,
		SlashEpoch:       3,
	}
	newDeals := map[abi.DealID]*market.DealState{
		abi.DealID(1): newDeal1,
		abi.DealID(2): newDeal2,
		abi.DealID(3): newDeal3,
	}

	// added
	newProp3 := &market.DealProposal{
		PieceCID:             dummyCid,
		PieceSize:            0,
		VerifiedDeal:         false,
		Client:               tutils.NewIDAddr(t, 1),
		Provider:             tutils.NewIDAddr(t, 1),
		StartEpoch:           4,
		EndEpoch:             4,
		StoragePricePerEpoch: big.Zero(),
		ProviderCollateral:   big.Zero(),
		ClientCollateral:     big.Zero(),
	}
	newProps := map[abi.DealID]*market.DealProposal{
		abi.DealID(1): oldProp1, // 1 was persisted
		// prop 2 was removed
		abi.DealID(3): newProp3, // new
		// NB: DealProposals cannot be modified, so don't test that case.
	}
	newStateC := createMarketState(ctx, t, store, newDeals, newProps)

	minerAddr, err := address.NewFromString("t00")
	require.NoError(t, err)
	oldState, err := mockTipset(minerAddr, 1)
	require.NoError(t, err)
	newState, err := mockTipset(minerAddr, 2)
	require.NoError(t, err)

	api := newMockAPI(bs)
	api.setActor(oldState.Key(), &types.Actor{Head: oldStateC})
	api.setActor(newState.Key(), &types.Actor{Head: newStateC})

	t.Run("deal ID predicate", func(t *testing.T) {
		preds := NewStatePredicates(api)

		dealIds := []abi.DealID{abi.DealID(1), abi.DealID(2)}
		diffIDFn := preds.OnStorageMarketActorChanged(preds.OnDealStateChanged(preds.DealStateChangedForIDs(dealIds)))

		// Diff a state against itself: expect no change
		changed, _, err := diffIDFn(ctx, oldState.Key(), oldState.Key())
		require.NoError(t, err)
		require.False(t, changed)

		// Diff old state against new state
		changed, valIDs, err := diffIDFn(ctx, oldState.Key(), newState.Key())
		require.NoError(t, err)
		require.True(t, changed)

		changedDealIDs, ok := valIDs.(ChangedDeals)
		require.True(t, ok)
		require.Len(t, changedDealIDs, 2)
		require.Contains(t, changedDealIDs, abi.DealID(1))
		require.Contains(t, changedDealIDs, abi.DealID(2))
		deal1 := changedDealIDs[abi.DealID(1)]
		if deal1.From.LastUpdatedEpoch != 2 || deal1.To.LastUpdatedEpoch != 3 {
			t.Fatal("Unexpected change to LastUpdatedEpoch")
		}
		deal2 := changedDealIDs[abi.DealID(2)]
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
		changed, _, err = actorDiffFn(ctx, oldState.Key(), oldState.Key())
		require.NoError(t, err)
		require.False(t, changed)

		// Test that OnDealStateChanged does not call the callback if the state has not changed
		diffDealStateFn := preds.OnDealStateChanged(func(context.Context, *adt.Array, *adt.Array) (bool, UserData, error) {
			t.Fatal("No state change so this should not be called")
			return false, nil, nil
		})
		marketState := createEmptyMarketState(t, store)
		changed, _, err = diffDealStateFn(ctx, marketState, marketState)
		require.NoError(t, err)
		require.False(t, changed)
	})

	t.Run("deal state array predicate", func(t *testing.T) {
		preds := NewStatePredicates(api)
		diffArrFn := preds.OnStorageMarketActorChanged(preds.OnDealStateChanged(preds.OnDealStateAmtChanged()))

		changed, _, err := diffArrFn(ctx, oldState.Key(), oldState.Key())
		require.NoError(t, err)
		require.False(t, changed)

		changed, valArr, err := diffArrFn(ctx, oldState.Key(), newState.Key())
		require.NoError(t, err)
		require.True(t, changed)

		changedDeals, ok := valArr.(*MarketDealStateChanges)
		require.True(t, ok)
		require.Len(t, changedDeals.Added, 1)
		require.Equal(t, abi.DealID(3), changedDeals.Added[0].ID)
		require.Equal(t, *newDeal3, changedDeals.Added[0].Deal)

		require.Len(t, changedDeals.Removed, 0)

		require.Len(t, changedDeals.Modified, 2)
		require.Equal(t, abi.DealID(1), changedDeals.Modified[0].ID)
		require.Equal(t, newDeal1, changedDeals.Modified[0].To)
		require.Equal(t, oldDeal1, changedDeals.Modified[0].From)

		require.Equal(t, abi.DealID(2), changedDeals.Modified[1].ID)
		require.Equal(t, oldDeal2, changedDeals.Modified[1].From)
		require.Equal(t, newDeal2, changedDeals.Modified[1].To)
	})

	t.Run("deal proposal array predicate", func(t *testing.T) {
		preds := NewStatePredicates(api)
		diffArrFn := preds.OnStorageMarketActorChanged(preds.OnDealProposalChanged(preds.OnDealProposalAmtChanged()))
		changed, _, err := diffArrFn(ctx, oldState.Key(), oldState.Key())
		require.NoError(t, err)
		require.False(t, changed)

		changed, valArr, err := diffArrFn(ctx, oldState.Key(), newState.Key())
		require.NoError(t, err)
		require.True(t, changed)

		changedProps, ok := valArr.(*MarketDealProposalChanges)
		require.True(t, ok)
		require.Len(t, changedProps.Added, 1)
		require.Equal(t, abi.DealID(3), changedProps.Added[0].ID)
		require.Equal(t, *newProp3, changedProps.Added[0].Proposal)

		// proposals cannot be modified -- no modified testing

		require.Len(t, changedProps.Removed, 1)
		require.Equal(t, abi.DealID(2), changedProps.Removed[0].ID)
		require.Equal(t, *oldProp2, changedProps.Removed[0].Proposal)
	})
}

func TestMinerSectorChange(t *testing.T) {
	ctx := context.Background()
	bs := bstore.NewBlockstore(ds_sync.MutexWrap(ds.NewMapDatastore()))
	store := cbornode.NewCborStore(bs)

	nextID := uint64(0)
	nextIDAddrF := func() address.Address {
		defer func() { nextID++ }()
		return tutils.NewIDAddr(t, nextID)
	}

	owner, worker := nextIDAddrF(), nextIDAddrF()
	si0 := newSectorOnChainInfo(0, tutils.MakeCID("0"), big.NewInt(0), abi.ChainEpoch(0), abi.ChainEpoch(10))
	si1 := newSectorOnChainInfo(1, tutils.MakeCID("1"), big.NewInt(1), abi.ChainEpoch(1), abi.ChainEpoch(11))
	si2 := newSectorOnChainInfo(2, tutils.MakeCID("2"), big.NewInt(2), abi.ChainEpoch(2), abi.ChainEpoch(11))
	oldMinerC := createMinerState(ctx, t, store, owner, worker, []miner.SectorOnChainInfo{si0, si1, si2})

	si3 := newSectorOnChainInfo(3, tutils.MakeCID("3"), big.NewInt(3), abi.ChainEpoch(3), abi.ChainEpoch(12))
	// 0 delete
	// 1 extend
	// 2 same
	// 3 added
	si1Ext := si1
	si1Ext.Info.Expiration++
	newMinerC := createMinerState(ctx, t, store, owner, worker, []miner.SectorOnChainInfo{si1Ext, si2, si3})

	minerAddr := nextIDAddrF()
	oldState, err := mockTipset(minerAddr, 1)
	require.NoError(t, err)
	newState, err := mockTipset(minerAddr, 2)
	require.NoError(t, err)

	api := newMockAPI(bs)
	api.setActor(oldState.Key(), &types.Actor{Head: oldMinerC})
	api.setActor(newState.Key(), &types.Actor{Head: newMinerC})

	preds := NewStatePredicates(api)

	minerDiffFn := preds.OnMinerActorChange(minerAddr, preds.OnMinerSectorChange())
	change, val, err := minerDiffFn(ctx, oldState.Key(), newState.Key())
	require.NoError(t, err)
	require.True(t, change)
	require.NotNil(t, val)

	sectorChanges, ok := val.(*MinerSectorChanges)
	require.True(t, ok)

	require.Equal(t, len(sectorChanges.Added), 1)
	require.Equal(t, 1, len(sectorChanges.Added))
	require.Equal(t, si3, sectorChanges.Added[0])

	require.Equal(t, 1, len(sectorChanges.Removed))
	require.Equal(t, si0, sectorChanges.Removed[0])

	require.Equal(t, 1, len(sectorChanges.Extended))
	require.Equal(t, si1, sectorChanges.Extended[0].From)
	require.Equal(t, si1Ext, sectorChanges.Extended[0].To)

	change, val, err = minerDiffFn(ctx, oldState.Key(), oldState.Key())
	require.NoError(t, err)
	require.False(t, change)
	require.Nil(t, val)

	change, val, err = minerDiffFn(ctx, newState.Key(), oldState.Key())
	require.NoError(t, err)
	require.True(t, change)
	require.NotNil(t, val)

	sectorChanges, ok = val.(*MinerSectorChanges)
	require.True(t, ok)

	require.Equal(t, 1, len(sectorChanges.Added))
	require.Equal(t, si0, sectorChanges.Added[0])

	require.Equal(t, 1, len(sectorChanges.Removed))
	require.Equal(t, si3, sectorChanges.Removed[0])

	require.Equal(t, 1, len(sectorChanges.Extended))
	require.Equal(t, si1, sectorChanges.Extended[0].To)
	require.Equal(t, si1Ext, sectorChanges.Extended[0].From)
}

func mockTipset(minerAddr address.Address, timestamp uint64) (*types.TipSet, error) {
	return types.NewTipSet([]*types.BlockHeader{{
		Miner:                 minerAddr,
		Height:                5,
		ParentStateRoot:       dummyCid,
		Messages:              dummyCid,
		ParentMessageReceipts: dummyCid,
		BlockSig:              &crypto.Signature{Type: crypto.SigTypeBLS},
		BLSAggregate:          &crypto.Signature{Type: crypto.SigTypeBLS},
		Timestamp:             timestamp,
	}})
}

func createMarketState(ctx context.Context, t *testing.T, store *cbornode.BasicIpldStore, deals map[abi.DealID]*market.DealState, props map[abi.DealID]*market.DealProposal) cid.Cid {
	dealRootCid := createDealAMT(ctx, t, store, deals)
	propRootCid := createProposalAMT(ctx, t, store, props)

	state := createEmptyMarketState(t, store)
	state.States = dealRootCid
	state.Proposals = propRootCid

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

func createDealAMT(ctx context.Context, t *testing.T, store *cbornode.BasicIpldStore, deals map[abi.DealID]*market.DealState) cid.Cid {
	root := amt.NewAMT(store)
	for dealID, dealState := range deals {
		err := root.Set(ctx, uint64(dealID), dealState)
		require.NoError(t, err)
	}
	rootCid, err := root.Flush(ctx)
	require.NoError(t, err)
	return rootCid
}

func createProposalAMT(ctx context.Context, t *testing.T, store *cbornode.BasicIpldStore, props map[abi.DealID]*market.DealProposal) cid.Cid {
	root := amt.NewAMT(store)
	for dealID, prop := range props {
		err := root.Set(ctx, uint64(dealID), prop)
		require.NoError(t, err)
	}
	rootCid, err := root.Flush(ctx)
	require.NoError(t, err)
	return rootCid
}

func createMinerState(ctx context.Context, t *testing.T, store *cbornode.BasicIpldStore, owner, worker address.Address, sectors []miner.SectorOnChainInfo) cid.Cid {
	rootCid := createSectorsAMT(ctx, t, store, sectors)

	state := createEmptyMinerState(ctx, t, store, owner, worker)
	state.Sectors = rootCid

	stateC, err := store.Put(ctx, state)
	require.NoError(t, err)
	return stateC
}

func createEmptyMinerState(ctx context.Context, t *testing.T, store *cbornode.BasicIpldStore, owner, worker address.Address) *miner.State {
	emptyArrayCid, err := amt.NewAMT(store).Flush(context.TODO())
	require.NoError(t, err)
	emptyMap, err := store.Put(context.TODO(), hamt.NewNode(store, hamt.UseTreeBitWidth(5)))
	require.NoError(t, err)

	emptyDeadlines := miner.ConstructDeadlines()
	emptyDeadlinesCid, err := store.Put(context.Background(), emptyDeadlines)
	require.NoError(t, err)

	state, err := miner.ConstructState(emptyArrayCid, emptyMap, emptyDeadlinesCid, owner, worker, abi.PeerID{'1'}, nil, abi.RegisteredSealProof_StackedDrg64GiBV1, 0)
	require.NoError(t, err)
	return state

}

func createSectorsAMT(ctx context.Context, t *testing.T, store *cbornode.BasicIpldStore, sectors []miner.SectorOnChainInfo) cid.Cid {
	root := amt.NewAMT(store)
	for _, sector := range sectors {
		sector := sector
		err := root.Set(ctx, uint64(sector.Info.SectorNumber), &sector)
		require.NoError(t, err)
	}
	rootCid, err := root.Flush(ctx)
	require.NoError(t, err)
	return rootCid
}

// returns a unique SectorOnChainInfo with each invocation with SectorNumber set to `sectorNo`.
func newSectorOnChainInfo(sectorNo abi.SectorNumber, sealed cid.Cid, weight big.Int, activation, expiration abi.ChainEpoch) miner.SectorOnChainInfo {
	info := newSectorPreCommitInfo(sectorNo, sealed, expiration)
	return miner.SectorOnChainInfo{
		Info:               *info,
		ActivationEpoch:    activation,
		DealWeight:         weight,
		VerifiedDealWeight: weight,
	}
}

const (
	sectorSealRandEpochValue = abi.ChainEpoch(1)
)

// returns a unique SectorPreCommitInfo with each invocation with SectorNumber set to `sectorNo`.
func newSectorPreCommitInfo(sectorNo abi.SectorNumber, sealed cid.Cid, expiration abi.ChainEpoch) *miner.SectorPreCommitInfo {
	return &miner.SectorPreCommitInfo{
		SealProof:     abi.RegisteredSealProof_StackedDrg32GiBV1,
		SectorNumber:  sectorNo,
		SealedCID:     sealed,
		SealRandEpoch: sectorSealRandEpochValue,
		DealIDs:       nil,
		Expiration:    expiration,
	}
}
