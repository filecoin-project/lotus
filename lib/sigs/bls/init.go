package bls

import (
	"fmt"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/specs-actors/actors/crypto"

	ffi "github.com/filecoin-project/filecoin-ffi"

	"github.com/filecoin-project/lotus/lib/sigs"
)

type blsSigner struct{}

func (blsSigner) GenPrivate() ([]byte, error) {
	pk := ffi.PrivateKeyGenerate()
	return pk[:], nil
}

func (blsSigner) ToPublic(priv []byte) ([]byte, error) {
	var pk ffi.PrivateKey
	copy(pk[:], priv)
	pub := ffi.PrivateKeyPublicKey(pk)
	return pub[:], nil
}

func (blsSigner) Sign(p []byte, msg []byte) ([]byte, error) {
	var pk ffi.PrivateKey
	copy(pk[:], p)
	sig := ffi.PrivateKeySign(pk, msg)
	return sig[:], nil
}

func (blsSigner) Verify(sig []byte, a address.Address, msg []byte) error {

	var pubk ffi.PublicKey
	copy(pubk[:], a.Payload())
	pubkeys := []ffi.PublicKey{pubk}
	digests := []ffi.Message{msg}

	var s ffi.Signature
	copy(s[:], sig)

	if !ffi.HashVerify(&s, digests, pubkeys) {
		return fmt.Errorf("bls signature failed to verify")
	}

	return nil
}

func init() {
	sigs.RegisterSignature(crypto.SigTypeBLS, blsSigner{})
}
