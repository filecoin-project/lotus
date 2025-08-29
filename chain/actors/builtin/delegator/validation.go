package delegator

import (
    "bytes"
    "encoding/hex"
    "fmt"
    mathbig "math/big"

    cbg "github.com/whyrusleeping/cbor-gen"
    stbig "github.com/filecoin-project/go-state-types/big"
)

// secp256k1 half order (n/2) as big.Int.
// 0x7fffffffffffffffffffffffffffffff5d576e7357a4501ddfe92f46681b20a0
var secp256k1HalfOrder = func() *mathbig.Int {
    n, _ := new(mathbig.Int).SetString("7fffffffffffffffffffffffffffffff5d576e7357a4501ddfe92f46681b20a0", 16)
    return n
}()

// DecodeAuthorizationTuples decodes CBOR-encoded array of authorization tuples
// of shape [chain_id, address(20b), nonce, y_parity, r_bytes, s_bytes].
// This mirrors the encoding used by ethtypes.CborEncodeEIP7702Authorizations.
func DecodeAuthorizationTuples(data []byte) ([]DelegationParam, error) {
    r := cbg.NewCborReader(bytes.NewReader(data))
    maj, l, err := r.ReadHeader()
    if err != nil {
        return nil, fmt.Errorf("read top-level header: %w", err)
    }
    if maj != cbg.MajArray {
        return nil, fmt.Errorf("top-level must be array")
    }
    res := make([]DelegationParam, 0, int(l))
    for i := 0; i < int(l); i++ {
        maj, innerLen, err := r.ReadHeader()
        if err != nil {
            return nil, fmt.Errorf("tuple[%d]: read header: %w", i, err)
        }
        if maj != cbg.MajArray || innerLen != 6 {
            return nil, fmt.Errorf("tuple[%d]: expected array(6)", i)
        }
        // chain_id
        maj, v, err := r.ReadHeader()
        if err != nil || maj != cbg.MajUnsignedInt {
            return nil, fmt.Errorf("tuple[%d]: chain_id invalid", i)
        }
        dp := DelegationParam{ChainID: v}
        // address (byte string of len 20)
        maj, blen, err := r.ReadHeader()
        if err != nil || maj != cbg.MajByteString || blen != 20 {
            return nil, fmt.Errorf("tuple[%d]: address invalid", i)
        }
        if _, err := r.Read(dp.Address[:]); err != nil {
            return nil, fmt.Errorf("tuple[%d]: address read: %w", i, err)
        }
        // nonce
        maj, v, err = r.ReadHeader()
        if err != nil || maj != cbg.MajUnsignedInt {
            return nil, fmt.Errorf("tuple[%d]: nonce invalid", i)
        }
        dp.Nonce = v
        // y_parity
        maj, v, err = r.ReadHeader()
        if err != nil || maj != cbg.MajUnsignedInt {
            return nil, fmt.Errorf("tuple[%d]: y_parity invalid", i)
        }
        dp.YParity = uint8(v)
        // r
        maj, blen, err = r.ReadHeader()
        if err != nil || maj != cbg.MajByteString {
            return nil, fmt.Errorf("tuple[%d]: r invalid", i)
        }
        rbytes := make([]byte, blen)
        if _, err := r.Read(rbytes); err != nil {
            return nil, fmt.Errorf("tuple[%d]: r read: %w", i, err)
        }
        // s
        maj, blen, err = r.ReadHeader()
        if err != nil || maj != cbg.MajByteString {
            return nil, fmt.Errorf("tuple[%d]: s invalid", i)
        }
        sbytes := make([]byte, blen)
        if _, err := r.Read(sbytes); err != nil {
            return nil, fmt.Errorf("tuple[%d]: s read: %w", i, err)
        }
        // Assign big.Int from big-endian bytes (allow leading zeros)
        dp.R = stbig.NewFromGo(new(mathbig.Int).SetBytes(rbytes))
        dp.S = stbig.NewFromGo(new(mathbig.Int).SetBytes(sbytes))
        res = append(res, dp)
    }
    return res, nil
}

// ValidateDelegations performs static validation on 7702 authorization tuples
// that does not require chain state: chainId set membership, y_parity in {0,1},
// nonzero r/s, and low-s (BIP-62 style) for s.
// localChainID is the EIP-155 chainId for the network; the EIP permits chainId ∈ {0, local}.
func ValidateDelegations(list []DelegationParam, localChainID uint64) error {
    if len(list) == 0 {
        return fmt.Errorf("authorization list must not be empty")
    }
    for i, a := range list {
        if !(a.ChainID == 0 || a.ChainID == localChainID) {
            return fmt.Errorf("authorization[%d]: invalid chain id %d", i, a.ChainID)
        }
        if a.YParity != 0 && a.YParity != 1 {
            return fmt.Errorf("authorization[%d]: y_parity must be 0 or 1", i)
        }
        if a.R.Sign() == 0 || a.S.Sign() == 0 {
            return fmt.Errorf("authorization[%d]: r/s must be non-zero", i)
        }
        sb, _ := a.S.Bytes()
        if new(mathbig.Int).SetBytes(sb).Cmp(secp256k1HalfOrder) > 0 {
            return fmt.Errorf("authorization[%d]: s must be low-s", i)
        }
    }
    return nil
}

// ApplyDelegationsFromCBOR decodes CBOR tuples and validates them against
// static rules. Intended to be called by the actor method once wired.
func ApplyDelegationsFromCBOR(data []byte, localChainID uint64) ([]DelegationParam, error) {
    list, err := DecodeAuthorizationTuples(data)
    if err != nil {
        return nil, err
    }
    if err := ValidateDelegations(list, localChainID); err != nil {
        return nil, err
    }
    return list, nil
}

// Debug helper (not used in production), returns hex of 20b address for logging.
func hex20(b [20]byte) string { return hex.EncodeToString(b[:]) }
