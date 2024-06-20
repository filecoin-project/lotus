package build

import (
	"sort"

	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
)

var DrandSchedule = buildconstants.DrandSchedule

func DrandConfigSchedule() dtypes.DrandSchedule {
	out := dtypes.DrandSchedule{}
	for start, network := range DrandSchedule {
		out = append(out, dtypes.DrandPoint{Start: start, Config: buildconstants.DrandConfigs[network]})
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].Start < out[j].Start
	})

	return out
}

type DrandEnum = buildconstants.DrandEnum

const (
	DrandMainnet    = buildconstants.DrandMainnet
	DrandTestnet    = buildconstants.DrandTestnet
	DrandDevnet     = buildconstants.DrandDevnet
	DrandLocalnet   = buildconstants.DrandLocalnet
	DrandIncentinet = buildconstants.DrandIncentinet
	DrandQuicknet   = buildconstants.DrandQuicknet
)

var DrandConfigs = buildconstants.DrandConfigs
