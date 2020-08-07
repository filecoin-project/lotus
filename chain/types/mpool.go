package types

import (
	"time"

	"github.com/filecoin-project/go-address"
)

type MpoolConfig struct {
	PriorityAddrs     []address.Address
	SizeLimitHigh     int
	SizeLimitLow      int
	ReplaceByFeeRatio float64
	PruneCooldown     time.Duration
}

func (mc *MpoolConfig) Clone() *MpoolConfig {
	r := new(MpoolConfig)
	*r = *mc
	return r
}
