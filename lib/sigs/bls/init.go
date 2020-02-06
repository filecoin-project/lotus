package bls

import (
	"fmt"

	ffi "github.com/filecoin-project/filecoin-ffi"
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/chain/types"
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
	digests := []ffi.Digest{ffi.Hash(ffi.Message(msg))}

	var pubk ffi.PublicKey
	copy(pubk[:], a.Payload())
	pubkeys := []ffi.PublicKey{pubk}

	var s ffi.Signature
	copy(s[:], sig)

	if !ffi.Verify(&s, digests, pubkeys) {
		return fmt.Errorf("bls signature failed to verify")
	}

	return nil
}

func init() {
	sigs.RegisterSignature(types.KTBLS, blsSigner{})
}
