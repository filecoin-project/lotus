//+build cgo

package types

import (
	"fmt"

	bls "github.com/filecoin-project/filecoin-ffi"
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/lib/crypto"
	"github.com/minio/blake2b-simd"
)

func (s *Signature) Verify(addr address.Address, msg []byte) error {
	if addr.Protocol() == address.ID {
		return fmt.Errorf("must resolve ID addresses before using them to verify a signature")
	}
	b2sum := blake2b.Sum256(msg)

	switch s.Type {
	case KTSecp256k1:
		pubk, err := crypto.EcRecover(b2sum[:], s.Data)
		if err != nil {
			return err
		}

		maybeaddr, err := address.NewSecp256k1Address(pubk)
		if err != nil {
			return err
		}

		if addr != maybeaddr {
			return fmt.Errorf("signature did not match")
		}

		return nil
	case KTBLS:
		digests := []bls.Digest{bls.Hash(bls.Message(msg))}

		var pubk bls.PublicKey
		copy(pubk[:], addr.Payload())
		pubkeys := []bls.PublicKey{pubk}

		var sig bls.Signature
		copy(sig[:], s.Data)

		if !bls.Verify(&sig, digests, pubkeys) {
			return fmt.Errorf("bls signature failed to verify")
		}

		return nil
	default:
		return fmt.Errorf("cannot verify signature of unsupported type: %s", s.Type)
	}
}
