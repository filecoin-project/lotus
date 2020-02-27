package validation

//
// ValidationConfig
//

type ValidationConfig struct {
	trackGas         bool
	checkExitCode    bool
	checkReturnValue bool
}

func (v ValidationConfig) ValidateGas() bool {
	return v.trackGas
}

func (v ValidationConfig) ValidateExitCode() bool {
	return v.checkExitCode
}

func (v ValidationConfig) ValidateReturnValue() bool {
	return v.checkReturnValue
}

