package build

import (
	"os"
	"slices"
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

func parseF3DisableActivationEnv() (contractAddrs []string, epochs []int64) {
	const F3DisableActivation = "LOTUS_DISABLE_F3_ACTIVATION"

	v, envVarSet := os.LookupEnv(F3DisableActivation)
	if !envVarSet || strings.TrimSpace(v) == "" {
		// Environment variable is not set or empty, activation is not disabled
		return
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
			contractAddrs = append(contractAddrs, value)
		case "epoch":
			parsedEpoch, err := strconv.ParseInt(value, 10, 64)
			if err == nil {
				epochs = append(epochs, parsedEpoch)
			} else {
				log.Warnf("error parsing %s env variable, cannot parse epoch", F3DisableActivation)
			}
		}
	}
	return contractAddrs, epochs
}

// IsF3EpochActivationDisabled checks if F3 activation is disabled for the given
// epoch number based on environment variable configuration.
func IsF3EpochActivationDisabled(epoch int64) bool {
	_, epochs := parseF3DisableActivationEnv()
	return slices.Contains(epochs, epoch)
}
