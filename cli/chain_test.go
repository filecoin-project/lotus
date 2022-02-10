package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/lotus/api"
	types "github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/types/mock"
	"github.com/filecoin-project/specs-actors/v7/actors/builtin"
	"github.com/golang/mock/gomock"
	cid "github.com/ipfs/go-cid"
	"github.com/stretchr/testify/assert"
)

func TestChainHead(t *testing.T) {
	app, mockApi, buf, done := NewMockAppWithFullAPI(t, WithCategory("chain", ChainHeadCmd))
	defer done()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ts := mock.TipSet(mock.MkBlock(nil, 0, 0))
	gomock.InOrder(
		mockApi.EXPECT().ChainHead(ctx).Return(ts, nil),
	)

	err := app.Run([]string{"chain", "head"})
	assert.NoError(t, err)

	assert.Regexp(t, regexp.MustCompile(ts.Cids()[0].String()), buf.String())
}

func TestGetBlock(t *testing.T) {
	app, mockApi, buf, done := NewMockAppWithFullAPI(t, WithCategory("chain", ChainGetBlock))
	defer done()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	block := mock.MkBlock(nil, 0, 0)
	blockMsgs := api.BlockMessages{}

	gomock.InOrder(
		mockApi.EXPECT().ChainGetBlock(ctx, block.Cid()).Return(block, nil),
		mockApi.EXPECT().ChainGetBlockMessages(ctx, block.Cid()).Return(&blockMsgs, nil),
		mockApi.EXPECT().ChainGetParentMessages(ctx, block.Cid()).Return([]api.Message{}, nil),
		mockApi.EXPECT().ChainGetParentReceipts(ctx, block.Cid()).Return([]*types.MessageReceipt{}, nil),
	)

	err := app.Run([]string{"chain", "getblock", block.Cid().String()})
	assert.NoError(t, err)

	// expected output format
	out := struct {
		types.BlockHeader
		BlsMessages    []*types.Message
		SecpkMessages  []*types.SignedMessage
		ParentReceipts []*types.MessageReceipt
		ParentMessages []cid.Cid
	}{}

	err = json.Unmarshal(buf.Bytes(), &out)
	assert.NoError(t, err)

	assert.True(t, block.Cid().Equals(out.Cid()))
}

func TestReadOjb(t *testing.T) {
	app, mockApi, buf, done := NewMockAppWithFullAPI(t, WithCategory("chain", ChainReadObjCmd))
	defer done()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	block := mock.MkBlock(nil, 0, 0)
	obj := new(bytes.Buffer)
	err := block.MarshalCBOR(obj)
	assert.NoError(t, err)

	gomock.InOrder(
		mockApi.EXPECT().ChainReadObj(ctx, block.Cid()).Return(obj.Bytes(), nil),
	)

	err = app.Run([]string{"chain", "read-obj", block.Cid().String()})
	assert.NoError(t, err)

	assert.Equal(t, buf.String(), fmt.Sprintf("%x\n", obj.Bytes()))
}

func TestChainDeleteObj(t *testing.T) {
	cmd := WithCategory("chain", ChainDeleteObjCmd)
	block := mock.MkBlock(nil, 0, 0)

	// given no force flag, it should return an error and no API calls should be made
	t.Run("no-really-do-it", func(t *testing.T) {
		app, _, _, done := NewMockAppWithFullAPI(t, cmd)
		defer done()

		err := app.Run([]string{"chain", "delete-obj", block.Cid().String()})
		assert.Error(t, err)
	})

	// given a force flag, it calls API delete
	t.Run("really-do-it", func(t *testing.T) {
		app, mockApi, buf, done := NewMockAppWithFullAPI(t, cmd)
		defer done()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		gomock.InOrder(
			mockApi.EXPECT().ChainDeleteObj(ctx, block.Cid()).Return(nil),
		)

		err := app.Run([]string{"chain", "delete-obj", "--really-do-it=true", block.Cid().String()})
		assert.NoError(t, err)

		assert.Contains(t, buf.String(), block.Cid().String())
	})
}

func TestChainStatObj(t *testing.T) {
	cmd := WithCategory("chain", ChainStatObjCmd)
	block := mock.MkBlock(nil, 0, 0)
	stat := api.ObjStat{Size: 123, Links: 321}

	checkOutput := func(buf *bytes.Buffer) {
		out := buf.String()
		outSplit := strings.Split(out, "\n")

		assert.Contains(t, outSplit[0], fmt.Sprintf("%d", stat.Links))
		assert.Contains(t, outSplit[1], fmt.Sprintf("%d", stat.Size))
	}

	// given no --base flag, it calls ChainStatObj with base=cid.Undef
	t.Run("no-base", func(t *testing.T) {
		app, mockApi, buf, done := NewMockAppWithFullAPI(t, cmd)
		defer done()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		gomock.InOrder(
			mockApi.EXPECT().ChainStatObj(ctx, block.Cid(), cid.Undef).Return(stat, nil),
		)

		err := app.Run([]string{"chain", "stat-obj", block.Cid().String()})
		assert.NoError(t, err)

		checkOutput(buf)
	})

	// given a --base flag, it calls ChainStatObj with that base
	t.Run("base", func(t *testing.T) {
		app, mockApi, buf, done := NewMockAppWithFullAPI(t, cmd)
		defer done()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		gomock.InOrder(
			mockApi.EXPECT().ChainStatObj(ctx, block.Cid(), block.Cid()).Return(stat, nil),
		)

		err := app.Run([]string{"chain", "stat-obj", fmt.Sprintf("-base=%s", block.Cid().String()), block.Cid().String()})
		assert.NoError(t, err)

		checkOutput(buf)
	})
}

func TestChainGetMsg(t *testing.T) {
	app, mockApi, buf, done := NewMockAppWithFullAPI(t, WithCategory("chain", ChainGetMsgCmd))
	defer done()

	from, err := mock.RandomActorAddress()
	assert.NoError(t, err)

	to, err := mock.RandomActorAddress()
	assert.NoError(t, err)

	msg := mock.UnsignedMessage(*from, *to, 0)

	obj := new(bytes.Buffer)
	err = msg.MarshalCBOR(obj)
	assert.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	gomock.InOrder(
		mockApi.EXPECT().ChainReadObj(ctx, msg.Cid()).Return(obj.Bytes(), nil),
	)

	err = app.Run([]string{"chain", "getmessage", msg.Cid().String()})
	assert.NoError(t, err)

	var out types.Message
	err = json.Unmarshal(buf.Bytes(), &out)
	assert.NoError(t, err)

	assert.Equal(t, *msg, out)
}

func TestSetHead(t *testing.T) {
	cmd := WithCategory("chain", ChainSetHeadCmd)
	genesis := mock.TipSet(mock.MkBlock(nil, 0, 0))
	ts := mock.TipSet(mock.MkBlock(genesis, 1, 0))
	epoch := abi.ChainEpoch(uint64(0))

	// given the -genesis flag, resets head to genesis ignoring the provided ts positional argument
	t.Run("genesis", func(t *testing.T) {
		app, mockApi, _, done := NewMockAppWithFullAPI(t, cmd)
		defer done()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		gomock.InOrder(
			mockApi.EXPECT().ChainGetGenesis(ctx).Return(genesis, nil),
			mockApi.EXPECT().ChainSetHead(ctx, genesis.Key()).Return(nil),
		)

		err := app.Run([]string{"chain", "sethead", "-genesis=true", ts.Key().String()})
		assert.NoError(t, err)
	})

	// given the -epoch flag, resets head to given epoch, ignoring the provided ts positional argument
	t.Run("epoch", func(t *testing.T) {
		app, mockApi, _, done := NewMockAppWithFullAPI(t, cmd)
		defer done()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		gomock.InOrder(
			mockApi.EXPECT().ChainGetTipSetByHeight(ctx, epoch, types.EmptyTSK).Return(genesis, nil),
			mockApi.EXPECT().ChainSetHead(ctx, genesis.Key()).Return(nil),
		)

		err := app.Run([]string{"chain", "sethead", fmt.Sprintf("-epoch=%s", epoch), ts.Key().String()})
		assert.NoError(t, err)
	})

	// given no flag, resets the head to given tipset key
	t.Run("default", func(t *testing.T) {
		app, mockApi, _, done := NewMockAppWithFullAPI(t, cmd)
		defer done()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		gomock.InOrder(
			mockApi.EXPECT().ChainGetBlock(ctx, ts.Key().Cids()[0]).Return(ts.Blocks()[0], nil),
			mockApi.EXPECT().ChainSetHead(ctx, ts.Key()).Return(nil),
		)

		// ts.Key should be passed as an array of arguments (CIDs)
		// since we have only one CID in the key, this is ok
		err := app.Run([]string{"chain", "sethead", ts.Key().Cids()[0].String()})
		assert.NoError(t, err)
	})
}

func TestInspectUsage(t *testing.T) {
	cmd := WithCategory("chain", ChainInspectUsage)
	ts := mock.TipSet(mock.MkBlock(nil, 0, 0))

	from, err := mock.RandomActorAddress()
	assert.NoError(t, err)

	to, err := mock.RandomActorAddress()
	assert.NoError(t, err)

	msg := mock.UnsignedMessage(*from, *to, 0)
	msgs := []api.Message{{Cid: msg.Cid(), Message: msg}}

	actor := &types.Actor{
		Code:    builtin.StorageMarketActorCodeID,
		Nonce:   0,
		Balance: big.NewInt(1000000000),
	}

	t.Run("default", func(t *testing.T) {
		app, mockApi, buf, done := NewMockAppWithFullAPI(t, cmd)
		defer done()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		gomock.InOrder(
			mockApi.EXPECT().ChainHead(ctx).Return(ts, nil),
			mockApi.EXPECT().ChainGetParentMessages(ctx, ts.Blocks()[0].Cid()).Return(msgs, nil),
			mockApi.EXPECT().ChainGetTipSet(ctx, ts.Parents()).Return(nil, nil),
			mockApi.EXPECT().StateGetActor(ctx, *to, ts.Key()).Return(actor, nil),
		)

		err := app.Run([]string{"chain", "inspect-usage"})
		assert.NoError(t, err)

		out := buf.String()

		// output is plaintext, had to do string matching
		assert.Contains(t, out, from.String())
		assert.Contains(t, out, to.String())
		assert.Contains(t, out, "Send")
	})
}

func TestChainList(t *testing.T) {
	cmd := WithCategory("chain", ChainListCmd)
	genesis := mock.TipSet(mock.MkBlock(nil, 0, 0))
	blk := mock.MkBlock(genesis, 0, 0)
	blk.Height = 1
	head := mock.TipSet(blk)

	head.Height()

	from, err := mock.RandomActorAddress()
	assert.NoError(t, err)

	to, err := mock.RandomActorAddress()
	assert.NoError(t, err)

	msg := mock.UnsignedMessage(*from, *to, 0)
	msgs := []api.Message{{Cid: msg.Cid(), Message: msg}}
	blockMsgs := &api.BlockMessages{}
	receipts := []*types.MessageReceipt{}

	t.Run("default", func(t *testing.T) {
		app, mockApi, buf, done := NewMockAppWithFullAPI(t, cmd)
		defer done()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// same method gets called mocked multiple times bcs it's called in a for loop for all tipsets (2 in this case)
		gomock.InOrder(
			mockApi.EXPECT().ChainHead(ctx).Return(head, nil),
			mockApi.EXPECT().ChainGetTipSet(ctx, head.Parents()).Return(genesis, nil),
			mockApi.EXPECT().ChainGetBlockMessages(ctx, genesis.Blocks()[0].Cid()).Return(blockMsgs, nil),
			mockApi.EXPECT().ChainGetParentMessages(ctx, head.Blocks()[0].Cid()).Return(msgs, nil),
			mockApi.EXPECT().ChainGetParentReceipts(ctx, head.Blocks()[0].Cid()).Return(receipts, nil),
			mockApi.EXPECT().ChainGetBlockMessages(ctx, head.Blocks()[0].Cid()).Return(blockMsgs, nil),
		)

		err := app.Run([]string{"chain", "love", "--gas-stats=true"}) // chain is love ❤️
		assert.NoError(t, err)

		out := buf.String()

		// should print out 2 blocks, indexed with 0: and 1:
		assert.Contains(t, out, "0:")
		assert.Contains(t, out, "1:")
	})
}

func TestChainGet(t *testing.T) {
	blk := mock.MkBlock(nil, 0, 0)
	ts := mock.TipSet(blk)
	cmd := WithCategory("chain", ChainGetCmd)

	// given no -as-type flag & ipfs prefix, should print object as JSON if it's marshalable
	t.Run("ipfs", func(t *testing.T) {
		path := fmt.Sprintf("/ipfs/%s", blk.Cid().String())

		app, mockApi, buf, done := NewMockAppWithFullAPI(t, cmd)
		defer done()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		gomock.InOrder(
			mockApi.EXPECT().ChainGetNode(ctx, path).Return(&api.IpldObject{Cid: blk.Cid(), Obj: blk}, nil),
		)

		err := app.Run([]string{"chain", "get", path})
		assert.NoError(t, err)

		var out types.BlockHeader
		err = json.Unmarshal(buf.Bytes(), &out)
		assert.NoError(t, err)
		assert.Equal(t, *blk, out)
	})

	// given no -as-type flag & ipfs prefix, should traverse from head.ParentStateRoot and print JSON if it's marshalable
	t.Run("pstate", func(t *testing.T) {
		p1 := "/pstate"
		p2 := fmt.Sprintf("/ipfs/%s", ts.ParentState().String())

		app, mockApi, buf, done := NewMockAppWithFullAPI(t, cmd)
		defer done()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		gomock.InOrder(
			mockApi.EXPECT().ChainHead(ctx).Return(ts, nil),
			mockApi.EXPECT().ChainGetNode(ctx, p2).Return(&api.IpldObject{Cid: blk.Cid(), Obj: blk}, nil),
		)

		err := app.Run([]string{"chain", "get", p1})
		assert.NoError(t, err)

		var out types.BlockHeader
		err = json.Unmarshal(buf.Bytes(), &out)
		assert.NoError(t, err)
		assert.Equal(t, *blk, out)
	})

	// given an unknown -as-type value, return an error
	t.Run("unknown-type", func(t *testing.T) {
		app, mockApi, _, done := NewMockAppWithFullAPI(t, cmd)
		defer done()

		path := fmt.Sprintf("/ipfs/%s", blk.Cid().String())

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		gomock.InOrder(
			mockApi.EXPECT().ChainGetNode(ctx, path).Return(&api.IpldObject{Cid: blk.Cid(), Obj: blk}, nil),
		)

		err := app.Run([]string{"chain", "get", "-as-type=foo", path})
		assert.Error(t, err)
	})
}

func TestChainBisect(t *testing.T) {
	blk1 := mock.MkBlock(nil, 0, 0)
	blk1.Height = 0
	ts1 := mock.TipSet(blk1)

	blk2 := mock.MkBlock(ts1, 0, 0)
	blk2.Height = 1
	ts2 := mock.TipSet(blk2)

	subpath := "whatever/its/mocked"
	minHeight := uint64(0)
	maxHeight := uint64(1)
	shell := "echo"

	path := fmt.Sprintf("/ipld/%s/%s", ts2.ParentState(), subpath)

	cmd := WithCategory("chain", ChainBisectCmd)

	app, mockApi, buf, done := NewMockAppWithFullAPI(t, cmd)
	defer done()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	gomock.InOrder(
		mockApi.EXPECT().ChainGetTipSetByHeight(ctx, abi.ChainEpoch(maxHeight), types.EmptyTSK).Return(ts2, nil),
		mockApi.EXPECT().ChainGetTipSetByHeight(ctx, abi.ChainEpoch(maxHeight), ts2.Key()).Return(ts2, nil),
		mockApi.EXPECT().ChainGetNode(ctx, path).Return(&api.IpldObject{Cid: blk2.Cid(), Obj: blk2}, nil),
	)

	err := app.Run([]string{"chain", "bisect", fmt.Sprintf("%d", minHeight), fmt.Sprintf("%d", maxHeight), subpath, shell})
	assert.NoError(t, err)

	out := buf.String()
	assert.Contains(t, out, path)
}

func TestChainExport(t *testing.T) {}

func TestSlashConsensusFault(t *testing.T) {}

func TestChainGasPrice(t *testing.T) {}

func TestChainDecode(t *testing.T) {}

func TestChainEncode(t *testing.T) {}
