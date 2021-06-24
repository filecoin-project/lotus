package node

import (
	"github.com/filecoin-project/lotus/node/config"
	"github.com/filecoin-project/lotus/node/repo"
)

func ApplyIfEnableLibP2P(r repo.Repo, opts ...Option) Option {
	return ApplyIf(func(settings *Settings) bool {
		lr, err := r.Lock(settings.nodeType)
		if err != nil {
			// log error
			return false
		}
		c, err := lr.Config()
		if err != nil {
			// log error
			return false
		}

		defer lr.Close()

		switch settings.nodeType {
		case repo.FullNode:
			return true
		case repo.StorageMiner:
			cfg, ok := c.(*config.StorageMiner)
			if !ok {
				// log error
				return false
			}

			enableLibP2P := cfg.Subsystems.EnableStorageMarket
			return enableLibP2P
		default:
			// log error
			return false
		}
	}, opts...)
}
