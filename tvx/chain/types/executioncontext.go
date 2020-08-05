package types

import (
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/specs-actors/actors/abi"
)

// ExecutionContext provides the context for execution of a message.
type ExecutionContext struct {
	Epoch abi.ChainEpoch  // The epoch number ("height") during which a message is executed.
	Miner address.Address // The miner actor which earns gas fees from message execution.
}

// NewExecutionContext builds a new execution context.
func NewExecutionContext(epoch int64, miner address.Address) *ExecutionContext {
	return &ExecutionContext{abi.ChainEpoch(epoch), miner}
}
