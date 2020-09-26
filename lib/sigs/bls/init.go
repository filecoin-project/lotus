package bls

import (
	"crypto/rand"
	"fmt"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/crypto"

	blst "github.com/supranational/blst/bindings/go"

	"github.com/filecoin-project/lotus/lib/sigs"
)

const DST = string("BLS_SIG_BLS12381G2_XMD:SHA-256_SSWU_RO_NUL_")

type SecretKey = blst.SecretKey
type PublicKey = blst.P1Affine
type Signature = blst.P2Affine
type AggregateSignature = blst.P2Aggregate

type blsSigner struct{}

func (blsSigner) GenPrivate() ([]byte, error) {
	// Generate 32 bytes of randomness
	var ikm [32]byte
	_, err := rand.Read(ikm[:])
	if err != nil {
		return nil, fmt.Errorf("bls signature error generating random data")
	}
	// Note private keys seem to be serialized little-endian!
	pk := blst.KeyGen(ikm[:]).ToLEndian()
	return pk, nil
}

func (blsSigner) ToPublic(priv []byte) ([]byte, error) {
	pk := new(SecretKey).FromLEndian(priv)
	if pk == nil || !pk.Valid() {
		return nil, fmt.Errorf("bls signature invalid private key")
	}
	return new(PublicKey).From(pk).Compress(), nil
}

func (blsSigner) Sign(p []byte, msg []byte) ([]byte, error) {
	pk := new(SecretKey).FromLEndian(p)
	if pk == nil || !pk.Valid() {
		return nil, fmt.Errorf("bls signature invalid private key")
	}
	return new(Signature).Sign(pk, msg, []byte(DST)).Compress(), nil
}

func (blsSigner) Verify(sig []byte, a address.Address, msg []byte) error {
	if !new(Signature).VerifyCompressed(sig, a.Payload()[:], msg, []byte(DST)) {
		return fmt.Errorf("bls signature failed to verify")
	}
	return nil
}

func init() {
	sigs.RegisterSignature(crypto.SigTypeBLS, blsSigner{})
}
