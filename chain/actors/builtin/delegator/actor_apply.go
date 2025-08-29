package delegator

import (
    "fmt"

    "github.com/filecoin-project/go-address"
)

// ApplyDelegationsCore decodes, validates, and applies EIP-7702 authorization tuples
// into the provided State, using the supplied authority nonces and resolved
// authorities. This helper mirrors the expected actor method semantics, but operates
// on the scaffold State and in-memory nonce map to enable thorough unit testing.
//
// In a full actor implementation, authorities are recovered from the tuple
// signatures; here, the caller supplies them in the same order as the tuples.
func ApplyDelegationsCore(
    st *State,
    nonces map[address.Address]uint64,
    authorities []address.Address,
    cborParams []byte,
    localChainID uint64,
) error {
    if st == nil {
        return fmt.Errorf("state is nil")
    }
    list, err := ApplyDelegationsFromCBOR(cborParams, localChainID)
    if err != nil {
        return err
    }
    return st.ApplyDelegationsWithAuthorities(nonces, authorities, list)
}

