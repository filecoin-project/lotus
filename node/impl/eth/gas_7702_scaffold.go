package eth

// This file contains scaffolding notes and helper stubs for EIP-7702 gas accounting.
// It is not wired yet. Once the Delegator actor is integrated and ToUnsignedFilecoinMessage
// builds a message targeting it, EthEstimateGas should include the intrinsic costs per
// authorization tuple and simulate the temporary state change when applying delegations.

// compute7702IntrinsicOverhead returns the additional intrinsic gas to charge for a
// 7702 transaction based on the authorization list length, constants per EIP-7702,
// and whether the target accounts are empty (refunds may apply later).
// TODO: Replace constants and logic with actual values from the EIP.
import (
    "bytes"

    cbg "github.com/whyrusleeping/cbor-gen"
    delegator "github.com/filecoin-project/lotus/chain/actors/builtin/delegator"
)

func compute7702IntrinsicOverhead(authCount int) int64 {
    if authCount <= 0 {
        return 0
    }
    return delegator.BaseOverheadGas + delegator.PerAuthBaseGas*int64(authCount)
}

// countAuthInDelegatorParams tries to CBOR-parse the delegator params and return
// the number of authorization tuples included. It expects the params to be the
// CBOR encoding of an array of 6-tuples, matching CborEncodeEIP7702Authorizations.
// Returns 0 on any parsing error (best effort for estimation headroom).
func countAuthInDelegatorParams(params []byte) int {
    r := cbg.NewCborReader(bytes.NewReader(params))
    maj, l, err := r.ReadHeader()
    if err != nil || maj != cbg.MajArray {
        return 0
    }
    if l == 0 {
        return 0
    }
    // Peek the next header; if it's an inner list (wrapper), return its length.
    maj1, l1, err := r.ReadHeader()
    if err != nil || maj1 != cbg.MajArray {
        return 0
    }
    // Heuristic: if this inner array is not a 6-tuple, we assume it's the wrapper list
    // and l1 is the number of tuples. If it is a 6-tuple, we are likely in legacy shape
    // and the top-level length l is the number of tuples.
    if l == 1 && l1 != 6 {
        return int(l1)
    }
    if l1 == 6 {
        // Legacy: count is top-level length
        return int(l)
    }
    // Fallback: if ambiguous, prefer inner length
    return int(l1)
}
