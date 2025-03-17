package state

import (
	"context"
	"testing"

	"github.com/ipfs/go-cid"
	cbornode "github.com/ipfs/go-ipld-cbor"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-bitfield"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	builtin2 "github.com/filecoin-project/specs-actors/v2/actors/builtin"
	market2 "github.com/filecoin-project/specs-actors/v2/actors/builtin/market"
	miner2 "github.com/filecoin-project/specs-actors/v2/actors/builtin/miner"
	adt2 "github.com/filecoin-project/specs-actors/v2/actors/util/adt"
	tutils "github.com/filecoin-project/specs-actors/v5/support/testing"

	bstore "github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/chain/actors/builtin/market"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	test "github.com/filecoin-project/lotus/chain/events/state/mock"
	"github.com/filecoin-project/lotus/chain/types"
)

var dummyCid cid.Cid

func init() {
	dummyCid, _ = cid.Parse("bafkqaaa")
}

func TestMarketPredicates(t *testing.T) {

	ctx := context.Background()
	bs := bstore.NewMemorySync()
	store := adt2.WrapStore(ctx, cbornode.NewCborStore(bs))

	oldDeal1 := &market2.DealState{
		SectorStartEpoch: 1,
		LastUpdatedEpoch: 2,
		SlashEpoch:       0,
	}
	oldDeal2 := &market2.DealState{
		SectorStartEpoch: 4,
		LastUpdatedEpoch: 5,
		SlashEpoch:       0,
	}
	oldDeals := map[abi.DealID]*market2.DealState{
		abi.DealID(1): oldDeal1,
		abi.DealID(2): oldDeal2,
	}

	oldProp1 := &market2.DealProposal{
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
	oldProp2 := &market2.DealProposal{
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
	oldProps := map[abi.DealID]*market2.DealProposal{
		abi.DealID(1): oldProp1,
		abi.DealID(2): oldProp2,
	}

	oldBalances := map[address.Address]balance{
		tutils.NewIDAddr(t, 1): {abi.NewTokenAmount(1000), abi.NewTokenAmount(1000)},
		tutils.NewIDAddr(t, 2): {abi.NewTokenAmount(2000), abi.NewTokenAmount(500)},
		tutils.NewIDAddr(t, 3): {abi.NewTokenAmount(3000), abi.NewTokenAmount(2000)},
		tutils.NewIDAddr(t, 5): {abi.NewTokenAmount(3000), abi.NewTokenAmount(1000)},
	}

	oldStateC := createMarketState(ctx, t, store, oldDeals, oldProps, oldBalances)

	newDeal1 := &market2.DealState{
		SectorStartEpoch: 1,
		LastUpdatedEpoch: 3,
		SlashEpoch:       0,
	}

	// deal 2 removed

	// added
	newDeal3 := &market2.DealState{
		SectorStartEpoch: 1,
		LastUpdatedEpoch: 2,
		SlashEpoch:       3,
	}
	newDeals := map[abi.DealID]*market2.DealState{
		abi.DealID(1): newDeal1,
		// deal 2 was removed
		abi.DealID(3): newDeal3,
	}

	// added
	newProp3 := &market2.DealProposal{
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
	newProps := map[abi.DealID]*market2.DealProposal{
		abi.DealID(1): oldProp1, // 1 was persisted
		// prop 2 was removed
		abi.DealID(3): newProp3, // new
		// NB: DealProposals cannot be modified, so don't test that case.
	}
	newBalances := map[address.Address]balance{
		tutils.NewIDAddr(t, 1): {abi.NewTokenAmount(3000), abi.NewTokenAmount(0)},
		tutils.NewIDAddr(t, 2): {abi.NewTokenAmount(2000), abi.NewTokenAmount(500)},
		tutils.NewIDAddr(t, 4): {abi.NewTokenAmount(5000), abi.NewTokenAmount(0)},
		tutils.NewIDAddr(t, 5): {abi.NewTokenAmount(1000), abi.NewTokenAmount(3000)},
	}

	newStateC := createMarketState(ctx, t, store, newDeals, newProps, newBalances)

	minerAddr, err := address.NewFromString("t00")
	require.NoError(t, err)
	oldState, err := test.MockTipset(minerAddr, 1)
	require.NoError(t, err)
	newState, err := test.MockTipset(minerAddr, 2)
	require.NoError(t, err)

	api := test.NewMockAPI(bs)
	api.SetActor(oldState.Key(), &types.Actor{Code: builtin2.StorageMarketActorCodeID, Head: oldStateC})
	api.SetActor(newState.Key(), &types.Actor{Code: builtin2.StorageMarketActorCodeID, Head: newStateC})

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
		if deal1.From.LastUpdatedEpoch() != 2 || deal1.To.LastUpdatedEpoch() != 3 {
			t.Fatal("Unexpected change to LastUpdatedEpoch")
		}
		deal2 := changedDealIDs[abi.DealID(2)]
		if deal2.From.LastUpdatedEpoch() != 5 || deal2.To != nil {
			t.Fatal("Expected To to be nil")
		}

		// Diff with non-existent deal.
		noDeal := []abi.DealID{4}
		diffNoDealFn := preds.OnStorageMarketActorChanged(preds.OnDealStateChanged(preds.DealStateChangedForIDs(noDeal)))
		changed, _, err = diffNoDealFn(ctx, oldState.Key(), newState.Key())
		require.NoError(t, err)
		require.False(t, changed)

		// Test that OnActorStateChanged does not call the callback if the state has not changed
		mockAddr, err := address.NewFromString("t01")
		require.NoError(t, err)
		actorDiffFn := preds.OnActorStateChanged(mockAddr, func(context.Context, *types.Actor, *types.Actor) (bool, UserData, error) {
			t.Fatal("No state change so this should not be called")
			return false, nil, nil
		})
		changed, _, err = actorDiffFn(ctx, oldState.Key(), oldState.Key())
		require.NoError(t, err)
		require.False(t, changed)

		// Test that OnDealStateChanged does not call the callback if the state has not changed
		diffDealStateFn := preds.OnDealStateChanged(func(context.Context, market.DealStates, market.DealStates) (bool, UserData, error) {
			t.Fatal("No state change so this should not be called")
			return false, nil, nil
		})
		marketState0 := test.CreateEmptyMarketState(t, store)
		marketCid, err := store.Put(ctx, marketState0)
		require.NoError(t, err)
		marketState, err := market.Load(store, &types.Actor{
			Code: builtin2.StorageMarketActorCodeID,
			Head: marketCid,
		})
		require.NoError(t, err)
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

		changedDeals, ok := valArr.(*market.DealStateChanges)
		require.True(t, ok)
		require.Len(t, changedDeals.Added, 1)
		require.Equal(t, abi.DealID(3), changedDeals.Added[0].ID)
		require.True(t, dealEquality(*newDeal3, changedDeals.Added[0].Deal))

		require.Len(t, changedDeals.Removed, 1)

		require.Len(t, changedDeals.Modified, 1)
		require.Equal(t, abi.DealID(1), changedDeals.Modified[0].ID)
		require.True(t, dealEquality(*newDeal1, changedDeals.Modified[0].To))
		require.True(t, dealEquality(*oldDeal1, changedDeals.Modified[0].From))

		require.Equal(t, abi.DealID(2), changedDeals.Removed[0].ID)
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

		changedProps, ok := valArr.(*market.DealProposalChanges)
		require.True(t, ok)
		require.Len(t, changedProps.Added, 1)
		require.Equal(t, abi.DealID(3), changedProps.Added[0].ID)

		// proposals cannot be modified -- no modified testing

		require.Len(t, changedProps.Removed, 1)
		require.Equal(t, abi.DealID(2), changedProps.Removed[0].ID)
	})

	t.Run("balances predicate", func(t *testing.T) {
		preds := NewStatePredicates(api)

		getAddresses := func() []address.Address {
			return []address.Address{tutils.NewIDAddr(t, 1), tutils.NewIDAddr(t, 2), tutils.NewIDAddr(t, 3), tutils.NewIDAddr(t, 4)}
		}
		diffBalancesFn := preds.OnStorageMarketActorChanged(preds.OnBalanceChanged(preds.AvailableBalanceChangedForAddresses(getAddresses)))

		// Diff a state against itself: expect no change
		changed, _, err := diffBalancesFn(ctx, oldState.Key(), oldState.Key())
		require.NoError(t, err)
		require.False(t, changed)

		// Diff old state against new state
		changed, valIDs, err := diffBalancesFn(ctx, oldState.Key(), newState.Key())
		require.NoError(t, err)
		require.True(t, changed)

		changedBalances, ok := valIDs.(ChangedBalances)
		require.True(t, ok)
		require.Len(t, changedBalances, 3)
		require.Contains(t, changedBalances, tutils.NewIDAddr(t, 1))
		require.Contains(t, changedBalances, tutils.NewIDAddr(t, 3))
		require.Contains(t, changedBalances, tutils.NewIDAddr(t, 4))

		balance1 := changedBalances[tutils.NewIDAddr(t, 1)]
		if !balance1.From.Equals(abi.NewTokenAmount(1000)) || !balance1.To.Equals(abi.NewTokenAmount(3000)) {
			t.Fatal("Unexpected change to balance")
		}
		balance3 := changedBalances[tutils.NewIDAddr(t, 3)]
		if !balance3.From.Equals(abi.NewTokenAmount(3000)) || !balance3.To.Equals(abi.NewTokenAmount(0)) {
			t.Fatal("Unexpected change to balance")
		}
		balance4 := changedBalances[tutils.NewIDAddr(t, 4)]
		if !balance4.From.Equals(abi.NewTokenAmount(0)) || !balance4.To.Equals(abi.NewTokenAmount(5000)) {
			t.Fatal("Unexpected change to balance")
		}

		// Diff with non-existent address.
		getNoAddress := func() []address.Address { return []address.Address{tutils.NewIDAddr(t, 6)} }
		diffNoAddressFn := preds.OnStorageMarketActorChanged(preds.OnBalanceChanged(preds.AvailableBalanceChangedForAddresses(getNoAddress)))
		changed, _, err = diffNoAddressFn(ctx, oldState.Key(), newState.Key())
		require.NoError(t, err)
		require.False(t, changed)

		// Test that OnBalanceChanged does not call the callback if the state has not changed
		diffDealBalancesFn := preds.OnBalanceChanged(func(context.Context, BalanceTables, BalanceTables) (bool, UserData, error) {
			t.Fatal("No state change so this should not be called")
			return false, nil, nil
		})
		marketState0 := test.CreateEmptyMarketState(t, store)
		marketCid, err := store.Put(ctx, marketState0)
		require.NoError(t, err)
		marketState, err := market.Load(store, &types.Actor{
			Code: builtin2.StorageMarketActorCodeID,
			Head: marketCid,
		})
		require.NoError(t, err)
		changed, _, err = diffDealBalancesFn(ctx, marketState, marketState)
		require.NoError(t, err)
		require.False(t, changed)
	})

}

func TestMinerSectorChange(t *testing.T) {
	ctx := context.Background()
	bs := bstore.NewMemorySync()
	store := adt2.WrapStore(ctx, cbornode.NewCborStore(bs))

	nextID := uint64(0)
	nextIDAddrF := func() address.Address {
		defer func() { nextID++ }()
		return tutils.NewIDAddr(t, nextID)
	}

	owner, worker := nextIDAddrF(), nextIDAddrF()
	si0 := newSectorOnChainInfo(0, tutils.MakeCID("0", &miner2.SealedCIDPrefix), big.NewInt(0), abi.ChainEpoch(0), abi.ChainEpoch(10))
	si1 := newSectorOnChainInfo(1, tutils.MakeCID("1", &miner2.SealedCIDPrefix), big.NewInt(1), abi.ChainEpoch(1), abi.ChainEpoch(11))
	si2 := newSectorOnChainInfo(2, tutils.MakeCID("2", &miner2.SealedCIDPrefix), big.NewInt(2), abi.ChainEpoch(2), abi.ChainEpoch(11))
	oldMinerC := createMinerState(ctx, t, store, owner, worker, []miner.SectorOnChainInfo{si0, si1, si2})

	si3 := newSectorOnChainInfo(3, tutils.MakeCID("3", &miner2.SealedCIDPrefix), big.NewInt(3), abi.ChainEpoch(3), abi.ChainEpoch(12))
	// 0 delete
	// 1 extend
	// 2 same
	// 3 added
	si1Ext := si1
	si1Ext.Expiration++
	newMinerC := createMinerState(ctx, t, store, owner, worker, []miner.SectorOnChainInfo{si1Ext, si2, si3})

	minerAddr := nextIDAddrF()
	oldState, err := test.MockTipset(minerAddr, 1)
	require.NoError(t, err)
	newState, err := test.MockTipset(minerAddr, 2)
	require.NoError(t, err)

	api := test.NewMockAPI(bs)
	api.SetActor(oldState.Key(), &types.Actor{Head: oldMinerC, Code: builtin2.StorageMinerActorCodeID})
	api.SetActor(newState.Key(), &types.Actor{Head: newMinerC, Code: builtin2.StorageMinerActorCodeID})

	preds := NewStatePredicates(api)

	minerDiffFn := preds.OnMinerActorChange(minerAddr, preds.OnMinerSectorChange())
	change, val, err := minerDiffFn(ctx, oldState.Key(), newState.Key())
	require.NoError(t, err)
	require.True(t, change)
	require.NotNil(t, val)

	sectorChanges, ok := val.(*miner.SectorChanges)
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

	sectorChanges, ok = val.(*miner.SectorChanges)
	require.True(t, ok)

	require.Equal(t, 1, len(sectorChanges.Added))
	require.Equal(t, si0, sectorChanges.Added[0])

	require.Equal(t, 1, len(sectorChanges.Removed))
	require.Equal(t, si3, sectorChanges.Removed[0])

	require.Equal(t, 1, len(sectorChanges.Extended))
	require.Equal(t, si1, sectorChanges.Extended[0].To)
	require.Equal(t, si1Ext, sectorChanges.Extended[0].From)
}

type balance struct {
	available abi.TokenAmount
	locked    abi.TokenAmount
}

func createMarketState(ctx context.Context, t *testing.T, store adt2.Store, deals map[abi.DealID]*market2.DealState, props map[abi.DealID]*market2.DealProposal, balances map[address.Address]balance) cid.Cid {
	dealRootCid := test.CreateDealAMT(ctx, t, store, deals)
	propRootCid := createProposalAMT(ctx, t, store, props)
	balancesCids := createBalanceTable(ctx, t, store, balances)
	state := test.CreateEmptyMarketState(t, store)
	state.States = dealRootCid
	state.Proposals = propRootCid
	state.EscrowTable = balancesCids[0]
	state.LockedTable = balancesCids[1]

	stateC, err := store.Put(ctx, state)
	require.NoError(t, err)
	return stateC
}

func createProposalAMT(ctx context.Context, t *testing.T, store adt2.Store, props map[abi.DealID]*market2.DealProposal) cid.Cid {
	root := adt2.MakeEmptyArray(store)
	for dealID, prop := range props {
		err := root.Set(uint64(dealID), prop)
		require.NoError(t, err)
	}
	rootCid, err := root.Root()
	require.NoError(t, err)
	return rootCid
}

func createBalanceTable(ctx context.Context, t *testing.T, store adt2.Store, balances map[address.Address]balance) [2]cid.Cid {
	escrowMapRoot := adt2.MakeEmptyMap(store)
	escrowMapRootCid, err := escrowMapRoot.Root()
	require.NoError(t, err)
	escrowRoot, err := adt2.AsBalanceTable(store, escrowMapRootCid)
	require.NoError(t, err)
	lockedMapRoot := adt2.MakeEmptyMap(store)
	lockedMapRootCid, err := lockedMapRoot.Root()
	require.NoError(t, err)
	lockedRoot, err := adt2.AsBalanceTable(store, lockedMapRootCid)
	require.NoError(t, err)

	for addr, balance := range balances {
		err := escrowRoot.Add(addr, big.Add(balance.available, balance.locked))
		require.NoError(t, err)
		err = lockedRoot.Add(addr, balance.locked)
		require.NoError(t, err)

	}
	escrowRootCid, err := escrowRoot.Root()
	require.NoError(t, err)
	lockedRootCid, err := lockedRoot.Root()
	require.NoError(t, err)
	return [2]cid.Cid{escrowRootCid, lockedRootCid}
}

func createMinerState(ctx context.Context, t *testing.T, store adt2.Store, owner, worker address.Address, sectors []miner.SectorOnChainInfo) cid.Cid {
	rootCid := createSectorsAMT(ctx, t, store, sectors)

	state := createEmptyMinerState(ctx, t, store, owner, worker)
	state.Sectors = rootCid

	stateC, err := store.Put(ctx, state)
	require.NoError(t, err)
	return stateC
}

func createEmptyMinerState(ctx context.Context, t *testing.T, store adt2.Store, owner, worker address.Address) *miner2.State {
	emptyArrayCid, err := adt2.MakeEmptyArray(store).Root()
	require.NoError(t, err)
	emptyMap, err := adt2.MakeEmptyMap(store).Root()
	require.NoError(t, err)

	emptyDeadline, err := store.Put(store.Context(), miner2.ConstructDeadline(emptyArrayCid))
	require.NoError(t, err)

	emptyVestingFunds := miner2.ConstructVestingFunds()
	emptyVestingFundsCid, err := store.Put(store.Context(), emptyVestingFunds)
	require.NoError(t, err)

	emptyDeadlines := miner2.ConstructDeadlines(emptyDeadline)
	emptyDeadlinesCid, err := store.Put(store.Context(), emptyDeadlines)
	require.NoError(t, err)

	minerInfo := emptyMap

	emptyBitfield := bitfield.NewFromSet(nil)
	emptyBitfieldCid, err := store.Put(store.Context(), emptyBitfield)
	require.NoError(t, err)

	state, err := miner2.ConstructState(minerInfo, 123, 4, emptyBitfieldCid, emptyArrayCid, emptyMap, emptyDeadlinesCid, emptyVestingFundsCid)
	require.NoError(t, err)
	return state

}

func createSectorsAMT(ctx context.Context, t *testing.T, store adt2.Store, sectors []miner.SectorOnChainInfo) cid.Cid {
	root := adt2.MakeEmptyArray(store)
	for _, sector := range sectors {
		edr := big.Zero()
		esp := big.Zero()
		if sector.ExpectedDayReward != nil {
			edr = *sector.ExpectedDayReward
		}
		if sector.ExpectedStoragePledge != nil {
			esp = *sector.ExpectedStoragePledge
		}
		sector := miner2.SectorOnChainInfo{
			SectorNumber:          sector.SectorNumber,
			SealProof:             sector.SealProof,
			SealedCID:             sector.SealedCID,
			DealIDs:               sector.DeprecatedDealIDs,
			Activation:            sector.Activation,
			Expiration:            sector.Expiration,
			DealWeight:            sector.DealWeight,
			VerifiedDealWeight:    sector.VerifiedDealWeight,
			InitialPledge:         sector.InitialPledge,
			ExpectedDayReward:     edr,
			ExpectedStoragePledge: esp,
			ReplacedSectorAge:     0,
			ReplacedDayReward:     big.NewInt(0),
		}
		err := root.Set(uint64(sector.SectorNumber), &sector)
		require.NoError(t, err)
	}
	rootCid, err := root.Root()
	require.NoError(t, err)
	return rootCid
}

// returns a unique SectorOnChainInfo with each invocation with SectorNumber set to `sectorNo`.
func newSectorOnChainInfo(sectorNo abi.SectorNumber, sealed cid.Cid, weight big.Int, activation, expiration abi.ChainEpoch) miner.SectorOnChainInfo {
	info := newSectorPreCommitInfo(sectorNo, sealed, expiration)
	bz := big.Zero()
	return miner.SectorOnChainInfo{
		SectorNumber:      info.SectorNumber,
		SealProof:         info.SealProof,
		SealedCID:         info.SealedCID,
		DeprecatedDealIDs: info.DealIDs,
		Expiration:        info.Expiration,

		Activation:            activation,
		DealWeight:            weight,
		VerifiedDealWeight:    weight,
		InitialPledge:         big.Zero(),
		ExpectedDayReward:     &bz,
		ExpectedStoragePledge: &bz,
		DailyFee:              big.Zero(),
	}
}

const (
	sectorSealRandEpochValue = abi.ChainEpoch(1)
)

// returns a unique SectorPreCommitInfo with each invocation with SectorNumber set to `sectorNo`.
func newSectorPreCommitInfo(sectorNo abi.SectorNumber, sealed cid.Cid, expiration abi.ChainEpoch) *miner2.SectorPreCommitInfo {
	return &miner2.SectorPreCommitInfo{
		SealProof:     abi.RegisteredSealProof_StackedDrg32GiBV1,
		SectorNumber:  sectorNo,
		SealedCID:     sealed,
		SealRandEpoch: sectorSealRandEpochValue,
		DealIDs:       nil,
		Expiration:    expiration,
	}
}

func dealEquality(expected market2.DealState, actual market.DealState) bool {
	return expected.LastUpdatedEpoch == actual.LastUpdatedEpoch() &&
		expected.SectorStartEpoch == actual.SectorStartEpoch() &&
		expected.SlashEpoch == actual.SlashEpoch()
}
