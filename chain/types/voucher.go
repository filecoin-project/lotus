package types

import (
	"encoding/base64"

	"github.com/filecoin-project/go-lotus/chain/address"
	cbor "github.com/ipfs/go-ipld-cbor"
)

func init() {
	cbor.RegisterCborType(Merge{})
	cbor.RegisterCborType(SignedVoucher{})
	cbor.RegisterCborType(ModVerifyParams{})
}

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
	return cbor.DumpObject(osv)
}

func (sv *SignedVoucher) EncodedString() (string, error) {
	data, err := cbor.DumpObject(sv)
	if err != nil {
		return "", err
	}

	return base64.RawURLEncoding.EncodeToString(data), nil
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
