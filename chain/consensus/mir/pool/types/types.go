package types

import (
	"github.com/filecoin-project/mir/pkg/pb/requestpb"
	t "github.com/filecoin-project/mir/pkg/types"
)

// ModuleConfig sets the module ids. All replicas are expected to use identical module configurations.
type ModuleConfig struct {
	Self   t.ModuleID // id of this module
	Hasher t.ModuleID
}

// ModuleParams sets the values for the parameters of an instance of the protocol.
// All replicas are expected to use identical module parameters.
type ModuleParams struct {
	MaxTransactionsInBatch int
}

// State represents the state accessible to all parts of the module implementation.
type State struct {
	// A channel is sent on this channel when Mir wants to notify Eudico
	// that Mir is ready for the next batch of input transactions.
	ToMir chan chan []*requestpb.Request
}
