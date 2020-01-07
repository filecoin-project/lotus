package miner

import (
	"context"
	"testing"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/chain/types"
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
		a1: &types.Actor{
			Nonce:   3,
			Balance: types.NewInt(1200),
		},
		a2: &types.Actor{
			Nonce:   1,
			Balance: types.NewInt(1000),
		},
	}

	af := func(ctx context.Context, addr address.Address, ts *types.TipSet) (*types.Actor, error) {
		return actors[addr], nil
	}

	msgs := []types.Message{
		types.Message{
			From:     a1,
			To:       a1,
			Nonce:    3,
			Value:    types.NewInt(500),
			GasLimit: types.NewInt(50),
			GasPrice: types.NewInt(1),
		},
		types.Message{
			From:     a1,
			To:       a1,
			Nonce:    4,
			Value:    types.NewInt(500),
			GasLimit: types.NewInt(50),
			GasPrice: types.NewInt(1),
		},
		types.Message{
			From:     a2,
			To:       a1,
			Nonce:    1,
			Value:    types.NewInt(800),
			GasLimit: types.NewInt(100),
			GasPrice: types.NewInt(1),
		},
		types.Message{
			From:     a2,
			To:       a1,
			Nonce:    0,
			Value:    types.NewInt(800),
			GasLimit: types.NewInt(100),
			GasPrice: types.NewInt(1),
		},
		types.Message{
			From:     a2,
			To:       a1,
			Nonce:    2,
			Value:    types.NewInt(150),
			GasLimit: types.NewInt(100),
			GasPrice: types.NewInt(1),
		},
	}

	outmsgs, err := selectMessages(ctx, af, &MiningBase{}, wrapMsgs(msgs))
	if err != nil {
		t.Fatal(err)
	}

	if len(outmsgs) != 3 {
		t.Fatal("filtering didnt work as expected")
	}

	m1 := outmsgs[2].Message
	if m1.From != msgs[2].From || m1.Nonce != msgs[2].Nonce {
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
