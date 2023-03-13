package itests

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/lotus/chain/index"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/itests/kit"
	"github.com/filecoin-project/lotus/node"
)

func init() {
	// adjust those to make tests snappy
	index.CoalesceMinDelay = time.Millisecond
	index.CoalesceMaxDelay = 10 * time.Millisecond
	index.CoalesceMergeInterval = time.Millisecond
}

func testMsgIndex(
	t *testing.T,
	name string,
	run func(t *testing.T, makeMsgIndex func(cs *store.ChainStore) (index.MsgIndex, error)),
	check func(t *testing.T, i int, msgIndex index.MsgIndex),
) {

	// create the message indices in the test context
	var mx sync.Mutex
	var tmpDirs []string
	var msgIndices []index.MsgIndex

	t.Cleanup(func() {
		for _, msgIndex := range msgIndices {
			_ = msgIndex.Close()
		}

		for _, tmp := range tmpDirs {
			_ = os.RemoveAll(tmp)
		}
	})

	makeMsgIndex := func(cs *store.ChainStore) (index.MsgIndex, error) {
		var err error
		tmp := t.TempDir()
		msgIndex, err := index.NewMsgIndex(context.Background(), tmp, cs)
		if err == nil {
			mx.Lock()
			tmpDirs = append(tmpDirs, tmp)
			msgIndices = append(msgIndices, msgIndex)
			mx.Unlock()
		}
		return msgIndex, err
	}

	t.Run(name, func(t *testing.T) {
		run(t, makeMsgIndex)
	})

	if len(msgIndices) == 0 {
		t.Fatal("no message indices")
	}

	for i, msgIndex := range msgIndices {
		check(t, i, msgIndex)
	}
}

func checkNonEmptyMsgIndex(t *testing.T, _ int, msgIndex index.MsgIndex) {
	mi, ok := msgIndex.(interface{ CountMessages() (int64, error) })
	if !ok {
		t.Fatal("index does not allow counting")
	}
	count, err := mi.CountMessages()
	require.NoError(t, err)
	require.NotEqual(t, count, 0)
}

func TestMsgIndex(t *testing.T) {
	testMsgIndex(t, "testSearchMsg", testSearchMsgWithIndex, checkNonEmptyMsgIndex)
}

func testSearchMsgWithIndex(t *testing.T, makeMsgIndex func(cs *store.ChainStore) (index.MsgIndex, error)) {
	suite := apiSuite{opts: []interface{}{kit.ConstructorOpts(node.Override(new(index.MsgIndex), makeMsgIndex))}}
	suite.testSearchMsg(t)
}
