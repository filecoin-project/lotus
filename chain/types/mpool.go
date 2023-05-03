package types

import (
	"time"

	"github.com/filecoin-project/go-address"
)

type MpoolOptionalMetrics struct {
	MpoolSize bool
}

type MpoolConfig struct {
	PriorityAddrs          []address.Address
	SizeLimitHigh          int
	SizeLimitLow           int
	ReplaceByFeeRatio      Percent
	PruneCooldown          time.Duration
	GasLimitOverestimation float64
	OptionalMetrics        MpoolOptionalMetrics
}

func (mc *MpoolConfig) Clone() *MpoolConfig {
	r := new(MpoolConfig)
	*r = *mc
	return r
}
