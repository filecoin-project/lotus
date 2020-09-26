package types

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/specs-actors/actors/builtin"
)

func TestEqualCall(t *testing.T) {
	m1 := &Message{
		To:    builtin.StoragePowerActorAddr,
		From:  builtin.SystemActorAddr,
		Nonce: 34,
		Value: big.Zero(),

		GasLimit:   123,
		GasFeeCap:  big.NewInt(234),
		GasPremium: big.NewInt(234),

		Method: 6,
		Params: []byte("hai"),
	}

	m2 := &Message{
		To:    builtin.StoragePowerActorAddr,
		From:  builtin.SystemActorAddr,
		Nonce: 34,
		Value: big.Zero(),

		GasLimit:   1236, // changed
		GasFeeCap:  big.NewInt(234),
		GasPremium: big.NewInt(234),

		Method: 6,
		Params: []byte("hai"),
	}

	m3 := &Message{
		To:    builtin.StoragePowerActorAddr,
		From:  builtin.SystemActorAddr,
		Nonce: 34,
		Value: big.Zero(),

		GasLimit:   123,
		GasFeeCap:  big.NewInt(4524), // changed
		GasPremium: big.NewInt(234),

		Method: 6,
		Params: []byte("hai"),
	}

	m4 := &Message{
		To:    builtin.StoragePowerActorAddr,
		From:  builtin.SystemActorAddr,
		Nonce: 34,
		Value: big.Zero(),

		GasLimit:   123,
		GasFeeCap:  big.NewInt(4524),
		GasPremium: big.NewInt(234),

		Method: 5, // changed
		Params: []byte("hai"),
	}

	require.True(t, m1.EqualCall(m2))
	require.True(t, m1.EqualCall(m3))
	require.False(t, m1.EqualCall(m4))
}
