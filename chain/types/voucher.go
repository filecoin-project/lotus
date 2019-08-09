package types

import (
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

type Merge struct {
	Lane  uint64
	Nonce uint64
}

type ModVerifyParams struct {
	Actor  address.Address
	Method uint64
	Data   []byte
}
