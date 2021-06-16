package paych

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/specs-actors/actors/builtin/paych"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/testground/sdk-go/sync"

	"github.com/filecoin-project/lotus/testplans/lotus-soup/testkit"
)

var SendersDoneState = sync.State("senders-done")
var ReceiverReadyState = sync.State("receiver-ready")
var ReceiverAddedVouchersState = sync.State("receiver-added-vouchers")

var VoucherTopic = sync.NewTopic("voucher", &paych.SignedVoucher{})
var SettleTopic = sync.NewTopic("settle", cid.Cid{})

type ClientMode uint64

const (
	ModeSender ClientMode = iota
	ModeReceiver
)

func (cm ClientMode) String() string {
	return [...]string{"Sender", "Receiver"}[cm]
}

func getClientMode(groupSeq int64) ClientMode {
	if groupSeq == 1 {
		return ModeReceiver
	}
	return ModeSender
}

// TODO Stress is currently WIP. We found blockers in Lotus that prevent us from
//  making progress. See https://github.com/filecoin-project/lotus/issues/2297.
func Stress(t *testkit.TestEnvironment) error {
	// Dispatch/forward non-client roles to defaults.
	if t.Role != "client" {
		return testkit.HandleDefaultRole(t)
	}

	// This is a client role.
	t.RecordMessage("running payments client")

	ctx := context.Background()
	cl, err := testkit.PrepareClient(t)
	if err != nil {
		return err
	}

	// are we the receiver or a sender?
	mode := getClientMode(t.GroupSeq)
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
	case ModeReceiver:
		err := runReceiver(t, ctx, cl)
		if err != nil {
			return err
		}

	case ModeSender:
		err := runSender(ctx, t, clients, cl)
		if err != nil {
			return err
		}
	}

	// Signal that the client is done
	t.SyncClient.MustSignalEntry(ctx, testkit.StateDone)

	// Signal to the miners to stop mining
	t.SyncClient.MustSignalEntry(ctx, testkit.StateStopMining)

	return nil
}

func runSender(ctx context.Context, t *testkit.TestEnvironment, clients []*testkit.ClientAddressesMsg, cl *testkit.LotusClient) error {
	var (
		// lanes to open; vouchers will be distributed across these lanes in round-robin fashion
		laneCount = t.IntParam("lane_count")
		// number of vouchers to send on each lane
		vouchersPerLane = t.IntParam("vouchers_per_lane")
		// increments in which to send payment vouchers
		increments = big.Mul(big.NewInt(int64(t.IntParam("increments"))), big.NewInt(int64(build.FilecoinPrecision)))
		// channel amount should be enough to cover all vouchers
		channelAmt = big.Mul(big.NewInt(int64(laneCount*vouchersPerLane)), increments)
	)

	// Lock up funds in the payment channel.
	recv := findReceiver(clients)
	balance, err := cl.FullApi.WalletBalance(ctx, cl.Wallet.Address)
	if err != nil {
		return fmt.Errorf("failed to acquire wallet balance: %w", err)
	}

	t.RecordMessage("my balance: %d", balance)
	t.RecordMessage("creating payment channel; from=%s, to=%s, funds=%d", cl.Wallet.Address, recv.WalletAddr, channelAmt)

	pid := os.Getpid()
	t.RecordMessage("sender pid: %d", pid)

	time.Sleep(20 * time.Second)

	channel, err := cl.FullApi.PaychGet(ctx, cl.Wallet.Address, recv.WalletAddr, channelAmt)
	if err != nil {
		return fmt.Errorf("failed to create payment channel: %w", err)
	}

	if addr := channel.Channel; addr != address.Undef {
		return fmt.Errorf("expected an Undef channel address, got: %s", addr)
	}

	t.RecordMessage("payment channel created; msg_cid=%s", channel.WaitSentinel)
	t.RecordMessage("waiting for payment channel message to appear on chain")

	// wait for the channel creation message to appear on chain.
	_, err = cl.FullApi.StateWaitMsg(ctx, channel.WaitSentinel, 2, api.LookbackNoLimit, true)
	if err != nil {
		return fmt.Errorf("failed while waiting for payment channel creation msg to appear on chain: %w", err)
	}

	// need to wait so that the channel is tracked.
	// the full API waits for build.MessageConfidence (=1 in tests) before tracking the channel.
	// we wait for 2 confirmations, so we have the assurance the channel is tracked.

	t.RecordMessage("get payment channel address")
	channelAddr, err := cl.FullApi.PaychGetWaitReady(ctx, channel.WaitSentinel)
	if err != nil {
		return fmt.Errorf("failed to get payment channel address: %w", err)
	}

	t.RecordMessage("channel address: %s", channelAddr)
	t.RecordMessage("allocating lanes; count=%d", laneCount)

	// allocate as many lanes as required
	var lanes []uint64
	for i := 0; i < laneCount; i++ {
		lane, err := cl.FullApi.PaychAllocateLane(ctx, channelAddr)
		if err != nil {
			return fmt.Errorf("failed to allocate lane: %w", err)
		}
		lanes = append(lanes, lane)
	}

	t.RecordMessage("lanes allocated; count=%d", laneCount)

	<-t.SyncClient.MustBarrier(ctx, ReceiverReadyState, 1).C

	t.RecordMessage("sending payments in round-robin fashion across lanes; increments=%d", increments)

	// create vouchers
	remaining := channelAmt
	for i := 0; i < vouchersPerLane; i++ {
		for _, lane := range lanes {
			voucherAmt := big.Mul(big.NewInt(int64(i+1)), increments)
			voucher, err := cl.FullApi.PaychVoucherCreate(ctx, channelAddr, voucherAmt, lane)
			if err != nil {
				return fmt.Errorf("failed to create voucher: %w", err)
			}
			t.RecordMessage("payment voucher created; lane=%d, nonce=%d, amount=%d", voucher.Voucher.Lane, voucher.Voucher.Nonce, voucher.Voucher.Amount)

			_, err = t.SyncClient.Publish(ctx, VoucherTopic, voucher.Voucher)
			if err != nil {
				return fmt.Errorf("failed to publish voucher: %w", err)
			}

			remaining = big.Sub(remaining, increments)
			t.RecordMessage("remaining balance: %d", remaining)
		}
	}

	t.RecordMessage("finished sending all payment vouchers")

	// Inform the receiver that all vouchers have been created
	t.SyncClient.MustSignalEntry(ctx, SendersDoneState)

	// Wait for the receiver to add all vouchers
	<-t.SyncClient.MustBarrier(ctx, ReceiverAddedVouchersState, 1).C

	t.RecordMessage("settle channel")

	// Settle the channel. When the receiver sees the settle message, they
	// should automatically submit all vouchers.
	settleMsgCid, err := cl.FullApi.PaychSettle(ctx, channelAddr)
	if err != nil {
		return fmt.Errorf("failed to settle payment channel: %w", err)
	}

	t.SyncClient.Publish(ctx, SettleTopic, settleMsgCid)
	if err != nil {
		return fmt.Errorf("failed to publish settle message cid: %w", err)
	}

	return nil
}

func findReceiver(clients []*testkit.ClientAddressesMsg) *testkit.ClientAddressesMsg {
	for _, c := range clients {
		if getClientMode(c.GroupSeq) == ModeReceiver {
			return c
		}
	}
	return nil
}

func runReceiver(t *testkit.TestEnvironment, ctx context.Context, cl *testkit.LotusClient) error {
	// lanes to open; vouchers will be distributed across these lanes in round-robin fashion
	laneCount := t.IntParam("lane_count")
	// number of vouchers to send on each lane
	vouchersPerLane := t.IntParam("vouchers_per_lane")
	totalVouchers := laneCount * vouchersPerLane

	vouchers := make(chan *paych.SignedVoucher)
	vouchersSub, err := t.SyncClient.Subscribe(ctx, VoucherTopic, vouchers)
	if err != nil {
		return fmt.Errorf("failed to subscribe to voucher topic: %w", err)
	}

	settleMsgChan := make(chan cid.Cid)
	settleSub, err := t.SyncClient.Subscribe(ctx, SettleTopic, settleMsgChan)
	if err != nil {
		return fmt.Errorf("failed to subscribe to settle topic: %w", err)
	}

	// inform the clients that the receiver is ready for incoming vouchers
	t.SyncClient.MustSignalEntry(ctx, ReceiverReadyState)

	t.RecordMessage("adding %d payment vouchers", totalVouchers)

	// Add each of the vouchers
	var addedVouchers []*paych.SignedVoucher
	for i := 0; i < totalVouchers; i++ {
		v := <-vouchers
		addedVouchers = append(addedVouchers, v)

		_, err := cl.FullApi.PaychVoucherAdd(ctx, v.ChannelAddr, v, nil, big.NewInt(0))
		if err != nil {
			return fmt.Errorf("failed to add voucher: %w", err)
		}
		spendable, err := cl.FullApi.PaychVoucherCheckSpendable(ctx, v.ChannelAddr, v, nil, nil)
		if err != nil {
			return fmt.Errorf("failed to check voucher spendable: %w", err)
		}
		if !spendable {
			return fmt.Errorf("expected voucher %d to be spendable", i)
		}

		t.RecordMessage("payment voucher added; lane=%d, nonce=%d, amount=%d", v.Lane, v.Nonce, v.Amount)
	}

	vouchersSub.Done()

	t.RecordMessage("finished adding all payment vouchers")

	// Inform the clients that the receiver has added all vouchers
	t.SyncClient.MustSignalEntry(ctx, ReceiverAddedVouchersState)

	// Wait for the settle message (put on chain by the sender)
	t.RecordMessage("waiting for client to put settle message on chain")
	settleMsgCid := <-settleMsgChan
	settleSub.Done()

	time.Sleep(5 * time.Second)

	t.RecordMessage("waiting for confirmation of settle message on chain: %s", settleMsgCid)
	_, err = cl.FullApi.StateWaitMsg(ctx, settleMsgCid, 10, api.LookbackNoLimit, true)
	if err != nil {
		return fmt.Errorf("failed to wait for settle message: %w", err)
	}

	// Note: Once the receiver sees the settle message on chain, it will
	// automatically call submit voucher with the best vouchers

	// TODO: Uncomment this section once this PR is merged:
	// https://github.com/filecoin-project/lotus/pull/3197
	//t.RecordMessage("checking that all %d vouchers are no longer spendable", len(addedVouchers))
	//for i, v := range addedVouchers {
	//	spendable, err := cl.FullApi.PaychVoucherCheckSpendable(ctx, v.ChannelAddr, v, nil, nil)
	//	if err != nil {
	//		return fmt.Errorf("failed to check voucher spendable: %w", err)
	//	}
	//	// Should no longer be spendable because the best voucher has been submitted
	//	if spendable {
	//		return fmt.Errorf("expected voucher %d to no longer be spendable", i)
	//	}
	//}

	t.RecordMessage("all vouchers were submitted successfully")

	return nil
}
