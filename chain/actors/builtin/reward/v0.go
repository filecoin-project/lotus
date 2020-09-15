package reward

import (
	"github.com/filecoin-project/specs-actors/actors/builtin/reward"
	"github.com/filecoin-project/specs-actors/actors/util/adt"
)

type v0State struct {
	reward.State
	store adt.Store
}
