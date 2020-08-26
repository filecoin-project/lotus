package test

import (
	"context"
	"testing"

	"github.com/filecoin-project/specs-actors/actors/abi"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/miner"
)

func SendFunds(ctx context.Context, t *testing.T, sender TestNode, addr address.Address, amount abi.TokenAmount) {
	senderAddr, err := sender.WalletDefaultAddress(ctx)
	if err != nil {
		t.Fatal(err)
	}

	msg := &types.Message{
		From:  senderAddr,
		To:    addr,
		Value: amount,
	}

	sm, err := sender.MpoolPushMessage(ctx, msg, nil)
	if err != nil {
		t.Fatal(err)
	}
	res, err := sender.StateWaitMsg(ctx, sm.Cid(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if res.Receipt.ExitCode != 0 {
		t.Fatal("did not successfully send money")
	}
}

func MineUntilBlock(ctx context.Context, t *testing.T, sn TestStorageNode, cb func()) {
	for i := 0; i < 1000; i++ {
		var success bool
		var err error
		wait := make(chan struct{})
		mineErr := sn.MineOne(ctx, miner.MineReq{
			Done: func(win bool, e error) {
				success = win
				err = e
				wait <- struct{}{}
			},
		})
		if mineErr != nil {
			t.Fatal(mineErr)
		}
		<-wait
		if err != nil {
			t.Fatal(err)
		}
		if success {
			if cb != nil {
				cb()
			}
			return
		}
		t.Log("did not mine block, trying again", i)
	}
	t.Fatal("failed to mine 1000 times in a row...")
}
