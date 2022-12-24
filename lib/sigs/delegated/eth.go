package delegated

import (
	"fmt"

	"golang.org/x/crypto/sha3"
)

// EthAddressFromPubKey returns the Ethereum address corresponding to an
// uncompressed secp256k1 public key.
//
// TODO move somewhere else, this likely doesn't belong here.
func EthAddressFromPubKey(pubk []byte) ([]byte, error) {
	// if we get an uncompressed public key (that's what we get from the library,
	// but putting this check here for defensiveness), strip the prefix
	if pubk[0] != 0x04 {
		return nil, fmt.Errorf("expected first byte of secp256k1 to be 0x04 (uncompressed)")
	}
	pubk = pubk[1:]

	// Calculate the Ethereum address based on the keccak hash of the pubkey.
	hasher := sha3.NewLegacyKeccak256()
	hasher.Write(pubk)
	ethAddr := hasher.Sum(nil)[12:]
	return ethAddr, nil
}
