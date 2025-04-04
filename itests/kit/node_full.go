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
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/exitcode"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/api/v1api"
	"github.com/filecoin-project/lotus/api/v2api"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/wallet/key"
	cliutil "github.com/filecoin-project/lotus/cli/util"
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
	V2 v2api.FullNode

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

// WaitTillChainOrError waits until a specified chain condition is met. It returns
// the first tipset where the condition is met. In the case of an error it will return the error.
func (f *TestFullNode) WaitTillChainOrError(ctx context.Context, pred ChainPredicate) (*types.TipSet, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	heads, err := f.ChainNotify(ctx)
	if err != nil {
		return nil, err
	}

	for chg := range heads {
		for _, c := range chg {
			if c.Type != "apply" {
				continue
			}
			if ts := c.Val; pred(ts) {
				return ts, nil
			}
		}
	}
	return nil, xerrors.New("chain condition not met")
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
