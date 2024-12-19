package build

import (
	"os"
	"strings"

	"github.com/libp2p/go-libp2p/core/protocol"

	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
)

// Core network constants

func BlocksTopic(netName dtypes.NetworkName) string   { return "/fil/blocks/" + string(netName) }
func MessagesTopic(netName dtypes.NetworkName) string { return "/fil/msgs/" + string(netName) }
func DhtProtocolName(netName dtypes.NetworkName) protocol.ID {
	return protocol.ID("/fil/kad/" + string(netName))
}

// Deprecated: Use buildconstants.SetAddressNetwork instead.
var SetAddressNetwork = buildconstants.SetAddressNetwork

// Deprecated: Use buildconstants.MustParseAddress instead.
var MustParseAddress = buildconstants.MustParseAddress

func IsF3Enabled() bool {
	const F3DisableEnvKey = "LOTUS_DISABLE_F3"
	if !buildconstants.F3Enabled {
		// Build constant takes precedence over environment variable.
		return false
	}
	v, disableEnvVarSet := os.LookupEnv(F3DisableEnvKey)
	if !disableEnvVarSet {
		// Environment variable to disable F3 is not set.
		return true
	}
	switch strings.TrimSpace(strings.ToLower(v)) {
	case "", "0", "false", "no":
		// Consider these values as "do not disable".
		return true
	default:
		// Consider any other value as disable.
		return false
	}
}

func IsF3PassiveTestingEnabled() bool {
	if !IsF3Enabled() {
		return false
	}
	const F3DisablePassiveTesting = "LOTUS_DISABLE_F3_PASSIVE_TESTING"

	v, disableEnvVarSet := os.LookupEnv(F3DisablePassiveTesting)
	if !disableEnvVarSet {
		// Environment variable to disable F3 passive testing is not set.
		return true
	}
	switch strings.TrimSpace(strings.ToLower(v)) {
	case "", "0", "false", "no":
		// Consider these values as "do not disable".
		return true
	default:
		// Consider any other value as disable.
		return false
	}
}
