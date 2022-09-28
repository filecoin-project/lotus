// Package mempool maintains the request pool used to send requests from Eudico's mempool to Mir.
package mempool

import (
	"github.com/filecoin-project/lotus/chain/consensus/mir/mempool/handlers"
	"github.com/filecoin-project/lotus/chain/consensus/mir/mempool/types"
	"github.com/filecoin-project/mir/pkg/dsl"
	"github.com/filecoin-project/mir/pkg/modules"
	"github.com/filecoin-project/mir/pkg/pb/requestpb"
	t "github.com/filecoin-project/mir/pkg/types"
)

// ModuleConfig sets the module ids. All replicas are expected to use identical module configurations.
type ModuleConfig = types.ModuleConfig

// ModuleParams sets the values for the parameters of an instance of the protocol.
// All replicas are expected to use identical module parameters.
type ModuleParams = types.ModuleParams

// Descriptor sets the values for configuration channel used to access the mempool.
type Descriptor = types.Descriptor

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
func NewModule(requestChan chan Descriptor, mc *ModuleConfig, params *ModuleParams) modules.Module {
	m := dsl.NewModule(mc.Self)

	commonState := &types.State{
		TxByID:         make(map[t.TxID]*requestpb.Request),
		DescriptorChan: requestChan,
	}

	handlers.IncludeComputationOfTransactionAndBatchIDs(m, mc, params, commonState)
	handlers.IncludeBatchCreation(m, mc, params, commonState)
	handlers.IncludeTransactionLookupByID(m, mc, params, commonState)

	return m
}
