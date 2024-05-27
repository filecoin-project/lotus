package kit

import (
	"testing"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/chain/wallet/key"
	libp2pcrypto "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
)

// TestUnmanagedMiner is a miner that's not managed by the storage/
// infrastructure, all tasks must be manually executed, managed and scheduled by
// the test or test kit.
type TestUnmanagedMiner struct {
	t       *testing.T
	options nodeOpts

	ActorAddr address.Address
	OwnerKey  *key.Key
	FullNode  *TestFullNode
	Libp2p    struct {
		PeerID  peer.ID
		PrivKey libp2pcrypto.PrivKey
	}
}
