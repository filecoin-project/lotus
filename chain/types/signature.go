package types

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"

	bls "github.com/filecoin-project/go-bls-sigs"
	"github.com/filecoin-project/go-lotus/chain/address"
	"github.com/filecoin-project/go-lotus/lib/crypto"
	cbor "github.com/ipfs/go-ipld-cbor"
	"github.com/minio/blake2b-simd"
	"github.com/polydawn/refmt/obj/atlas"
	cbg "github.com/whyrusleeping/cbor-gen"
)

const SignatureMaxLength = 200

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
	case 0:
		ts = KTSecp256k1
	case 1:
		ts = KTBLS
	default:
		return Signature{}, fmt.Errorf("unsupported signature type: %d", val)
	}

	return Signature{
		Type: ts,
		Data: x[1:],
	}, nil
}

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

		if !bls.Verify(sig, digests, pubkeys) {
			return fmt.Errorf("bls signature failed to verify")
		}

		return nil
	default:
		return fmt.Errorf("cannot verify signature of unsupported type: %s", s.Type)
	}
}

func (s *Signature) TypeCode() int {
	switch s.Type {
	case KTSecp256k1:
		return 0
	case KTBLS:
		return 1
	default:
		panic("unsupported signature type")
	}
}

func (s *Signature) MarshalCBOR(w io.Writer) error {
	header := cbg.CborEncodeMajorType(cbg.MajByteString, uint64(len(s.Data)+1))

	if _, err := w.Write(header); err != nil {
		return err
	}

	if _, err := w.Write([]byte{byte(s.TypeCode())}); err != nil {
		return err
	}

	if _, err := w.Write(s.Data); err != nil {
		return err
	}

	return nil
}

func (s *Signature) UnmarshalCBOR(br cbg.ByteReader) error {
	maj, l, err := cbg.CborReadHeader(br)
	if err != nil {
		return err
	}

	if maj != cbg.MajByteString {
		return fmt.Errorf("cbor input for signature was not a byte string")
	}

	if l > SignatureMaxLength {
		return fmt.Errorf("cbor byte array for signature was too long")
	}

	buf := make([]byte, l)
	if _, err := io.ReadFull(br, buf); err != nil {
		return err
	}

	switch buf[0] {
	default:
		return fmt.Errorf("invalid signature type in cbor input: %d", buf[0])
	case 0:
		s.Type = KTSecp256k1
	case 1:
		s.Type = KTBLS
	}
	s.Data = buf[1:]

	return nil
}

func (s *Signature) Equals(o *Signature) bool {
	return s.Type == o.Type && bytes.Equal(s.Data, o.Data)
}
