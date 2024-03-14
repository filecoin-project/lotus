package filter

import (
	"context"
	pseudo "math/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/builtin"
	"github.com/filecoin-project/go-state-types/crypto"

	"github.com/filecoin-project/lotus/chain/types"
)

func TestCachedActorResolver(t *testing.T) {
	rng := pseudo.New(pseudo.NewSource(299792458))
	a1 := randomF4Addr(t, rng)
	a2 := randomF4Addr(t, rng)

	a1ID := abi.ActorID(1)
	a2ID := abi.ActorID(2)

	addrMap := addressMap{}
	addrMap.add(a1ID, a1)
	addrMap.add(a2ID, a2)

	ctx := context.Background()
	t.Run("when nil tipset caching enabled", func(t *testing.T) {
		const (
			size           = 1
			expiry         = time.Minute
			cacheNilTipSet = true
		)
		var delegateCallCount int
		subject := NewCachedActorResolver(func(ctx context.Context, addr address.Address, ts *types.TipSet) (abi.ActorID, error) {
			delegateCallCount++
			return addrMap.ResolveActor(ctx, addr, ts)
		}, size, expiry, cacheNilTipSet)

		t.Run("cache miss calls delegate", func(t *testing.T) {
			got, err := subject(ctx, a1, nil)
			require.NoError(t, err)
			require.Equal(t, a1ID, got)
			require.Equal(t, 1, delegateCallCount)
		})

		t.Run("cache hit does not call delegate", func(t *testing.T) {
			got, err := subject(ctx, a1, nil)
			require.NoError(t, err)
			require.Equal(t, a1ID, got)
			require.Equal(t, 1, delegateCallCount)
		})
		t.Run("size is respected", func(t *testing.T) {
			// Cause a cache miss.
			got, err := subject(ctx, a2, nil)
			require.NoError(t, err)
			require.Equal(t, a2ID, got)
			require.Equal(t, 2, delegateCallCount)

			// Assert size is respected and a1 is no longer cached
			got, err = subject(ctx, a1, nil)
			require.NoError(t, err)
			require.Equal(t, a1ID, got)
			require.Equal(t, 3, delegateCallCount)
		})
	})

	t.Run("when nil tipset chaching disabled", func(t *testing.T) {
		const (
			size           = 1
			expiry         = time.Minute
			cacheNilTipSet = false
		)
		var delegateCallCount int
		subject := NewCachedActorResolver(func(ctx context.Context, addr address.Address, ts *types.TipSet) (abi.ActorID, error) {
			delegateCallCount++
			return addrMap.ResolveActor(ctx, addr, ts)
		}, size, expiry, cacheNilTipSet)

		t.Run("delegate is called every time", func(t *testing.T) {
			got, err := subject(ctx, a1, nil)
			require.NoError(t, err)
			require.Equal(t, a1ID, got)
			require.Equal(t, 1, delegateCallCount)

			got, err = subject(ctx, a1, nil)
			require.NoError(t, err)
			require.Equal(t, a1ID, got)
			require.Equal(t, 2, delegateCallCount)
		})

		t.Run("non nil tipset is cached", func(t *testing.T) {
			dummyCid := randomCid(t, rng)
			ts, err := types.NewTipSet([]*types.BlockHeader{
				{
					Height:                123,
					Miner:                 builtin.SystemActorAddr,
					Ticket:                &types.Ticket{VRFProof: []byte{byte(123 % 2)}},
					ParentStateRoot:       dummyCid,
					Messages:              dummyCid,
					ParentMessageReceipts: dummyCid,

					BlockSig:     &crypto.Signature{Type: crypto.SigTypeBLS},
					BLSAggregate: &crypto.Signature{Type: crypto.SigTypeBLS},

					ParentBaseFee: big.NewInt(0),
				},
			})
			require.NoError(t, err)

			got, err := subject(ctx, a1, ts)
			require.NoError(t, err)
			require.Equal(t, a1ID, got)
			require.Equal(t, 3, delegateCallCount)

			got, err = subject(ctx, a1, ts)
			require.NoError(t, err)
			require.Equal(t, a1ID, got)
			require.Equal(t, 3, delegateCallCount)
		})

		t.Run("cached by tipset", func(t *testing.T) {
			dummyCid := randomCid(t, rng)
			ts, err := types.NewTipSet([]*types.BlockHeader{
				{
					Height:                123,
					Miner:                 builtin.SystemActorAddr,
					Ticket:                &types.Ticket{VRFProof: []byte{byte(123 % 2)}},
					ParentStateRoot:       dummyCid,
					Messages:              dummyCid,
					ParentMessageReceipts: dummyCid,

					BlockSig:     &crypto.Signature{Type: crypto.SigTypeBLS},
					BLSAggregate: &crypto.Signature{Type: crypto.SigTypeBLS},

					ParentBaseFee: big.NewInt(0),
				},
			})
			require.NoError(t, err)

			dummyCid2 := randomCid(t, rng)
			ts2, err := types.NewTipSet([]*types.BlockHeader{
				{
					Height:                124,
					Miner:                 builtin.SystemActorAddr,
					Ticket:                &types.Ticket{VRFProof: []byte{byte(124 % 2)}},
					ParentStateRoot:       dummyCid2,
					Messages:              dummyCid2,
					ParentMessageReceipts: dummyCid2,

					BlockSig:     &crypto.Signature{Type: crypto.SigTypeBLS},
					BLSAggregate: &crypto.Signature{Type: crypto.SigTypeBLS},

					ParentBaseFee: big.NewInt(0),
				},
			})
			require.NoError(t, err)

			got, err := subject(ctx, a1, ts)
			require.NoError(t, err)
			require.Equal(t, a1ID, got)
			require.Equal(t, 4, delegateCallCount)

			got, err = subject(ctx, a1, ts2)
			require.NoError(t, err)
			require.Equal(t, a1ID, got)
			require.Equal(t, 5, delegateCallCount)

			got, err = subject(ctx, a1, ts2)
			require.NoError(t, err)
			require.Equal(t, a1ID, got)
			require.Equal(t, 5, delegateCallCount)
		})
	})
}
