package node

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	"github.com/filecoin-project/lotus/node/config"
)

func TestConfigFullNodeIndexerDependency(t *testing.T) {
	apply := func(cfg *config.FullNode) error {
		return ConfigFullNode(cfg)(&Settings{
			modules: map[interface{}]fx.Option{},
			invokes: make([]fx.Option, _nInvokes),
		})
	}

	require.NoError(t, apply(config.DefaultFullNode()))

	cfg := config.DefaultFullNode()
	cfg.ChainIndexer.EnableIndexer = false
	require.ErrorContains(t, apply(cfg), "ChainIndexer.EnableIndexer is false")
	cfg.Events.EnableActorEventsAPI = false
	require.ErrorContains(t, apply(cfg), "ChainIndexer.EnableIndexer is false")
	cfg.Events.EnableActorEventsAPI = true
	cfg.Fevm.EnableEthRPC = false
	require.ErrorContains(t, apply(cfg), "ChainIndexer.EnableIndexer is false")
	cfg.Events.EnableActorEventsAPI = false
	require.NoError(t, apply(cfg))
}
