package paych

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/abi/big"
	"github.com/testground/sdk-go/sync"

	"github.com/filecoin-project/oni/lotus-soup/testkit"
)

var SendersDoneState = sync.State("senders-done")

// TODO Stress is currently WIP. We found blockers in Lotus that prevent us from
//  making progress. See https://github.com/filecoin-project/lotus/issues/2297.
func Stress(t *testkit.TestEnvironment) error {
	// Dispatch/forward non-client roles to defaults.
	if t.Role != "client" {
		return testkit.HandleDefaultRole(t)
	}

	// This is a client role.
	t.RecordMessage("running payments client")

	var (
		// lanes to open; vouchers will be distributed across these lanes in round-robin fashion
		laneCount = t.IntParam("lane_count")
		// increments in which to send payment vouchers
		increments = big.Mul(big.NewInt(int64(t.IntParam("increments"))), abi.TokenPrecision)
	)

	ctx := context.Background()
	cl, err := testkit.PrepareClient(t)
	if err != nil {
		return err
	}

	// are we the receiver or a sender?
	mode := "sender"
	if t.GroupSeq == 1 {
		mode = "receiver"
	}

	t.RecordMessage("acting as %s", mode)

	var clients []*testkit.ClientAddressesMsg
	sctx, cancel := context.WithCancel(ctx)
	clientsCh := make(chan *testkit.ClientAddressesMsg)
	t.SyncClient.MustSubscribe(sctx, testkit.ClientsAddrsTopic, clientsCh)
	for i := 0; i < t.TestGroupInstanceCount; i++ {
		clients = append(clients, <-clientsCh)
	}
	cancel()

	switch mode {
	case "receiver":
		// one receiver, everyone else is a sender.
		<-t.SyncClient.MustBarrier(ctx, SendersDoneState, t.TestGroupInstanceCount-1).C

	case "sender":
		// we're going to lock up all our funds into this one payment channel.
		recv := clients[0]
		balance, err := cl.FullApi.WalletBalance(ctx, cl.Wallet.Address)
		if err != nil {
			return fmt.Errorf("failed to acquire wallet balance: %w", err)
		}

		t.RecordMessage("my balance: %d", balance)
		t.RecordMessage("creating payment channel; from=%s, to=%s, funds=%d", cl.Wallet.Address, recv.WalletAddr, balance)

		pid := os.Getpid()
		t.RecordMessage("sender pid: %d", pid)

		time.Sleep(20 * time.Second)

		channel, err := cl.FullApi.PaychGet(ctx, cl.Wallet.Address, recv.WalletAddr, balance)
		if err != nil {
			return fmt.Errorf("failed to create payment channel: %w", err)
		}

		if addr := channel.Channel; addr != address.Undef {
			return fmt.Errorf("expected an Undef channel address, got: %s", addr)
		}

		t.RecordMessage("payment channel created; msg_cid=%s", channel.ChannelMessage)
		t.RecordMessage("waiting for payment channel message to appear on chain")

		// wait for the channel creation message to appear on chain.
		_, err = cl.FullApi.StateWaitMsg(ctx, channel.ChannelMessage, 2)
		if err != nil {
			return fmt.Errorf("failed while waiting for payment channel creation msg to appear on chain: %w", err)
		}

		// need to wait so that the channel is tracked.
		// the full API waits for build.MessageConfidence (=1 in tests) before tracking the channel.
		// we wait for 2 confirmations, so we have the assurance the channel is tracked.

		t.RecordMessage("reloading paych; now it should have an address")
		channel, err = cl.FullApi.PaychGet(ctx, cl.Wallet.Address, recv.WalletAddr, big.Zero())
		if err != nil {
			return fmt.Errorf("failed to reload payment channel: %w", err)
		}

		t.RecordMessage("channel address: %s", channel.Channel)
		t.RecordMessage("allocating lanes; count=%d", laneCount)

		// allocate as many lanes as required
		var lanes []uint64
		for i := 0; i < laneCount; i++ {
			lane, err := cl.FullApi.PaychAllocateLane(ctx, channel.Channel)
			if err != nil {
				return fmt.Errorf("failed to allocate lane: %w", err)
			}
			lanes = append(lanes, lane)
		}

		t.RecordMessage("lanes allocated; count=%d", laneCount)
		t.RecordMessage("sending payments in round-robin fashion across lanes; increments=%d", increments)

		// start sending payments
		zero := big.Zero()

	Outer:
		for remaining := balance; remaining.GreaterThan(zero); {
			for _, lane := range lanes {
				voucher, err := cl.FullApi.PaychVoucherCreate(ctx, channel.Channel, increments, lane)
				if err != nil {
					return fmt.Errorf("failed to create voucher: %w", err)
				}
				t.RecordMessage("payment voucher created; lane=%d, nonce=%d, amount=%d", voucher.Lane, voucher.Nonce, voucher.Amount)

				cid, err := cl.FullApi.PaychVoucherSubmit(ctx, channel.Channel, voucher)
				if err != nil {
					return fmt.Errorf("failed to submit voucher: %w", err)
				}
				t.RecordMessage("payment voucher submitted; msg_cid=%s, lane=%d, nonce=%d, amount=%d", cid, voucher.Lane, voucher.Nonce, voucher.Amount)

				remaining = types.BigSub(remaining, increments)
				t.RecordMessage("remaining balance: %d", remaining)
				if remaining.LessThanEqual(zero) {
					// we have no more funds remaining.
					break Outer
				}
			}
		}

		t.RecordMessage("finished sending all payment vouchers")

		t.SyncClient.MustSignalEntry(ctx, SendersDoneState)
	}

	return nil

}
