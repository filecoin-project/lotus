package events

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/ipfs/go-cid"
	"github.com/multiformats/go-multihash"
	"github.com/stretchr/testify/require"
	"gotest.tools/assert"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/crypto"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/actors/policy"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
)

var dummyCid cid.Cid

func init() {
	dummyCid, _ = cid.Parse("bafkqaaa")
}

type fakeMsg struct {
	bmsgs []*types.Message
	smsgs []*types.SignedMessage
}

type fakeCS struct {
	t   *testing.T
	h   abi.ChainEpoch
	tsc *tipSetCache

	msgs    map[cid.Cid]fakeMsg
	blkMsgs map[cid.Cid]cid.Cid

	tipsets map[types.TipSetKey]*types.TipSet

	mu         sync.Mutex
	waitSub    chan struct{}
	subCh      chan<- []*api.HeadChange
	callNumber map[string]int
}

func newFakeCS(t *testing.T) *fakeCS {
	fcs := &fakeCS{
		t:          t,
		h:          1,
		msgs:       make(map[cid.Cid]fakeMsg),
		blkMsgs:    make(map[cid.Cid]cid.Cid),
		tipsets:    make(map[types.TipSetKey]*types.TipSet),
		tsc:        newTSCache(nil, 2*policy.ChainFinality),
		callNumber: map[string]int{},
		waitSub:    make(chan struct{}),
	}
	require.NoError(t, fcs.tsc.add(fcs.makeTs(t, nil, 1, dummyCid)))
	return fcs
}

func (fcs *fakeCS) ChainHead(ctx context.Context) (*types.TipSet, error) {
	fcs.mu.Lock()
	defer fcs.mu.Unlock()
	fcs.callNumber["ChainHead"] = fcs.callNumber["ChainHead"] + 1
	panic("implement me")
}

func (fcs *fakeCS) ChainGetPath(ctx context.Context, from, to types.TipSetKey) ([]*api.HeadChange, error) {
	fcs.mu.Lock()
	fcs.callNumber["ChainGetPath"] = fcs.callNumber["ChainGetPath"] + 1
	fcs.mu.Unlock()

	fromTs, err := fcs.ChainGetTipSet(ctx, from)
	if err != nil {
		return nil, err
	}

	toTs, err := fcs.ChainGetTipSet(ctx, to)
	if err != nil {
		return nil, err
	}

	// copied from the chainstore
	revert, apply, err := store.ReorgOps(ctx, func(ctx context.Context, tsk types.TipSetKey) (*types.TipSet, error) {
		return fcs.ChainGetTipSet(ctx, tsk)
	}, fromTs, toTs)
	if err != nil {
		return nil, err
	}

	path := make([]*api.HeadChange, len(revert)+len(apply))
	for i, r := range revert {
		path[i] = &api.HeadChange{Type: store.HCRevert, Val: r}
	}
	for j, i := 0, len(apply)-1; i >= 0; j, i = j+1, i-1 {
		path[j+len(revert)] = &api.HeadChange{Type: store.HCApply, Val: apply[i]}
	}
	return path, nil
}

func (fcs *fakeCS) ChainGetTipSet(ctx context.Context, key types.TipSetKey) (*types.TipSet, error) {
	fcs.mu.Lock()
	defer fcs.mu.Unlock()
	fcs.callNumber["ChainGetTipSet"] = fcs.callNumber["ChainGetTipSet"] + 1
	return fcs.tipsets[key], nil
}

func (fcs *fakeCS) StateSearchMsg(ctx context.Context, from types.TipSetKey, msg cid.Cid, limit abi.ChainEpoch, allowReplaced bool) (*api.MsgLookup, error) {
	fcs.mu.Lock()
	defer fcs.mu.Unlock()
	fcs.callNumber["StateSearchMsg"] = fcs.callNumber["StateSearchMsg"] + 1
	return nil, nil
}

func (fcs *fakeCS) StateGetActor(ctx context.Context, actor address.Address, tsk types.TipSetKey) (*types.Actor, error) {
	fcs.mu.Lock()
	defer fcs.mu.Unlock()
	fcs.callNumber["StateGetActor"] = fcs.callNumber["StateGetActor"] + 1
	panic("Not Implemented")
}

func (fcs *fakeCS) ChainGetTipSetByHeight(context.Context, abi.ChainEpoch, types.TipSetKey) (*types.TipSet, error) {
	fcs.mu.Lock()
	defer fcs.mu.Unlock()
	fcs.callNumber["ChainGetTipSetByHeight"] = fcs.callNumber["ChainGetTipSetByHeight"] + 1
	panic("Not Implemented")
}
func (fcs *fakeCS) ChainGetTipSetAfterHeight(context.Context, abi.ChainEpoch, types.TipSetKey) (*types.TipSet, error) {
	fcs.mu.Lock()
	defer fcs.mu.Unlock()
	fcs.callNumber["ChainGetTipSetAfterHeight"] = fcs.callNumber["ChainGetTipSetAfterHeight"] + 1
	panic("Not Implemented")
}

func (fcs *fakeCS) makeTs(t *testing.T, parents []cid.Cid, h abi.ChainEpoch, msgcid cid.Cid) *types.TipSet {
	a, _ := address.NewFromString("t00")
	b, _ := address.NewFromString("t02")
	var ts, err = types.NewTipSet([]*types.BlockHeader{
		{
			Height: h,
			Miner:  a,

			Parents: parents,

			Ticket: &types.Ticket{VRFProof: []byte{byte(h % 2)}},

			ParentStateRoot:       dummyCid,
			Messages:              msgcid,
			ParentMessageReceipts: dummyCid,

			BlockSig:     &crypto.Signature{Type: crypto.SigTypeBLS},
			BLSAggregate: &crypto.Signature{Type: crypto.SigTypeBLS},
		},
		{
			Height: h,
			Miner:  b,

			Parents: parents,

			Ticket: &types.Ticket{VRFProof: []byte{byte((h + 1) % 2)}},

			ParentStateRoot:       dummyCid,
			Messages:              msgcid,
			ParentMessageReceipts: dummyCid,

			BlockSig:     &crypto.Signature{Type: crypto.SigTypeBLS},
			BLSAggregate: &crypto.Signature{Type: crypto.SigTypeBLS},
		},
	})

	require.NoError(t, err)

	fcs.mu.Lock()
	defer fcs.mu.Unlock()

	if fcs.tipsets == nil {
		fcs.tipsets = map[types.TipSetKey]*types.TipSet{}
	}
	fcs.tipsets[ts.Key()] = ts

	return ts
}

func (fcs *fakeCS) ChainNotify(ctx context.Context) (<-chan []*api.HeadChange, error) {
	fcs.mu.Lock()
	defer fcs.mu.Unlock()
	fcs.callNumber["ChainNotify"] = fcs.callNumber["ChainNotify"] + 1

	out := make(chan []*api.HeadChange, 1)
	if fcs.subCh != nil {
		close(out)
		fcs.t.Error("already subscribed to notifications")
		return out, nil
	}

	best, err := fcs.tsc.ChainHead(ctx)
	if err != nil {
		return nil, err
	}

	out <- []*api.HeadChange{{Type: store.HCCurrent, Val: best}}
	fcs.subCh = out
	close(fcs.waitSub)

	return out, nil
}

func (fcs *fakeCS) ChainGetBlockMessages(ctx context.Context, blk cid.Cid) (*api.BlockMessages, error) {
	fcs.mu.Lock()
	defer fcs.mu.Unlock()
	fcs.callNumber["ChainGetBlockMessages"] = fcs.callNumber["ChainGetBlockMessages"] + 1
	messages, ok := fcs.blkMsgs[blk]
	if !ok {
		return &api.BlockMessages{}, nil
	}

	ms, ok := fcs.msgs[messages]
	if !ok {
		return &api.BlockMessages{}, nil
	}

	cids := make([]cid.Cid, len(ms.bmsgs)+len(ms.smsgs))
	for i, m := range ms.bmsgs {
		cids[i] = m.Cid()
	}
	for i, m := range ms.smsgs {
		cids[i+len(ms.bmsgs)] = m.Cid()
	}

	return &api.BlockMessages{BlsMessages: ms.bmsgs, SecpkMessages: ms.smsgs, Cids: cids}, nil
}

func (fcs *fakeCS) fakeMsgs(m fakeMsg) cid.Cid {
	n := len(fcs.msgs)
	c, err := cid.Prefix{
		Version:  1,
		Codec:    cid.Raw,
		MhType:   multihash.IDENTITY,
		MhLength: -1,
	}.Sum([]byte(fmt.Sprintf("%d", n)))
	require.NoError(fcs.t, err)

	fcs.msgs[c] = m
	return c
}

func (fcs *fakeCS) dropSub() {
	fcs.mu.Lock()

	if fcs.subCh == nil {
		fcs.mu.Unlock()
		fcs.t.Fatal("sub not be nil")
	}

	waitCh := make(chan struct{})
	fcs.waitSub = waitCh
	close(fcs.subCh)
	fcs.subCh = nil
	fcs.mu.Unlock()

	<-waitCh
}

func (fcs *fakeCS) sub(rev, app []*types.TipSet) {
	<-fcs.waitSub
	notif := make([]*api.HeadChange, len(rev)+len(app))

	for i, r := range rev {
		notif[i] = &api.HeadChange{
			Type: store.HCRevert,
			Val:  r,
		}
	}
	for i, r := range app {
		notif[i+len(rev)] = &api.HeadChange{
			Type: store.HCApply,
			Val:  r,
		}
	}

	fcs.subCh <- notif
}

func (fcs *fakeCS) advance(rev, app, drop int, msgs map[int]cid.Cid, nulls ...int) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	nullm := map[int]struct{}{}
	for _, v := range nulls {
		nullm[v] = struct{}{}
	}

	var revs []*types.TipSet
	for i := 0; i < rev; i++ {
		fcs.t.Log("revert", fcs.h)
		from, err := fcs.tsc.ChainHead(ctx)
		require.NoError(fcs.t, err)

		if _, ok := nullm[int(from.Height())]; !ok {
			require.NoError(fcs.t, fcs.tsc.revert(from))

			if drop == 0 {
				revs = append(revs, from)
			}
		}
		if drop > 0 {
			drop--
			if drop == 0 {
				fcs.dropSub()
			}
		}
		fcs.h--
	}

	var apps []*types.TipSet
	for i := 0; i < app; i++ {
		fcs.h++
		fcs.t.Log("apply", fcs.h)

		mc, hasMsgs := msgs[i]
		if !hasMsgs {
			mc = dummyCid
		}

		if _, ok := nullm[int(fcs.h)]; !ok {
			best, err := fcs.tsc.ChainHead(ctx)
			require.NoError(fcs.t, err)
			ts := fcs.makeTs(fcs.t, best.Key().Cids(), fcs.h, mc)
			require.NoError(fcs.t, fcs.tsc.add(ts))

			if hasMsgs {
				fcs.blkMsgs[ts.Blocks()[0].Cid()] = mc
			}

			if drop == 0 {
				apps = append(apps, ts)
			}
		}

		if drop > 0 {
			drop--
			if drop == 0 {
				fcs.dropSub()
			}
		}
	}

	fcs.sub(revs, apps)

	// Wait for the last round to finish.
	fcs.sub(nil, nil)
	fcs.sub(nil, nil)
}

var _ EventHelperAPI = &fakeCS{}

func TestAt(t *testing.T) {
	fcs := newFakeCS(t)
	events, err := NewEvents(context.Background(), fcs)
	require.NoError(t, err)

	var applied bool
	var reverted bool

	err = events.ChainAt(context.Background(), func(_ context.Context, ts *types.TipSet, curH abi.ChainEpoch) error {
		require.Equal(t, 5, int(ts.Height()))
		require.Equal(t, 8, int(curH))
		applied = true
		return nil
	}, func(_ context.Context, ts *types.TipSet) error {
		reverted = true
		return nil
	}, 3, 5)
	require.NoError(t, err)

	fcs.advance(0, 3, 0, nil)
	require.Equal(t, false, applied)
	require.Equal(t, false, reverted)

	fcs.advance(0, 3, 0, nil)
	require.Equal(t, false, applied)
	require.Equal(t, false, reverted)

	fcs.advance(0, 3, 0, nil)
	require.Equal(t, true, applied)
	require.Equal(t, false, reverted)
	applied = false

	fcs.advance(0, 3, 0, nil)
	require.Equal(t, false, applied)
	require.Equal(t, false, reverted)

	fcs.advance(10, 10, 0, nil)
	require.Equal(t, true, applied)
	require.Equal(t, true, reverted)
	applied = false
	reverted = false

	fcs.advance(10, 1, 0, nil)
	require.Equal(t, false, applied)
	require.Equal(t, true, reverted)
	reverted = false

	fcs.advance(0, 1, 0, nil)
	require.Equal(t, false, applied)
	require.Equal(t, false, reverted)

	fcs.advance(0, 2, 0, nil)
	require.Equal(t, false, applied)
	require.Equal(t, false, reverted)

	fcs.advance(0, 1, 0, nil) // 8
	require.Equal(t, true, applied)
	require.Equal(t, false, reverted)
}

func TestAtNullTrigger(t *testing.T) {
	fcs := newFakeCS(t)
	events, err := NewEvents(context.Background(), fcs)
	require.NoError(t, err)

	var applied bool
	var reverted bool

	err = events.ChainAt(context.Background(), func(_ context.Context, ts *types.TipSet, curH abi.ChainEpoch) error {
		require.Equal(t, abi.ChainEpoch(6), ts.Height())
		require.Equal(t, 8, int(curH))
		applied = true
		return nil
	}, func(_ context.Context, ts *types.TipSet) error {
		reverted = true
		return nil
	}, 3, 5)
	require.NoError(t, err)

	fcs.advance(0, 6, 0, nil, 5)
	require.Equal(t, false, applied)
	require.Equal(t, false, reverted)

	fcs.advance(0, 3, 0, nil)
	require.Equal(t, true, applied)
	require.Equal(t, false, reverted)
	applied = false
}

func TestAtNullConf(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	fcs := newFakeCS(t)

	events, err := NewEvents(ctx, fcs)
	require.NoError(t, err)

	var applied bool
	var reverted bool

	err = events.ChainAt(ctx, func(_ context.Context, ts *types.TipSet, curH abi.ChainEpoch) error {
		require.Equal(t, 5, int(ts.Height()))
		require.Equal(t, 8, int(curH))
		applied = true
		return nil
	}, func(_ context.Context, ts *types.TipSet) error {
		reverted = true
		return nil
	}, 3, 5)
	require.NoError(t, err)

	fcs.advance(0, 6, 0, nil)
	require.Equal(t, false, applied)
	require.Equal(t, false, reverted)

	fcs.advance(0, 3, 0, nil, 8)
	require.Equal(t, true, applied)
	require.Equal(t, false, reverted)
	applied = false

	fcs.advance(7, 1, 0, nil)
	require.Equal(t, false, applied)
	require.Equal(t, true, reverted)
	reverted = false
}

func TestAtStart(t *testing.T) {
	fcs := newFakeCS(t)

	events, err := NewEvents(context.Background(), fcs)
	require.NoError(t, err)

	fcs.advance(0, 5, 0, nil) // 6

	var applied bool
	var reverted bool

	err = events.ChainAt(context.Background(), func(_ context.Context, ts *types.TipSet, curH abi.ChainEpoch) error {
		require.Equal(t, 5, int(ts.Height()))
		require.Equal(t, 8, int(curH))
		applied = true
		return nil
	}, func(_ context.Context, ts *types.TipSet) error {
		reverted = true
		return nil
	}, 3, 5)
	require.NoError(t, err)

	require.Equal(t, false, applied)
	require.Equal(t, false, reverted)

	fcs.advance(0, 5, 0, nil) // 11
	require.Equal(t, true, applied)
	require.Equal(t, false, reverted)
}

func TestAtStartConfidence(t *testing.T) {
	fcs := newFakeCS(t)

	events, err := NewEvents(context.Background(), fcs)
	require.NoError(t, err)

	fcs.advance(0, 10, 0, nil) // 11

	var applied bool
	var reverted bool

	err = events.ChainAt(context.Background(), func(_ context.Context, ts *types.TipSet, curH abi.ChainEpoch) error {
		require.Equal(t, 5, int(ts.Height()))
		require.Equal(t, 11, int(curH))
		applied = true
		return nil
	}, func(_ context.Context, ts *types.TipSet) error {
		reverted = true
		return nil
	}, 3, 5)
	require.NoError(t, err)

	require.Equal(t, true, applied)
	require.Equal(t, false, reverted)
}

func TestAtChained(t *testing.T) {
	fcs := newFakeCS(t)

	events, err := NewEvents(context.Background(), fcs)
	require.NoError(t, err)

	var applied bool
	var reverted bool

	err = events.ChainAt(context.Background(), func(_ context.Context, ts *types.TipSet, curH abi.ChainEpoch) error {
		return events.ChainAt(context.Background(), func(_ context.Context, ts *types.TipSet, curH abi.ChainEpoch) error {
			require.Equal(t, 10, int(ts.Height()))
			applied = true
			return nil
		}, func(_ context.Context, ts *types.TipSet) error {
			reverted = true
			return nil
		}, 3, 10)
	}, func(_ context.Context, ts *types.TipSet) error {
		reverted = true
		return nil
	}, 3, 5)
	require.NoError(t, err)

	fcs.advance(0, 15, 0, nil)

	require.Equal(t, true, applied)
	require.Equal(t, false, reverted)
}

func TestAtChainedConfidence(t *testing.T) {
	fcs := newFakeCS(t)

	events, err := NewEvents(context.Background(), fcs)
	require.NoError(t, err)

	fcs.advance(0, 15, 0, nil)

	var applied bool
	var reverted bool

	err = events.ChainAt(context.Background(), func(_ context.Context, ts *types.TipSet, curH abi.ChainEpoch) error {
		return events.ChainAt(context.Background(), func(_ context.Context, ts *types.TipSet, curH abi.ChainEpoch) error {
			require.Equal(t, 10, int(ts.Height()))
			applied = true
			return nil
		}, func(_ context.Context, ts *types.TipSet) error {
			reverted = true
			return nil
		}, 3, 10)
	}, func(_ context.Context, ts *types.TipSet) error {
		reverted = true
		return nil
	}, 3, 5)
	require.NoError(t, err)

	require.Equal(t, true, applied)
	require.Equal(t, false, reverted)
}

func TestAtChainedConfidenceNull(t *testing.T) {
	fcs := newFakeCS(t)

	events, err := NewEvents(context.Background(), fcs)
	require.NoError(t, err)

	fcs.advance(0, 15, 0, nil, 5)

	var applied bool
	var reverted bool

	err = events.ChainAt(context.Background(), func(_ context.Context, ts *types.TipSet, curH abi.ChainEpoch) error {
		applied = true
		require.Equal(t, 6, int(ts.Height()))
		return nil
	}, func(_ context.Context, ts *types.TipSet) error {
		reverted = true
		return nil
	}, 3, 5)
	require.NoError(t, err)

	require.Equal(t, true, applied)
	require.Equal(t, false, reverted)
}

func matchAddrMethod(to address.Address, m abi.MethodNum) func(msg *types.Message) (matched bool, err error) {
	return func(msg *types.Message) (matched bool, err error) {
		return to == msg.To && m == msg.Method, nil
	}
}

func TestCalled(t *testing.T) {
	fcs := newFakeCS(t)

	events, err := NewEvents(context.Background(), fcs)
	require.NoError(t, err)

	t0123, err := address.NewFromString("t0123")
	require.NoError(t, err)

	more := true
	var applied, reverted bool
	var appliedMsg *types.Message
	var appliedTs *types.TipSet
	var appliedH abi.ChainEpoch

	err = events.Called(context.Background(), func(ctx context.Context, ts *types.TipSet) (d bool, m bool, e error) {
		return false, true, nil
	}, func(msg *types.Message, rec *types.MessageReceipt, ts *types.TipSet, curH abi.ChainEpoch) (bool, error) {
		require.Equal(t, false, applied)
		applied = true
		appliedMsg = msg
		appliedTs = ts
		appliedH = curH
		return more, nil
	}, func(_ context.Context, ts *types.TipSet) error {
		reverted = true
		return nil
	}, 3, 20, matchAddrMethod(t0123, 5))
	require.NoError(t, err)

	// create few blocks to make sure nothing gets randomly called

	fcs.advance(0, 4, 0, nil) // H=5
	require.Equal(t, false, applied)
	require.Equal(t, false, reverted)

	// create blocks with message (but below confidence threshold)

	fcs.advance(0, 3, 0, map[int]cid.Cid{ // msg at H=6; H=8 (confidence=2)
		0: fcs.fakeMsgs(fakeMsg{
			bmsgs: []*types.Message{
				{To: t0123, From: t0123, Method: 5, Nonce: 1},
			},
		}),
	})

	require.Equal(t, false, applied)
	require.Equal(t, false, reverted)

	// create additional block so we are above confidence threshold

	fcs.advance(0, 2, 0, nil) // H=10 (confidence=3, apply)

	require.Equal(t, true, applied)
	require.Equal(t, false, reverted)
	applied = false

	// dip below confidence
	fcs.advance(2, 2, 0, nil) // H=10 (confidence=3, apply)

	require.Equal(t, false, applied)
	require.Equal(t, false, reverted)

	require.Equal(t, abi.ChainEpoch(7), appliedTs.Height())
	require.Equal(t, "bafkqaaa", appliedTs.Blocks()[0].Messages.String())
	require.Equal(t, abi.ChainEpoch(10), appliedH)
	require.Equal(t, t0123, appliedMsg.To)
	require.Equal(t, uint64(1), appliedMsg.Nonce)
	require.Equal(t, abi.MethodNum(5), appliedMsg.Method)

	// revert some blocks, keep the message

	fcs.advance(3, 1, 0, nil) // H=8 (confidence=1)
	require.Equal(t, false, applied)
	require.Equal(t, false, reverted)

	// revert the message

	fcs.advance(2, 1, 0, nil) // H=7, we reverted ts with the msg execution, but not the msg itself

	require.Equal(t, false, applied)
	require.Equal(t, true, reverted)
	reverted = false

	// send new message on different height

	n2msg := fcs.fakeMsgs(fakeMsg{
		bmsgs: []*types.Message{
			{To: t0123, From: t0123, Method: 5, Nonce: 2},
		},
	})

	fcs.advance(0, 3, 0, map[int]cid.Cid{ // (n2msg confidence=1)
		0: n2msg,
	})

	require.Equal(t, true, applied) // msg from H=7, which had reverted execution
	require.Equal(t, false, reverted)
	require.Equal(t, abi.ChainEpoch(10), appliedH)
	applied = false

	fcs.advance(0, 2, 0, nil) // (confidence=3)

	require.Equal(t, true, applied)
	require.Equal(t, false, reverted)
	applied = false

	require.Equal(t, abi.ChainEpoch(9), appliedTs.Height())
	require.Equal(t, "bafkqaaa", appliedTs.Blocks()[0].Messages.String())
	require.Equal(t, abi.ChainEpoch(12), appliedH)
	require.Equal(t, t0123, appliedMsg.To)
	require.Equal(t, uint64(2), appliedMsg.Nonce)
	require.Equal(t, abi.MethodNum(5), appliedMsg.Method)

	// revert and apply at different height

	fcs.advance(8, 6, 0, map[int]cid.Cid{ // (confidence=3)
		1: n2msg,
	})

	// TODO: We probably don't want to call revert/apply, as restarting certain
	//  actions may be expensive, and in this case the message is still
	//  on-chain, just at different height
	require.Equal(t, true, applied)
	require.Equal(t, true, reverted)
	reverted = false
	applied = false

	require.Equal(t, abi.ChainEpoch(7), appliedTs.Height())
	require.Equal(t, "bafkqaaa", appliedTs.Blocks()[0].Messages.String())
	require.Equal(t, abi.ChainEpoch(10), appliedH)
	require.Equal(t, t0123, appliedMsg.To)
	require.Equal(t, uint64(2), appliedMsg.Nonce)
	require.Equal(t, abi.MethodNum(5), appliedMsg.Method)

	// call method again

	fcs.advance(0, 5, 0, map[int]cid.Cid{
		0: n2msg,
	})

	require.Equal(t, true, applied)
	require.Equal(t, false, reverted)
	applied = false

	// send and revert below confidence, then cross confidence
	fcs.advance(0, 2, 0, map[int]cid.Cid{
		0: fcs.fakeMsgs(fakeMsg{
			bmsgs: []*types.Message{
				{To: t0123, From: t0123, Method: 5, Nonce: 3},
			},
		}),
	})

	fcs.advance(2, 5, 0, nil) // H=19, but message reverted

	require.Equal(t, false, applied)
	require.Equal(t, false, reverted)

	// test timeout (it's set to 20 in the call to `events.Called` above)

	fcs.advance(0, 6, 0, nil)

	require.Equal(t, false, applied) // not calling timeout as we received messages
	require.Equal(t, false, reverted)

	// test unregistering with more

	more = false
	fcs.advance(0, 5, 0, map[int]cid.Cid{
		0: fcs.fakeMsgs(fakeMsg{
			bmsgs: []*types.Message{
				{To: t0123, From: t0123, Method: 5, Nonce: 4}, // this signals we don't want more
			},
		}),
	})

	require.Equal(t, true, applied)
	require.Equal(t, false, reverted)
	applied = false

	fcs.advance(0, 5, 0, map[int]cid.Cid{
		0: fcs.fakeMsgs(fakeMsg{
			bmsgs: []*types.Message{
				{To: t0123, From: t0123, Method: 5, Nonce: 5},
			},
		}),
	})

	require.Equal(t, false, applied) // should not get any further notifications
	require.Equal(t, false, reverted)

	// revert after disabled

	fcs.advance(5, 1, 0, nil) // try reverting msg sent after disabling

	require.Equal(t, false, applied)
	require.Equal(t, false, reverted)

	fcs.advance(5, 1, 0, nil) // try reverting msg sent before disabling

	require.Equal(t, false, applied)
	require.Equal(t, true, reverted)
}

func TestCalledTimeout(t *testing.T) {
	fcs := newFakeCS(t)

	events, err := NewEvents(context.Background(), fcs)
	require.NoError(t, err)

	t0123, err := address.NewFromString("t0123")
	require.NoError(t, err)

	called := false

	err = events.Called(context.Background(), func(ctx context.Context, ts *types.TipSet) (d bool, m bool, e error) {
		return false, true, nil
	}, func(msg *types.Message, rec *types.MessageReceipt, ts *types.TipSet, curH abi.ChainEpoch) (bool, error) {
		called = true
		require.Nil(t, msg)
		require.Equal(t, abi.ChainEpoch(20), ts.Height())
		require.Equal(t, abi.ChainEpoch(23), curH)
		return false, nil
	}, func(_ context.Context, ts *types.TipSet) error {
		t.Fatal("revert on timeout")
		return nil
	}, 3, 20, matchAddrMethod(t0123, 5))
	require.NoError(t, err)

	fcs.advance(0, 21, 0, nil)
	require.False(t, called)

	fcs.advance(0, 5, 0, nil)
	require.True(t, called)
	called = false

	// with check func reporting done

	fcs = newFakeCS(t)

	events, err = NewEvents(context.Background(), fcs)
	require.NoError(t, err)

	err = events.Called(context.Background(), func(ctx context.Context, ts *types.TipSet) (d bool, m bool, e error) {
		return true, true, nil
	}, func(msg *types.Message, rec *types.MessageReceipt, ts *types.TipSet, curH abi.ChainEpoch) (bool, error) {
		called = true
		require.Nil(t, msg)
		require.Equal(t, abi.ChainEpoch(20), ts.Height())
		require.Equal(t, abi.ChainEpoch(23), curH)
		return false, nil
	}, func(_ context.Context, ts *types.TipSet) error {
		t.Fatal("revert on timeout")
		return nil
	}, 3, 20, matchAddrMethod(t0123, 5))
	require.NoError(t, err)

	fcs.advance(0, 21, 0, nil)
	require.False(t, called)

	fcs.advance(0, 5, 0, nil)
	require.False(t, called)
}

func TestCalledOrder(t *testing.T) {
	fcs := newFakeCS(t)

	events, err := NewEvents(context.Background(), fcs)
	require.NoError(t, err)

	t0123, err := address.NewFromString("t0123")
	require.NoError(t, err)

	at := 0

	err = events.Called(context.Background(), func(ctx context.Context, ts *types.TipSet) (d bool, m bool, e error) {
		return false, true, nil
	}, func(msg *types.Message, rec *types.MessageReceipt, ts *types.TipSet, curH abi.ChainEpoch) (bool, error) {
		switch at {
		case 0:
			require.Equal(t, uint64(1), msg.Nonce)
			require.Equal(t, abi.ChainEpoch(4), ts.Height())
		case 1:
			require.Equal(t, uint64(2), msg.Nonce)
			require.Equal(t, abi.ChainEpoch(5), ts.Height())
		default:
			t.Fatal("apply should only get called twice, at: ", at)
		}
		at++
		return true, nil
	}, func(_ context.Context, ts *types.TipSet) error {
		switch at {
		case 2:
			require.Equal(t, abi.ChainEpoch(5), ts.Height())
		case 3:
			require.Equal(t, abi.ChainEpoch(4), ts.Height())
		default:
			t.Fatal("revert should only get called twice, at: ", at)
		}
		at++
		return nil
	}, 3, 20, matchAddrMethod(t0123, 5))
	require.NoError(t, err)

	fcs.advance(0, 10, 0, map[int]cid.Cid{
		1: fcs.fakeMsgs(fakeMsg{
			bmsgs: []*types.Message{
				{To: t0123, From: t0123, Method: 5, Nonce: 1},
			},
		}),
		2: fcs.fakeMsgs(fakeMsg{
			bmsgs: []*types.Message{
				{To: t0123, From: t0123, Method: 5, Nonce: 2},
			},
		}),
	})

	fcs.advance(9, 1, 0, nil)
}

func TestCalledNull(t *testing.T) {
	fcs := newFakeCS(t)

	events, err := NewEvents(context.Background(), fcs)
	require.NoError(t, err)

	t0123, err := address.NewFromString("t0123")
	require.NoError(t, err)

	more := true
	var applied, reverted bool

	err = events.Called(context.Background(), func(ctx context.Context, ts *types.TipSet) (d bool, m bool, e error) {
		return false, true, nil
	}, func(msg *types.Message, rec *types.MessageReceipt, ts *types.TipSet, curH abi.ChainEpoch) (bool, error) {
		require.Equal(t, false, applied)
		applied = true
		return more, nil
	}, func(_ context.Context, ts *types.TipSet) error {
		reverted = true
		return nil
	}, 3, 20, matchAddrMethod(t0123, 5))
	require.NoError(t, err)

	// create few blocks to make sure nothing gets randomly called

	fcs.advance(0, 4, 0, nil) // H=5
	require.Equal(t, false, applied)
	require.Equal(t, false, reverted)

	// create blocks with message (but below confidence threshold)

	fcs.advance(0, 3, 0, map[int]cid.Cid{ // msg at H=6; H=8 (confidence=2)
		0: fcs.fakeMsgs(fakeMsg{
			bmsgs: []*types.Message{
				{To: t0123, From: t0123, Method: 5, Nonce: 1},
			},
		}),
	})

	require.Equal(t, false, applied)
	require.Equal(t, false, reverted)

	// create additional blocks so we are above confidence threshold, but with null tipset at the height
	// of application

	fcs.advance(0, 3, 0, nil, 10) // H=11 (confidence=3, apply)

	require.Equal(t, true, applied)
	require.Equal(t, false, reverted)
	applied = false

	fcs.advance(5, 1, 0, nil, 10)

	require.Equal(t, false, applied)
	require.Equal(t, true, reverted)
}

func TestRemoveTriggersOnMessage(t *testing.T) {
	fcs := newFakeCS(t)

	events, err := NewEvents(context.Background(), fcs)
	require.NoError(t, err)

	t0123, err := address.NewFromString("t0123")
	require.NoError(t, err)

	more := true
	var applied, reverted bool

	err = events.Called(context.Background(), func(ctx context.Context, ts *types.TipSet) (d bool, m bool, e error) {
		return false, true, nil
	}, func(msg *types.Message, rec *types.MessageReceipt, ts *types.TipSet, curH abi.ChainEpoch) (bool, error) {
		require.Equal(t, false, applied)
		applied = true
		return more, nil
	}, func(_ context.Context, ts *types.TipSet) error {
		reverted = true
		return nil
	}, 3, 20, matchAddrMethod(t0123, 5))
	require.NoError(t, err)

	// create few blocks to make sure nothing gets randomly called

	fcs.advance(0, 4, 0, nil) // H=5
	require.Equal(t, false, applied)
	require.Equal(t, false, reverted)

	// create blocks with message (but below confidence threshold)

	fcs.advance(0, 3, 0, map[int]cid.Cid{ // msg occurs at H=5, applied at H=6; H=8 (confidence=2)
		0: fcs.fakeMsgs(fakeMsg{
			bmsgs: []*types.Message{
				{To: t0123, From: t0123, Method: 5, Nonce: 1},
			},
		}),
	})

	require.Equal(t, false, applied)
	require.Equal(t, false, reverted)

	// revert applied TS & message TS
	fcs.advance(3, 1, 0, nil) // H=6 (tipset message applied in reverted, AND message reverted)
	require.Equal(t, false, applied)
	require.Equal(t, false, reverted)

	// create additional blocks so we are above confidence threshold, but message not applied
	// as it was reverted
	fcs.advance(0, 5, 0, nil) // H=11 (confidence=3, apply)
	require.Equal(t, false, applied)
	require.Equal(t, false, reverted)

	// create blocks with message again (but below confidence threshold)

	fcs.advance(0, 3, 0, map[int]cid.Cid{ // msg occurs at H=12, applied at H=13; H=15 (confidence=2)
		0: fcs.fakeMsgs(fakeMsg{
			bmsgs: []*types.Message{
				{To: t0123, From: t0123, Method: 5, Nonce: 2},
			},
		}),
	})
	require.Equal(t, false, applied)
	require.Equal(t, false, reverted)

	// revert applied height TS, but don't remove message trigger
	fcs.advance(2, 1, 0, nil) // H=13 (tipset message applied in reverted, by tipset with message not reverted)
	require.Equal(t, false, applied)
	require.Equal(t, false, reverted)

	// create additional blocks so we are above confidence threshold
	fcs.advance(0, 4, 0, nil) // H=18 (confidence=3, apply)

	require.Equal(t, true, applied)
	require.Equal(t, false, reverted)
}

type testStateChange struct {
	from string
	to   string
}

func TestStateChanged(t *testing.T) {
	fcs := newFakeCS(t)

	events, err := NewEvents(context.Background(), fcs)
	require.NoError(t, err)

	more := true
	var applied, reverted bool
	var appliedData StateChange
	var appliedOldTs *types.TipSet
	var appliedNewTs *types.TipSet
	var appliedH abi.ChainEpoch
	var matchData StateChange

	confidence := 3
	timeout := abi.ChainEpoch(20)

	err = events.StateChanged(func(ctx context.Context, ts *types.TipSet) (d bool, m bool, e error) {
		return false, true, nil
	}, func(oldTs, newTs *types.TipSet, data StateChange, curH abi.ChainEpoch) (bool, error) {
		if data != nil {
			require.Equal(t, oldTs.Key(), newTs.Parents())
		}
		require.Equal(t, false, applied)
		applied = true
		appliedData = data
		appliedOldTs = oldTs
		appliedNewTs = newTs
		appliedH = curH
		return more, nil
	}, func(_ context.Context, ts *types.TipSet) error {
		reverted = true
		return nil
	}, confidence, timeout, func(oldTs, newTs *types.TipSet) (bool, StateChange, error) {
		require.Equal(t, oldTs.Key(), newTs.Parents())
		if matchData == nil {
			return false, matchData, nil
		}

		d := matchData
		matchData = nil
		return true, d, nil
	})
	require.NoError(t, err)

	// create few blocks to make sure nothing gets randomly called

	fcs.advance(0, 4, 0, nil) // H=5
	require.Equal(t, false, applied)
	require.Equal(t, false, reverted)

	// create state change (but below confidence threshold)
	matchData = testStateChange{from: "a", to: "b"}
	fcs.advance(0, 3, 0, nil)

	require.Equal(t, false, applied)
	require.Equal(t, false, reverted)

	// create additional block so we are above confidence threshold

	fcs.advance(0, 2, 0, nil) // H=10 (confidence=3, apply)

	require.Equal(t, true, applied)
	require.Equal(t, false, reverted)
	applied = false

	// dip below confidence (should not apply again)
	fcs.advance(2, 2, 0, nil) // H=10 (confidence=3, apply)

	require.Equal(t, false, applied)
	require.Equal(t, false, reverted)

	// Change happens from 5 -> 6
	require.Equal(t, abi.ChainEpoch(5), appliedOldTs.Height())
	require.Equal(t, abi.ChainEpoch(6), appliedNewTs.Height())

	// Actually applied (with confidence) at 9
	require.Equal(t, abi.ChainEpoch(9), appliedH)

	// Make sure the state change was correctly passed through
	rcvd := appliedData.(testStateChange)
	require.Equal(t, "a", rcvd.from)
	require.Equal(t, "b", rcvd.to)
}

func TestStateChangedRevert(t *testing.T) {
	fcs := newFakeCS(t)

	events, err := NewEvents(context.Background(), fcs)
	require.NoError(t, err)

	more := true
	var applied, reverted bool
	var matchData StateChange

	confidence := 1
	timeout := abi.ChainEpoch(20)

	err = events.StateChanged(func(ctx context.Context, ts *types.TipSet) (d bool, m bool, e error) {
		return false, true, nil
	}, func(oldTs, newTs *types.TipSet, data StateChange, curH abi.ChainEpoch) (bool, error) {
		if data != nil {
			require.Equal(t, oldTs.Key(), newTs.Parents())
		}
		require.Equal(t, false, applied)
		applied = true
		return more, nil
	}, func(_ context.Context, ts *types.TipSet) error {
		reverted = true
		return nil
	}, confidence, timeout, func(oldTs, newTs *types.TipSet) (bool, StateChange, error) {
		require.Equal(t, oldTs.Key(), newTs.Parents())

		if matchData == nil {
			return false, matchData, nil
		}

		d := matchData
		matchData = nil
		return true, d, nil
	})
	require.NoError(t, err)

	fcs.advance(0, 2, 0, nil) // H=3

	// Make a state change from TS at height 3 to TS at height 4
	matchData = testStateChange{from: "a", to: "b"}
	fcs.advance(0, 1, 0, nil) // H=4

	// Haven't yet reached confidence
	require.Equal(t, false, applied)
	require.Equal(t, false, reverted)

	// Advance to reach confidence level
	fcs.advance(0, 1, 0, nil) // H=5

	// Should now have called the handler
	require.Equal(t, true, applied)
	require.Equal(t, false, reverted)
	applied = false

	// Advance 3 more TS
	fcs.advance(0, 3, 0, nil) // H=8

	require.Equal(t, false, applied)
	require.Equal(t, false, reverted)

	// Regress but not so far as to cause a revert
	fcs.advance(3, 1, 0, nil) // H=6

	require.Equal(t, false, applied)
	require.Equal(t, false, reverted)

	// Regress back to state where change happened
	fcs.advance(3, 1, 0, nil) // H=4

	// Expect revert to have happened
	require.Equal(t, false, applied)
	require.Equal(t, true, reverted)
}

func TestStateChangedTimeout(t *testing.T) {
	timeoutHeight := abi.ChainEpoch(20)
	confidence := 3

	testCases := []struct {
		name          string
		checkFn       CheckFunc
		nilBlocks     []int
		expectTimeout bool
	}{{
		// Verify that the state changed timeout is called at the expected height
		name: "state changed timeout",
		checkFn: func(ctx context.Context, ts *types.TipSet) (d bool, m bool, e error) {
			return false, true, nil
		},
		expectTimeout: true,
	}, {
		// Verify that the state changed timeout is called even if the timeout
		// falls on nil block
		name: "state changed timeout falls on nil block",
		checkFn: func(ctx context.Context, ts *types.TipSet) (d bool, m bool, e error) {
			return false, true, nil
		},
		nilBlocks:     []int{20, 21, 22, 23},
		expectTimeout: true,
	}, {
		// Verify that the state changed timeout is not called if the check
		// function reports that it's complete
		name: "no timeout callback if check func reports done",
		checkFn: func(ctx context.Context, ts *types.TipSet) (d bool, m bool, e error) {
			return true, true, nil
		},
		expectTimeout: false,
	}}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			fcs := newFakeCS(t)

			events, err := NewEvents(context.Background(), fcs)
			require.NoError(t, err)

			// Track whether the callback was called
			called := false

			// Set up state change tracking that will timeout at the given height
			err = events.StateChanged(
				tc.checkFn,
				func(oldTs, newTs *types.TipSet, data StateChange, curH abi.ChainEpoch) (bool, error) {
					// Expect the callback to be called at the timeout height with nil data
					called = true
					require.Nil(t, data)
					require.Equal(t, timeoutHeight, newTs.Height())
					require.Equal(t, timeoutHeight+abi.ChainEpoch(confidence), curH)
					return false, nil
				}, func(_ context.Context, ts *types.TipSet) error {
					t.Fatal("revert on timeout")
					return nil
				}, confidence, timeoutHeight, func(oldTs, newTs *types.TipSet) (bool, StateChange, error) {
					return false, nil, nil
				})

			require.NoError(t, err)

			// Advance to timeout height
			fcs.advance(0, int(timeoutHeight)+1, 0, nil)
			require.False(t, called)

			// Advance past timeout height
			fcs.advance(0, 5, 0, nil, tc.nilBlocks...)
			require.Equal(t, tc.expectTimeout, called)
			called = false
		})
	}
}

func TestCalledMultiplePerEpoch(t *testing.T) {
	fcs := newFakeCS(t)

	events, err := NewEvents(context.Background(), fcs)
	require.NoError(t, err)

	t0123, err := address.NewFromString("t0123")
	require.NoError(t, err)

	at := 0

	err = events.Called(context.Background(), func(ctx context.Context, ts *types.TipSet) (d bool, m bool, e error) {
		return false, true, nil
	}, func(msg *types.Message, rec *types.MessageReceipt, ts *types.TipSet, curH abi.ChainEpoch) (bool, error) {
		switch at {
		case 0:
			require.Equal(t, uint64(1), msg.Nonce)
			require.Equal(t, abi.ChainEpoch(4), ts.Height())
		case 1:
			require.Equal(t, uint64(2), msg.Nonce)
			require.Equal(t, abi.ChainEpoch(4), ts.Height())
		default:
			t.Fatal("apply should only get called twice, at: ", at)
		}
		at++
		return true, nil
	}, func(_ context.Context, ts *types.TipSet) error {
		switch at {
		case 2:
			require.Equal(t, abi.ChainEpoch(4), ts.Height())
		case 3:
			require.Equal(t, abi.ChainEpoch(4), ts.Height())
		default:
			t.Fatal("revert should only get called twice, at: ", at)
		}
		at++
		return nil
	}, 3, 20, matchAddrMethod(t0123, 5))
	require.NoError(t, err)

	fcs.advance(0, 10, 0, map[int]cid.Cid{
		1: fcs.fakeMsgs(fakeMsg{
			bmsgs: []*types.Message{
				{To: t0123, From: t0123, Method: 5, Nonce: 1},
				{To: t0123, From: t0123, Method: 5, Nonce: 2},
			},
		}),
	})

	fcs.advance(9, 1, 0, nil)
}

func TestCachedSameBlock(t *testing.T) {
	fcs := newFakeCS(t)

	_, err := NewEvents(context.Background(), fcs)
	require.NoError(t, err)

	fcs.advance(0, 10, 0, map[int]cid.Cid{})
	assert.Assert(t, fcs.callNumber["ChainGetBlockMessages"] == 20, "expect call ChainGetBlockMessages %d but got ", 20, fcs.callNumber["ChainGetBlockMessages"])

	fcs.advance(5, 10, 0, map[int]cid.Cid{})
	assert.Assert(t, fcs.callNumber["ChainGetBlockMessages"] == 30, "expect call ChainGetBlockMessages %d but got ", 30, fcs.callNumber["ChainGetBlockMessages"])
}

type testObserver struct {
	t    *testing.T
	head *types.TipSet
}

func (t *testObserver) Apply(_ context.Context, from, to *types.TipSet) error {
	if t.head != nil {
		require.True(t.t, t.head.Equals(from))
	}
	t.head = to
	return nil
}

func (t *testObserver) Revert(_ context.Context, from, to *types.TipSet) error {
	if t.head != nil {
		require.True(t.t, t.head.Equals(from))
	}
	t.head = to
	return nil
}

func TestReconnect(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	fcs := newFakeCS(t)

	events, err := NewEvents(ctx, fcs)
	require.NoError(t, err)

	fcs.advance(0, 1, 0, nil)

	events.Observe(&testObserver{t: t})

	fcs.advance(0, 3, 0, nil)

	// Drop on apply
	fcs.advance(0, 6, 2, nil)
	require.True(t, fcs.callNumber["ChainGetPath"] == 1)

	// drop across revert/apply boundary
	fcs.advance(4, 2, 3, nil)
	require.True(t, fcs.callNumber["ChainGetPath"] == 2)
	fcs.advance(0, 6, 0, nil)

	// drop on revert
	fcs.advance(3, 0, 2, nil)
	require.True(t, fcs.callNumber["ChainGetPath"] == 3)

	// drop with nulls
	fcs.advance(0, 5, 2, nil, 0, 1, 3)
	require.True(t, fcs.callNumber["ChainGetPath"] == 4)
}

func TestUnregister(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	fcs := newFakeCS(t)

	events, err := NewEvents(ctx, fcs)
	require.NoError(t, err)

	tsObs := &testObserver{t: t}
	events.Observe(tsObs)

	// observer receives heads as the chain advances
	fcs.advance(0, 1, 0, nil)
	headBeforeDeregister := events.lastTs
	require.Equal(t, tsObs.head, headBeforeDeregister)

	// observer unregistered successfully
	found := events.Unregister(tsObs)
	require.True(t, found)

	// observer stops receiving heads as the chain advances
	fcs.advance(0, 1, 0, nil)
	require.Equal(t, tsObs.head, headBeforeDeregister)
	require.NotEqual(t, tsObs.head, events.lastTs)

	// unregistering an invalid observer returns false
	dneObs := &testObserver{t: t}
	found = events.Unregister(dneObs)
	require.False(t, found)
}
