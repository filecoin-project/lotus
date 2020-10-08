package build

import (
	"github.com/filecoin-project/go-address"

	"github.com/libp2p/go-libp2p-core/protocol"

	"github.com/filecoin-project/lotus/node/modules/dtypes"
)

// Core network constants

func BlocksTopic(netName dtypes.NetworkName) string   { return "/fil/blocks/" + string(netName) }
func MessagesTopic(netName dtypes.NetworkName) string { return "/fil/msgs/" + string(netName) }
func DhtProtocolName(netName dtypes.NetworkName) protocol.ID {
	return protocol.ID("/fil/kad/" + string(netName))
}

func UseNewestNetwork() bool {
	// TODO: Put these in a container we can iterate over
	if UpgradeBreezeHeight <= 0 && UpgradeSmokeHeight <= 0 && UpgradeActorsV2Height <= 0 {
		return true
	}
	return false
}

func SetAddressNetwork(n address.Network) {
	address.CurrentNetwork = n
}
