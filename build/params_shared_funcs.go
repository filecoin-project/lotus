package build

import (
	"os"

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
	return buildconstants.F3Enabled && len(os.Getenv(F3DisableEnvKey)) == 0
}
