package cli

import (
	"bytes"
	"testing"

	"github.com/golang/mock/gomock"
	ucli "github.com/urfave/cli/v2"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/api/mocks"
)

// NewMockAppWithFullAPI returns a gomock-ed CLI app used for unit tests
// see cli/util/api.go:GetFullNodeAPI for mock API injection
func NewMockAppWithFullAPI(t *testing.T, cmd *ucli.Command) (*ucli.App, *mocks.MockFullNode, *bytes.Buffer, func()) {
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
