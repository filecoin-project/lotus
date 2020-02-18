package types

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"

	"github.com/filecoin-project/specs-actors/actors/crypto"
	cbg "github.com/whyrusleeping/cbor-gen"
)

const SignatureMaxLength = 200

type Signature crypto.Signature

func SignatureFromBytes(x []byte) (Signature, error) {
	val, nr := binary.Uvarint(x)
	if nr != 1 {
		return Signature{}, fmt.Errorf("signatures with type field longer than one byte are invalid")
	}

	return Signature{
		Type: crypto.SigType(val),
		Data: x[1:],
	}, nil
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

	if _, err := w.Write([]byte{byte(s.Type)}); err != nil {
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

	if buf[0] != byte(crypto.SigTypeSecp256k1) && buf[0] != byte(crypto.SigTypeBLS) {
		return fmt.Errorf("invalid signature type in cbor input: %d", buf[0])
	}

	s.Type = crypto.SigType(buf[0])
	s.Data = buf[1:]

	return nil
}

func (s *Signature) Equals(o *Signature) bool {
	if s == nil || o == nil {
		return s == o
	}
	return s.Type == o.Type && bytes.Equal(s.Data, o.Data)
}
