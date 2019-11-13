package validation

import (
	"math/big"
	"testing"

	"github.com/filecoin-project/chain-validation/pkg/chain"
	"github.com/filecoin-project/chain-validation/pkg/state"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/wallet"
)

func TestMessageFactory(t *testing.T) {
	ks := wallet.NewMemKeyStore()
	wallet, err := wallet.NewWallet(ks)
	require.NoError(t, err)
	factory := NewMessageFactory(wallet)

	gasPrice := big.NewInt(1)
	gasLimit := state.GasUnit(1000)
	p := chain.NewMessageProducer(factory, gasLimit, gasPrice)

	sender, err := wallet.GenerateKey(types.KTSecp256k1)
	require.NoError(t, err)

	bfAddr := factory.FromSingletonAddress(state.BurntFundsAddress)
	m, err := p.Transfer(state.Address(sender.Bytes()), bfAddr, 0, 1)
	require.NoError(t, err)

	messages := p.Messages()
	assert.Equal(t, 1, len(messages))
	msg := m.(*types.Message)
	assert.Equal(t, m, msg)
	assert.Equal(t, sender, msg.From)
	assert.Equal(t, actors.BurntFundsAddress, msg.To)
	assert.Equal(t, types.NewInt(1), msg.Value)
}
