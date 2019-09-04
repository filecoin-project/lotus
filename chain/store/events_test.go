package store

import (
	"fmt"
	"github.com/filecoin-project/go-lotus/build"
	"github.com/filecoin-project/go-lotus/chain/address"
	"github.com/filecoin-project/go-lotus/chain/types"
	"github.com/ipfs/go-cid"
	"github.com/multiformats/go-multihash"
	"github.com/stretchr/testify/require"
	"testing"
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
	h   uint64
	tsc *tipSetCache

	msgs map[cid.Cid]fakeMsg

	sub func(rev, app []*types.TipSet) error
}

func makeTs(t *testing.T, h uint64, msgcid cid.Cid) *types.TipSet {
	ts, err := types.NewTipSet([]*types.BlockHeader{
		{
			Height: h,

			StateRoot:       dummyCid,
			Messages:        msgcid,
			MessageReceipts: dummyCid,
		},
	})

	require.NoError(t, err)

	return ts
}

func (fcs *fakeCS) SubscribeHeadChanges(f func(rev, app []*types.TipSet) error) {
	if fcs.sub != nil {
		fcs.t.Fatal("sub should be nil")
	}
	fcs.sub = f
}

func (fcs *fakeCS) GetHeaviestTipSet() *types.TipSet {
	return fcs.tsc.best()
}

func (fcs *fakeCS) MessagesForBlock(b *types.BlockHeader) ([]*types.Message, []*types.SignedMessage, error) {
	ms, ok := fcs.msgs[b.Messages]
	if ok {
		return ms.bmsgs, ms.smsgs, nil
	}

	return []*types.Message{}, []*types.SignedMessage{}, nil
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

func (fcs *fakeCS) advance(rev, app int, msgs map[int]cid.Cid) { // todo: allow msgs
	if fcs.sub == nil {
		fcs.t.Fatal("sub not be nil")
	}

	var revs []*types.TipSet
	for i := 0; i < rev; i++ {
		ts := fcs.tsc.best()

		revs = append(revs, ts)
		fcs.h--
		require.NoError(fcs.t, fcs.tsc.revert(ts))
	}

	var apps []*types.TipSet
	for i := 0; i < app; i++ {
		fcs.h++

		mc, _ := msgs[i]
		if mc == cid.Undef {
			mc = dummyCid
		}

		ts := makeTs(fcs.t, fcs.h, mc)
		require.NoError(fcs.t, fcs.tsc.add(ts))

		apps = append(apps, ts)
	}

	err := fcs.sub(revs, apps)
	require.NoError(fcs.t, err)
}

var _ eventChainStore = &fakeCS{}

func TestAt(t *testing.T) {
	fcs := &fakeCS{
		t:   t,
		h:   1,
		tsc: newTSCache(2 * build.ForkLengthThreshold),
	}
	require.NoError(t, fcs.tsc.add(makeTs(t, 1, dummyCid)))

	events := NewEvents(fcs)

	var applied bool
	var reverted bool

	err := events.ChainAt(func(ts *types.TipSet, curH uint64) error {
		require.Equal(t, 5, int(ts.Height()))
		require.Equal(t, 8, int(curH))
		applied = true
		return nil
	}, func(ts *types.TipSet) error {
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

func TestCalled(t *testing.T) {
	fcs := &fakeCS{
		t: t,
		h: 1,

		msgs: map[cid.Cid]fakeMsg{},
		tsc:  newTSCache(2 * build.ForkLengthThreshold),
	}
	require.NoError(t, fcs.tsc.add(makeTs(t, 1, dummyCid)))

	events := NewEvents(fcs)

	t0123, err := address.NewFromString("t0123")
	require.NoError(t, err)

	var applied, reverted bool
	var appliedMsg *types.Message
	var appliedTs *types.TipSet
	var appliedH uint64

	err = events.Called(func(ts *types.TipSet) (b bool, e error) {
		return false, nil
	}, func(msg *types.Message, ts *types.TipSet, curH uint64) error {
		applied = true
		appliedMsg = msg
		appliedTs = ts
		appliedH = curH
		return nil
	}, func(ts *types.TipSet) error {
		reverted = true
		return nil
	}, 3, t0123, 5)
	require.NoError(t, err)

	// create few blocks to make sure nothing get's randomly called

	fcs.advance(0, 4, nil) // H=5
	require.Equal(t, false, applied)
	require.Equal(t, false, reverted)

	// create blocks with message (but below confidence threshold)

	fcs.advance(0, 3, map[int]cid.Cid{ // msg at H=6; H=8 (confidence=2)
		0: fcs.fakeMsgs(fakeMsg{
			bmsgs: []*types.Message{
				{To: t0123, Method: 5, Nonce: 1},
			},
		}),
	})

	require.Equal(t, false, applied)
	require.Equal(t, false, reverted)

	// create additional block so we are above confidence threshold

	fcs.advance(0, 1, nil) // H=9 (confidence=3, apply)

	require.Equal(t, true, applied)
	require.Equal(t, false, reverted)
	applied = false

	require.Equal(t, uint64(6), appliedTs.Height())
	require.Equal(t, "bafkqaajq", appliedTs.Blocks()[0].Messages.String())
	require.Equal(t, uint64(9), appliedH)
	require.Equal(t, t0123, appliedMsg.To)
	require.Equal(t, uint64(1), appliedMsg.Nonce)
	require.Equal(t, uint64(5), appliedMsg.Method)

	// revert some blocks, keep the message

	fcs.advance(3, 1, nil) // H=7 (confidence=1)
	require.Equal(t, false, applied)
	require.Equal(t, false, reverted)

	// revert the message

	fcs.advance(2, 1, nil) // H=6, we reverted ts with the msg

	require.Equal(t, false, applied)
	require.Equal(t, true, reverted)
	reverted = false

	// send new message on different height

	n2msg := fcs.fakeMsgs(fakeMsg{
		bmsgs: []*types.Message{
			{To: t0123, Method: 5, Nonce: 2},
		},
	})

	fcs.advance(0, 4, map[int]cid.Cid{ // msg at H=7; H=10 (confidence=3)
		0: n2msg,
	})

	require.Equal(t, true, applied)
	require.Equal(t, false, reverted)
	applied = false

	require.Equal(t, uint64(7), appliedTs.Height())
	require.Equal(t, "bafkqaajr", appliedTs.Blocks()[0].Messages.String())
	require.Equal(t, uint64(10), appliedH)
	require.Equal(t, t0123, appliedMsg.To)
	require.Equal(t, uint64(2), appliedMsg.Nonce)
	require.Equal(t, uint64(5), appliedMsg.Method)

	// revert and apply at different height

	fcs.advance(4, 5, map[int]cid.Cid{ // msg at H=8; H=11 (confidence=3)
		1: n2msg,
	})

	// TODO: We probably don't want to call revert/apply, as restarting certain
	//  actions may be expensive, and in this case the message is still
	//  on-chain, just at different height
	require.Equal(t, true, applied)
	require.Equal(t, true, reverted)
	reverted = false
	applied = false

	require.Equal(t, uint64(8), appliedTs.Height())
	require.Equal(t, "bafkqaajr", appliedTs.Blocks()[0].Messages.String())
	require.Equal(t, uint64(11), appliedH)
	require.Equal(t, t0123, appliedMsg.To)
	require.Equal(t, uint64(2), appliedMsg.Nonce)
	require.Equal(t, uint64(5), appliedMsg.Method)

	// call method again

	fcs.advance(0, 4, map[int]cid.Cid{ // msg at H=12; H=15
		0: n2msg,
	})

	require.Equal(t, true, applied)
	require.Equal(t, false, reverted)
	applied = false

	// send and revert below confidence, then cross confidence
	fcs.advance(0, 1, map[int]cid.Cid{ // msg at H=16; H=16
		0: fcs.fakeMsgs(fakeMsg{
			bmsgs: []*types.Message{
				{To: t0123, Method: 5, Nonce: 3},
			},
		}),
	})

	fcs.advance(1, 4, nil) // H=19, but message reverted

	require.Equal(t, false, applied)
	require.Equal(t, false, reverted)

}
