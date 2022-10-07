// Package pool maintains the request pool used to send requests from Eudico's mempool to Mir.
package pool

import (
	"github.com/filecoin-project/lotus/chain/consensus/mir/pool/handlers"
	"github.com/filecoin-project/lotus/chain/consensus/mir/pool/types"
	"github.com/filecoin-project/mir/pkg/dsl"
	"github.com/filecoin-project/mir/pkg/modules"
	"github.com/filecoin-project/mir/pkg/pb/requestpb"
)

// ModuleConfig sets the module ids. All replicas are expected to use identical module configurations.
type ModuleConfig = types.ModuleConfig

// ModuleParams sets the values for the parameters of an instance of the protocol.
// All replicas are expected to use identical module parameters.
type ModuleParams = types.ModuleParams

// DefaultModuleConfig returns a valid module config with default names for all modules.
func DefaultModuleConfig() *ModuleConfig {
	return &ModuleConfig{
		Self:   "availability",
		Hasher: "hasher",
	}
}

// NewModule creates a new instance of a request pool module implementation. It passively waits for
// eventpb.NewRequests events and stores them in a local map.
//
// On a batch request, this implementation creates a batch that consists of as many requests received since the
// previous batch request as possible with respect to params.MaxTransactionsInBatch.
//
// This implementation uses the hash function provided by the mc.Hasher module to compute transaction IDs and batch IDs.
func NewModule(requestChan chan chan []*requestpb.Request, mc *ModuleConfig, params *ModuleParams) modules.Module {
	m := dsl.NewModule(mc.Self)

	state := &types.State{
		ToMir: requestChan,
	}

	handlers.IncludeComputationOfTransactionAndBatchIDs(m, mc, params)
	handlers.IncludeBatchCreation(m, mc, params, state)

	return m
}
