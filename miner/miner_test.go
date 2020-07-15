package miner

import (
	"context"
	"testing"

	"github.com/filecoin-project/go-address"

	"github.com/filecoin-project/lotus/chain/types"
	"github.com/stretchr/testify/assert"
)

func mustIDAddr(i uint64) address.Address {
	a, err := address.NewIDAddress(i)
	if err != nil {
		panic(err)
	}

	return a
}

func TestMessageFiltering(t *testing.T) {
	ctx := context.TODO()
	a1 := mustIDAddr(1)
	a2 := mustIDAddr(2)

	actors := map[address.Address]*types.Actor{
		a1: {
			Nonce:   3,
			Balance: types.FromFil(1200),
		},
		a2: {
			Nonce:   1,
			Balance: types.FromFil(1000),
		},
	}

	af := func(ctx context.Context, addr address.Address, tsk types.TipSetKey) (*types.Actor, error) {
		return actors[addr], nil
	}

	msgs := []types.Message{
		{
			From:     a1,
			To:       a1,
			Nonce:    3,
			Value:    types.FromFil(500),
			GasLimit: 100_000_000,
			GasPrice: types.NewInt(1),
		},
		{
			From:     a1,
			To:       a1,
			Nonce:    4,
			Value:    types.FromFil(500),
			GasLimit: 100_000_000,
			GasPrice: types.NewInt(1),
		},
		{
			From:     a2,
			To:       a1,
			Nonce:    1,
			Value:    types.FromFil(800),
			GasLimit: 100_000_000,
			GasPrice: types.NewInt(1),
		},
		{
			From:     a2,
			To:       a1,
			Nonce:    0,
			Value:    types.FromFil(800),
			GasLimit: 100_000_000,
			GasPrice: types.NewInt(1),
		},
		{
			From:     a2,
			To:       a1,
			Nonce:    2,
			Value:    types.FromFil(150),
			GasLimit: 100,
			GasPrice: types.NewInt(1),
		},
	}

	outmsgs, err := SelectMessages(ctx, af, &types.TipSet{}, wrapMsgs(msgs))
	if err != nil {
		t.Fatal(err)
	}

	assert.Len(t, outmsgs, 3, "filtering didnt work as expected")

	was, expected := outmsgs[0].Message, msgs[2]
	if was.From != expected.From || was.Nonce != expected.Nonce {
		t.Fatal("filtering bad")
	}
}

func wrapMsgs(msgs []types.Message) []*types.SignedMessage {
	var out []*types.SignedMessage
	for _, m := range msgs {
		out = append(out, &types.SignedMessage{Message: m})
	}
	return out
}
