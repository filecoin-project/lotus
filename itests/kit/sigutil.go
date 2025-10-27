package kit

import (
	typescrypto "github.com/filecoin-project/go-state-types/crypto"

	"github.com/filecoin-project/lotus/lib/sigs"
)

// SigDelegatedSign signs the given preimage with the provided private key using
// the delegated signature type, returning a 65-byte r||s||v signature wrapper.
func SigDelegatedSign(privKey []byte, preimage []byte) (*typescrypto.Signature, error) {
	return sigs.Sign(typescrypto.SigTypeDelegated, privKey, preimage)
}
