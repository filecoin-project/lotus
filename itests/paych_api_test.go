package itests

import (
	"context"
	"testing"
	"time"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/lotus/itests/kit"
	"github.com/ipfs/go-cid"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	cbor "github.com/ipfs/go-ipld-cbor"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors/adt"
	"github.com/filecoin-project/lotus/chain/actors/builtin"
	"github.com/filecoin-project/lotus/chain/actors/builtin/paych"
	"github.com/filecoin-project/lotus/chain/actors/policy"
	"github.com/filecoin-project/lotus/chain/events"
	"github.com/filecoin-project/lotus/chain/events/state"
	"github.com/filecoin-project/lotus/chain/types"
)

func TestPaymentChannelsAPI(t *testing.T) {
	kit.QuietMiningLogs()

	ctx := context.Background()
	blockTime := 5 * time.Millisecond

	var (
		paymentCreator  kit.TestFullNode
		paymentReceiver kit.TestFullNode
		miner           kit.TestMiner
	)

	ens := kit.NewEnsemble(t, kit.MockProofs()).
		FullNode(&paymentCreator).
		FullNode(&paymentReceiver).
		Miner(&miner, &paymentCreator, kit.WithAllSubsystems()).
		Start().
		InterconnectAll()
	bms := ens.BeginMining(blockTime)
	bm := bms[0]

	// send some funds to register the receiver
	receiverAddr, err := paymentReceiver.WalletNew(ctx, types.KTSecp256k1)
	require.NoError(t, err)

	kit.SendFunds(ctx, t, &paymentCreator, receiverAddr, abi.NewTokenAmount(1e18))

	// setup the payment channel
	createrAddr, err := paymentCreator.WalletDefaultAddress(ctx)
	require.NoError(t, err)

	channelAmt := int64(7000)
	channelInfo, err := paymentCreator.PaychGet(ctx, createrAddr, receiverAddr, abi.NewTokenAmount(channelAmt))
	require.NoError(t, err)

	channel, err := paymentCreator.PaychGetWaitReady(ctx, channelInfo.WaitSentinel)
	require.NoError(t, err)

	// allocate three lanes
	var lanes []uint64
	for i := 0; i < 3; i++ {
		lane, err := paymentCreator.PaychAllocateLane(ctx, channel)
		require.NoError(t, err)
		lanes = append(lanes, lane)
	}

	// Make two vouchers each for each lane, then save on the other side
	// Note that the voucher with a value of 2000 has a higher nonce, so it
	// supersedes the voucher with a value of 1000
	for _, lane := range lanes {
		vouch1, err := paymentCreator.PaychVoucherCreate(ctx, channel, abi.NewTokenAmount(1000), lane)
		require.NoError(t, err)
		require.NotNil(t, vouch1.Voucher, "Not enough funds to create voucher: missing %d", vouch1.Shortfall)

		vouch2, err := paymentCreator.PaychVoucherCreate(ctx, channel, abi.NewTokenAmount(2000), lane)
		require.NoError(t, err)
		require.NotNil(t, vouch2.Voucher, "Not enough funds to create voucher: missing %d", vouch2.Shortfall)

		delta1, err := paymentReceiver.PaychVoucherAdd(ctx, channel, vouch1.Voucher, nil, abi.NewTokenAmount(1000))
		require.NoError(t, err)
		require.EqualValues(t, abi.NewTokenAmount(1000), delta1, "voucher didn't have the right amount")

		delta2, err := paymentReceiver.PaychVoucherAdd(ctx, channel, vouch2.Voucher, nil, abi.NewTokenAmount(1000))
		require.NoError(t, err)
		require.EqualValues(t, abi.NewTokenAmount(1000), delta2, "voucher didn't have the right amount")
	}

	// settle the payment channel
	settleMsgCid, err := paymentCreator.PaychSettle(ctx, channel)
	require.NoError(t, err)

	res := waitForMessage(ctx, t, paymentCreator, settleMsgCid, time.Second*10, "settle")
	require.EqualValues(t, 0, res.Receipt.ExitCode, "Unable to settle payment channel")

	creatorStore := adt.WrapStore(ctx, cbor.NewCborStore(blockstore.NewAPIBlockstore(paymentCreator)))

	// wait for the receiver to submit their vouchers
	ev, err := events.NewEvents(ctx, paymentCreator)
	require.NoError(t, err)
	preds := state.NewStatePredicates(paymentCreator)
	finished := make(chan struct{})
	err = ev.StateChanged(func(ctx context.Context, ts *types.TipSet) (done bool, more bool, err error) {
		act, err := paymentCreator.StateGetActor(ctx, channel, ts.Key())
		if err != nil {
			return false, false, err
		}
		state, err := paych.Load(creatorStore, act)
		if err != nil {
			return false, false, err
		}
		toSend, err := state.ToSend()
		if err != nil {
			return false, false, err
		}
		if toSend.GreaterThanEqual(abi.NewTokenAmount(6000)) {
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
	}, int(build.MessageConfidence)+1, build.Finality, func(oldTs, newTs *types.TipSet) (bool, events.StateChange, error) {
		return preds.OnPaymentChannelActorChanged(channel, preds.OnToSendAmountChanges())(ctx, oldTs.Key(), newTs.Key())
	})
	require.NoError(t, err)

	select {
	case <-finished:
	case <-time.After(10 * time.Second):
		t.Fatal("Timed out waiting for receiver to submit vouchers")
	}

	// Create a new voucher now that some vouchers have already been submitted
	vouchRes, err := paymentCreator.PaychVoucherCreate(ctx, channel, abi.NewTokenAmount(1000), 3)
	require.NoError(t, err)
	require.NotNil(t, vouchRes.Voucher, "Not enough funds to create voucher: missing %d", vouchRes.Shortfall)

	vdelta, err := paymentReceiver.PaychVoucherAdd(ctx, channel, vouchRes.Voucher, nil, abi.NewTokenAmount(1000))
	require.NoError(t, err)
	require.EqualValues(t, abi.NewTokenAmount(1000), vdelta, "voucher didn't have the right amount")

	// Create a new voucher whose value would exceed the channel balance
	excessAmt := abi.NewTokenAmount(1000)
	vouchRes, err = paymentCreator.PaychVoucherCreate(ctx, channel, excessAmt, 4)
	require.NoError(t, err)
	require.Nil(t, vouchRes.Voucher, "Expected not to be able to create voucher whose value would exceed channel balance")
	require.EqualValues(t, excessAmt, vouchRes.Shortfall, "Expected voucher shortfall of %d, got %d", excessAmt, vouchRes.Shortfall)

	// Add a voucher whose value would exceed the channel balance
	vouch := &paych.SignedVoucher{ChannelAddr: channel, Amount: excessAmt, Lane: 4, Nonce: 1}
	vb, err := vouch.SigningBytes()
	require.NoError(t, err)

	sig, err := paymentCreator.WalletSign(ctx, createrAddr, vb)
	require.NoError(t, err)

	vouch.Signature = sig
	_, err = paymentReceiver.PaychVoucherAdd(ctx, channel, vouch, nil, abi.NewTokenAmount(1000))
	require.Errorf(t, err, "Expected shortfall error of %d", excessAmt)

	// wait for the settlement period to pass before collecting
	waitForBlocks(ctx, t, bm, paymentReceiver, receiverAddr, policy.PaychSettleDelay)

	creatorPreCollectBalance, err := paymentCreator.WalletBalance(ctx, createrAddr)
	require.NoError(t, err)

	// collect funds (from receiver, though either party can do it)
	collectMsg, err := paymentReceiver.PaychCollect(ctx, channel)
	require.NoError(t, err)

	res, err = paymentReceiver.StateWaitMsg(ctx, collectMsg, 3, api.LookbackNoLimit, true)
	require.NoError(t, err)
	require.EqualValues(t, 0, res.Receipt.ExitCode, "unable to collect on payment channel")

	// Finally, check the balance for the creator
	currentCreatorBalance, err := paymentCreator.WalletBalance(ctx, createrAddr)
	require.NoError(t, err)

	// The highest nonce voucher that the creator sent on each lane is 2000
	totalVouchers := int64(len(lanes) * 2000)

	// When receiver submits the tokens to the chain, creator should get a
	// refund on the remaining balance, which is
	// channel amount - total voucher value
	expectedRefund := channelAmt - totalVouchers
	delta := big.Sub(currentCreatorBalance, creatorPreCollectBalance)
	require.EqualValues(t, abi.NewTokenAmount(expectedRefund), delta, "did not send correct funds from creator: expected %d, got %d", expectedRefund, delta)
}

func waitForBlocks(ctx context.Context, t *testing.T, bm *kit.BlockMiner, paymentReceiver kit.TestFullNode, receiverAddr address.Address, count int) {
	// We need to add null blocks in batches, if we add too many the chain can't sync
	batchSize := 60
	for i := 0; i < count; i += batchSize {
		size := batchSize
		if i > count {
			size = count - i
		}

		// Add a batch of null blocks to advance the chain quicker through finalities.
		bm.InjectNulls(abi.ChainEpoch(size - 1))

		// Add a real block
		m, err := paymentReceiver.MpoolPushMessage(ctx, &types.Message{
			To:    builtin.BurntFundsActorAddr,
			From:  receiverAddr,
			Value: types.NewInt(0),
		}, nil)
		require.NoError(t, err)

		_, err = paymentReceiver.StateWaitMsg(ctx, m.Cid(), 1, api.LookbackNoLimit, true)
		require.NoError(t, err)
	}
}

func waitForMessage(ctx context.Context, t *testing.T, paymentCreator kit.TestFullNode, msgCid cid.Cid, duration time.Duration, desc string) *api.MsgLookup {
	ctx, cancel := context.WithTimeout(ctx, duration)
	defer cancel()

	t.Log("Waiting for", desc)

	res, err := paymentCreator.StateWaitMsg(ctx, msgCid, 1, api.LookbackNoLimit, true)
	require.NoError(t, err)
	require.EqualValues(t, 0, res.Receipt.ExitCode, "did not successfully send %s", desc)

	t.Log("Confirmed", desc)
	return res
}
