package types

import (
	"github.com/filecoin-project/specs-actors/actors/abi/big"
	"github.com/filecoin-project/specs-actors/actors/runtime/exitcode"
)

// MessageReceipt is the return value of message application.
type MessageReceipt struct {
	ExitCode    exitcode.ExitCode
	ReturnValue []byte

	GasUsed GasUnits
}

type GasUnits int64

func (gu GasUnits) Big() big.Int {
	return big.NewInt(int64(gu))
}
