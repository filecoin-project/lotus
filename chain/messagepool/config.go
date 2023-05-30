package messagepool

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ipfs/go-datastore"

	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
)

var (
	ReplaceByFeePercentageMinimum types.Percent = 110
	ReplaceByFeePercentageDefault types.Percent = 125
)

var (
	MemPoolSizeLimitHiDefault = 30000
	MemPoolSizeLimitLoDefault = 20000
	PruneCooldownDefault      = time.Minute
	GasLimitOverestimation    = 1.25

	ConfigKey = datastore.NewKey("/mpool/config")
)

func loadConfig(ctx context.Context, ds dtypes.MetadataDS) (*types.MpoolConfig, error) {
	haveCfg, err := ds.Has(ctx, ConfigKey)
	if err != nil {
		return nil, err
	}

	if !haveCfg {
		return DefaultConfig(), nil
	}

	cfgBytes, err := ds.Get(ctx, ConfigKey)
	if err != nil {
		return nil, err
	}
	cfg := new(types.MpoolConfig)
	err = json.Unmarshal(cfgBytes, cfg)
	return cfg, err
}

func saveConfig(ctx context.Context, cfg *types.MpoolConfig, ds dtypes.MetadataDS) error {
	cfgBytes, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	return ds.Put(ctx, ConfigKey, cfgBytes)
}

func (mp *MessagePool) GetConfig() *types.MpoolConfig {
	return mp.getConfig().Clone()
}

func (mp *MessagePool) getConfig() *types.MpoolConfig {
	mp.cfgLk.RLock()
	defer mp.cfgLk.RUnlock()
	return mp.cfg
}

func validateConfg(cfg *types.MpoolConfig) error {
	if cfg.ReplaceByFeeRatio < ReplaceByFeePercentageMinimum {
		return fmt.Errorf("'ReplaceByFeeRatio' is less than required %s < %s",
			cfg.ReplaceByFeeRatio, ReplaceByFeePercentageMinimum)
	}
	if cfg.GasLimitOverestimation < 1 {
		return fmt.Errorf("'GasLimitOverestimation' cannot be less than 1")
	}
	return nil
}

func (mp *MessagePool) SetConfig(ctx context.Context, cfg *types.MpoolConfig) error {
	if err := validateConfg(cfg); err != nil {
		return err
	}
	cfg = cfg.Clone()

	mp.cfgLk.Lock()
	mp.cfg = cfg
	err := saveConfig(ctx, cfg, mp.ds)
	if err != nil {
		log.Warnf("error persisting mpool config: %s", err)
	}
	mp.cfgLk.Unlock()

	return nil
}

func DefaultConfig() *types.MpoolConfig {
	return &types.MpoolConfig{
		SizeLimitHigh:          MemPoolSizeLimitHiDefault,
		SizeLimitLow:           MemPoolSizeLimitLoDefault,
		ReplaceByFeeRatio:      ReplaceByFeePercentageDefault,
		PruneCooldown:          PruneCooldownDefault,
		GasLimitOverestimation: GasLimitOverestimation,
	}
}
