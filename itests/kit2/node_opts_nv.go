package kit2

import (
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/network"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/node"
)

func LatestActorsAt(upgradeHeight abi.ChainEpoch) node.Option {
	// Attention: Update this when introducing new actor versions or your tests will be sad
	return NetworkUpgradeAt(network.Version13, upgradeHeight)
}

func NetworkUpgradeAt(version network.Version, upgradeHeight abi.ChainEpoch) node.Option {
	fullSchedule := stmgr.UpgradeSchedule{{
		// prepare for upgrade.
		Network:   network.Version9,
		Height:    1,
		Migration: stmgr.UpgradeActorsV2,
	}, {
		Network:   network.Version10,
		Height:    2,
		Migration: stmgr.UpgradeActorsV3,
	}, {
		Network:   network.Version12,
		Height:    3,
		Migration: stmgr.UpgradeActorsV4,
	}, {
		Network:   network.Version13,
		Height:    4,
		Migration: stmgr.UpgradeActorsV5,
	}}

	schedule := stmgr.UpgradeSchedule{}
	for _, upgrade := range fullSchedule {
		if upgrade.Network > version {
			break
		}

		schedule = append(schedule, upgrade)
	}

	if upgradeHeight > 0 {
		schedule[len(schedule)-1].Height = upgradeHeight
	}

	return node.Override(new(stmgr.UpgradeSchedule), schedule)
}

func SDRUpgradeAt(calico, persian abi.ChainEpoch) node.Option {
	return node.Override(new(stmgr.UpgradeSchedule), stmgr.UpgradeSchedule{{
		Network:   network.Version6,
		Height:    1,
		Migration: stmgr.UpgradeActorsV2,
	}, {
		Network:   network.Version7,
		Height:    calico,
		Migration: stmgr.UpgradeCalico,
	}, {
		Network: network.Version8,
		Height:  persian,
	}})

}
