package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/messagepool"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/types/mock"
	"github.com/filecoin-project/lotus/chain/wallet"
)

func TestStat(t *testing.T) {

	t.Run("local", func(t *testing.T) {
		app, mockApi, buf, done := NewMockAppWithFullAPI(t, WithCategory("mpool", MpoolStat))
		defer done()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// add blocks to the chain
		first := mock.TipSet(mock.MkBlock(nil, 5, 4))
		head := mock.TipSet(mock.MkBlock(first, 15, 7))

		// create a signed message to be returned as a pending message
		w, _ := wallet.NewWallet(wallet.NewMemKeyStore())
		senderAddr, err := w.WalletNew(context.Background(), types.KTSecp256k1)
		if err != nil {
			t.Fatal(err)
		}
		toAddr, err := w.WalletNew(context.Background(), types.KTSecp256k1)
		if err != nil {
			t.Fatal(err)
		}
		sm := mock.MkMessage(senderAddr, toAddr, 1, w)

		// mock actor to return for the sender
		actor := types.Actor{Nonce: 2, Balance: big.NewInt(200000)}

		gomock.InOrder(
			mockApi.EXPECT().ChainHead(ctx).Return(head, nil),
			mockApi.EXPECT().ChainGetTipSet(ctx, head.Parents()).Return(first, nil),
			mockApi.EXPECT().WalletList(ctx).Return([]address.Address{senderAddr, toAddr}, nil),
			mockApi.EXPECT().MpoolPending(ctx, types.EmptyTSK).Return([]*types.SignedMessage{sm}, nil),
			mockApi.EXPECT().StateGetActor(ctx, senderAddr, head.Key()).Return(&actor, nil),
		)

		err = app.Run([]string{"mpool", "stat", "--basefee-lookback", "1", "--local"})
		assert.NoError(t, err)

		assert.Contains(t, buf.String(), "Nonce past: 1")
	})

	t.Run("all", func(t *testing.T) {
		app, mockApi, buf, done := NewMockAppWithFullAPI(t, WithCategory("mpool", MpoolStat))
		defer done()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// add blocks to the chain
		first := mock.TipSet(mock.MkBlock(nil, 5, 4))
		head := mock.TipSet(mock.MkBlock(first, 15, 7))

		// create a signed message to be returned as a pending message
		w, _ := wallet.NewWallet(wallet.NewMemKeyStore())
		senderAddr, err := w.WalletNew(context.Background(), types.KTSecp256k1)
		if err != nil {
			t.Fatal(err)
		}
		toAddr, err := w.WalletNew(context.Background(), types.KTSecp256k1)
		if err != nil {
			t.Fatal(err)
		}
		sm := mock.MkMessage(senderAddr, toAddr, 1, w)

		// mock actor to return for the sender
		actor := types.Actor{Nonce: 2, Balance: big.NewInt(200000)}

		gomock.InOrder(
			mockApi.EXPECT().ChainHead(ctx).Return(head, nil),
			mockApi.EXPECT().ChainGetTipSet(ctx, head.Parents()).Return(first, nil),
			mockApi.EXPECT().MpoolPending(ctx, types.EmptyTSK).Return([]*types.SignedMessage{sm}, nil),
			mockApi.EXPECT().StateGetActor(ctx, senderAddr, head.Key()).Return(&actor, nil),
		)

		err = app.Run([]string{"mpool", "stat", "--basefee-lookback", "1"})
		assert.NoError(t, err)

		assert.Contains(t, buf.String(), "Nonce past: 1")
	})
}

func TestPending(t *testing.T) {
	t.Run("all", func(t *testing.T) {

		app, mockApi, buf, done := NewMockAppWithFullAPI(t, WithCategory("mpool", MpoolPending))
		defer done()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// create a signed message to be returned as a pending message
		w, _ := wallet.NewWallet(wallet.NewMemKeyStore())
		senderAddr, err := w.WalletNew(context.Background(), types.KTSecp256k1)
		if err != nil {
			t.Fatal(err)
		}
		toAddr, err := w.WalletNew(context.Background(), types.KTSecp256k1)
		if err != nil {
			t.Fatal(err)
		}
		sm := mock.MkMessage(senderAddr, toAddr, 1, w)

		gomock.InOrder(
			mockApi.EXPECT().MpoolPending(ctx, types.EmptyTSK).Return([]*types.SignedMessage{sm}, nil),
		)

		err = app.Run([]string{"mpool", "pending", "--cids"})
		assert.NoError(t, err)

		assert.Contains(t, buf.String(), sm.Cid().String())
	})

	t.Run("local", func(t *testing.T) {

		app, mockApi, buf, done := NewMockAppWithFullAPI(t, WithCategory("mpool", MpoolPending))
		defer done()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// create a signed message to be returned as a pending message
		w, _ := wallet.NewWallet(wallet.NewMemKeyStore())
		senderAddr, err := w.WalletNew(context.Background(), types.KTSecp256k1)
		if err != nil {
			t.Fatal(err)
		}
		toAddr, err := w.WalletNew(context.Background(), types.KTSecp256k1)
		if err != nil {
			t.Fatal(err)
		}
		sm := mock.MkMessage(senderAddr, toAddr, 1, w)

		gomock.InOrder(
			mockApi.EXPECT().WalletList(ctx).Return([]address.Address{senderAddr}, nil),
			mockApi.EXPECT().MpoolPending(ctx, types.EmptyTSK).Return([]*types.SignedMessage{sm}, nil),
		)

		err = app.Run([]string{"mpool", "pending", "--local"})
		assert.NoError(t, err)

		assert.Contains(t, buf.String(), sm.Cid().String())
	})

	t.Run("to", func(t *testing.T) {

		app, mockApi, buf, done := NewMockAppWithFullAPI(t, WithCategory("mpool", MpoolPending))
		defer done()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// create a signed message to be returned as a pending message
		w, _ := wallet.NewWallet(wallet.NewMemKeyStore())
		senderAddr, err := w.WalletNew(context.Background(), types.KTSecp256k1)
		if err != nil {
			t.Fatal(err)
		}
		toAddr, err := w.WalletNew(context.Background(), types.KTSecp256k1)
		if err != nil {
			t.Fatal(err)
		}
		sm := mock.MkMessage(senderAddr, toAddr, 1, w)

		gomock.InOrder(
			mockApi.EXPECT().MpoolPending(ctx, types.EmptyTSK).Return([]*types.SignedMessage{sm}, nil),
		)

		err = app.Run([]string{"mpool", "pending", "--to", sm.Message.To.String()})
		assert.NoError(t, err)

		assert.Contains(t, buf.String(), sm.Cid().String())
	})

	t.Run("from", func(t *testing.T) {

		app, mockApi, buf, done := NewMockAppWithFullAPI(t, WithCategory("mpool", MpoolPending))
		defer done()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// create a signed message to be returned as a pending message
		w, _ := wallet.NewWallet(wallet.NewMemKeyStore())
		senderAddr, err := w.WalletNew(context.Background(), types.KTSecp256k1)
		if err != nil {
			t.Fatal(err)
		}
		toAddr, err := w.WalletNew(context.Background(), types.KTSecp256k1)
		if err != nil {
			t.Fatal(err)
		}
		sm := mock.MkMessage(senderAddr, toAddr, 1, w)

		gomock.InOrder(
			mockApi.EXPECT().MpoolPending(ctx, types.EmptyTSK).Return([]*types.SignedMessage{sm}, nil),
		)

		err = app.Run([]string{"mpool", "pending", "--from", sm.Message.From.String()})
		assert.NoError(t, err)

		assert.Contains(t, buf.String(), sm.Cid().String())
	})

}

func TestReplace(t *testing.T) {
	t.Run("manual", func(t *testing.T) {
		app, mockApi, buf, done := NewMockAppWithFullAPI(t, WithCategory("mpool", MpoolReplaceCmd))
		defer done()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// create a signed message to be returned as a pending message
		w, _ := wallet.NewWallet(wallet.NewMemKeyStore())
		senderAddr, err := w.WalletNew(context.Background(), types.KTSecp256k1)
		if err != nil {
			t.Fatal(err)
		}
		toAddr, err := w.WalletNew(context.Background(), types.KTSecp256k1)
		if err != nil {
			t.Fatal(err)
		}
		sm := mock.MkMessage(senderAddr, toAddr, 1, w)

		gomock.InOrder(
			mockApi.EXPECT().ChainGetMessage(ctx, sm.Cid()).Return(&sm.Message, nil),
			mockApi.EXPECT().ChainHead(ctx).Return(nil, nil),
			mockApi.EXPECT().MpoolPending(ctx, types.EmptyTSK).Return([]*types.SignedMessage{sm}, nil),
			mockApi.EXPECT().WalletSignMessage(ctx, sm.Message.From, &sm.Message).Return(sm, nil),
			mockApi.EXPECT().MpoolPush(ctx, sm).Return(sm.Cid(), nil),
		)

		err = app.Run([]string{"mpool", "replace", "--gas-premium", "1", "--gas-feecap", "100", sm.Cid().String()})

		assert.NoError(t, err)
		assert.Contains(t, buf.String(), sm.Cid().String())
	})

	t.Run("auto", func(t *testing.T) {
		app, mockApi, buf, done := NewMockAppWithFullAPI(t, WithCategory("mpool", MpoolReplaceCmd))
		defer done()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// create a signed message to be returned as a pending message
		w, _ := wallet.NewWallet(wallet.NewMemKeyStore())
		senderAddr, err := w.WalletNew(context.Background(), types.KTSecp256k1)
		if err != nil {
			t.Fatal(err)
		}
		toAddr, err := w.WalletNew(context.Background(), types.KTSecp256k1)
		if err != nil {
			t.Fatal(err)
		}
		sm := mock.MkMessage(senderAddr, toAddr, 1, w)

		// gas fee param should be equal to the one passed in the cli invocation (used below)
		maxFee := "1000000"
		parsedFee, err := types.ParseFIL(maxFee)
		if err != nil {
			t.Fatal(err)
		}
		mss := api.MessageSendSpec{MaxFee: abi.TokenAmount(parsedFee)}

		gomock.InOrder(
			mockApi.EXPECT().ChainGetMessage(ctx, sm.Cid()).Return(&sm.Message, nil),
			mockApi.EXPECT().ChainHead(ctx).Return(nil, nil),
			mockApi.EXPECT().MpoolPending(ctx, types.EmptyTSK).Return([]*types.SignedMessage{sm}, nil),
			mockApi.EXPECT().MpoolGetConfig(ctx).Return(messagepool.DefaultConfig(), nil),
			// use gomock.any to match the message in expected api calls
			// since the replace function modifies the message between calls, it would be pointless to try to match the exact argument
			mockApi.EXPECT().GasEstimateMessageGas(ctx, gomock.Any(), &mss, types.EmptyTSK).Return(&sm.Message, nil),
			mockApi.EXPECT().WalletSignMessage(ctx, sm.Message.From, gomock.Any()).Return(sm, nil),
			mockApi.EXPECT().MpoolPush(ctx, sm).Return(sm.Cid(), nil),
		)

		err = app.Run([]string{"mpool", "replace", "--auto", "--fee-limit", maxFee, sm.Cid().String()})

		assert.NoError(t, err)
		assert.Contains(t, buf.String(), sm.Cid().String())
	})

	t.Run("sender / nonce", func(t *testing.T) {
		app, mockApi, buf, done := NewMockAppWithFullAPI(t, WithCategory("mpool", MpoolReplaceCmd))
		defer done()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// create a signed message to be returned as a pending message
		w, _ := wallet.NewWallet(wallet.NewMemKeyStore())
		senderAddr, err := w.WalletNew(context.Background(), types.KTSecp256k1)
		if err != nil {
			t.Fatal(err)
		}
		toAddr, err := w.WalletNew(context.Background(), types.KTSecp256k1)
		if err != nil {
			t.Fatal(err)
		}
		sm := mock.MkMessage(senderAddr, toAddr, 1, w)

		// gas fee param should be equal to the one passed in the cli invocation (used below)
		maxFee := "1000000"
		parsedFee, err := types.ParseFIL(maxFee)
		if err != nil {
			t.Fatal(err)
		}
		mss := api.MessageSendSpec{MaxFee: abi.TokenAmount(parsedFee)}

		gomock.InOrder(
			mockApi.EXPECT().ChainHead(ctx).Return(nil, nil),
			mockApi.EXPECT().MpoolPending(ctx, types.EmptyTSK).Return([]*types.SignedMessage{sm}, nil),
			mockApi.EXPECT().MpoolGetConfig(ctx).Return(messagepool.DefaultConfig(), nil),
			// use gomock.any to match the message in expected api calls
			// since the replace function modifies the message between calls, it would be pointless to try to match the exact argument
			mockApi.EXPECT().GasEstimateMessageGas(ctx, gomock.Any(), &mss, types.EmptyTSK).Return(&sm.Message, nil),
			mockApi.EXPECT().WalletSignMessage(ctx, sm.Message.From, gomock.Any()).Return(sm, nil),
			mockApi.EXPECT().MpoolPush(ctx, sm).Return(sm.Cid(), nil),
		)

		err = app.Run([]string{"mpool", "replace", "--auto", "--fee-limit", maxFee, sm.Message.From.String(), fmt.Sprint(sm.Message.Nonce)})

		assert.NoError(t, err)
		assert.Contains(t, buf.String(), sm.Cid().String())
	})
}

func TestFindMsg(t *testing.T) {
	t.Run("from", func(t *testing.T) {
		app, mockApi, buf, done := NewMockAppWithFullAPI(t, WithCategory("mpool", MpoolFindCmd))
		defer done()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// create a signed message to be returned as a pending message
		w, _ := wallet.NewWallet(wallet.NewMemKeyStore())
		senderAddr, err := w.WalletNew(context.Background(), types.KTSecp256k1)
		if err != nil {
			t.Fatal(err)
		}
		toAddr, err := w.WalletNew(context.Background(), types.KTSecp256k1)
		if err != nil {
			t.Fatal(err)
		}
		sm := mock.MkMessage(senderAddr, toAddr, 1, w)

		gomock.InOrder(
			mockApi.EXPECT().MpoolPending(ctx, types.EmptyTSK).Return([]*types.SignedMessage{sm}, nil),
		)

		err = app.Run([]string{"mpool", "find", "--from", sm.Message.From.String()})

		assert.NoError(t, err)
		assert.Contains(t, buf.String(), sm.Cid().String())
	})

	t.Run("to", func(t *testing.T) {
		app, mockApi, buf, done := NewMockAppWithFullAPI(t, WithCategory("mpool", MpoolFindCmd))
		defer done()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// create a signed message to be returned as a pending message
		w, _ := wallet.NewWallet(wallet.NewMemKeyStore())
		senderAddr, err := w.WalletNew(context.Background(), types.KTSecp256k1)
		if err != nil {
			t.Fatal(err)
		}
		toAddr, err := w.WalletNew(context.Background(), types.KTSecp256k1)
		if err != nil {
			t.Fatal(err)
		}
		sm := mock.MkMessage(senderAddr, toAddr, 1, w)

		gomock.InOrder(
			mockApi.EXPECT().MpoolPending(ctx, types.EmptyTSK).Return([]*types.SignedMessage{sm}, nil),
		)

		err = app.Run([]string{"mpool", "find", "--to", sm.Message.To.String()})

		assert.NoError(t, err)
		assert.Contains(t, buf.String(), sm.Cid().String())
	})

	t.Run("method", func(t *testing.T) {
		app, mockApi, buf, done := NewMockAppWithFullAPI(t, WithCategory("mpool", MpoolFindCmd))
		defer done()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// create a signed message to be returned as a pending message
		w, _ := wallet.NewWallet(wallet.NewMemKeyStore())
		senderAddr, err := w.WalletNew(context.Background(), types.KTSecp256k1)
		if err != nil {
			t.Fatal(err)
		}
		toAddr, err := w.WalletNew(context.Background(), types.KTSecp256k1)
		if err != nil {
			t.Fatal(err)
		}
		sm := mock.MkMessage(senderAddr, toAddr, 1, w)

		gomock.InOrder(
			mockApi.EXPECT().MpoolPending(ctx, types.EmptyTSK).Return([]*types.SignedMessage{sm}, nil),
		)

		err = app.Run([]string{"mpool", "find", "--method", sm.Message.Method.String()})

		assert.NoError(t, err)
		assert.Contains(t, buf.String(), sm.Cid().String())
	})
}

func TestGasPerf(t *testing.T) {
	t.Run("all", func(t *testing.T) {
		app, mockApi, buf, done := NewMockAppWithFullAPI(t, WithCategory("mpool", MpoolGasPerfCmd))
		defer done()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// add blocks to the chain
		first := mock.TipSet(mock.MkBlock(nil, 5, 4))
		head := mock.TipSet(mock.MkBlock(first, 15, 7))

		// create a signed message to be returned as a pending message
		w, _ := wallet.NewWallet(wallet.NewMemKeyStore())
		senderAddr, err := w.WalletNew(context.Background(), types.KTSecp256k1)
		if err != nil {
			t.Fatal(err)
		}
		toAddr, err := w.WalletNew(context.Background(), types.KTSecp256k1)
		if err != nil {
			t.Fatal(err)
		}
		sm := mock.MkMessage(senderAddr, toAddr, 13, w)

		gomock.InOrder(
			mockApi.EXPECT().MpoolPending(ctx, types.EmptyTSK).Return([]*types.SignedMessage{sm}, nil),
			mockApi.EXPECT().ChainHead(ctx).Return(head, nil),
		)

		err = app.Run([]string{"mpool", "gas-perf", "--all", "true"})
		assert.NoError(t, err)

		assert.Contains(t, buf.String(), sm.Message.From.String())
		assert.Contains(t, buf.String(), fmt.Sprint(sm.Message.Nonce))
	})

	t.Run("local", func(t *testing.T) {
		app, mockApi, buf, done := NewMockAppWithFullAPI(t, WithCategory("mpool", MpoolGasPerfCmd))
		defer done()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// add blocks to the chain
		first := mock.TipSet(mock.MkBlock(nil, 5, 4))
		head := mock.TipSet(mock.MkBlock(first, 15, 7))

		// create a signed message to be returned as a pending message
		w, _ := wallet.NewWallet(wallet.NewMemKeyStore())
		senderAddr, err := w.WalletNew(context.Background(), types.KTSecp256k1)
		if err != nil {
			t.Fatal(err)
		}
		toAddr, err := w.WalletNew(context.Background(), types.KTSecp256k1)
		if err != nil {
			t.Fatal(err)
		}
		sm := mock.MkMessage(senderAddr, toAddr, 13, w)

		gomock.InOrder(
			mockApi.EXPECT().MpoolPending(ctx, types.EmptyTSK).Return([]*types.SignedMessage{sm}, nil),
			mockApi.EXPECT().WalletList(ctx).Return([]address.Address{senderAddr}, nil),
			mockApi.EXPECT().ChainHead(ctx).Return(head, nil),
		)

		err = app.Run([]string{"mpool", "gas-perf"})
		assert.NoError(t, err)

		assert.Contains(t, buf.String(), sm.Message.From.String())
		assert.Contains(t, buf.String(), fmt.Sprint(sm.Message.Nonce))
	})
}

func TestConfig(t *testing.T) {
	t.Run("get", func(t *testing.T) {
		app, mockApi, buf, done := NewMockAppWithFullAPI(t, WithCategory("mpool", MpoolConfig))
		defer done()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		w, _ := wallet.NewWallet(wallet.NewMemKeyStore())
		senderAddr, err := w.WalletNew(context.Background(), types.KTSecp256k1)
		if err != nil {
			t.Fatal(err)
		}

		mpoolCfg := &types.MpoolConfig{PriorityAddrs: []address.Address{senderAddr}, SizeLimitHigh: 1234567, SizeLimitLow: 6, ReplaceByFeeRatio: types.Percent(25)}
		gomock.InOrder(
			mockApi.EXPECT().MpoolGetConfig(ctx).Return(mpoolCfg, nil),
		)

		err = app.Run([]string{"mpool", "config"})
		assert.NoError(t, err)

		assert.Contains(t, buf.String(), mpoolCfg.PriorityAddrs[0].String())
		assert.Contains(t, buf.String(), fmt.Sprint(mpoolCfg.SizeLimitHigh))
		assert.Contains(t, buf.String(), fmt.Sprint(mpoolCfg.SizeLimitLow))
		assert.Contains(t, buf.String(), fmt.Sprint(mpoolCfg.ReplaceByFeeRatio))
	})

	t.Run("set", func(t *testing.T) {
		app, mockApi, _, done := NewMockAppWithFullAPI(t, WithCategory("mpool", MpoolConfig))
		defer done()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		w, _ := wallet.NewWallet(wallet.NewMemKeyStore())
		senderAddr, err := w.WalletNew(context.Background(), types.KTSecp256k1)
		if err != nil {
			t.Fatal(err)
		}

		mpoolCfg := &types.MpoolConfig{PriorityAddrs: []address.Address{senderAddr}, SizeLimitHigh: 234567, SizeLimitLow: 3, ReplaceByFeeRatio: types.Percent(33)}
		gomock.InOrder(
			mockApi.EXPECT().MpoolSetConfig(ctx, mpoolCfg).Return(nil),
		)

		bytes, err := json.Marshal(mpoolCfg)
		if err != nil {
			t.Fatal(err)
		}

		err = app.Run([]string{"mpool", "config", string(bytes)})
		assert.NoError(t, err)
	})
}
