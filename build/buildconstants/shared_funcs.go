package buildconstants

import (
	logging "github.com/ipfs/go-log/v2"
	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/chain/actors/policy"
)

// moved from now-defunct build/paramfetch.go
var log = logging.Logger("build/buildtypes")

func SetAddressNetwork(n address.Network) {
	address.CurrentNetwork = n
}

func MustParseAddress(addr string) address.Address {
	ret, err := address.NewFromString(addr)
	if err != nil {
		panic(err)
	}

	return ret
}

func IsNearUpgrade(epoch, upgradeEpoch abi.ChainEpoch) bool {
	if upgradeEpoch < 0 {
		return false
	}
	return epoch > upgradeEpoch-policy.ChainFinality && epoch < upgradeEpoch+policy.ChainFinality
}

func MustParseID(id string) peer.ID {
	p, err := peer.Decode(id)
	if err != nil {
		panic(err)
	}
	return p
}
