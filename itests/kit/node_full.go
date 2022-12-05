package kit

import (
	"context"
	"fmt"
	"testing"
	"time"

	libp2pcrypto "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/api/v1api"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/wallet/key"
	cliutil "github.com/filecoin-project/lotus/cli/util"
	"github.com/filecoin-project/lotus/node"
)

type Libp2p struct {
	PeerID  peer.ID
	PrivKey libp2pcrypto.PrivKey
}

// TestFullNode represents a full node enrolled in an Ensemble.
type TestFullNode struct {
	v1api.FullNode

	t *testing.T

	// ListenAddr is the address on which an API server is listening, if an
	// API server is created for this Node.
	ListenAddr multiaddr.Multiaddr
	ListenURL  string
	DefaultKey *key.Key

	Pkey *Libp2p

	Stop node.StopFunc

	options nodeOpts
}

func MergeFullNodes(fullNodes []*TestFullNode) *TestFullNode {
	var wrappedFullNode TestFullNode
	var fns api.FullNodeStruct
	wrappedFullNode.FullNode = &fns

	cliutil.FullNodeProxy(fullNodes, &fns)

	wrappedFullNode.t = fullNodes[0].t
	wrappedFullNode.ListenAddr = fullNodes[0].ListenAddr
	wrappedFullNode.DefaultKey = fullNodes[0].DefaultKey
	wrappedFullNode.Stop = fullNodes[0].Stop
	wrappedFullNode.options = fullNodes[0].options

	return &wrappedFullNode
}

func (f TestFullNode) Shutdown(ctx context.Context) error {
	return f.Stop(ctx)
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

func (f *TestFullNode) WaitForSectorActive(ctx context.Context, t *testing.T, sn abi.SectorNumber, maddr address.Address) {
	for {
		active, err := f.StateMinerActiveSectors(ctx, maddr, types.EmptyTSK)
		require.NoError(t, err)
		for _, si := range active {
			if si.SectorNumber == sn {
				fmt.Printf("ACTIVE\n")
				return
			}
		}

		time.Sleep(time.Second)
	}
}

func (f *TestFullNode) AssignPrivKey(pkey *Libp2p) {
	f.Pkey = pkey
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
