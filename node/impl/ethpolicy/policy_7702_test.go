package ethpolicy

import (
    "testing"

    "github.com/stretchr/testify/require"
    "github.com/filecoin-project/go-address"
    "github.com/filecoin-project/go-state-types/abi"
    "github.com/filecoin-project/lotus/chain/types"
)

func TestCountPendingDelegations(t *testing.T) {
    from, _ := address.NewIDAddress(100)
    other, _ := address.NewIDAddress(101)
    delegator, _ := address.NewIDAddress(2000)
    apply := abi.MethodNum(2)

    mk := func(f, to address.Address, m abi.MethodNum) *types.SignedMessage {
        return &types.SignedMessage{Message: types.Message{From: f, To: to, Method: m}}
    }
    pending := []*types.SignedMessage{
        mk(from, delegator, apply),
        mk(from, delegator, apply),
        mk(from, other, apply),
        mk(other, delegator, apply),
        mk(from, delegator, 3),
    }
    require.Equal(t, 2, CountPendingDelegations(pending, from, delegator, apply))
    require.Equal(t, 1, CountPendingDelegations(pending, other, delegator, apply))
}

func TestShouldRejectNewDelegation(t *testing.T) {
    require.False(t, ShouldRejectNewDelegation(0, 2))
    require.False(t, ShouldRejectNewDelegation(1, 2))
    require.True(t, ShouldRejectNewDelegation(2, 2))
    require.True(t, ShouldRejectNewDelegation(3, 2))
    require.False(t, ShouldRejectNewDelegation(10, 0)) // cap disabled
}

