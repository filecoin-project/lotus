package cli

import (
	"bytes"
	"context"
	"regexp"
	"testing"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/api/mocks"
	"github.com/filecoin-project/lotus/chain/types/mock"
	"github.com/golang/mock/gomock"
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
