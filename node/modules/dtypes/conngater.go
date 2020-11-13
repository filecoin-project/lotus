package dtypes

import (
	"github.com/libp2p/go-libp2p-core/peer"
)

type NetBlockList struct {
	Peers     []peer.ID
	IPAddrs   []string
	IPSubnets []string
}
