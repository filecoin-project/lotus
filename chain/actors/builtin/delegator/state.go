package delegator

import (
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

