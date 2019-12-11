package vm

import (
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/types"
)

// Actual type is defined in chain/types/vmcontext.go because the VMContext interface is there

func DefaultSyscalls() *types.VMSyscalls {
	return &types.VMSyscalls{
		ValidatePoRep: actors.ValidatePoRep,
	}
}
