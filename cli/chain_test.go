package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"regexp"
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

// TestGetBlock checks if "lotus chain getblock" returns the block information in the expected format
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
