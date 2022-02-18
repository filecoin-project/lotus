package kit

import (
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/network"

	"github.com/filecoin-project/lotus/chain/consensus/filcns"
	"github.com/filecoin-project/lotus/chain/stmgr"
)

func UpgradeSchedule(upgrades ...stmgr.Upgrade) EnsembleOpt {
	return func(opts *ensembleOpts) error {
		opts.upgradeSchedule = upgrades
		return opts.upgradeSchedule.Validate()
	}
}

// GenesisNetworkVersion sets the network version of genesis.
func GenesisNetworkVersion(nv network.Version) EnsembleOpt {
	return UpgradeSchedule(stmgr.Upgrade{
		Network: nv,
		Height:  -1,
	})
}

func SDRUpgradeAt(calico, persian abi.ChainEpoch) EnsembleOpt {
	return UpgradeSchedule(stmgr.Upgrade{
		Network: network.Version6,
		Height:  -1,
	}, stmgr.Upgrade{
		Network:   network.Version7,
		Height:    calico,
		Migration: filcns.UpgradeCalico,
	}, stmgr.Upgrade{
		Network: network.Version8,
		Height:  persian,
	})
}

func LatestActorsAt(upgradeHeight abi.ChainEpoch) EnsembleOpt {
	/* inline-gen template
		return UpgradeSchedule(stmgr.Upgrade{
			Network: network.Version{{add .latestNetworkVersion -1}},
			Height:  -1,
		}, stmgr.Upgrade{
			Network:   network.Version{{.latestNetworkVersion}},
			Height:    upgradeHeight,
			Migration: filcns.UpgradeActorsV{{.latestActorsVersion}},
		})
	/* inline-gen start */
	return UpgradeSchedule(stmgr.Upgrade{
		Network: network.Version14,
		Height:  -1,
	}, stmgr.Upgrade{
		Network:   network.Version15,
		Height:    upgradeHeight,
		Migration: filcns.UpgradeActorsV7,
	})
	/* inline-gen end */
}

func TurboUpgradeAt(upgradeHeight abi.ChainEpoch) EnsembleOpt {
	return UpgradeSchedule(stmgr.Upgrade{
		Network: network.Version11,
		Height:  -1,
	}, stmgr.Upgrade{
		Network:   network.Version12,
		Height:    upgradeHeight,
		Migration: filcns.UpgradeActorsV4,
	})
}
