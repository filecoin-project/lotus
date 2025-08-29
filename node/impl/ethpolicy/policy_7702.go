package ethpolicy

import (
    "github.com/filecoin-project/go-address"
    "github.com/filecoin-project/go-state-types/abi"
    "github.com/filecoin-project/lotus/chain/types"
)

// CountPendingDelegations returns the number of pending messages from `from`
// that target the Delegator actor and call the ApplyDelegations method.
func CountPendingDelegations(pending []*types.SignedMessage, from address.Address, delegator address.Address, applyMethod abi.MethodNum) int {
    count := 0
    for _, sm := range pending {
        if sm.Message.From == from && sm.Message.To == delegator && sm.Message.Method == applyMethod {
            count++
        }
    }
    return count
}

// ShouldRejectNewDelegation enforces a simple per-EOA cap for pending delegation
// messages. If the count is already at or above the cap, a new delegation from
// this sender should be rejected.
func ShouldRejectNewDelegation(pendingCount int, cap int) bool {
    if cap <= 0 {
        return false
    }
    return pendingCount >= cap
}

