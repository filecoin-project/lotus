package delegator

import (
    "github.com/filecoin-project/go-state-types/abi"
    "github.com/filecoin-project/go-state-types/big"
    ethtypes "github.com/filecoin-project/lotus/chain/types/ethtypes"
)

// Method numbers are placeholders until the actor is finalized and registered.
const (
    MethodApplyDelegations abi.MethodNum = 2
)

// DelegationParam mirrors an EIP-7702 authorization tuple
// [chain_id, address, nonce, y_parity, r, s].
type DelegationParam struct {
    ChainID uint64
    Address ethtypes.EthAddress
    Nonce   uint64
    YParity uint8
    R       big.Int
    S       big.Int
}

