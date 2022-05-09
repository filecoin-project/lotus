package build

import (
	"github.com/filecoin-project/lotus/node/modules/dtypes"
)

func DrandConfigSchedule() dtypes.DrandSchedule {
	return activeNetworkParams.DrandSchedule
}
