package cli

import (
	"context"
	"fmt"
	"testing"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/crypto"
	"github.com/filecoin-project/lotus/api"
	mocks "github.com/filecoin-project/lotus/api/v0api/v0mocks"
	types "github.com/filecoin-project/lotus/chain/types"
	gomock "github.com/golang/mock/gomock"
	cid "github.com/ipfs/go-cid"
	"github.com/stretchr/testify/assert"
)

type markerKeyType struct{}

var markerKey = markerKeyType{}

type contextMatcher struct {
	marker *int
}

// Matches returns whether x is a match.
func (cm contextMatcher) Matches(x interface{}) bool {
	ctx, ok := x.(context.Context)
	if !ok {
		return false
	}
	maybeMarker, ok := ctx.Value(markerKey).(*int)
	if !ok {
		return false
	}

	return cm.marker == maybeMarker
}

func (cm contextMatcher) String() string {
	return fmt.Sprintf("Context with Value(%v/%T, %p)", markerKey, markerKey, cm.marker)
}

func ContextWithMarker(ctx context.Context) (context.Context, gomock.Matcher) {
	marker := new(int)
	outCtx := context.WithValue(ctx, markerKey, marker)
	return outCtx, contextMatcher{marker: marker}

}

func setupMockSrvcs(t *testing.T) (*ServicesImpl, *mocks.MockFullNode) {
	mockCtrl := gomock.NewController(t)

	mockApi := mocks.NewMockFullNode(mockCtrl)

	srvcs := &ServicesImpl{
		api:    mockApi,
		closer: mockCtrl.Finish,
	}
	return srvcs, mockApi
}

func fakeSign(msg *types.Message) *types.SignedMessage {
	return &types.SignedMessage{
		Message:   *msg,
		Signature: crypto.Signature{Type: crypto.SigTypeSecp256k1, Data: make([]byte, 32)},
	}
}

func makeMessageSigner() (*cid.Cid, interface{}) {
	smCid := cid.Undef
	return &smCid,
		func(_ context.Context, msg *types.Message, _ *api.MessageSendSpec) (*types.SignedMessage, error) {
			sm := fakeSign(msg)
			smCid = sm.Cid()
			return sm, nil
		}
}

type MessageMatcher SendParams

var _ gomock.Matcher = MessageMatcher{}

// Matches returns whether x is a match.
func (mm MessageMatcher) Matches(x interface{}) bool {
	m, ok := x.(*types.Message)
	if !ok {
		return false
	}

	if mm.From != address.Undef && mm.From != m.From {
		return false
	}
	if mm.To != address.Undef && mm.To != m.To {
		return false
	}

	if types.BigCmp(mm.Val, m.Value) != 0 {
		return false
	}

	if mm.Nonce != nil && *mm.Nonce != m.Nonce {
		return false
	}

	if mm.GasPremium != nil && big.Cmp(*mm.GasPremium, m.GasPremium) != 0 {
		return false
	}
	if mm.GasPremium == nil && m.GasPremium.Sign() != 0 {
		return false
	}

	if mm.GasFeeCap != nil && big.Cmp(*mm.GasFeeCap, m.GasFeeCap) != 0 {
		return false
	}
	if mm.GasFeeCap == nil && m.GasFeeCap.Sign() != 0 {
		return false
	}

	if mm.GasLimit != nil && *mm.GasLimit != m.GasLimit {
		return false
	}

	if mm.GasLimit == nil && m.GasLimit != 0 {
		return false
	}
	// handle rest of options
	return true
}

// String describes what the matcher matches.
func (mm MessageMatcher) String() string {
	return fmt.Sprintf("%#v", SendParams(mm))
}

func TestSendService(t *testing.T) {
	addrGen := address.NewForTestGetter()
	a1 := addrGen()
	a2 := addrGen()

	const balance = 10000

	params := SendParams{
		From: a1,
		To:   a2,
		Val:  types.NewInt(balance - 100),
	}

	ctx, ctxM := ContextWithMarker(context.Background())

	t.Run("happy", func(t *testing.T) {
		params := params
		srvcs, mockApi := setupMockSrvcs(t)
		defer srvcs.Close() //nolint:errcheck
		msgCid, sign := makeMessageSigner()
		gomock.InOrder(
			mockApi.EXPECT().WalletBalance(ctxM, params.From).Return(types.NewInt(balance), nil),
			mockApi.EXPECT().MpoolPushMessage(ctxM, MessageMatcher(params), nil).DoAndReturn(sign),
		)

		c, err := srvcs.Send(ctx, params)
		assert.NoError(t, err)
		assert.Equal(t, *msgCid, c)
	})

	t.Run("balance-too-low", func(t *testing.T) {
		params := params
		srvcs, mockApi := setupMockSrvcs(t)
		defer srvcs.Close() //nolint:errcheck
		gomock.InOrder(
			mockApi.EXPECT().WalletBalance(ctxM, a1).Return(types.NewInt(balance-200), nil),
			// no MpoolPushMessage
		)

		c, err := srvcs.Send(ctx, params)
		assert.Equal(t, c, cid.Undef)
		assert.ErrorIs(t, err, ErrSendBalanceTooLow)
	})

	t.Run("force", func(t *testing.T) {
		params := params
		params.Force = true
		srvcs, mockApi := setupMockSrvcs(t)
		defer srvcs.Close() //nolint:errcheck
		msgCid, sign := makeMessageSigner()
		gomock.InOrder(
			mockApi.EXPECT().WalletBalance(ctxM, a1).Return(types.NewInt(balance-200), nil).AnyTimes(),
			mockApi.EXPECT().MpoolPushMessage(ctxM, MessageMatcher(params), nil).DoAndReturn(sign),
		)

		c, err := srvcs.Send(ctx, params)
		assert.NoError(t, err)
		assert.Equal(t, *msgCid, c)
	})

	t.Run("default-from", func(t *testing.T) {
		params := params
		params.From = address.Undef
		mm := MessageMatcher(params)
		mm.From = a1

		srvcs, mockApi := setupMockSrvcs(t)
		defer srvcs.Close() //nolint:errcheck
		msgCid, sign := makeMessageSigner()
		gomock.InOrder(
			mockApi.EXPECT().WalletDefaultAddress(ctxM).Return(a1, nil),
			mockApi.EXPECT().WalletBalance(ctxM, a1).Return(types.NewInt(balance), nil),
			mockApi.EXPECT().MpoolPushMessage(ctxM, mm, nil).DoAndReturn(sign),
		)

		c, err := srvcs.Send(ctx, params)
		assert.NoError(t, err)
		assert.Equal(t, *msgCid, c)
	})

	t.Run("set-nonce", func(t *testing.T) {
		params := params
		n := uint64(5)
		params.Nonce = &n
		mm := MessageMatcher(params)

		srvcs, mockApi := setupMockSrvcs(t)
		defer srvcs.Close() //nolint:errcheck
		_, _ = mm, mockApi

		var sm *types.SignedMessage
		gomock.InOrder(
			mockApi.EXPECT().WalletBalance(ctxM, a1).Return(types.NewInt(balance), nil),
			mockApi.EXPECT().WalletSignMessage(ctxM, a1, mm).DoAndReturn(
				func(_ context.Context, _ address.Address, msg *types.Message) (*types.SignedMessage, error) {
					sm = fakeSign(msg)

					// now we expect MpoolPush with that SignedMessage
					mockApi.EXPECT().MpoolPush(ctxM, sm).Return(sm.Cid(), nil)
					return sm, nil
				}),
		)

		c, err := srvcs.Send(ctx, params)
		assert.NoError(t, err)
		assert.Equal(t, sm.Cid(), c)
	})

	t.Run("gas-params", func(t *testing.T) {
		params := params
		limit := int64(1)
		params.GasLimit = &limit
		gfc := big.NewInt(100)
		params.GasFeeCap = &gfc
		gp := big.NewInt(10)
		params.GasPremium = &gp

		srvcs, mockApi := setupMockSrvcs(t)
		defer srvcs.Close() //nolint:errcheck
		msgCid, sign := makeMessageSigner()
		gomock.InOrder(
			mockApi.EXPECT().WalletBalance(ctxM, params.From).Return(types.NewInt(balance), nil),
			mockApi.EXPECT().MpoolPushMessage(ctxM, MessageMatcher(params), nil).DoAndReturn(sign),
		)

		c, err := srvcs.Send(ctx, params)
		assert.NoError(t, err)
		assert.Equal(t, *msgCid, c)
	})
}
