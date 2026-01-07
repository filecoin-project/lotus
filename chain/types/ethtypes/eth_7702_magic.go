package ethtypes

import (
	"encoding/binary"

	"golang.org/x/crypto/sha3"
)

// EIP-7702 authorization tuple signature domain separator.
// Inner tuple signatures must be over keccak256(0x05 || rlp([chain_id, address, nonce])).
const SetCodeAuthorizationMagic byte = 0x05

// EIP-7702 delegated bytecode indicator prefix and version.
// Code written to the authority account must be 0xef 0x01 0x00 || <20-byte address>.
const (
	Eip7702BytecodeMagicHi byte = 0xEF
	Eip7702BytecodeMagicLo byte = 0x01
	Eip7702BytecodeVersion byte = 0x00
)

// AuthorizationPreimage constructs the exact byte sequence that must be signed
// for an authorization tuple: 0x05 || rlp([chain_id, address, nonce]).
// The returned slice is a freshly allocated buffer.
func AuthorizationPreimage(chainID uint64, address EthAddress, nonce uint64) ([]byte, error) {
	// RLP-encode [chain_id, address, nonce]
	ci, err := formatUint64(chainID)
	if err != nil {
		return nil, err
	}
	ni, err := formatUint64(nonce)
	if err != nil {
		return nil, err
	}
	rl, err := EncodeRLP([]interface{}{ci, address[:], ni})
	if err != nil {
		return nil, err
	}
	// Prefix with magic 0x05
	out := make([]byte, 1+len(rl))
	out[0] = SetCodeAuthorizationMagic
	copy(out[1:], rl)
	return out, nil
}

// AuthorizationKeccak returns keccak256(AuthorizationPreimage(...)).
func AuthorizationKeccak(chainID uint64, address EthAddress, nonce uint64) ([32]byte, error) {
	pre, err := AuthorizationPreimage(chainID, address, nonce)
	if err != nil {
		return [32]byte{}, err
	}
	h := sha3.NewLegacyKeccak256()
	_, _ = h.Write(pre)
	var sum [32]byte
	copy(sum[:], h.Sum(nil))
	return sum, nil
}

// MethodHash computes the FRC-42 4-byte method number for the given name by
// taking the first 4 bytes of keccak256(name) as a big-endian uint32.
func MethodHash(name string) uint64 {
	h := sha3.NewLegacyKeccak256()
	_, _ = h.Write([]byte(name))
	sum := h.Sum(nil)
	return uint64(binary.BigEndian.Uint32(sum[:4]))
}
