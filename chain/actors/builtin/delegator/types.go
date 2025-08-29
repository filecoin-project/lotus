package delegator

import (
    "github.com/filecoin-project/go-state-types/abi"
    "github.com/filecoin-project/go-state-types/big"
)

// Method numbers are placeholders until the actor is finalized and registered.
const (
    MethodApplyDelegations abi.MethodNum = 2
)

// DelegationParam mirrors an EIP-7702 authorization tuple
// [chain_id, address, nonce, y_parity, r, s].
type DelegationParam struct {
    ChainID uint64
    // 20-byte Ethereum-style address
    Address [20]byte
    Nonce   uint64
    YParity uint8
    R       big.Int
    S       big.Int
}
