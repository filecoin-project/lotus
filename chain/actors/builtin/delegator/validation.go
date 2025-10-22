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

    // Two accepted shapes:
    // 1) Wrapper: [ list ] where list is an array of 6-tuples
    // 2) Legacy: [ tuple, tuple, ... ]

    // Helper to read a single tuple
    readTuple := func(idx int, r *cbg.CborReader) (DelegationParam, error) {
        maj, innerLen, err := r.ReadHeader()
        if err != nil {
            return DelegationParam{}, fmt.Errorf("tuple[%d]: read header: %w", idx, err)
        }
        if maj != cbg.MajArray || innerLen != 6 {
            return DelegationParam{}, fmt.Errorf("tuple[%d]: expected array(6)", idx)
        }
        // chain_id
        maj, v, err := r.ReadHeader()
        if err != nil || maj != cbg.MajUnsignedInt {
            return DelegationParam{}, fmt.Errorf("tuple[%d]: chain_id invalid", idx)
        }
        dp := DelegationParam{ChainID: v}
        // address (byte string of len 20)
        maj, blen, err := r.ReadHeader()
        if err != nil || maj != cbg.MajByteString || blen != 20 {
            return DelegationParam{}, fmt.Errorf("tuple[%d]: address invalid", idx)
        }
        if _, err := r.Read(dp.Address[:]); err != nil {
            return DelegationParam{}, fmt.Errorf("tuple[%d]: address read: %w", idx, err)
        }
        // nonce
        maj, v, err = r.ReadHeader()
        if err != nil || maj != cbg.MajUnsignedInt {
            return DelegationParam{}, fmt.Errorf("tuple[%d]: nonce invalid", idx)
        }
        dp.Nonce = v
        // y_parity
        maj, v, err = r.ReadHeader()
        if err != nil || maj != cbg.MajUnsignedInt {
            return DelegationParam{}, fmt.Errorf("tuple[%d]: y_parity invalid", idx)
        }
        dp.YParity = uint8(v)
        // r
        maj, blen, err = r.ReadHeader()
        if err != nil || maj != cbg.MajByteString {
            return DelegationParam{}, fmt.Errorf("tuple[%d]: r invalid", idx)
        }
        rbytes := make([]byte, blen)
        if _, err := r.Read(rbytes); err != nil {
            return DelegationParam{}, fmt.Errorf("tuple[%d]: r read: %w", idx, err)
        }
        // s
        maj, blen, err = r.ReadHeader()
        if err != nil || maj != cbg.MajByteString {
            return DelegationParam{}, fmt.Errorf("tuple[%d]: s invalid", idx)
        }
        sbytes := make([]byte, blen)
        if _, err := r.Read(sbytes); err != nil {
            return DelegationParam{}, fmt.Errorf("tuple[%d]: s read: %w", idx, err)
        }
        // Assign big.Int from big-endian bytes (allow leading zeros)
        dp.R = stbig.NewFromGo(new(mathbig.Int).SetBytes(rbytes))
        dp.S = stbig.NewFromGo(new(mathbig.Int).SetBytes(sbytes))
        return dp, nil
    }

    res := make([]DelegationParam, 0)

    if l == 0 {
        return res, nil
    }

    // Peek by reading the next header; if it's an inner array of tuples and not a tuple itself,
    // detect wrapper form. We need to read it, so branch and process accordingly.
    // Save reader position by re-parsing from original bytes when needed.
    r2 := cbg.NewCborReader(bytes.NewReader(data))
    // consume top-level header again
    if _, _, err := r2.ReadHeader(); err != nil {
        return nil, fmt.Errorf("re-read top-level header: %w", err)
    }
    maj0, l0, err := r2.ReadHeader()
    if err != nil {
        return nil, fmt.Errorf("peek inner header: %w", err)
    }
    if maj0 == cbg.MajArray && l == 1 && l0 != 6 {
        // Wrapper: inner is the list of tuples (length=l0)
        for i := 0; i < int(l0); i++ {
            dp, err := readTuple(i, r2)
            if err != nil {
                return nil, err
            }
            res = append(res, dp)
        }
        return res, nil
    }

    // Legacy: top-level is a list of tuples (length=l)
    // We've already read the first inner header (maj0,l0) for the first element; rewind logic is
    // not available, so we rebuild a reader and consume top-level, then iterate reading tuples.
    r = cbg.NewCborReader(bytes.NewReader(data))
    if _, _, err := r.ReadHeader(); err != nil {
        return nil, fmt.Errorf("re-read for legacy: %w", err)
    }
    for i := 0; i < int(l); i++ {
        dp, err := readTuple(i, r)
        if err != nil {
            return nil, err
        }
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
