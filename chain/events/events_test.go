package events

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/crypto"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/store"

	"github.com/ipfs/go-cid"
	"github.com/multiformats/go-multihash"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/build"
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

	sub func(rev, app []*types.TipSet)
}

func (fcs *fakeCS) StateGetReceipt(context.Context, cid.Cid, types.TipSetKey) (*types.MessageReceipt, error) {
	return nil, nil
}

func (fcs *fakeCS) StateGetActor(ctx context.Context, actor address.Address, tsk types.TipSetKey) (*types.Actor, error) {
	panic("Not Implemented")
}

func (fcs *fakeCS) ChainGetTipSetByHeight(context.Context, abi.ChainEpoch, types.TipSetKey) (*types.TipSet, error) {
	panic("Not Implemented")
}

func makeTs(t *testing.T, h abi.ChainEpoch, msgcid cid.Cid) *types.TipSet {
	a, _ := address.NewFromString("t00")
	b, _ := address.NewFromString("t02")
	var ts, err = types.NewTipSet([]*types.BlockHeader{
		{
			Height: h,
			Miner:  a,

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

			Ticket: &types.Ticket{VRFProof: []byte{byte((h + 1) % 2)}},

			ParentStateRoot:       dummyCid,
			Messages:              msgcid,
			ParentMessageReceipts: dummyCid,

			BlockSig:     &crypto.Signature{Type: crypto.SigTypeBLS},
			BLSAggregate: &crypto.Signature{Type: crypto.SigTypeBLS},
		},
	})

	require.NoError(t, err)

	return ts
}

func (fcs *fakeCS) ChainNotify(context.Context) (<-chan []*api.HeadChange, error) {
	out := make(chan []*api.HeadChange, 1)
	out <- []*api.HeadChange{{Type: store.HCCurrent, Val: fcs.tsc.best()}}

	fcs.sub = func(rev, app []*types.TipSet) {
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

		out <- notif
	}

	return out, nil
}

func (fcs *fakeCS) ChainGetBlockMessages(ctx context.Context, blk cid.Cid) (*api.BlockMessages, error) {
	messages, ok := fcs.blkMsgs[blk]
	if !ok {
		return &api.BlockMessages{}, nil
	}

	ms, ok := fcs.msgs[messages]
	if !ok {
		return &api.BlockMessages{}, nil
	}
	return &api.BlockMessages{BlsMessages: ms.bmsgs, SecpkMessages: ms.smsgs}, nil

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

func (fcs *fakeCS) advance(rev, app int, msgs map[int]cid.Cid, nulls ...int) { // todo: allow msgs
	if fcs.sub == nil {
		fcs.t.Fatal("sub not be nil")
	}

	nullm := map[int]struct{}{}
	for _, v := range nulls {
		nullm[v] = struct{}{}
	}

	var revs []*types.TipSet
	for i := 0; i < rev; i++ {
		ts := fcs.tsc.best()

		if _, ok := nullm[int(ts.Height())]; !ok {
			revs = append(revs, ts)
			require.NoError(fcs.t, fcs.tsc.revert(ts))
		}
		fcs.h--
	}

	var apps []*types.TipSet
	for i := 0; i < app; i++ {
		fcs.h++

		mc, hasMsgs := msgs[i]
		if !hasMsgs {
			mc = dummyCid
		}

		if _, ok := nullm[int(fcs.h)]; ok {
			continue
		}

		ts := makeTs(fcs.t, fcs.h, mc)
		require.NoError(fcs.t, fcs.tsc.add(ts))

		if hasMsgs {
			fcs.blkMsgs[ts.Blocks()[0].Cid()] = mc
		}

		apps = append(apps, ts)
	}

	fcs.sub(revs, apps)
	time.Sleep(100 * time.Millisecond) // TODO: :c
}

var _ eventApi = &fakeCS{}

func TestAt(t *testing.T) {
	fcs := &fakeCS{
		t:   t,
		h:   1,
		tsc: newTSCache(2*build.ForkLengthThreshold, nil),
	}
	require.NoError(t, fcs.tsc.add(makeTs(t, 1, dummyCid)))

	events := NewEvents(context.Background(), fcs)

	var applied bool
	var reverted bool

	err := events.ChainAt(func(_ context.Context, ts *types.TipSet, curH abi.ChainEpoch) error {
		require.Equal(t, 5, int(ts.Height()))
		require.Equal(t, 8, int(curH))
		applied = true
		return nil
	}, func(_ context.Context, ts *types.TipSet) error {
		reverted = true
		return nil
	}, 3, 5)
	require.NoError(t, err)

	fcs.advance(0, 3, nil)
	require.Equal(t, false, applied)
	require.Equal(t, false, reverted)

	fcs.advance(0, 3, nil)
	require.Equal(t, false, applied)
	require.Equal(t, false, reverted)

	fcs.advance(0, 3, nil)
	require.Equal(t, true, applied)
	require.Equal(t, false, reverted)
	applied = false

	fcs.advance(0, 3, nil)
	require.Equal(t, false, applied)
	require.Equal(t, false, reverted)

	fcs.advance(10, 10, nil)
	require.Equal(t, true, applied)
	require.Equal(t, true, reverted)
	applied = false
	reverted = false

	fcs.advance(10, 1, nil)
	require.Equal(t, false, applied)
	require.Equal(t, true, reverted)
	reverted = false

	fcs.advance(0, 1, nil)
	require.Equal(t, false, applied)
	require.Equal(t, false, reverted)

	fcs.advance(0, 2, nil)
	require.Equal(t, false, applied)
	require.Equal(t, false, reverted)

	fcs.advance(0, 1, nil) // 8
	require.Equal(t, true, applied)
	require.Equal(t, false, reverted)
}

func TestAtDoubleTrigger(t *testing.T) {
	fcs := &fakeCS{
		t:   t,
		h:   1,
		tsc: newTSCache(2*build.ForkLengthThreshold, nil),
	}
	require.NoError(t, fcs.tsc.add(makeTs(t, 1, dummyCid)))

	events := NewEvents(context.Background(), fcs)

	var applied bool
	var reverted bool

	err := events.ChainAt(func(_ context.Context, ts *types.TipSet, curH abi.ChainEpoch) error {
		require.Equal(t, 5, int(ts.Height()))
		require.Equal(t, 8, int(curH))
		applied = true
		return nil
	}, func(_ context.Context, ts *types.TipSet) error {
		reverted = true
		return nil
	}, 3, 5)
	require.NoError(t, err)

	fcs.advance(0, 6, nil)
	require.False(t, applied)
	require.False(t, reverted)

	fcs.advance(0, 1, nil)
	require.True(t, applied)
	require.False(t, reverted)
	applied = false

	fcs.advance(2, 2, nil)
	require.False(t, applied)
	require.False(t, reverted)

	fcs.advance(4, 4, nil)
	require.True(t, applied)
	require.True(t, reverted)
}

func TestAtNullTrigger(t *testing.T) {
	fcs := &fakeCS{
		t:   t,
		h:   1,
		tsc: newTSCache(2*build.ForkLengthThreshold, nil),
	}
	require.NoError(t, fcs.tsc.add(makeTs(t, 1, dummyCid)))

	events := NewEvents(context.Background(), fcs)

	var applied bool
	var reverted bool

	err := events.ChainAt(func(_ context.Context, ts *types.TipSet, curH abi.ChainEpoch) error {
		require.Equal(t, abi.ChainEpoch(6), ts.Height())
		require.Equal(t, 8, int(curH))
		applied = true
		return nil
	}, func(_ context.Context, ts *types.TipSet) error {
		reverted = true
		return nil
	}, 3, 5)
	require.NoError(t, err)

	fcs.advance(0, 6, nil, 5)
	require.Equal(t, false, applied)
	require.Equal(t, false, reverted)

	fcs.advance(0, 3, nil)
	require.Equal(t, true, applied)
	require.Equal(t, false, reverted)
	applied = false
}

func TestAtNullConf(t *testing.T) {
	fcs := &fakeCS{
		t:   t,
		h:   1,
		tsc: newTSCache(2*build.ForkLengthThreshold, nil),
	}
	require.NoError(t, fcs.tsc.add(makeTs(t, 1, dummyCid)))

	events := NewEvents(context.Background(), fcs)

	var applied bool
	var reverted bool

	err := events.ChainAt(func(_ context.Context, ts *types.TipSet, curH abi.ChainEpoch) error {
		require.Equal(t, 5, int(ts.Height()))
		require.Equal(t, 8, int(curH))
		applied = true
		return nil
	}, func(_ context.Context, ts *types.TipSet) error {
		reverted = true
		return nil
	}, 3, 5)
	require.NoError(t, err)

	fcs.advance(0, 6, nil)
	require.Equal(t, false, applied)
	require.Equal(t, false, reverted)

	fcs.advance(0, 3, nil, 8)
	require.Equal(t, true, applied)
	require.Equal(t, false, reverted)
	applied = false

	fcs.advance(7, 1, nil)
	require.Equal(t, false, applied)
	require.Equal(t, true, reverted)
	reverted = false
}

func TestAtStart(t *testing.T) {
	fcs := &fakeCS{
		t:   t,
		h:   1,
		tsc: newTSCache(2*build.ForkLengthThreshold, nil),
	}
	require.NoError(t, fcs.tsc.add(makeTs(t, 1, dummyCid)))

	events := NewEvents(context.Background(), fcs)

	fcs.advance(0, 5, nil) // 6

	var applied bool
	var reverted bool

	err := events.ChainAt(func(_ context.Context, ts *types.TipSet, curH abi.ChainEpoch) error {
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

	fcs.advance(0, 5, nil) // 11
	require.Equal(t, true, applied)
	require.Equal(t, false, reverted)
}

func TestAtStartConfidence(t *testing.T) {
	fcs := &fakeCS{
		t:   t,
		h:   1,
		tsc: newTSCache(2*build.ForkLengthThreshold, nil),
	}
	require.NoError(t, fcs.tsc.add(makeTs(t, 1, dummyCid)))

	events := NewEvents(context.Background(), fcs)

	fcs.advance(0, 10, nil) // 11

	var applied bool
	var reverted bool

	err := events.ChainAt(func(_ context.Context, ts *types.TipSet, curH abi.ChainEpoch) error {
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
	fcs := &fakeCS{
		t:   t,
		h:   1,
		tsc: newTSCache(2*build.ForkLengthThreshold, nil),
	}
	require.NoError(t, fcs.tsc.add(makeTs(t, 1, dummyCid)))

	events := NewEvents(context.Background(), fcs)

	var applied bool
	var reverted bool

	err := events.ChainAt(func(_ context.Context, ts *types.TipSet, curH abi.ChainEpoch) error {
		return events.ChainAt(func(_ context.Context, ts *types.TipSet, curH abi.ChainEpoch) error {
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

	fcs.advance(0, 15, nil)

	require.Equal(t, true, applied)
	require.Equal(t, false, reverted)
}

func TestAtChainedConfidence(t *testing.T) {
	fcs := &fakeCS{
		t:   t,
		h:   1,
		tsc: newTSCache(2*build.ForkLengthThreshold, nil),
	}
	require.NoError(t, fcs.tsc.add(makeTs(t, 1, dummyCid)))

	events := NewEvents(context.Background(), fcs)

	fcs.advance(0, 15, nil)

	var applied bool
	var reverted bool

	err := events.ChainAt(func(_ context.Context, ts *types.TipSet, curH abi.ChainEpoch) error {
		return events.ChainAt(func(_ context.Context, ts *types.TipSet, curH abi.ChainEpoch) error {
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
	fcs := &fakeCS{
		t:   t,
		h:   1,
		tsc: newTSCache(2*build.ForkLengthThreshold, nil),
	}
	require.NoError(t, fcs.tsc.add(makeTs(t, 1, dummyCid)))

	events := NewEvents(context.Background(), fcs)

	fcs.advance(0, 15, nil, 5)

	var applied bool
	var reverted bool

	err := events.ChainAt(func(_ context.Context, ts *types.TipSet, curH abi.ChainEpoch) error {
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

func matchAddrMethod(to address.Address, m abi.MethodNum) func(msg *types.Message) (bool, error) {
	return func(msg *types.Message) (bool, error) {
		return to == msg.To && m == msg.Method, nil
	}
}

func TestCalled(t *testing.T) {
	fcs := &fakeCS{
		t: t,
		h: 1,

		msgs:    map[cid.Cid]fakeMsg{},
		blkMsgs: map[cid.Cid]cid.Cid{},
		tsc:     newTSCache(2*build.ForkLengthThreshold, nil),
	}
	require.NoError(t, fcs.tsc.add(makeTs(t, 1, dummyCid)))

	events := NewEvents(context.Background(), fcs)

	t0123, err := address.NewFromString("t0123")
	require.NoError(t, err)

	more := true
	var applied, reverted bool
	var appliedMsg *types.Message
	var appliedTs *types.TipSet
	var appliedH abi.ChainEpoch

	err = events.Called(func(ts *types.TipSet) (d bool, m bool, e error) {
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

	// create few blocks to make sure nothing get's randomly called

	fcs.advance(0, 4, nil) // H=5
	require.Equal(t, false, applied)
	require.Equal(t, false, reverted)

	// create blocks with message (but below confidence threshold)

	fcs.advance(0, 3, map[int]cid.Cid{ // msg at H=6; H=8 (confidence=2)
		0: fcs.fakeMsgs(fakeMsg{
			bmsgs: []*types.Message{
				{To: t0123, From: t0123, Method: 5, Nonce: 1},
			},
		}),
	})

	require.Equal(t, false, applied)
	require.Equal(t, false, reverted)

	// create additional block so we are above confidence threshold

	fcs.advance(0, 2, nil) // H=10 (confidence=3, apply)

	require.Equal(t, true, applied)
	require.Equal(t, false, reverted)
	applied = false

	// dip below confidence
	fcs.advance(2, 2, nil) // H=10 (confidence=3, apply)

	require.Equal(t, false, applied)
	require.Equal(t, false, reverted)

	require.Equal(t, abi.ChainEpoch(7), appliedTs.Height())
	require.Equal(t, "bafkqaaa", appliedTs.Blocks()[0].Messages.String())
	require.Equal(t, abi.ChainEpoch(10), appliedH)
	require.Equal(t, t0123, appliedMsg.To)
	require.Equal(t, uint64(1), appliedMsg.Nonce)
	require.Equal(t, abi.MethodNum(5), appliedMsg.Method)

	// revert some blocks, keep the message

	fcs.advance(3, 1, nil) // H=8 (confidence=1)
	require.Equal(t, false, applied)
	require.Equal(t, false, reverted)

	// revert the message

	fcs.advance(2, 1, nil) // H=7, we reverted ts with the msg

	require.Equal(t, false, applied)
	require.Equal(t, true, reverted)
	reverted = false

	// send new message on different height

	n2msg := fcs.fakeMsgs(fakeMsg{
		bmsgs: []*types.Message{
			{To: t0123, From: t0123, Method: 5, Nonce: 2},
		},
	})

	fcs.advance(0, 5, map[int]cid.Cid{ // (confidence=3)
		0: n2msg,
	})

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

	fcs.advance(4, 6, map[int]cid.Cid{ // (confidence=3)
		1: n2msg,
	})

	// TODO: We probably don't want to call revert/apply, as restarting certain
	//  actions may be expensive, and in this case the message is still
	//  on-chain, just at different height
	require.Equal(t, true, applied)
	require.Equal(t, true, reverted)
	reverted = false
	applied = false

	require.Equal(t, abi.ChainEpoch(11), appliedTs.Height())
	require.Equal(t, "bafkqaaa", appliedTs.Blocks()[0].Messages.String())
	require.Equal(t, abi.ChainEpoch(14), appliedH)
	require.Equal(t, t0123, appliedMsg.To)
	require.Equal(t, uint64(2), appliedMsg.Nonce)
	require.Equal(t, abi.MethodNum(5), appliedMsg.Method)

	// call method again

	fcs.advance(0, 5, map[int]cid.Cid{
		0: n2msg,
	})

	require.Equal(t, true, applied)
	require.Equal(t, false, reverted)
	applied = false

	// send and revert below confidence, then cross confidence
	fcs.advance(0, 2, map[int]cid.Cid{
		0: fcs.fakeMsgs(fakeMsg{
			bmsgs: []*types.Message{
				{To: t0123, From: t0123, Method: 5, Nonce: 3},
			},
		}),
	})

	fcs.advance(1, 4, nil) // H=19, but message reverted

	require.Equal(t, false, applied)
	require.Equal(t, false, reverted)

	// test timeout (it's set to 20 in the call to `events.Called` above)

	fcs.advance(0, 6, nil)

	require.Equal(t, false, applied) // not calling timeout as we received messages
	require.Equal(t, false, reverted)

	// test unregistering with more

	more = false
	fcs.advance(0, 5, map[int]cid.Cid{
		0: fcs.fakeMsgs(fakeMsg{
			bmsgs: []*types.Message{
				{To: t0123, From: t0123, Method: 5, Nonce: 4}, // this signals we don't want more
			},
		}),
	})

	require.Equal(t, true, applied)
	require.Equal(t, false, reverted)
	applied = false

	fcs.advance(0, 5, map[int]cid.Cid{
		0: fcs.fakeMsgs(fakeMsg{
			bmsgs: []*types.Message{
				{To: t0123, From: t0123, Method: 5, Nonce: 5},
			},
		}),
	})

	require.Equal(t, false, applied) // should not get any further notifications
	require.Equal(t, false, reverted)

	// revert after disabled

	fcs.advance(5, 1, nil) // try reverting msg sent after disabling

	require.Equal(t, false, applied)
	require.Equal(t, false, reverted)

	fcs.advance(5, 1, nil) // try reverting msg sent before disabling

	require.Equal(t, false, applied)
	require.Equal(t, true, reverted)
}

func TestCalledTimeout(t *testing.T) {
	fcs := &fakeCS{
		t: t,
		h: 1,

		msgs:    map[cid.Cid]fakeMsg{},
		blkMsgs: map[cid.Cid]cid.Cid{},
		tsc:     newTSCache(2*build.ForkLengthThreshold, nil),
	}
	require.NoError(t, fcs.tsc.add(makeTs(t, 1, dummyCid)))

	events := NewEvents(context.Background(), fcs)

	t0123, err := address.NewFromString("t0123")
	require.NoError(t, err)

	called := false

	err = events.Called(func(ts *types.TipSet) (d bool, m bool, e error) {
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

	fcs.advance(0, 21, nil)
	require.False(t, called)

	fcs.advance(0, 5, nil)
	require.True(t, called)
	called = false

	// with check func reporting done

	fcs = &fakeCS{
		t: t,
		h: 1,

		msgs:    map[cid.Cid]fakeMsg{},
		blkMsgs: map[cid.Cid]cid.Cid{},
		tsc:     newTSCache(2*build.ForkLengthThreshold, nil),
	}
	require.NoError(t, fcs.tsc.add(makeTs(t, 1, dummyCid)))

	events = NewEvents(context.Background(), fcs)

	err = events.Called(func(ts *types.TipSet) (d bool, m bool, e error) {
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

	fcs.advance(0, 21, nil)
	require.False(t, called)

	fcs.advance(0, 5, nil)
	require.False(t, called)
}

func TestCalledOrder(t *testing.T) {
	fcs := &fakeCS{
		t: t,
		h: 1,

		msgs:    map[cid.Cid]fakeMsg{},
		blkMsgs: map[cid.Cid]cid.Cid{},
		tsc:     newTSCache(2*build.ForkLengthThreshold, nil),
	}
	require.NoError(t, fcs.tsc.add(makeTs(t, 1, dummyCid)))

	events := NewEvents(context.Background(), fcs)

	t0123, err := address.NewFromString("t0123")
	require.NoError(t, err)

	at := 0

	err = events.Called(func(ts *types.TipSet) (d bool, m bool, e error) {
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

	fcs.advance(0, 10, map[int]cid.Cid{
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

	fcs.advance(9, 1, nil)
}

func TestCalledNull(t *testing.T) {
	fcs := &fakeCS{
		t: t,
		h: 1,

		msgs:    map[cid.Cid]fakeMsg{},
		blkMsgs: map[cid.Cid]cid.Cid{},
		tsc:     newTSCache(2*build.ForkLengthThreshold, nil),
	}
	require.NoError(t, fcs.tsc.add(makeTs(t, 1, dummyCid)))

	events := NewEvents(context.Background(), fcs)

	t0123, err := address.NewFromString("t0123")
	require.NoError(t, err)

	more := true
	var applied, reverted bool

	err = events.Called(func(ts *types.TipSet) (d bool, m bool, e error) {
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

	// create few blocks to make sure nothing get's randomly called

	fcs.advance(0, 4, nil) // H=5
	require.Equal(t, false, applied)
	require.Equal(t, false, reverted)

	// create blocks with message (but below confidence threshold)

	fcs.advance(0, 3, map[int]cid.Cid{ // msg at H=6; H=8 (confidence=2)
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

	fcs.advance(0, 3, nil, 10) // H=11 (confidence=3, apply)

	require.Equal(t, true, applied)
	require.Equal(t, false, reverted)
}
