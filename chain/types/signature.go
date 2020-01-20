package types

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"

	cbg "github.com/whyrusleeping/cbor-gen"
)

const SignatureMaxLength = 200

const (
	KTSecp256k1 = "secp256k1"
	KTBLS       = "bls"
)

const (
	IKTUnknown = -1

	IKTSecp256k1 = iota
	IKTBLS
)

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
	case IKTSecp256k1:
		ts = KTSecp256k1
	case IKTBLS:
		ts = KTBLS
	default:
		return Signature{}, fmt.Errorf("unsupported signature type: %d", val)
	}

	return Signature{
		Type: ts,
		Data: x[1:],
	}, nil
}

func (s *Signature) TypeCode() int {
	switch s.Type {
	case KTSecp256k1:
		return IKTSecp256k1
	case KTBLS:
		return IKTBLS
	default:
		return IKTUnknown
	}
}

func (s *Signature) MarshalCBOR(w io.Writer) error {
	if s == nil {
		_, err := w.Write(cbg.CborNull)
		return err
	}

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

func (s *Signature) UnmarshalCBOR(br io.Reader) error {
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
	case IKTSecp256k1:
		s.Type = KTSecp256k1
	case IKTBLS:
		s.Type = KTBLS
	}
	s.Data = buf[1:]

	return nil
}

func (s *Signature) Equals(o *Signature) bool {
	if s == nil || o == nil {
		return s == o
	}
	return s.Type == o.Type && bytes.Equal(s.Data, o.Data)
}
