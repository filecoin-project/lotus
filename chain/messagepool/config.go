package messagepool

import (
	"time"

	"github.com/filecoin-project/lotus/chain/types"
)

var (
	ReplaceByFeeRatioDefault  = 1.25
	MemPoolSizeLimitHiDefault = 30000
	MemPoolSizeLimitLoDefault = 20000
	PruneCooldownDefault      = time.Minute
)

func (mp *MessagePool) GetConfig() *types.MpoolConfig {
	mp.cfgLk.Lock()
	defer mp.cfgLk.Unlock()
	return mp.cfg.Clone()
}

func (mp *MessagePool) SetConfig(cfg *types.MpoolConfig) {
	cfg = cfg.Clone()
	mp.cfgLk.Lock()
	mp.cfg = cfg
	mp.rbfNum = types.NewInt(uint64((cfg.ReplaceByFeeRatio - 1) * RbfDenom))
	mp.cfgLk.Unlock()
}

func DefaultConfig() *types.MpoolConfig {
	return &types.MpoolConfig{
		SizeLimitHigh:     MemPoolSizeLimitHiDefault,
		SizeLimitLow:      MemPoolSizeLimitLoDefault,
		ReplaceByFeeRatio: ReplaceByFeeRatioDefault,
		PruneCooldown:     PruneCooldownDefault,
	}
}
