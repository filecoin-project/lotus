package validation

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	vchain "github.com/filecoin-project/chain-validation/pkg/chain"
	vactors "github.com/filecoin-project/chain-validation/pkg/state/actors"
	vaddress "github.com/filecoin-project/chain-validation/pkg/state/address"
	vtypes "github.com/filecoin-project/chain-validation/pkg/state/types"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/wallet"
	_ "github.com/filecoin-project/lotus/lib/sigs/secp"
)

func TestMessageFactory(t *testing.T) {
	ks := wallet.NewMemKeyStore()
	wallet, err := wallet.NewWallet(ks)
	require.NoError(t, err)
	factory := NewMessageFactory(&walletWrapper{wallet})

	gasPrice := vtypes.NewInt(1)
	gasLimit := vtypes.GasUnit(1000)
	p := vchain.NewMessageProducer(factory, gasLimit, gasPrice)

	sender, err := wallet.GenerateKey(types.KTSecp256k1)
	require.NoError(t, err)

	bfAddr := factory.FromSingletonAddress(vactors.BurntFundsAddress)
	addr, err := vaddress.NewFromBytes(sender.Bytes())
	require.NoError(t, err)
	m, err := p.Transfer(addr, bfAddr, 0, 1)
	require.NoError(t, err)

	messages := p.Messages()
	assert.Equal(t, 1, len(messages))
	msg := m.(*types.Message)
	assert.Equal(t, m, msg)
	assert.Equal(t, sender, msg.From)
	assert.Equal(t, actors.BurntFundsAddress, msg.To)
	assert.Equal(t, types.NewInt(1), msg.Value)
}

type walletWrapper struct {
	w *wallet.Wallet
}

func (ww *walletWrapper) Sign(ctx context.Context, vaddr vaddress.Address, msg []byte) (*types.Signature, error) {
	addr, err := address.NewFromBytes(vaddr.Bytes())
	if err != nil {
		return nil, err
	}
	return ww.w.Sign(ctx, addr, msg)
}
