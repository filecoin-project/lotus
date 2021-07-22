package storageadapter

import (
	"context"
	"testing"

	"github.com/filecoin-project/lotus/chain/events"
	"golang.org/x/sync/errgroup"

	cbornode "github.com/ipfs/go-ipld-cbor"

	adt2 "github.com/filecoin-project/specs-actors/v2/actors/util/adt"
	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	bstore "github.com/filecoin-project/lotus/blockstore"
	test "github.com/filecoin-project/lotus/chain/events/state/mock"
	builtin2 "github.com/filecoin-project/specs-actors/v2/actors/builtin"

	market2 "github.com/filecoin-project/specs-actors/v2/actors/builtin/market"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/lotus/chain/events/state"
	"github.com/filecoin-project/lotus/chain/types"
)

func TestDealStateMatcher(t *testing.T) {
	ctx := context.Background()
	bs := bstore.NewMemorySync()
	store := adt2.WrapStore(ctx, cbornode.NewCborStore(bs))

	deal1 := &market2.DealState{
		SectorStartEpoch: 1,
		LastUpdatedEpoch: 2,
	}
	deal2 := &market2.DealState{
		SectorStartEpoch: 4,
		LastUpdatedEpoch: 5,
	}
	deal3 := &market2.DealState{
		SectorStartEpoch: 7,
		LastUpdatedEpoch: 8,
	}
	deals1 := map[abi.DealID]*market2.DealState{
		abi.DealID(1): deal1,
	}
	deals2 := map[abi.DealID]*market2.DealState{
		abi.DealID(1): deal2,
	}
	deals3 := map[abi.DealID]*market2.DealState{
		abi.DealID(1): deal3,
	}

	deal1StateC := createMarketState(ctx, t, store, deals1)
	deal2StateC := createMarketState(ctx, t, store, deals2)
	deal3StateC := createMarketState(ctx, t, store, deals3)

	minerAddr, err := address.NewFromString("t00")
	require.NoError(t, err)
	ts1, err := test.MockTipset(minerAddr, 1)
	require.NoError(t, err)
	ts2, err := test.MockTipset(minerAddr, 2)
	require.NoError(t, err)
	ts3, err := test.MockTipset(minerAddr, 3)
	require.NoError(t, err)

	api := test.NewMockAPI(bs)
	api.SetActor(ts1.Key(), &types.Actor{Code: builtin2.StorageMarketActorCodeID, Head: deal1StateC})
	api.SetActor(ts2.Key(), &types.Actor{Code: builtin2.StorageMarketActorCodeID, Head: deal2StateC})
	api.SetActor(ts3.Key(), &types.Actor{Code: builtin2.StorageMarketActorCodeID, Head: deal3StateC})

	t.Run("caching", func(t *testing.T) {
		dsm := newDealStateMatcher(state.NewStatePredicates(api))
		matcher := dsm.matcher(ctx, abi.DealID(1))

		// Call matcher with tipsets that have the same state
		ok, stateChange, err := matcher(ts1, ts1)
		require.NoError(t, err)
		require.False(t, ok)
		require.Nil(t, stateChange)
		// Should call StateGetActor once for each tipset
		require.Equal(t, 2, api.StateGetActorCallCount())

		// Call matcher with tipsets that have different state
		api.ResetCallCounts()
		ok, stateChange, err = matcher(ts1, ts2)
		require.NoError(t, err)
		require.True(t, ok)
		require.NotNil(t, stateChange)
		// Should call StateGetActor once for each tipset
		require.Equal(t, 2, api.StateGetActorCallCount())

		// Call matcher again with the same tipsets as above, should be cached
		api.ResetCallCounts()
		ok, stateChange, err = matcher(ts1, ts2)
		require.NoError(t, err)
		require.True(t, ok)
		require.NotNil(t, stateChange)
		// Should not call StateGetActor (because it should hit the cache)
		require.Equal(t, 0, api.StateGetActorCallCount())

		// Call matcher with different tipsets, should not be cached
		api.ResetCallCounts()
		ok, stateChange, err = matcher(ts2, ts3)
		require.NoError(t, err)
		require.True(t, ok)
		require.NotNil(t, stateChange)
		// Should call StateGetActor once for each tipset
		require.Equal(t, 2, api.StateGetActorCallCount())
	})

	t.Run("parallel", func(t *testing.T) {
		api.ResetCallCounts()
		dsm := newDealStateMatcher(state.NewStatePredicates(api))
		matcher := dsm.matcher(ctx, abi.DealID(1))

		// Call matcher with lots of go-routines in parallel
		var eg errgroup.Group
		res := make([]struct {
			ok          bool
			stateChange events.StateChange
		}, 20)
		for i := 0; i < len(res); i++ {
			i := i
			eg.Go(func() error {
				ok, stateChange, err := matcher(ts1, ts2)
				res[i].ok = ok
				res[i].stateChange = stateChange
				return err
			})
		}
		err := eg.Wait()
		require.NoError(t, err)

		// All go-routines should have got the same (cached) result
		for i := 1; i < len(res); i++ {
			require.Equal(t, res[i].ok, res[i-1].ok)
			require.Equal(t, res[i].stateChange, res[i-1].stateChange)
		}

		// Only one go-routine should have called StateGetActor
		// (once for each tipset)
		require.Equal(t, 2, api.StateGetActorCallCount())
	})
}

func createMarketState(ctx context.Context, t *testing.T, store adt2.Store, deals map[abi.DealID]*market2.DealState) cid.Cid {
	dealRootCid := test.CreateDealAMT(ctx, t, store, deals)
	state := test.CreateEmptyMarketState(t, store)
	state.States = dealRootCid

	stateC, err := store.Put(ctx, state)
	require.NoError(t, err)
	return stateC
}
