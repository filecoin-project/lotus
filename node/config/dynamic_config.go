package config

type DealmakingConfiger interface {
	GetDealmakingConfig() DealmakingConfig
	SetDealmakingConfig(DealmakingConfig)
}

func (c *StorageMiner) GetDealmakingConfig() DealmakingConfig {
	return c.Dealmaking
}

func (c *StorageMiner) SetDealmakingConfig(other DealmakingConfig) {
	c.Dealmaking = other
}

type SealingConfiger interface {
	GetSealingConfig() SealingConfig
	SetSealingConfig(SealingConfig)
}

func (c *StorageMiner) GetSealingConfig() SealingConfig {
	return c.Sealing
}

func (c *StorageMiner) SetSealingConfig(other SealingConfig) {
	c.Sealing = other
}
