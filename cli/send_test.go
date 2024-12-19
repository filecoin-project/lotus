package cli

import (
	"bytes"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
	ucli "github.com/urfave/cli/v2"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/builtin"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/types/ethtypes"
)

func mustAddr(a address.Address, err error) address.Address {
	if err != nil {
		panic(err)
	}
	return a
}

func newMockApp(t *testing.T, cmd *ucli.Command) (*ucli.App, *MockServicesAPI, *bytes.Buffer, func()) {
	app := ucli.NewApp()
	app.Commands = ucli.Commands{cmd}
	app.Setup()

	mockCtrl := gomock.NewController(t)
	mockSrvcs := NewMockServicesAPI(mockCtrl)
	app.Metadata["test-services"] = mockSrvcs

	buf := &bytes.Buffer{}
	app.Writer = buf

	return app, mockSrvcs, buf, mockCtrl.Finish
}

func TestSendCLI(t *testing.T) {
	oneFil := abi.TokenAmount(types.MustParseFIL("1"))

	t.Run("simple", func(t *testing.T) {
		app, mockSrvcs, buf, done := newMockApp(t, SendCmd)
		defer done()

		arbtProto := &api.MessagePrototype{
			Message: types.Message{
				From:  mustAddr(address.NewIDAddress(1)),
				To:    mustAddr(address.NewIDAddress(1)),
				Value: oneFil,
			},
		}
		sigMsg := fakeSign(&arbtProto.Message)

		gomock.InOrder(
			mockSrvcs.EXPECT().MessageForSend(gomock.Any(), SendParams{
				From: mustAddr(address.NewIDAddress(1)),
				To:   mustAddr(address.NewIDAddress(1)),
				Val:  oneFil,
			}).Return(arbtProto, nil),
			mockSrvcs.EXPECT().PublishMessage(gomock.Any(), arbtProto, false).
				Return(sigMsg, nil, nil),
			mockSrvcs.EXPECT().Close(),
		)
		err := app.Run([]string{"lotus", "send", "--from", "t01", "t01", "1"})
		require.NoError(t, err)
		require.Contains(t, buf.String(), sigMsg.Cid().String()+"\n")
	})
}

func TestSendEthereum(t *testing.T) {
	oneFil := abi.TokenAmount(types.MustParseFIL("1"))

	t.Run("simple", func(t *testing.T) {
		app, mockSrvcs, buf, done := newMockApp(t, SendCmd)
		defer done()

		testEthAddr, err := ethtypes.CastEthAddress(make([]byte, 20))
		require.NoError(t, err)
		testAddr := mustAddr(testEthAddr.ToFilecoinAddress())

		params := abi.CborBytes([]byte{1, 2, 3, 4})
		var paramsBuf bytes.Buffer
		require.NoError(t, params.MarshalCBOR(&paramsBuf))

		arbtProto := &api.MessagePrototype{
			Message: types.Message{
				From:   testAddr,
				To:     mustAddr(address.NewIDAddress(1)),
				Value:  oneFil,
				Method: builtin.MethodsEVM.InvokeContract,
				Params: paramsBuf.Bytes(),
			},
		}
		sigMsg := fakeSign(&arbtProto.Message)

		gomock.InOrder(
			mockSrvcs.EXPECT().MessageForSend(gomock.Any(), SendParams{
				From:   testAddr,
				To:     mustAddr(address.NewIDAddress(1)),
				Val:    oneFil,
				Method: builtin.MethodsEVM.InvokeContract,
				Params: paramsBuf.Bytes(),
			}).Return(arbtProto, nil),
			mockSrvcs.EXPECT().PublishMessage(gomock.Any(), arbtProto, false).
				Return(sigMsg, nil, nil),
			mockSrvcs.EXPECT().Close(),
		)
		err = app.Run([]string{"lotus", "send", "--from-eth-addr", testEthAddr.String(), "--params-hex", "01020304", "f01", "1"})
		require.NoError(t, err)
		require.Contains(t, buf.String(), sigMsg.Cid().String()+"\n")
	})
}
