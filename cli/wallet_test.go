package cli

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/ipfs/go-cid"
	"github.com/multiformats/go-multihash"
	"github.com/stretchr/testify/assert"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/crypto"

	"github.com/filecoin-project/lotus/api"
	apitypes "github.com/filecoin-project/lotus/api/types"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/types/mock"
)

func TestWalletNew(t *testing.T) {
	app, mockApi, buffer, done := NewMockAppWithFullAPI(t, WithCategory("wallet", walletNew))
	defer done()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	keyType := types.KeyType("secp256k1")
	address, err := address.NewFromString("t0123")
	assert.NoError(t, err)

	mockApi.EXPECT().WalletNew(ctx, keyType).Return(address, nil)

	err = app.Run([]string{"wallet", "new"})
	assert.NoError(t, err)
	assert.Contains(t, buffer.String(), address.String())
}

func TestWalletList(t *testing.T) {

	addr, err := address.NewIDAddress(1234)
	addresses := []address.Address{addr}
	assert.NoError(t, err)

	cid := cid.Cid{}
	key := types.NewTipSetKey(cid)

	actor := types.Actor{
		Code:    cid,
		Head:    cid,
		Nonce:   0,
		Balance: big.NewInt(100),
	}

	t.Run("wallet-list-addr-only", func(t *testing.T) {

		app, mockApi, buf, done := NewMockAppWithFullAPI(t, WithCategory("wallet", walletList))
		defer done()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		gomock.InOrder(
			mockApi.EXPECT().WalletList(ctx).Return(addresses, nil),
			mockApi.EXPECT().WalletDefaultAddress(ctx).Return(addr, nil),
		)

		err := app.Run([]string{"wallet", "list", "--addr-only"})
		assert.NoError(t, err)
		assert.Contains(t, buf.String(), addr.String())
	})
	t.Run("wallet-list-id", func(t *testing.T) {

		app, mockApi, _, done := NewMockAppWithFullAPI(t, WithCategory("wallet", walletList))
		defer done()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		gomock.InOrder(
			mockApi.EXPECT().WalletList(ctx).Return(addresses, nil),
			mockApi.EXPECT().WalletDefaultAddress(ctx).Return(addr, nil),
			mockApi.EXPECT().StateGetActor(ctx, addr, key).Return(&actor, nil),
			mockApi.EXPECT().StateLookupID(ctx, addr, key).Return(addr, nil),
		)

		err := app.Run([]string{"wallet", "list", "--id"})
		assert.NoError(t, err)
	})
	t.Run("wallet-list-market", func(t *testing.T) {

		app, mockApi, _, done := NewMockAppWithFullAPI(t, WithCategory("wallet", walletList))
		defer done()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		balance := api.MarketBalance{
			Escrow: big.NewInt(1234),
			Locked: big.NewInt(123),
		}

		gomock.InOrder(
			mockApi.EXPECT().WalletList(ctx).Return(addresses, nil),
			mockApi.EXPECT().WalletDefaultAddress(ctx).Return(addr, nil),
			mockApi.EXPECT().StateGetActor(ctx, addr, key).Return(&actor, nil),
			mockApi.EXPECT().StateMarketBalance(ctx, addr, key).Return(balance, nil),
		)

		err := app.Run([]string{"wallet", "list", "--market"})
		assert.NoError(t, err)
	})
}

func TestWalletBalance(t *testing.T) {
	app, mockApi, buffer, done := NewMockAppWithFullAPI(t, WithCategory("wallet", walletBalance))
	defer done()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	addr, err := address.NewIDAddress(1234)
	assert.NoError(t, err)

	balance := big.NewInt(1234)

	// add blocks to the chain
	first := mock.TipSet(mock.MkBlock(nil, 5, 4))
	head := mock.TipSet(mock.MkBlock(first, 15, 7))

	mockApi.EXPECT().ChainHead(ctx).Return(head, nil)
	mockApi.EXPECT().WalletBalance(ctx, addr).Return(balance, nil)

	err = app.Run([]string{"wallet", "balance", "f01234"})
	assert.NoError(t, err)
	assert.Contains(t, buffer.String(), balance.String())
}

func TestWalletGetDefault(t *testing.T) {
	app, mockApi, buffer, done := NewMockAppWithFullAPI(t, WithCategory("wallet", walletGetDefault))
	defer done()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	addr, err := address.NewFromString("t0123")
	assert.NoError(t, err)

	mockApi.EXPECT().WalletDefaultAddress(ctx).Return(addr, nil)

	err = app.Run([]string{"wallet", "default"})
	assert.NoError(t, err)
	assert.Contains(t, buffer.String(), addr.String())
}

func TestWalletSetDefault(t *testing.T) {
	app, mockApi, _, done := NewMockAppWithFullAPI(t, WithCategory("wallet", walletSetDefault))
	defer done()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	addr, err := address.NewIDAddress(1234)
	assert.NoError(t, err)

	mockApi.EXPECT().WalletSetDefault(ctx, addr).Return(nil)

	err = app.Run([]string{"wallet", "set-default", "f01234"})
	assert.NoError(t, err)
}

func TestWalletExport(t *testing.T) {
	addr, err := address.NewIDAddress(1234)
	assert.NoError(t, err)

	pkBytes := make([]byte, 32)
	for i := range pkBytes {
		pkBytes[i] = byte(i + 1)
	}
	pkHex := hex.EncodeToString(pkBytes)

	t.Run("default-format-is-hex-lotus", func(t *testing.T) {
		app, mockApi, buffer, done := NewMockAppWithFullAPI(t, WithCategory("wallet", walletExport))
		defer done()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		ki := types.KeyInfo{Type: types.KTSecp256k1, PrivateKey: pkBytes}
		mockApi.EXPECT().WalletExport(ctx, addr).Return(&ki, nil)

		expected, err := json.Marshal(ki)
		assert.NoError(t, err)

		err = app.Run([]string{"wallet", "export", "f01234"})
		assert.NoError(t, err)
		assert.Contains(t, buffer.String(), hex.EncodeToString(expected))
	})

	t.Run("json-lotus", func(t *testing.T) {
		app, mockApi, buffer, done := NewMockAppWithFullAPI(t, WithCategory("wallet", walletExport))
		defer done()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		ki := types.KeyInfo{Type: types.KTSecp256k1, PrivateKey: pkBytes}
		mockApi.EXPECT().WalletExport(ctx, addr).Return(&ki, nil)

		expected, err := json.Marshal(ki)
		assert.NoError(t, err)

		err = app.Run([]string{"wallet", "export", "--format", "json-lotus", "f01234"})
		assert.NoError(t, err)
		assert.Contains(t, buffer.String(), string(expected))
	})

	t.Run("hex-eth", func(t *testing.T) {
		app, mockApi, buffer, done := NewMockAppWithFullAPI(t, WithCategory("wallet", walletExport))
		defer done()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		ki := types.KeyInfo{Type: types.KTDelegated, PrivateKey: pkBytes}
		mockApi.EXPECT().WalletExport(ctx, addr).Return(&ki, nil)

		err = app.Run([]string{"wallet", "export", "--format", "hex-eth", "f01234"})
		assert.NoError(t, err)
		assert.Contains(t, buffer.String(), "0x"+pkHex)
	})

	t.Run("hex-eth-rejects-bls", func(t *testing.T) {
		app, mockApi, _, done := NewMockAppWithFullAPI(t, WithCategory("wallet", walletExport))
		defer done()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		ki := types.KeyInfo{Type: types.KTBLS, PrivateKey: pkBytes}
		mockApi.EXPECT().WalletExport(ctx, addr).Return(&ki, nil)

		err = app.Run([]string{"wallet", "export", "--format", "hex-eth", "f01234"})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "hex-eth")
	})

	t.Run("unknown-format", func(t *testing.T) {
		app, mockApi, _, done := NewMockAppWithFullAPI(t, WithCategory("wallet", walletExport))
		defer done()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		ki := types.KeyInfo{Type: types.KTSecp256k1, PrivateKey: pkBytes}
		mockApi.EXPECT().WalletExport(ctx, addr).Return(&ki, nil)

		err = app.Run([]string{"wallet", "export", "--format", "bogus", "f01234"})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unrecognized format")
	})
}

func TestWalletImportHexEth(t *testing.T) {
	addr, err := address.NewIDAddress(1234)
	assert.NoError(t, err)

	pkBytes := make([]byte, 32)
	for i := range pkBytes {
		pkBytes[i] = byte(i + 1)
	}
	pkHex := hex.EncodeToString(pkBytes)

	writeTmp := func(t *testing.T, content string) string {
		t.Helper()
		f, err := os.CreateTemp(t.TempDir(), "key-*.hex")
		assert.NoError(t, err)
		_, err = f.WriteString(content)
		assert.NoError(t, err)
		assert.NoError(t, f.Close())
		return f.Name()
	}

	t.Run("delegated-default", func(t *testing.T) {
		app, mockApi, _, done := NewMockAppWithFullAPI(t, WithCategory("wallet", walletImport))
		defer done()

		path := writeTmp(t, pkHex)

		expected := &types.KeyInfo{Type: types.KTDelegated, PrivateKey: pkBytes}
		mockApi.EXPECT().WalletImport(gomock.Any(), expected).Return(addr, nil)

		err = app.Run([]string{"wallet", "import", "--format", "hex-eth", path})
		assert.NoError(t, err)
	})

	t.Run("secp256k1-explicit", func(t *testing.T) {
		app, mockApi, _, done := NewMockAppWithFullAPI(t, WithCategory("wallet", walletImport))
		defer done()

		path := writeTmp(t, pkHex)

		expected := &types.KeyInfo{Type: types.KTSecp256k1, PrivateKey: pkBytes}
		mockApi.EXPECT().WalletImport(gomock.Any(), expected).Return(addr, nil)

		err = app.Run([]string{"wallet", "import", "--format", "hex-eth", "--type", "secp256k1", path})
		assert.NoError(t, err)
	})

	t.Run("strips-0x-prefix-and-whitespace", func(t *testing.T) {
		app, mockApi, _, done := NewMockAppWithFullAPI(t, WithCategory("wallet", walletImport))
		defer done()

		path := writeTmp(t, "  0x"+pkHex+"\n")

		expected := &types.KeyInfo{Type: types.KTDelegated, PrivateKey: pkBytes}
		mockApi.EXPECT().WalletImport(gomock.Any(), expected).Return(addr, nil)

		err = app.Run([]string{"wallet", "import", "--format", "hex-eth", path})
		assert.NoError(t, err)
	})

	t.Run("rejects-wrong-length", func(t *testing.T) {
		app, _, _, done := NewMockAppWithFullAPI(t, WithCategory("wallet", walletImport))
		defer done()

		path := writeTmp(t, "deadbeef")

		err = app.Run([]string{"wallet", "import", "--format", "hex-eth", path})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "32-byte")
	})

	t.Run("rejects-invalid-type", func(t *testing.T) {
		app, _, _, done := NewMockAppWithFullAPI(t, WithCategory("wallet", walletImport))
		defer done()

		path := writeTmp(t, pkHex)

		err = app.Run([]string{"wallet", "import", "--format", "hex-eth", "--type", "bls", path})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "must be 'secp256k1' or 'delegated'")
	})
}

func TestWrapMessageFRC0102(t *testing.T) {
	// Test that wrapMessageFRC0102 produces the correct FRC-0102 envelope
	msg := []byte("hello")
	wrapped := wrapMessageFRC0102(msg)

	// Expected: "\x19Filecoin Signed Message:\n5hello"
	expected := append([]byte("\x19Filecoin Signed Message:\n5"), msg...)
	assert.Equal(t, expected, wrapped)

	// Test with empty message
	emptyWrapped := wrapMessageFRC0102([]byte{})
	expectedEmpty := []byte("\x19Filecoin Signed Message:\n0")
	assert.Equal(t, expectedEmpty, emptyWrapped)

	// Test with longer message
	longMsg := []byte("this is a longer test message")
	longWrapped := wrapMessageFRC0102(longMsg)
	expectedLong := append([]byte(fmt.Sprintf("\x19Filecoin Signed Message:\n%d", len(longMsg))), longMsg...)
	assert.Equal(t, expectedLong, longWrapped)
}

func TestWalletSign(t *testing.T) {
	t.Run("with FRC-0102 envelope (default)", func(t *testing.T) {
		app, mockApi, buffer, done := NewMockAppWithFullAPI(t, WithCategory("wallet", walletSign))
		defer done()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		addr, err := address.NewFromString("f01234")
		assert.NoError(t, err)

		rawMsg, err := hex.DecodeString("01")
		assert.NoError(t, err)

		// The CLI now wraps the message with FRC-0102 envelope
		wrappedMsg := wrapMessageFRC0102(rawMsg)

		signature := crypto.Signature{
			Type: crypto.SigTypeSecp256k1,
			Data: []byte{0x01},
		}

		mockApi.EXPECT().WalletSign(ctx, addr, wrappedMsg).Return(&signature, nil)

		sigBytes := append([]byte{byte(signature.Type)}, signature.Data...)

		err = app.Run([]string{"wallet", "sign", "f01234", "01"})
		assert.NoError(t, err)
		assert.Contains(t, buffer.String(), hex.EncodeToString(sigBytes))
	})

	t.Run("with --raw flag", func(t *testing.T) {
		app, mockApi, buffer, done := NewMockAppWithFullAPI(t, WithCategory("wallet", walletSign))
		defer done()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		addr, err := address.NewFromString("f01234")
		assert.NoError(t, err)

		rawMsg, err := hex.DecodeString("01")
		assert.NoError(t, err)

		signature := crypto.Signature{
			Type: crypto.SigTypeSecp256k1,
			Data: []byte{0x01},
		}

		// With --raw, the message should NOT be wrapped
		mockApi.EXPECT().WalletSign(ctx, addr, rawMsg).Return(&signature, nil)

		sigBytes := append([]byte{byte(signature.Type)}, signature.Data...)

		err = app.Run([]string{"wallet", "sign", "--raw", "f01234", "01"})
		assert.NoError(t, err)
		assert.Contains(t, buffer.String(), hex.EncodeToString(sigBytes))
	})
}

func TestWalletVerify(t *testing.T) {
	t.Run("with FRC-0102 envelope (default)", func(t *testing.T) {
		app, mockApi, buffer, done := NewMockAppWithFullAPI(t, WithCategory("wallet", walletVerify))
		defer done()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		addr, err := address.NewIDAddress(1234)
		assert.NoError(t, err)

		rawMsg := []byte{1}
		// The CLI now wraps the message with FRC-0102 envelope
		wrappedMsg := wrapMessageFRC0102(rawMsg)

		signature := crypto.Signature{
			Type: crypto.SigTypeSecp256k1,
			Data: []byte{},
		}

		mockApi.EXPECT().WalletVerify(ctx, addr, wrappedMsg, &signature).Return(true, nil)

		err = app.Run([]string{"wallet", "verify", "f01234", "01", "01"})
		assert.NoError(t, err)
		assert.Contains(t, buffer.String(), "valid")
	})

	t.Run("with --raw flag", func(t *testing.T) {
		app, mockApi, buffer, done := NewMockAppWithFullAPI(t, WithCategory("wallet", walletVerify))
		defer done()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		addr, err := address.NewIDAddress(1234)
		assert.NoError(t, err)

		rawMsg := []byte{1}
		signature := crypto.Signature{
			Type: crypto.SigTypeSecp256k1,
			Data: []byte{},
		}

		// With --raw, the message should NOT be wrapped
		mockApi.EXPECT().WalletVerify(ctx, addr, rawMsg, &signature).Return(true, nil)

		err = app.Run([]string{"wallet", "verify", "--raw", "f01234", "01", "01"})
		assert.NoError(t, err)
		assert.Contains(t, buffer.String(), "valid")
	})
}

func TestWalletDelete(t *testing.T) {
	app, mockApi, _, done := NewMockAppWithFullAPI(t, WithCategory("wallet", walletDelete))
	defer done()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	addr, err := address.NewIDAddress(1234)
	assert.NoError(t, err)

	mockApi.EXPECT().WalletDelete(ctx, addr).Return(nil)

	err = app.Run([]string{"wallet", "delete", "f01234"})
	assert.NoError(t, err)
}

func TestWalletMarketWithdraw(t *testing.T) {
	app, mockApi, buffer, done := NewMockAppWithFullAPI(t, WithCategory("wallet", walletMarket))
	defer done()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	addr, err := address.NewIDAddress(1234)
	assert.NoError(t, err)

	balance := api.MarketBalance{
		Escrow: big.NewInt(100),
		Locked: big.NewInt(10),
	}

	h, err := hex.DecodeString("12209cbc07c3f991725836a3aa2a581ca2029198aa420b9d99bc0e131d9f3e2cbe47")
	assert.NoError(t, err)
	cid := cid.NewCidV0(multihash.Multihash(h))
	msgLookup := api.MsgLookup{}

	var networkVers apitypes.NetworkVersion

	gomock.InOrder(
		mockApi.EXPECT().StateMarketBalance(ctx, addr, types.TipSetKey{}).Return(balance, nil),
		// mock reserve to 10
		mockApi.EXPECT().MarketGetReserved(ctx, addr).Return(big.NewInt(10), nil),
		// available should be 80.. escrow - locked - reserve
		mockApi.EXPECT().MarketWithdraw(ctx, addr, addr, big.NewInt(80)).Return(cid, nil),
		mockApi.EXPECT().StateWaitMsg(ctx, cid, uint64(5), abi.ChainEpoch(int64(-1)), true).Return(&msgLookup, nil),
		mockApi.EXPECT().StateNetworkVersion(ctx, types.TipSetKey{}).Return(networkVers, nil),
	)

	err = app.Run([]string{"wallet", "market", "withdraw", "--wallet", addr.String()})
	assert.NoError(t, err)
	assert.Contains(t, buffer.String(), fmt.Sprintf("WithdrawBalance message cid: %s", cid))
}

func TestWalletMarketAdd(t *testing.T) {
	app, mockApi, buffer, done := NewMockAppWithFullAPI(t, WithCategory("wallet", walletMarket))
	defer done()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	toAddr := address.Address{}
	defaultAddr := address.Address{}

	h, err := hex.DecodeString("12209cbc07c3f991725836a3aa2a581ca2029198aa420b9d99bc0e131d9f3e2cbe47")
	assert.NoError(t, err)
	cid := cid.NewCidV0(multihash.Multihash(h))

	gomock.InOrder(
		mockApi.EXPECT().WalletDefaultAddress(ctx).Return(defaultAddr, nil),
		mockApi.EXPECT().MarketAddBalance(ctx, defaultAddr, toAddr, big.NewInt(80)).Return(cid, nil),
	)

	err = app.Run([]string{"wallet", "market", "add", "0.000000000000000080", "--address", toAddr.String()})
	assert.NoError(t, err)
	assert.Contains(t, buffer.String(), fmt.Sprintf("AddBalance message cid: %s", cid))
}
