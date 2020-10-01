package stmgr

type Option func(*config)

type config struct {
	upgradeSchedule UpgradeSchedule
}

func parseOptions(opts ...Option) *config {
	cfg := &config{
		upgradeSchedule: DefaultUpgradeSchedule,
	}
	for _, opt := range opts {
		opt(cfg)
	}
	return cfg
}

func WithUpgradeSchedule(schedule UpgradeSchedule) Option {
	return func(cfg *config) {
		cfg.upgradeSchedule = schedule
	}
}
