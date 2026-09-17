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
	"github.com/filecoin-project/go-bitfield"
	"github.com/filecoin-project/go-state-types/abi"
	stminer "github.com/filecoin-project/go-state-types/builtin/v19/miner"
	"github.com/filecoin-project/go-state-types/dline"
	"github.com/filecoin-project/go-state-types/exitcode"
	gstStore "github.com/filecoin-project/go-state-types/store"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/api/v1api"
	"github.com/filecoin-project/lotus/api/v2api"
	"github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
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

// CurrentProvingDeadline returns the miner's proving deadline at the current
// chain head, rewound if necessary so that di.Open <= head height.
//
// StateMinerProvingDeadline reads the parent state of the queried tipset. At a
// deadline boundary that follows a null round the miner cron has not yet
// advanced the recorded deadline, so NextNotElapsed reports the same deadline
// index one proving period later, and a test waiting for
// di.Open + di.WPoStProvingPeriod would wait two periods instead of one.
func (f *TestFullNode) CurrentProvingDeadline(ctx context.Context, maddr address.Address) *dline.Info {
	head, err := f.ChainHead(ctx)
	require.NoError(f.t, err)

	di, err := f.StateMinerProvingDeadline(ctx, maddr, head.Key())
	require.NoError(f.t, err)

	return DeadlineNotAfter(di, head.Height())
}

// MinerQAP returns the miner's and the whole network's quality-adjusted power at tsk.
func (f *TestFullNode) MinerQAP(ctx context.Context, maddr address.Address, tsk types.TipSetKey) (minerQAP, networkQAP uint64) {
	power, err := f.StateMinerPower(ctx, maddr, tsk)
	require.NoError(f.t, err)
	return power.MinerPower.QualityAdjPower.Uint64(), power.TotalPower.QualityAdjPower.Uint64()
}

// MinerRawPower returns the miner's raw byte power at tsk.
func (f *TestFullNode) MinerRawPower(ctx context.Context, maddr address.Address, tsk types.TipSetKey) uint64 {
	power, err := f.StateMinerPower(ctx, maddr, tsk)
	require.NoError(f.t, err)
	return power.MinerPower.RawBytePower.Uint64()
}

// MustSectorInfo reads a sector that has to be committed at tsk.
func (f *TestFullNode) MustSectorInfo(ctx context.Context, maddr address.Address, sn abi.SectorNumber, tsk types.TipSetKey) *miner.SectorOnChainInfo {
	info, err := f.StateSectorGetInfo(ctx, maddr, sn, tsk)
	require.NoError(f.t, err)
	require.NotNil(f.t, info, "sector %d of miner %s must be committed", sn, maddr)
	return info
}

// Store gives an IPLD store backed by this node, for sneaky state reads.
func (f *TestFullNode) Store(ctx context.Context) gstStore.Store {
	return gstStore.WrapBlockStore(ctx, blockstore.NewAPIBlockstore(f))
}

// UpgradeSectorQualityParams builds the parameters for raising one sector's quality.
func (f *TestFullNode) UpgradeSectorQualityParams(ctx context.Context, maddr address.Address, sector abi.SectorNumber, tsk types.TipSetKey) []byte {
	loc, err := f.StateSectorPartition(ctx, maddr, sector, tsk)
	require.NoError(f.t, err)

	params, err := actors.SerializeParams(&stminer.UpgradeSectorQualityParams{
		Upgrades: []stminer.UpgradeSectorQuality{{
			Deadline:  loc.Deadline,
			Partition: loc.Partition,
			Sectors:   bitfield.NewFromSet([]uint64{uint64(sector)}),
		}},
	})
	require.NoError(f.t, err)
	return params
}

// MinerState loads a miner actor's v19 state at tsk.
func (f *TestFullNode) MinerState(ctx context.Context, maddr address.Address, tsk types.TipSetKey) *stminer.State {
	act, err := f.StateGetActor(ctx, maddr, tsk)
	require.NoError(f.t, err)
	var state stminer.State
	require.NoError(f.t, f.Store(ctx).Get(ctx, act.Head, &state))
	return &state
}

// WaitForDeadlineIndex advances the chain to the window in which idx is the miner's current proving
// deadline, and returns a tipset inside it.
func (f *TestFullNode) WaitForDeadlineIndex(ctx context.Context, maddr address.Address, idx uint64) types.TipSetKey {
	for range 3 {
		head, err := f.ChainHead(ctx)
		require.NoError(f.t, err)
		di, err := f.StateMinerProvingDeadline(ctx, maddr, head.Key())
		require.NoError(f.t, err)
		di = DeadlineForHeight(di, head.Height())
		if di.Index == idx {
			return head.Key()
		}
		steps := (idx + di.WPoStPeriodDeadlines - di.Index) % di.WPoStPeriodDeadlines
		f.WaitTillChain(ctx, HeightAtLeast(di.Open+abi.ChainEpoch(steps)*di.WPoStChallengeWindow+5))
	}
	require.FailNow(f.t, "never landed inside a deadline", "miner %s deadline %d", maddr, idx)
	return types.EmptyTSK
}

// DeadlineForHead returns the deadline the chain head falls in, for a miner whose recorded deadline
// may be stale because it is not enrolled in cron.
func (f *TestFullNode) DeadlineForHead(ctx context.Context, maddr address.Address) *dline.Info {
	head, err := f.ChainHead(ctx)
	require.NoError(f.t, err)

	di, err := f.StateMinerProvingDeadline(ctx, maddr, head.Key())
	require.NoError(f.t, err)

	return DeadlineForHeight(di, head.Height())
}

// DeadlineCloseAfter returns the first epoch at or after `from` at which deadline dlIdx closes, which
// is when the actor next settles that deadline's faults and fees.
func DeadlineCloseAfter(di *dline.Info, dlIdx uint64, from abi.ChainEpoch) abi.ChainEpoch {
	close := di.PeriodStart + abi.ChainEpoch(dlIdx+1)*di.WPoStChallengeWindow
	for close < from {
		close += di.WPoStProvingPeriod
	}
	return close
}

// DeadlineForHeight returns the deadline that height falls in, on di's schedule. Index, Open, Close
// and Challenge all come from that one deadline, unlike the recorded deadline a miner out of cron
// reports.
func DeadlineForHeight(di *dline.Info, height abi.ChainEpoch) *dline.Info {
	// Only the phase of PeriodStart matters: shift it to the period containing height.
	periodStart := di.PeriodStart
	periods := (height - periodStart) / di.WPoStProvingPeriod
	if rem := (height - periodStart) % di.WPoStProvingPeriod; rem < 0 {
		periods-- // Go truncates towards zero, and this period started before PeriodStart
	}
	periodStart += periods * di.WPoStProvingPeriod

	return dline.NewInfo(periodStart, uint64((height-periodStart)/di.WPoStChallengeWindow), height,
		di.WPoStPeriodDeadlines, di.WPoStProvingPeriod, di.WPoStChallengeWindow,
		di.WPoStChallengeLookback, di.FaultDeclarationCutoff)
}

// DeadlineNotAfter rewinds di by whole proving periods until di.Open <= height.
// The result keeps di.Index and may already have closed at height, so callers
// should rely only on Open, PeriodStart and Index.
func DeadlineNotAfter(di *dline.Info, height abi.ChainEpoch) *dline.Info {
	for di.Open > height {
		di = dline.NewInfo(di.PeriodStart-di.WPoStProvingPeriod, di.Index, height,
			di.WPoStPeriodDeadlines, di.WPoStProvingPeriod, di.WPoStChallengeWindow,
			di.WPoStChallengeLookback, di.FaultDeclarationCutoff)
	}
	return di
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

// TipsetAtOrAfter skips null epochs and returns the first tipset at or after target.
func TipsetAtOrAfter(ctx context.Context, t *testing.T, node api.FullNode, target abi.ChainEpoch) *types.TipSet {
	t.Helper()
	req := require.New(t)
	head, err := node.ChainHead(ctx)
	req.NoError(err)
	req.GreaterOrEqual(head.Height(), target, "chain head has not reached target epoch")
	for height := target; height <= head.Height(); height++ {
		ts, err := node.ChainGetTipSetByHeight(ctx, height, head.Key())
		req.NoError(err)
		if ts.Height() >= target {
			return ts
		}
	}
	req.FailNow("no non-null tipset after target", "target epoch %d through head %d", target, head.Height())
	return nil
}

func RequireMessageSuccess(t *testing.T, lookup *api.MsgLookup) {
	t.Helper()
	require.True(t, lookup.Receipt.ExitCode.IsSuccess(), lookup.Receipt.ExitCode.String())
}
