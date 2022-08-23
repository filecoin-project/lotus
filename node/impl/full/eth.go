package full

import (
	"context"
	"fmt"

	"go.uber.org/fx"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/store"
)

type EthModuleAPI interface {
	EthBlockNumber(context.Context) (string, error)
}

var _ EthModuleAPI = *new(api.FullNode)

// EthModule provides a default implementation of EthModuleAPI.
// It can be swapped out with another implementation through Dependency
// Injection (for example with a thin RPC client).
type EthModule struct {
	fx.In

	Chain *store.ChainStore
}

var _ EthModuleAPI = (*EthModule)(nil)

type EthAPI struct {
	fx.In

	Chain *store.ChainStore

	EthModuleAPI
}

func (a *EthModule) EthBlockNumber(context.Context) (string, error) {
	height := a.Chain.GetHeaviestTipSet().Height()
	return fmt.Sprintf("0x%x", int(height)), nil
}
