package eth

// This file contains scaffolding notes and helper stubs for EIP-7702 gas accounting.
// It is behavioral and uses placeholder constants. EthEstimateGas adds intrinsic
// costs per authorization tuple when targeting EthAccount.ApplyAndCall.

// compute7702IntrinsicOverhead returns the additional intrinsic gas to charge for a
// 7702 transaction based on the authorization list length, constants per EIP-7702,
// and whether the target accounts are empty (refunds may apply later).
// TODO: Replace constants and logic with actual values from the EIP.
import (
	"bytes"

	cbg "github.com/whyrusleeping/cbor-gen"
)

const (
	baseOverheadGas int64 = 1000
	perAuthBaseGas  int64 = 500
)

func compute7702IntrinsicOverhead(authCount int) int64 {
	if authCount <= 0 {
		return 0
	}
	return baseOverheadGas + perAuthBaseGas*int64(authCount)
}

// countAuthInApplyAndCallParams tries to CBOR-parse ApplyAndCall params and return
// the number of authorization tuples included. It expects the params to be the
// CBOR encoding of [ [tuple...], call-tuple ].
// Returns 0 on any parsing error (best effort for estimation headroom).
func countAuthInApplyAndCallParams(params []byte) int {
	r := cbg.NewCborReader(bytes.NewReader(params))
	maj, topLen, err := r.ReadHeader()
	if err != nil || maj != cbg.MajArray {
		return 0
	}
	if topLen == 0 {
		return 0
	}
	// Shape on the wire:
	// ApplyAndCall: [ list-of-tuples, call-tuple ]
	maj1, l1, err := r.ReadHeader()
	if err != nil || maj1 != cbg.MajArray {
		return 0
	}
	// l1 is the number of tuples in the inner list
	return int(l1)
}
