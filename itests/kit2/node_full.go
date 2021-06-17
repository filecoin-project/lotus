package kit2

import (
	"context"
	"testing"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/api/v1api"
	"github.com/filecoin-project/lotus/chain/wallet"
	"github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/require"
)

// TestFullNode represents a full node enrolled in an Ensemble.
type TestFullNode struct {
	v1api.FullNode

	t *testing.T

	// ListenAddr is the address on which an API server is listening, if an
	// API server is created for this Node.
	ListenAddr multiaddr.Multiaddr
	DefaultKey *wallet.Key

	options nodeOpts
}

// CreateImportFile creates a random file with the specified seed and size, and
// imports it into the full node.
func (f *TestFullNode) CreateImportFile(ctx context.Context, rseed int, size int) (res *api.ImportRes, path string) {
	path = CreateRandomFile(f.t, rseed, size)
	res, err := f.ClientImport(ctx, api.FileRef{Path: path})
	require.NoError(f.t, err)
	return res, path
}
