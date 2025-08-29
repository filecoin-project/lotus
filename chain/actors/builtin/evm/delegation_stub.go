package evm

// This file provides a placeholder hook for delegated execution (EIP-7702).
// When a message targets an EOA with empty code, the runtime should consult
// the Delegator mapping to determine whether to execute delegate code instead.

// In the actual implementation, the runtime will:
//  - detect empty-code EOA callee
//  - query Delegator actor state for a delegate code address
//  - load delegate bytecode and proceed as if callee had that code
//
// This placeholder exists to anchor integration points and ease future wiring.

// checkDelegatedExecution would return true and the codehash/selector for the
// delegate, if any. Returning false indicates normal execution.
//
// TODO: implement in FVM runtime once Delegator actor is wired.
func checkDelegatedExecution() (bool, [20]byte) {
    return false, [20]byte{}
}

