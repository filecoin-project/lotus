package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/api/mocks"
	types "github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/types/mock"
	"github.com/golang/mock/gomock"
	cid "github.com/ipfs/go-cid"
	"github.com/stretchr/testify/assert"
	ucli "github.com/urfave/cli/v2"
)

// newMockAppWithFullAPI returns a gomock-ed CLI app used for unit tests
// see cli/util/api.go:GetFullNodeAPI for mock API injection
func newMockAppWithFullAPI(t *testing.T, cmd *ucli.Command) (*ucli.App, *mocks.MockFullNode, *bytes.Buffer, func()) {
	app := ucli.NewApp()
	app.Commands = ucli.Commands{cmd}
	app.Setup()

	// create and inject the mock API into app Metadata
	ctrl := gomock.NewController(t)
	mockFullNode := mocks.NewMockFullNode(ctrl)
	var fullNode api.FullNode = mockFullNode
	app.Metadata["test-full-api"] = fullNode

	// this will only work if the implementation uses the app.Writer,
	// if it uses fmt.*, it has to be refactored
	buf := &bytes.Buffer{}
	app.Writer = buf

	return app, mockFullNode, buf, ctrl.Finish
}

func TestChainHead(t *testing.T) {
	app, mockApi, buf, done := newMockAppWithFullAPI(t, WithCategory("chain", ChainHeadCmd))
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

// TestGetBlock checks if "chain getblock" returns the block information in the expected format
func TestGetBlock(t *testing.T) {
	app, mockApi, buf, done := newMockAppWithFullAPI(t, WithCategory("chain", ChainGetBlock))
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

// TestChainReadObj checks if "chain read-obj" prints the referenced IPLD node as hex, if exists
func TestReadOjb(t *testing.T) {
	app, mockApi, buf, done := newMockAppWithFullAPI(t, WithCategory("chain", ChainReadObjCmd))
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

// TestChainDeleteObj checks if "chain delete-obj" deletes an object from the chain blockstore, respecting the --really-do-it flag
func TestChainDeleteObj(t *testing.T) {
	cmd := WithCategory("chain", ChainDeleteObjCmd)
	block := mock.MkBlock(nil, 0, 0)

	// given no force flag, it should return an error and no API calls should be made
	t.Run("no-really-do-it", func(t *testing.T) {
		app, _, _, done := newMockAppWithFullAPI(t, cmd)
		defer done()

		err := app.Run([]string{"chain", "delete-obj", block.Cid().String()})
		assert.Error(t, err)
	})

	// given a force flag, it calls API delete
	t.Run("really-do-it", func(t *testing.T) {
		app, mockApi, buf, done := newMockAppWithFullAPI(t, cmd)
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

// TestChainStatObj checks if "chain delete-obj" prints size and IPLD link counts for object, respecting the --base flag
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
		app, mockApi, buf, done := newMockAppWithFullAPI(t, cmd)
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
		app, mockApi, buf, done := newMockAppWithFullAPI(t, cmd)
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

// TestChainGetMsg checks if "chain getmessage" properly decodes and serializes as JSON a Message fetched from the IPLD store
func TestChainGetMsg(t *testing.T) {
	app, mockApi, buf, done := newMockAppWithFullAPI(t, WithCategory("chain", ChainGetMsgCmd))
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
