package delegator

import (
    "fmt"
	"github.com/filecoin-project/go-address"
)

// State is a scaffold for a Delegator system actor that stores a mapping
// from EOA addresses to delegate code addresses (20-byte Ethereum addresses).
//
// NOTE: This is a placeholder for design and planning. The real on-chain
// actor will use canonical IPLD data structures (e.g., HAMT/AMT) and proper
// CBOR codegen/types. This scaffold exists to anchor method signatures and
// help wire higher-level code paths.
type State struct {
    // Delegations maps an authority Filecoin address (EOA f4) to a
    // 20-byte Ethereum-style address whose code should be executed when
    // the authority is called.
    Delegations map[address.Address][20]byte
}

// Ensure maps are initialised before use.
func (s *State) ensure() {
    if s.Delegations == nil {
        s.Delegations = make(map[address.Address][20]byte)
    }
}

// ApplyDelegationsWithAuthorities applies delegation mappings for the provided
// authorities to the given delegate addresses from the validated list. This is a
// scaffold helper used for unit testing and higher-level integration work prior
// to full actor/FVM wiring. It performs nonce matching and increments the
// authority nonce on success.
//
// NOTE: In a full implementation, authority addresses are recovered from the
// signatures (r,s,y_parity) in the tuple, not passed in. This helper expects
// the caller to supply the authority addresses in the same order as the list.
func (s *State) ApplyDelegationsWithAuthorities(
    nonces map[address.Address]uint64,
    authorities []address.Address,
    list []DelegationParam,
) error {
    if len(authorities) != len(list) {
        return fmt.Errorf("authorities length %d must match list length %d", len(authorities), len(list))
    }
    s.ensure()
    for i, a := range list {
        auth := authorities[i]
        // Nonce must match current authority nonce
        cur := nonces[auth]
        if a.Nonce != cur {
            return fmt.Errorf("authorization[%d]: nonce mismatch: have %d expect %d", i, cur, a.Nonce)
        }
        // Write mapping and bump nonce
        s.Delegations[auth] = a.Address
        nonces[auth] = cur + 1
    }
    return nil
}
