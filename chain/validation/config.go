package validation

//
// Config
//

type Config struct {
	trackGas         bool
	checkExitCode    bool
	checkReturnValue bool
}

func NewConfig(gas, exit, ret bool) *Config {
	return &Config{
		trackGas:         gas,
		checkExitCode:    exit,
		checkReturnValue: ret,
	}
}

func (v Config) ValidateGas() bool {
	return v.trackGas
}

func (v Config) ValidateExitCode() bool {
	return v.checkExitCode
}

func (v Config) ValidateReturnValue() bool {
	return v.checkReturnValue
}
