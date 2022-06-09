//go:build debug
// +build debug

package build

func init() {
	// TODO: This doesn't enable insecure validation in the actors, which is likely a problem.
	// We _shouldn't_ use separate actor bundles for that. Instead, we need an option in the FVM
	// constructor.
	InsecurePoStValidation = true
	BuildType |= BuildDebug
}

// NOTE: Also includes settings from params_2k
