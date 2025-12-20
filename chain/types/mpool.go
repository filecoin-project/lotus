package types

import (
	"time"

	"github.com/filecoin-project/go-address"
)

type MpoolConfig struct {
	PriorityAddrs          []address.Address
	SizeLimitHigh          int
	SizeLimitLow           int
	ReplaceByFeeRatio      Percent
	PruneCooldown          time.Duration
	GasLimitOverestimation float64

	// EnableReservationPrePack enables reservation-aware pre-pack simulation in
	// the mpool selector. When combined with the tipset reservation feature
	// gating, this provides an advisory check that per-sender Σ(gas_limit *
	// gas_fee_cap) across the selected block does not exceed the sender's
	// on-chain balance at the selection base state.
	EnableReservationPrePack bool
}

func (mc *MpoolConfig) Clone() *MpoolConfig {
	r := new(MpoolConfig)
	*r = *mc
	return r
}
