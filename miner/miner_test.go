package miner

import (
	"context"
	"testing"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/types/mock"
	"github.com/filecoin-project/specs-actors/actors/abi"
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
			Balance: types.NewInt(1200),
		},
		a2: {
			Nonce:   1,
			Balance: types.NewInt(1000),
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
			Value:    types.NewInt(500),
			GasLimit: 50,
			GasPrice: types.NewInt(1),
		},
		{
			From:     a1,
			To:       a1,
			Nonce:    4,
			Value:    types.NewInt(500),
			GasLimit: 50,
			GasPrice: types.NewInt(1),
		},
		{
			From:     a2,
			To:       a1,
			Nonce:    1,
			Value:    types.NewInt(800),
			GasLimit: 100,
			GasPrice: types.NewInt(1),
		},
		{
			From:     a2,
			To:       a1,
			Nonce:    0,
			Value:    types.NewInt(800),
			GasLimit: 100,
			GasPrice: types.NewInt(1),
		},
		{
			From:     a2,
			To:       a1,
			Nonce:    2,
			Value:    types.NewInt(150),
			GasLimit: (100),
			GasPrice: types.NewInt(1),
		},
	}

	outmsgs, err := SelectMessages(ctx, af, &types.TipSet{}, wrapMsgs(msgs))
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

type testMinerApi struct {
	MinerApiProvider

	head *types.TipSet
}

func (t *testMinerApi) ChainHead(ctx context.Context) (*types.TipSet, error) {
	return t.head, nil
}

func TestMiningLoopNonceIncrement(t *testing.T) {
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	mapi := &testMinerApi{}
	addr, err := address.NewIDAddress(100)
	if err != nil {
		t.Fatal(err)
	}

	enterWait := make(chan struct{})
	mineCh := make(chan struct{})
	mineOneWait := make(chan struct{})
	m := NewMiner(mapi, nil, addr)
	m.waitFunc = func(ctx context.Context, baseTime uint64) (func(bool), error) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case enterWait <- struct{}{}:
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-mineCh:
		}
		return func(bool) {}, nil
	}

	m.mineOneFn = func(ctx context.Context, base *MiningBase) (*types.BlockMsg, error) {
		mineOneWait <- struct{}{}
		return nil, nil
	}

	mapi.head = mock.TipSet(mock.MkBlock(nil, 1, 1))

	m.Start(ctx)

	<-enterWait
	mineCh <- struct{}{}
	<-mineOneWait

	<-enterWait
	assert.Equal(t, m.lastWork.NullRounds, abi.ChainEpoch(1))

	mineCh <- struct{}{}
	<-mineOneWait

	<-enterWait
	assert.Equal(t, m.lastWork.NullRounds, abi.ChainEpoch(2))

}
