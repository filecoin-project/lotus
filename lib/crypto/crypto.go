package crypto

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"io"

	secp256k1 "github.com/ipsn/go-secp256k1"
)

// PrivateKeyBytes is the size of a serialized private key.
const PrivateKeyBytes = 32

// PublicKeyBytes is the size of a serialized public key.
const PublicKeyBytes = 65

// PublicKey returns the public key for this private key.
func PublicKey(sk []byte) []byte {
	x, y := secp256k1.S256().ScalarBaseMult(sk)
	return elliptic.Marshal(secp256k1.S256(), x, y)
}

// Sign signs the given message, which must be 32 bytes long.
func Sign(sk, msg []byte) ([]byte, error) {
	return secp256k1.Sign(msg, sk)
}

// Equals compares two private key for equality and returns true if they are the same.
func Equals(sk, other []byte) bool {
	return bytes.Equal(sk, other)
}

// Verify checks the given signature and returns true if it is valid.
func Verify(pk, msg, signature []byte) bool {
	if len(signature) == 65 {
		// Drop the V (1byte) in [R | S | V] style signatures.
		// The V (1byte) is the recovery bit and is not apart of the signature verification.
		return secp256k1.VerifySignature(pk[:], msg, signature[:len(signature)-1])
	}

	return secp256k1.VerifySignature(pk[:], msg, signature)
}

// GenerateKeyFromSeed generates a new key from the given reader.
func GenerateKeyFromSeed(seed io.Reader) ([]byte, error) {
	key, err := ecdsa.GenerateKey(secp256k1.S256(), seed)
	if err != nil {
		return nil, err
	}

	privkey := make([]byte, PrivateKeyBytes)
	blob := key.D.Bytes()

	// the length is guaranteed to be fixed, given the serialization rules for secp2561k curve points.
	copy(privkey[PrivateKeyBytes-len(blob):], blob)

	return privkey, nil
}

// GenerateKey creates a new key using secure randomness from crypto.rand.
func GenerateKey() ([]byte, error) {
	return GenerateKeyFromSeed(rand.Reader)
}

// EcRecover recovers the public key from a message, signature pair.
func EcRecover(msg, signature []byte) ([]byte, error) {
	return secp256k1.RecoverPubkey(msg, signature)
}
