package paychmgr

import (
	"testing"

	"github.com/ipfs/go-cid"

	"github.com/stretchr/testify/require"

	"golang.org/x/xerrors"
)

func testCids() []cid.Cid {
	c1, _ := cid.Decode("QmdmGQmRgRjazArukTbsXuuxmSHsMCcRYPAZoGhd6e3MuS")
	c2, _ := cid.Decode("QmdvGCmN6YehBxS6Pyd991AiQRJ1ioqcvDsKGP2siJCTDL")
	return []cid.Cid{c1, c2}
}

func TestMsgListener(t *testing.T) {
	var ml msgListeners

	done := false
	experr := xerrors.Errorf("some err")
	cids := testCids()
	ml.onMsg(cids[0], func(err error) {
		require.Equal(t, experr, err)
		done = true
	})

	ml.fireMsgComplete(cids[0], experr)

	if !done {
		t.Fatal("failed to fire event")
	}
}

func TestMsgListenerNilErr(t *testing.T) {
	var ml msgListeners

	done := false
	cids := testCids()
	ml.onMsg(cids[0], func(err error) {
		require.Nil(t, err)
		done = true
	})

	ml.fireMsgComplete(cids[0], nil)

	if !done {
		t.Fatal("failed to fire event")
	}
}

func TestMsgListenerUnsub(t *testing.T) {
	var ml msgListeners

	done := false
	experr := xerrors.Errorf("some err")
	cids := testCids()
	id1 := ml.onMsg(cids[0], func(err error) {
		t.Fatal("should not call unsubscribed listener")
	})
	ml.onMsg(cids[0], func(err error) {
		require.Equal(t, experr, err)
		done = true
	})

	ml.unsubscribe(id1)
	ml.fireMsgComplete(cids[0], experr)

	if !done {
		t.Fatal("failed to fire event")
	}
}

func TestMsgListenerMulti(t *testing.T) {
	var ml msgListeners

	count := 0
	cids := testCids()
	ml.onMsg(cids[0], func(err error) {
		count++
	})
	ml.onMsg(cids[0], func(err error) {
		count++
	})
	ml.onMsg(cids[1], func(err error) {
		count++
	})

	ml.fireMsgComplete(cids[0], nil)
	require.Equal(t, 2, count)

	ml.fireMsgComplete(cids[1], nil)
	require.Equal(t, 3, count)
}
