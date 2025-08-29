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
    type headerReader interface{ ReadHeader() (byte, uint64, error) }
    r := cbg.NewCborReader(bytes.NewReader(params))
    maj, l, err := r.ReadHeader()
    if err != nil || maj != cbg.MajArray {
        return 0
    }
    return int(l)
}
