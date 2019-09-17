package types

import (
	"bytes"
)

type MessageReceipt struct {
	ExitCode uint8
	Return   []byte
	GasUsed  BigInt
}

func (mr *MessageReceipt) Equals(o *MessageReceipt) bool {
	return mr.ExitCode == o.ExitCode && bytes.Equal(mr.Return, o.Return) && BigCmp(mr.GasUsed, o.GasUsed) == 0
}
