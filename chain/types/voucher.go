package types

import (
	"bytes"
	"encoding/base64"

	"github.com/filecoin-project/go-address"
	cborrpc "github.com/filecoin-project/go-cbor-util"
	cbor "github.com/ipfs/go-ipld-cbor"
)

type SignedVoucher struct {
	TimeLock       uint64
	SecretPreimage []byte
	Extra          *ModVerifyParams
	Lane           uint64
	Nonce          uint64
	Amount         BigInt
	MinCloseHeight uint64

	Merges []Merge

	Signature *Signature
}

func (sv *SignedVoucher) SigningBytes() ([]byte, error) {
	osv := *sv
	osv.Signature = nil

	buf := new(bytes.Buffer)
	if err := osv.MarshalCBOR(buf); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func (sv *SignedVoucher) EncodedString() (string, error) {
	buf := new(bytes.Buffer)
	if err := sv.MarshalCBOR(buf); err != nil {
		return "", err
	}

	return base64.RawURLEncoding.EncodeToString(buf.Bytes()), nil
}

func (sv *SignedVoucher) Equals(other *SignedVoucher) bool {
	// TODO: make this less bad

	selfB, err := cborrpc.Dump(sv)
	if err != nil {
		log.Errorf("SignedVoucher.Equals: dump self: %s", err)
		return false
	}

	otherB, err := cborrpc.Dump(other)
	if err != nil {
		log.Errorf("SignedVoucher.Equals: dump other: %s", err)
		return false
	}

	return bytes.Equal(selfB, otherB)
}

func DecodeSignedVoucher(s string) (*SignedVoucher, error) {
	data, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return nil, err
	}

	var sv SignedVoucher
	if err := cbor.DecodeInto(data, &sv); err != nil {
		return nil, err
	}

	return &sv, nil
}

type Merge struct {
	Lane  uint64
	Nonce uint64
}

type ModVerifyParams struct {
	Actor  address.Address
	Method uint64
	Data   []byte
}
