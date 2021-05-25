package kit

import (
	"testing"

	"github.com/filecoin-project/lotus/api/v1api"
	"github.com/filecoin-project/lotus/chain/wallet"
	"github.com/multiformats/go-multiaddr"
)

type TestFullNode struct {
	v1api.FullNode

	t *testing.T

	// ListenAddr is the address on which an API server is listening, if an
	// API server is created for this Node.
	ListenAddr multiaddr.Multiaddr
	DefaultKey *wallet.Key

	options NodeOpts
}
