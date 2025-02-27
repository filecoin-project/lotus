package build

import (
	"os"
	"strconv"
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
	const F3DisableEnvKey = "LOTUS_DISABLE_F3_SUBSYSTEM"
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

// IsF3ActivationDisabledFor checks if F3 activation is disabled for the given contract address
// or epoch number based on environment variable configuration.
func IsF3ActivationDisabledFor(contractAddr string, epoch int64) bool {
	if !IsF3Enabled() {
		// If F3 is disabled entirely, then activation is also disabled
		return true
	}

	const F3DisableActivation = "LOTUS_DISABLE_F3_ACTIVATION"

	v, envVarSet := os.LookupEnv(F3DisableActivation)
	if !envVarSet || strings.TrimSpace(v) == "" {
		// Environment variable is not set or empty, activation is not disabled
		return false
	}

	// Parse the variable which can be in format "contract:addrs" or "epoch:epochnumber" or both
	parts := strings.Split(v, ",")
	for _, part := range parts {
		kv := strings.SplitN(part, ":", 2)
		if len(kv) != 2 {
			continue
		}

		key := strings.TrimSpace(strings.ToLower(kv[0]))
		value := strings.TrimSpace(kv[1])

		switch key {
		case "contract":
			// If contract address matches, disable activation
			if value != "" && value == contractAddr {
				return true
			}
		case "epoch":
			parsedEpoch, err := strconv.ParseInt(value, 10, 64)
			// If epoch matches, disable activation
			if err == nil && parsedEpoch == epoch {
				return true
			}
		}
	}
	return false
}
