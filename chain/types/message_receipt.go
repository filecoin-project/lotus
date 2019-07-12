package types

import (
	"bytes"

	cbor "github.com/ipfs/go-ipld-cbor"
)

func init() {
	cbor.RegisterCborType(MessageReceipt{})
}

type MessageReceipt struct {
	ExitCode uint8
	Return   []byte
	GasUsed  BigInt
}

func (mr *MessageReceipt) Equals(o *MessageReceipt) bool {
	return mr.ExitCode == o.ExitCode && bytes.Equal(mr.Return, o.Return) && BigCmp(mr.GasUsed, o.GasUsed) == 0
}
