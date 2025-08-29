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

// ApplyDelegationsParams is the CBOR-encoded parameter for the ApplyDelegations method.
// It contains an array of 7702 authorization tuples. A future revision may include
// additional call data or flags.
type ApplyDelegationsParams struct {
    Authorizations []DelegationParam
}

// Gas cost placeholders for Lotus-side estimation; the actor implementation
// should define the authoritative charges/refunds.
const (
    // BaseOverheadGas is a fixed overhead charged for 7702 processing.
    BaseOverheadGas = int64(2100)
    // PerAuthBaseGas is charged per authorization tuple in the list.
    PerAuthBaseGas = int64(25000)
)
