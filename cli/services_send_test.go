package cli

import (
	"context"
	"testing"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/crypto"
	"github.com/filecoin-project/lotus/api/mocks"
	types "github.com/filecoin-project/lotus/chain/types"
	gomock "github.com/golang/mock/gomock"
	cid "github.com/ipfs/go-cid"
	"github.com/stretchr/testify/assert"
)

func TestSendService(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockApi := mocks.NewMockFullNode(mockCtrl)

	srvcs := &ServicesImpl{
		api:    mockApi,
		closer: func() {},
	}

	addrGen := address.NewForTestGetter()
	a1 := addrGen()
	a2 := addrGen()

	const balance = 10000

	params := SendParams{
		From: a1,
		To:   a2,
		Val:  types.NewInt(balance - 100),
	}
	ctx, done := context.WithCancel(context.Background())
	defer done()

	msgCid := cid.Undef
	gomock.InOrder(
		mockApi.EXPECT().WalletBalance(ctx, params.From).Return(types.NewInt(balance), nil),
		mockApi.EXPECT().MpoolPushMessage(ctx, gomock.Any(), nil).DoAndReturn(
			func(_ context.Context, msg *types.Message, _ interface{}) (*types.SignedMessage, error) {
				msgCid = msg.Cid()
				assert.Equal(t, params.From, msg.From)
				assert.Equal(t, params.To, msg.To)
				assert.Equal(t, params.Val, msg.Value)

				sm := types.SignedMessage{
					Message:   *msg,
					Signature: crypto.Signature{1, []byte{1}},
				}
				msgCid = sm.Cid()
				return &sm, nil
			}),
	)

	c, err := srvcs.Send(ctx, params)
	assert.NoError(t, err)
	assert.Equal(t, msgCid, c)

}
