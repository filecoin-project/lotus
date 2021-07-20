package kit

import (
	"context"
	"testing"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/api/v1api"
	"github.com/filecoin-project/lotus/chain/types"
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

func (f *TestFullNode) ClientImportCARFile(ctx context.Context, rseed int, size int) (res *api.ImportRes, carv1FilePath string, origFilePath string) {
	carv1FilePath, origFilePath = CreateRandomCARv1(f.t, rseed, size)
	res, err := f.ClientImport(ctx, api.FileRef{Path: carv1FilePath, IsCAR: true})
	require.NoError(f.t, err)
	return res, carv1FilePath, origFilePath
}

// CreateImportFile creates a random file with the specified seed and size, and
// imports it into the full node.
func (f *TestFullNode) CreateImportFile(ctx context.Context, rseed int, size int) (res *api.ImportRes, path string) {
	path = CreateRandomFile(f.t, rseed, size)
	res, err := f.ClientImport(ctx, api.FileRef{Path: path})
	require.NoError(f.t, err)
	return res, path
}

// WaitTillChain waits until a specified chain condition is met. It returns
// the first tipset where the condition is met.
func (f *TestFullNode) WaitTillChain(ctx context.Context, pred ChainPredicate) *types.TipSet {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	heads, err := f.ChainNotify(ctx)
	require.NoError(f.t, err)

	for chg := range heads {
		for _, c := range chg {
			if c.Type != "apply" {
				continue
			}
			if ts := c.Val; pred(ts) {
				return ts
			}
		}
	}
	require.Fail(f.t, "chain condition not met")
	return nil
}

// ChainPredicate encapsulates a chain condition.
type ChainPredicate func(set *types.TipSet) bool

// HeightAtLeast returns a ChainPredicate that is satisfied when the chain
// height is equal or higher to the target.
func HeightAtLeast(target abi.ChainEpoch) ChainPredicate {
	return func(ts *types.TipSet) bool {
		return ts.Height() >= target
	}
}

// BlockMinedBy returns a ChainPredicate that is satisfied when we observe the
// first block mined by the specified miner.
func BlockMinedBy(miner address.Address) ChainPredicate {
	return func(ts *types.TipSet) bool {
		for _, b := range ts.Blocks() {
			if b.Miner == miner {
				return true
			}
		}
		return false
	}
}
