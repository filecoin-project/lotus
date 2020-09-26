package test

import (
	"context"
	"fmt"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/filecoin-project/specs-actors/actors/builtin"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/specs-actors/actors/builtin/paych"
	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/go-address"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/events"
	"github.com/filecoin-project/lotus/chain/events/state"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/wallet"
)

func TestPaymentChannels(t *testing.T, b APIBuilder, blocktime time.Duration) {
	_ = os.Setenv("BELLMAN_NO_GPU", "1")

	ctx := context.Background()
	n, sn := b(t, 2, OneMiner)

	paymentCreator := n[0]
	paymentReceiver := n[1]
	miner := sn[0]

	// get everyone connected
	addrs, err := paymentCreator.NetAddrsListen(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if err := paymentReceiver.NetConnect(ctx, addrs); err != nil {
		t.Fatal(err)
	}

	if err := miner.NetConnect(ctx, addrs); err != nil {
		t.Fatal(err)
	}

	// start mining blocks
	bm := NewBlockMiner(ctx, t, miner, blocktime)
	bm.MineBlocks()

	// send some funds to register the receiver
	receiverAddr, err := paymentReceiver.WalletNew(ctx, wallet.ActSigType("secp256k1"))
	if err != nil {
		t.Fatal(err)
	}

	SendFunds(ctx, t, paymentCreator, receiverAddr, abi.NewTokenAmount(1e18))

	// setup the payment channel
	createrAddr, err := paymentCreator.WalletDefaultAddress(ctx)
	if err != nil {
		t.Fatal(err)
	}

	channelAmt := int64(100000)
	channelInfo, err := paymentCreator.PaychGet(ctx, createrAddr, receiverAddr, abi.NewTokenAmount(channelAmt))
	if err != nil {
		t.Fatal(err)
	}

	channel, err := paymentCreator.PaychGetWaitReady(ctx, channelInfo.WaitSentinel)
	if err != nil {
		t.Fatal(err)
	}

	// allocate three lanes
	var lanes []uint64
	for i := 0; i < 3; i++ {
		lane, err := paymentCreator.PaychAllocateLane(ctx, channel)
		if err != nil {
			t.Fatal(err)
		}
		lanes = append(lanes, lane)
	}

	// Make two vouchers each for each lane, then save on the other side
	// Note that the voucher with a value of 2000 has a higher nonce, so it
	// supersedes the voucher with a value of 1000
	for _, lane := range lanes {
		vouch1, err := paymentCreator.PaychVoucherCreate(ctx, channel, abi.NewTokenAmount(1000), lane)
		if err != nil {
			t.Fatal(err)
		}
		if vouch1.Voucher == nil {
			t.Fatal(fmt.Errorf("Not enough funds to create voucher: missing %d", vouch1.Shortfall))
		}
		vouch2, err := paymentCreator.PaychVoucherCreate(ctx, channel, abi.NewTokenAmount(2000), lane)
		if err != nil {
			t.Fatal(err)
		}
		if vouch2.Voucher == nil {
			t.Fatal(fmt.Errorf("Not enough funds to create voucher: missing %d", vouch2.Shortfall))
		}
		delta1, err := paymentReceiver.PaychVoucherAdd(ctx, channel, vouch1.Voucher, nil, abi.NewTokenAmount(1000))
		if err != nil {
			t.Fatal(err)
		}
		if !delta1.Equals(abi.NewTokenAmount(1000)) {
			t.Fatal("voucher didn't have the right amount")
		}
		delta2, err := paymentReceiver.PaychVoucherAdd(ctx, channel, vouch2.Voucher, nil, abi.NewTokenAmount(1000))
		if err != nil {
			t.Fatal(err)
		}
		if !delta2.Equals(abi.NewTokenAmount(1000)) {
			t.Fatal("voucher didn't have the right amount")
		}
	}

	// settle the payment channel
	settleMsgCid, err := paymentCreator.PaychSettle(ctx, channel)
	if err != nil {
		t.Fatal(err)
	}

	res := waitForMessage(ctx, t, paymentCreator, settleMsgCid, time.Second*10, "settle")
	if res.Receipt.ExitCode != 0 {
		t.Fatal("Unable to settle payment channel")
	}

	// wait for the receiver to submit their vouchers
	ev := events.NewEvents(ctx, paymentCreator)
	preds := state.NewStatePredicates(paymentCreator)
	finished := make(chan struct{})
	err = ev.StateChanged(func(ts *types.TipSet) (done bool, more bool, err error) {
		act, err := paymentCreator.StateReadState(ctx, channel, ts.Key())
		if err != nil {
			return false, false, err
		}
		state := act.State.(paych.State)
		if state.ToSend.GreaterThanEqual(abi.NewTokenAmount(6000)) {
			return true, false, nil
		}
		return false, true, nil
	}, func(oldTs, newTs *types.TipSet, states events.StateChange, curH abi.ChainEpoch) (more bool, err error) {
		toSendChange := states.(*state.PayChToSendChange)
		if toSendChange.NewToSend.GreaterThanEqual(abi.NewTokenAmount(6000)) {
			close(finished)
			return false, nil
		}
		return true, nil
	}, func(ctx context.Context, ts *types.TipSet) error {
		return nil
	}, int(build.MessageConfidence)+1, build.SealRandomnessLookbackLimit, func(oldTs, newTs *types.TipSet) (bool, events.StateChange, error) {
		return preds.OnPaymentChannelActorChanged(channel, preds.OnToSendAmountChanges())(ctx, oldTs.Key(), newTs.Key())
	})
	if err != nil {
		t.Fatal(err)
	}

	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("Timed out waiting for receiver to submit vouchers")
	}

	// wait for the settlement period to pass before collecting
	waitForBlocks(ctx, t, bm, paymentReceiver, receiverAddr, paych.SettleDelay)

	creatorPreCollectBalance, err := paymentCreator.WalletBalance(ctx, createrAddr)
	if err != nil {
		t.Fatal(err)
	}

	// collect funds (from receiver, though either party can do it)
	collectMsg, err := paymentReceiver.PaychCollect(ctx, channel)
	if err != nil {
		t.Fatal(err)
	}
	res, err = paymentReceiver.StateWaitMsg(ctx, collectMsg, 3)
	if err != nil {
		t.Fatal(err)
	}
	if res.Receipt.ExitCode != 0 {
		t.Fatal("unable to collect on payment channel")
	}

	// Finally, check the balance for the creator
	currentCreatorBalance, err := paymentCreator.WalletBalance(ctx, createrAddr)
	if err != nil {
		t.Fatal(err)
	}

	// The highest nonce voucher that the creator sent on each lane is 2000
	totalVouchers := int64(len(lanes) * 2000)

	// When receiver submits the tokens to the chain, creator should get a
	// refund on the remaining balance, which is
	// channel amount - total voucher value
	expectedRefund := channelAmt - totalVouchers
	delta := big.Sub(currentCreatorBalance, creatorPreCollectBalance)
	if !delta.Equals(abi.NewTokenAmount(expectedRefund)) {
		t.Fatalf("did not send correct funds from creator: expected %d, got %d", expectedRefund, delta)
	}

	// shut down mining
	bm.Stop()
}

func waitForBlocks(ctx context.Context, t *testing.T, bm *BlockMiner, paymentReceiver TestNode, receiverAddr address.Address, count int) {
	// We need to add null blocks in batches, if we add too many the chain can't sync
	batchSize := 60
	for i := 0; i < count; i += batchSize {
		size := batchSize
		if i > count {
			size = count - i
		}

		// Add a batch of null blocks
		atomic.StoreInt64(&bm.nulls, int64(size-1))

		// Add a real block
		m, err := paymentReceiver.MpoolPushMessage(ctx, &types.Message{
			To:    builtin.BurntFundsActorAddr,
			From:  receiverAddr,
			Value: types.NewInt(0),
		}, nil)
		if err != nil {
			t.Fatal(err)
		}

		_, err = paymentReceiver.StateWaitMsg(ctx, m.Cid(), 1)
		if err != nil {
			t.Fatal(err)
		}
	}
}

func waitForMessage(ctx context.Context, t *testing.T, paymentCreator TestNode, msgCid cid.Cid, duration time.Duration, desc string) *api.MsgLookup {
	ctx, cancel := context.WithTimeout(ctx, duration)
	defer cancel()

	fmt.Println("Waiting for", desc)
	res, err := paymentCreator.StateWaitMsg(ctx, msgCid, 1)
	if err != nil {
		fmt.Println("Error waiting for", desc, err)
		t.Fatal(err)
	}
	if res.Receipt.ExitCode != 0 {
		t.Fatalf("did not successfully send %s", desc)
	}
	fmt.Println("Confirmed", desc)
	return res
}
