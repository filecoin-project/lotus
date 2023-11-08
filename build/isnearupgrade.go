package build

import (
	"github.com/filecoin-project/go-state-types/abi"
)

func IsNearUpgrade(epoch, upgradeEpoch abi.ChainEpoch) bool {
	return upgradeEpoch >= 0 && epoch > upgradeEpoch-Finality && epoch < upgradeEpoch+Finality
}
