package drivers

type Config struct {
	checkExitCode    bool
	checkReturnValue bool
}

func NewConfig(exit, ret bool) *Config {
	return &Config{
		checkExitCode:    exit,
		checkReturnValue: ret,
	}
}

func (v Config) ValidateExitCode() bool {
	return v.checkExitCode
}

func (v Config) ValidateReturnValue() bool {
	return v.checkReturnValue
}
