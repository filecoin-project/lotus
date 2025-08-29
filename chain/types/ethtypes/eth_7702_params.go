package ethtypes

import (
    "bytes"

    cbg "github.com/whyrusleeping/cbor-gen"
    "github.com/filecoin-project/go-state-types/big"
)

// CborEncodeEIP7702Authorizations encodes the authorizationList into CBOR
// as an array of 6-tuples [chain_id, address(20b), nonce, y_parity, r, s].
// Intended for params to the Delegator actor's ApplyDelegations method.
func CborEncodeEIP7702Authorizations(list []EthAuthorization) ([]byte, error) {
    var buf bytes.Buffer
    if err := cbg.CborWriteHeader(&buf, cbg.MajArray, uint64(len(list))); err != nil {
        return nil, err
    }
    for _, a := range list {
        if err := cbg.CborWriteHeader(&buf, cbg.MajArray, 6); err != nil {
            return nil, err
        }
        if err := cbg.CborWriteHeader(&buf, cbg.MajUnsignedInt, uint64(a.ChainID)); err != nil {
            return nil, err
        }
        if err := cbg.WriteByteArray(&buf, a.Address[:]); err != nil {
            return nil, err
        }
        if err := cbg.CborWriteHeader(&buf, cbg.MajUnsignedInt, uint64(a.Nonce)); err != nil {
            return nil, err
        }
        if err := cbg.CborWriteHeader(&buf, cbg.MajUnsignedInt, uint64(a.YParity)); err != nil {
            return nil, err
        }
        rbig := (big.Int)(a.R)
        rb, err := rbig.Bytes()
        if err != nil {
            return nil, err
        }
        if err := cbg.WriteByteArray(&buf, rb); err != nil {
            return nil, err
        }
        sbig := (big.Int)(a.S)
        sb, err := sbig.Bytes()
        if err != nil {
            return nil, err
        }
        if err := cbg.WriteByteArray(&buf, sb); err != nil {
            return nil, err
        }
    }
    return buf.Bytes(), nil
}
