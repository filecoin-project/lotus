package cli

import (
	"bytes"
	"errors"
	"testing"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	types "github.com/filecoin-project/lotus/chain/types"
	gomock "github.com/golang/mock/gomock"
	cid "github.com/ipfs/go-cid"
	"github.com/stretchr/testify/assert"
	ucli "github.com/urfave/cli/v2"
)

var arbtCid = (&types.Message{
	From:  mustAddr(address.NewIDAddress(2)),
	To:    mustAddr(address.NewIDAddress(1)),
	Value: types.NewInt(1000),
}).Cid()

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
		app, mockSrvcs, buf, done := newMockApp(t, sendCmd)
		defer done()

		gomock.InOrder(
			mockSrvcs.EXPECT().Send(gomock.Any(), SendParams{
				To:  mustAddr(address.NewIDAddress(1)),
				Val: oneFil,
			}).Return(arbtCid, nil),
			mockSrvcs.EXPECT().Close(),
		)
		err := app.Run([]string{"lotus", "send", "t01", "1"})
		assert.NoError(t, err)
		assert.EqualValues(t, arbtCid.String()+"\n", buf.String())
	})
	t.Run("ErrSendBalanceTooLow", func(t *testing.T) {
		app, mockSrvcs, _, done := newMockApp(t, sendCmd)
		defer done()

		gomock.InOrder(
			mockSrvcs.EXPECT().Send(gomock.Any(), SendParams{
				To:  mustAddr(address.NewIDAddress(1)),
				Val: oneFil,
			}).Return(cid.Undef, ErrSendBalanceTooLow),
			mockSrvcs.EXPECT().Close(),
		)
		err := app.Run([]string{"lotus", "send", "t01", "1"})
		assert.ErrorIs(t, err, ErrSendBalanceTooLow)
	})
	t.Run("generic-err-is-forwarded", func(t *testing.T) {
		app, mockSrvcs, _, done := newMockApp(t, sendCmd)
		defer done()

		errMark := errors.New("something")
		gomock.InOrder(
			mockSrvcs.EXPECT().Send(gomock.Any(), SendParams{
				To:  mustAddr(address.NewIDAddress(1)),
				Val: oneFil,
			}).Return(cid.Undef, errMark),
			mockSrvcs.EXPECT().Close(),
		)
		err := app.Run([]string{"lotus", "send", "t01", "1"})
		assert.ErrorIs(t, err, errMark)
	})

	t.Run("from-specific", func(t *testing.T) {
		app, mockSrvcs, buf, done := newMockApp(t, sendCmd)
		defer done()

		gomock.InOrder(
			mockSrvcs.EXPECT().Send(gomock.Any(), SendParams{
				To:   mustAddr(address.NewIDAddress(1)),
				From: mustAddr(address.NewIDAddress(2)),
				Val:  oneFil,
			}).Return(arbtCid, nil),
			mockSrvcs.EXPECT().Close(),
		)
		err := app.Run([]string{"lotus", "send", "--from=t02", "t01", "1"})
		assert.NoError(t, err)
		assert.EqualValues(t, arbtCid.String()+"\n", buf.String())
	})

	t.Run("nonce-specific", func(t *testing.T) {
		app, mockSrvcs, buf, done := newMockApp(t, sendCmd)
		defer done()
		zero := uint64(0)

		gomock.InOrder(
			mockSrvcs.EXPECT().Send(gomock.Any(), SendParams{
				To:    mustAddr(address.NewIDAddress(1)),
				Nonce: &zero,
				Val:   oneFil,
			}).Return(arbtCid, nil),
			mockSrvcs.EXPECT().Close(),
		)
		err := app.Run([]string{"lotus", "send", "--nonce=0", "t01", "1"})
		assert.NoError(t, err)
		assert.EqualValues(t, arbtCid.String()+"\n", buf.String())
	})

}
