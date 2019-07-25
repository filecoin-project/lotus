package types

import (
	"encoding/binary"
	"fmt"

	"github.com/filecoin-project/go-lotus/chain/address"
	"github.com/filecoin-project/go-lotus/lib/crypto"
	cbor "github.com/ipfs/go-ipld-cbor"
	"github.com/minio/blake2b-simd"
	"github.com/polydawn/refmt/obj/atlas"
)

const (
	KTSecp256k1 = "secp256k1"
	KTBLS       = "bls"
)

func init() {
	cbor.RegisterCborType(atlas.BuildEntry(Signature{}).Transform().
		TransformMarshal(atlas.MakeMarshalTransformFunc(
			func(s Signature) ([]byte, error) {
				buf := make([]byte, 4)
				n := binary.PutUvarint(buf, uint64(s.TypeCode()))
				return append(buf[:n], s.Data...), nil
			})).
		TransformUnmarshal(atlas.MakeUnmarshalTransformFunc(
			func(x []byte) (Signature, error) {
				return SignatureFromBytes(x)
			})).
		Complete())
}

type Signature struct {
	Type string
	Data []byte
}

func SignatureFromBytes(x []byte) (Signature, error) {
	val, nr := binary.Uvarint(x)
	if nr != 1 {
		return Signature{}, fmt.Errorf("signatures with type field longer than one byte are invalid")
	}
	var ts string
	switch val {
	case 1:
		ts = KTSecp256k1
	default:
		return Signature{}, fmt.Errorf("unsupported signature type: %d", val)
	}

	return Signature{
		Type: ts,
		Data: x[1:],
	}, nil
}

func (s *Signature) Verify(addr address.Address, msg []byte) error {
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
	default:
		return fmt.Errorf("cannot verify signature of unsupported type: %s", s.Type)
	}
}

func (s *Signature) TypeCode() int {
	switch s.Type {
	case KTSecp256k1:
		return 1
	case KTBLS:
		return 2
	default:
		panic("unsupported signature type")
	}
}
