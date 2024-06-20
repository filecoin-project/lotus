//go:build debug
// +build debug

package buildconstants

func init() {
	InsecurePoStValidation = true
	BuildType |= BuildDebug
}

// NOTE: Also includes settings from params_2k
