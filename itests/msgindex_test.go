package itests

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/exitcode"

	lapi "github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/index"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
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
	// copy of apiSuite.testSearchMsgWith; needs to be copied or else CI is angry, tests are built individually there
	ctx := context.Background()

	full, _, ens := kit.EnsembleMinimal(t, kit.ConstructorOpts(node.Override(new(index.MsgIndex), makeMsgIndex)))

	senderAddr, err := full.WalletDefaultAddress(ctx)
	require.NoError(t, err)

	msg := &types.Message{
		From:  senderAddr,
		To:    senderAddr,
		Value: big.Zero(),
	}

	ens.BeginMining(100 * time.Millisecond)

	sm, err := full.MpoolPushMessage(ctx, msg, nil)
	require.NoError(t, err)

	//stm: @CHAIN_STATE_WAIT_MSG_001
	res, err := full.StateWaitMsg(ctx, sm.Cid(), 1, lapi.LookbackNoLimit, true)
	require.NoError(t, err)

	require.Equal(t, exitcode.Ok, res.Receipt.ExitCode, "message not successful")

	//stm: @CHAIN_STATE_SEARCH_MSG_001
	searchRes, err := full.StateSearchMsg(ctx, types.EmptyTSK, sm.Cid(), lapi.LookbackNoLimit, true)
	require.NoError(t, err)
	require.NotNil(t, searchRes)

	require.Equalf(t, res.TipSet, searchRes.TipSet, "search ts: %s, different from wait ts: %s", searchRes.TipSet, res.TipSet)
}
