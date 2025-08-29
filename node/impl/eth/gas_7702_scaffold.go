package eth

// This file contains scaffolding notes and helper stubs for EIP-7702 gas accounting.
// It is not wired yet. Once the Delegator actor is integrated and ToUnsignedFilecoinMessage
// builds a message targeting it, EthEstimateGas should include the intrinsic costs per
// authorization tuple and simulate the temporary state change when applying delegations.

// compute7702IntrinsicOverhead returns the additional intrinsic gas to charge for a
// 7702 transaction based on the authorization list length, constants per EIP-7702,
// and whether the target accounts are empty (refunds may apply later).
// TODO: Replace constants and logic with actual values from the EIP.
func compute7702IntrinsicOverhead(authCount int) int64 {
    if authCount <= 0 {
        return 0
    }
    const perAuthCost = 25000 // placeholder; set to PER_AUTH_BASE_COST
    return int64(authCount) * perAuthCost
}

