package kit

import (
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/network"
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
		Migration: stmgr.UpgradeCalico,
	}, stmgr.Upgrade{
		Network: network.Version8,
		Height:  persian,
	})
}

func LatestActorsAt(upgradeHeight abi.ChainEpoch) EnsembleOpt {
	return UpgradeSchedule(stmgr.Upgrade{
		Network: network.Version12,
		Height:  -1,
	}, stmgr.Upgrade{
		Network:   network.Version13,
		Height:    upgradeHeight,
		Migration: stmgr.UpgradeActorsV5,
	})
}

func TurboUpgradeAt(upgradeHeight abi.ChainEpoch) EnsembleOpt {
	return UpgradeSchedule(stmgr.Upgrade{
		Network: network.Version11,
		Height:  -1,
	}, stmgr.Upgrade{
		Network:   network.Version12,
		Height:    upgradeHeight,
		Migration: stmgr.UpgradeActorsV4,
	})
}
