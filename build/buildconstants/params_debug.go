//go:build debug

package buildconstants

var InsecurePoStValidation = true

func init() {
	BuildType |= BuildDebug
}

// NOTE: Also includes settings from params_2k
