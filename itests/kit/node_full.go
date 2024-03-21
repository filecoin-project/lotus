package kit

import (
	"bytes"
	"context"
	"fmt"
	"testing"
	"time"

	libp2pcrypto "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/require"
	cbg "github.com/whyrusleeping/cbor-gen"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/exitcode"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/api/v1api"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/wallet/key"
	cliutil "github.com/filecoin-project/lotus/cli/util"
	"github.com/filecoin-project/lotus/cmd/lotus-provider/deps"
	"github.com/filecoin-project/lotus/gateway"
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

	// gateway handler makes it convenient to register callbalks per topic, so we
	// also use it for tests
	EthSubRouter *gateway.EthSubHandler

	options nodeOpts
}

// TestProviderNode represents a Provider node enrolled in an Ensemble.
type TestProviderNode struct {
	v1api.LotusProviderStruct

	t *testing.T

	*deps.Deps

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

type SendCall struct {
	Method abi.MethodNum
	Params []byte
}

func (f *TestFullNode) MakeSendCall(m abi.MethodNum, params cbg.CBORMarshaler) SendCall {
	var b bytes.Buffer
	err := params.MarshalCBOR(&b)
	require.NoError(f.t, err)
	return SendCall{
		Method: m,
		Params: b.Bytes(),
	}
}

func (f *TestFullNode) ExpectSend(ctx context.Context, from, to address.Address, value types.BigInt, errContains string, sc ...SendCall) *types.SignedMessage {
	msg := &types.Message{From: from, To: to, Value: value}

	if len(sc) == 1 {
		msg.Method = sc[0].Method
		msg.Params = sc[0].Params
	}

	_, err := f.GasEstimateMessageGas(ctx, msg, nil, types.EmptyTSK)
	if errContains != "" {
		require.ErrorContains(f.t, err, errContains)
		return nil
	}
	require.NoError(f.t, err)

	if errContains == "" {
		m, err := f.MpoolPushMessage(ctx, msg, nil)
		require.NoError(f.t, err)

		r, err := f.StateWaitMsg(ctx, m.Cid(), 1, api.LookbackNoLimit, true)
		require.NoError(f.t, err)

		require.Equal(f.t, exitcode.Ok, r.Receipt.ExitCode)
		return m
	}

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

// BlocksMinedByAll returns a ChainPredicate that is satisfied when we observe a
// tipset including blocks from all the specified miners, in no particular order.
func BlocksMinedByAll(miner ...address.Address) ChainPredicate {
	return func(ts *types.TipSet) bool {
		seen := make([]bool, len(miner))
		var done int
		for _, b := range ts.Blocks() {
			for i, m := range miner {
				if b.Miner != m || seen[i] {
					continue
				}
				seen[i] = true
				if done++; done == len(miner) {
					return true
				}
			}
		}
		return false
	}
}
