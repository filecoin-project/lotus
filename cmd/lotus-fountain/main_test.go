package main

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	rice "github.com/GeertJohan/go.rice"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/crypto"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/api/v0api"
	"github.com/filecoin-project/lotus/chain/types"
)

func TestFountainDataCapRemoved(t *testing.T) {
	captchaCalls := stubFountainCaptcha(t)
	node := &fountainNodeStub{}
	routes := newFountainTestHandler(t, node).routes()

	for _, path := range []string{"/datacap", "/datacap.html"} {
		for _, method := range []string{http.MethodGet, http.MethodPost} {
			t.Run(method+path, func(t *testing.T) {
				form := url.Values{"g-recaptcha-response": {"test-token"}, "address": {"t0101"}}
				req := httptest.NewRequest(method, path, strings.NewReader(form.Encode()))
				req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
				res := httptest.NewRecorder()

				routes.ServeHTTP(res, req)

				require.Equal(t, http.StatusNotFound, res.Code)
				require.Zero(t, captchaCalls.Load(), "removed routes must not call captcha verification")
				require.Empty(t, node.messages, "removed routes must not submit messages")
			})
		}
	}
}

func TestFountainSend(t *testing.T) {
	captchaCalls := stubFountainCaptcha(t)
	native, err := address.NewIDAddress(1234)
	require.NoError(t, err)
	eth, err := address.NewDelegatedAddress(10, bytes.Repeat([]byte{0x11}, 20))
	require.NoError(t, err)

	for _, tc := range []struct {
		name  string
		input string
		want  address.Address
	}{
		{name: "native", input: native.String(), want: native},
		{name: "ethereum", input: "0x1111111111111111111111111111111111111111", want: eth},
	} {
		t.Run(tc.name, func(t *testing.T) {
			node := &fountainNodeStub{}
			h := newFountainTestHandler(t, node)
			form := url.Values{"g-recaptcha-response": {"test-token"}, "address": {tc.input}}
			req := httptest.NewRequest(http.MethodPost, "/send", strings.NewReader(form.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			res := httptest.NewRecorder()

			h.routes().ServeHTTP(res, req)

			require.Equal(t, http.StatusOK, res.Code, res.Body.String())
			require.Len(t, node.messages, 1)
			msg := node.messages[0]
			require.Equal(t, h.from, msg.From)
			require.Equal(t, tc.want, msg.To)
			require.Equal(t, types.BigInt(h.sendPerRequest), msg.Value)
			require.Zero(t, msg.Method)
			require.Empty(t, msg.Params)
			require.Equal(t, "text/html", res.Header().Get("Content-Type"))
			require.Contains(t, res.Body.String(), node.signed.Cid().String())
		})
	}
	require.EqualValues(t, 2, captchaCalls.Load())
}

func newFountainTestHandler(t *testing.T, node *fountainNodeStub) *handler {
	t.Helper()
	box, err := rice.FindBox("site")
	require.NoError(t, err)
	from, err := address.NewIDAddress(100)
	require.NoError(t, err)
	amount, err := types.ParseFIL("50")
	require.NoError(t, err)
	return &handler{
		ctx:            context.Background(),
		api:            node,
		from:           from,
		sendPerRequest: amount,
		limiter: NewLimiter(LimiterConfig{
			TotalRate:   time.Hour,
			TotalBurst:  1,
			IPRate:      time.Hour,
			IPBurst:     1,
			WalletRate:  time.Hour,
			WalletBurst: 1,
		}),
		recapThreshold: 0.5,
		box:            box,
	}
}

func stubFountainCaptcha(t *testing.T) *atomic.Int32 {
	t.Helper()
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		_, _ = w.Write([]byte(`{"success":true,"score":1}`))
	}))
	t.Cleanup(server.Close)
	previous := VerifyURL
	t.Cleanup(func() { VerifyURL = previous })
	var err error
	VerifyURL, err = url.Parse(server.URL)
	require.NoError(t, err)
	return &calls
}

type fountainNodeStub struct {
	v0api.FullNode
	messages []*types.Message
	signed   *types.SignedMessage
}

func (n *fountainNodeStub) MpoolPushMessage(_ context.Context, msg *types.Message, _ *api.MessageSendSpec) (*types.SignedMessage, error) {
	n.messages = append(n.messages, msg)
	n.signed = &types.SignedMessage{
		Message:   *msg,
		Signature: crypto.Signature{Type: crypto.SigTypeBLS},
	}
	n.signed.Message.GasFeeCap = types.NewInt(0)
	n.signed.Message.GasPremium = types.NewInt(0)
	return n.signed, nil
}
