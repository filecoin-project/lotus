package itests

import (
	"context"
	"testing"
	"time"

	"github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	actorstypes "github.com/filecoin-project/go-state-types/actors"
	"github.com/filecoin-project/go-state-types/big"
	init20 "github.com/filecoin-project/go-state-types/builtin/v20/init"
	paychtypes "github.com/filecoin-project/go-state-types/builtin/v8/paych"
	"github.com/filecoin-project/go-state-types/exitcode"
	"github.com/filecoin-project/go-state-types/network"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/adt"
	"github.com/filecoin-project/lotus/chain/actors/builtin"
	"github.com/filecoin-project/lotus/chain/actors/builtin/paych"
	"github.com/filecoin-project/lotus/chain/actors/policy"
	"github.com/filecoin-project/lotus/chain/consensus/filcns"
	"github.com/filecoin-project/lotus/chain/events"
	"github.com/filecoin-project/lotus/chain/events/state"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/itests/kit"
	"github.com/filecoin-project/lotus/node/config"
)

func TestPaymentChannelsAPI(t *testing.T) {

	kit.QuietMiningLogs()

	ctx := context.Background()
	blockTime := 10 * time.Millisecond
	const upgradeHeight abi.ChainEpoch = 1000

	var (
		paymentCreator  kit.TestFullNode
		paymentReceiver kit.TestFullNode
		miner           kit.TestMiner
	)

	enablePaychOpt := kit.WithCfgOpt(func(cfg *config.FullNode) error {
		cfg.PaymentChannels = config.PaymentChannelsConfig{
			EnablePaymentChannelManager: true,
		}
		return nil
	})
	ens := kit.NewEnsemble(t, kit.MockProofs(), kit.UpgradeSchedule(
		stmgr.Upgrade{Network: network.Version29, Height: -1},
		stmgr.Upgrade{Network: network.Version30, Height: upgradeHeight, Migration: filcns.UpgradeActorsV20},
	)).
		FullNode(&paymentCreator, kit.ThroughRPC(), enablePaychOpt).
		FullNode(&paymentReceiver, kit.ThroughRPC(), enablePaychOpt).
		Miner(&miner, &paymentCreator, kit.WithAllSubsystems()).
		Start().
		InterconnectAll()
	ens.BeginMiningMustPost(blockTime)
	// PaychGetWaitReady and the other paych calls wait on the chain with this context.
	ctx = ens.FailureContext(ctx)

	waitRecvInSync := func() {
		// paymentCreator is the block miner, in some cases paymentReceiver may fall behind, so we wait for it to catch up

		head, err := paymentReceiver.ChainHead(ctx)
		require.NoError(t, err)

		paymentReceiver.WaitTillChain(ctx, kit.HeightAtLeast(head.Height()))
	}

	// send some funds to register the receiver
	receiverAddr, err := paymentReceiver.WalletNew(ctx, types.KTSecp256k1)
	require.NoError(t, err)

	kit.SendFunds(ctx, t, &paymentCreator, receiverAddr, abi.NewTokenAmount(1e18))

	// setup the payment channel
	createrAddr, err := paymentCreator.WalletDefaultAddress(ctx)
	require.NoError(t, err)

	channelAmt := int64(6000)
	channelInfo, err := paymentCreator.PaychGet(ctx, createrAddr, receiverAddr, abi.NewTokenAmount(channelAmt), api.PaychGetOpts{
		OffChain: false,
	})
	require.NoError(t, err)

	channel, err := paymentCreator.PaychGetWaitReady(ctx, channelInfo.WaitSentinel)
	require.NoError(t, err)

	before, err := paymentCreator.StateGetActor(ctx, channel, types.EmptyTSK)
	require.NoError(t, err)
	nv, err := paymentCreator.StateNetworkVersion(ctx, types.EmptyTSK)
	require.NoError(t, err)
	require.Equal(t, network.Version29, nv)
	paymentCreator.WaitTillChain(ctx, kit.HeightAtLeast(upgradeHeight+5))
	paymentReceiver.WaitTillChain(ctx, kit.HeightAtLeast(upgradeHeight+5))
	after, err := paymentCreator.StateGetActor(ctx, channel, types.EmptyTSK)
	require.NoError(t, err)
	require.NotEqual(t, before.Code, after.Code)
	require.Equal(t, before.Head, after.Head)
	require.Equal(t, before.Balance, after.Balance)

	topUp, err := paymentCreator.PaychFund(ctx, createrAddr, receiverAddr, abi.NewTokenAmount(1000))
	require.NoError(t, err)
	fundedChannel, err := paymentCreator.PaychGetWaitReady(ctx, topUp.WaitSentinel)
	require.NoError(t, err)
	require.Equal(t, channel, fundedChannel)
	channelAmt += 1000

	waitRecvInSync()

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

	waitRecvInSync()

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
	}, int(buildconstants.MessageConfidence)+1, policy.ChainFinality, func(oldTs, newTs *types.TipSet) (bool, events.StateChange, error) {
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
	vouch := &paychtypes.SignedVoucher{ChannelAddr: channel, Amount: excessAmt, Lane: 4, Nonce: 1}
	vb, err := vouch.SigningBytes()
	require.NoError(t, err)

	sig, err := paymentCreator.WalletSign(ctx, createrAddr, vb)
	require.NoError(t, err)

	vouch.Signature = sig
	_, err = paymentReceiver.PaychVoucherAdd(ctx, channel, vouch, nil, abi.NewTokenAmount(1000))
	require.Errorf(t, err, "Expected shortfall error of %d", excessAmt)

	// wait for the settlement period to pass before collecting
	head, err := paymentReceiver.ChainHead(ctx)
	require.NoError(t, err)

	settleHeight := head.Height() + policy.PaychSettleDelay + 5
	paymentReceiver.WaitTillChain(ctx, kit.HeightAtLeast(settleHeight))
	paymentCreator.WaitTillChain(ctx, kit.HeightAtLeast(settleHeight))

	creatorPreCollectBalance, err := paymentCreator.WalletBalance(ctx, createrAddr)
	require.NoError(t, err)

	// collect funds (from receiver, though either party can do it)
	collectMsg, err := paymentReceiver.PaychCollect(ctx, channel)
	require.NoError(t, err)

	res, err = paymentReceiver.WaitMsgResult(ctx, collectMsg, 3)
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

func TestPaymentChannelsNV30(t *testing.T) {
	kit.QuietMiningLogs()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	const upgradeHeight abi.ChainEpoch = 500
	client, _, ens := kit.EnsembleMinimal(t, kit.MockProofs(), kit.ThroughRPC(),
		kit.WithCfgOpt(func(cfg *config.FullNode) error {
			cfg.PaymentChannels.EnablePaymentChannelManager = true
			return nil
		}),
		kit.UpgradeSchedule(
			stmgr.Upgrade{Network: network.Version29, Height: -1},
			stmgr.Upgrade{Network: network.Version30, Height: upgradeHeight, Migration: filcns.UpgradeActorsV20},
		))
	ens.InterconnectAll().BeginMiningMustPost(20 * time.Millisecond)
	ctx = ens.FailureContext(ctx)
	from := client.DefaultKey.Address
	to, err := client.WalletNew(ctx, types.KTBLS)
	require.NoError(t, err)
	kit.SendFunds(ctx, t, client, to, abi.NewTokenAmount(1e18))
	info, err := client.PaychGet(ctx, from, to, abi.NewTokenAmount(1000), api.PaychGetOpts{})
	require.NoError(t, err)
	channel, err := client.PaychGetWaitReady(ctx, info.WaitSentinel)
	require.NoError(t, err)
	voucher := &paychtypes.SignedVoucher{
		ChannelAddr: channel, Amount: abi.NewTokenAmount(300), Lane: 42, Nonce: 7,
		MinSettleHeight: upgradeHeight + 50,
	}
	encoded, err := voucher.SigningBytes()
	require.NoError(t, err)
	voucher.Signature, err = client.WalletSign(ctx, from, encoded)
	require.NoError(t, err)
	update, err := paych.Message(actorstypes.Version19, to).Update(channel, voucher, nil)
	require.NoError(t, err)
	signed, err := client.MpoolPushMessage(ctx, update, nil)
	require.NoError(t, err)
	updated, err := client.WaitMsgResult(ctx, signed.Cid(), 3)
	require.NoError(t, err)
	kit.RequireMessageSuccess(t, updated)
	settleCID, err := client.PaychSettle(ctx, channel)
	require.NoError(t, err)
	settled, err := client.WaitMsgResult(ctx, settleCID, 3)
	require.NoError(t, err)
	kit.RequireMessageSuccess(t, settled)
	before, err := client.ChainHead(ctx)
	require.NoError(t, err)
	require.Less(t, before.Height(), upgradeHeight)
	oldActor, err := client.StateGetActor(ctx, channel, before.Key())
	require.NoError(t, err)
	store := adt.WrapStore(ctx, cbor.NewCborStore(blockstore.NewAPIBlockstore(client)))
	oldState, err := paych.Load(store, oldActor)
	require.NoError(t, err)
	settlingAt, err := oldState.SettlingAt()
	require.NoError(t, err)
	require.Greater(t, settlingAt, upgradeHeight)

	oldCreate, err := paych.Message(actorstypes.Version19, from).Create(to, abi.NewTokenAmount(1000))
	require.NoError(t, err)
	allowed, err := client.StateCall(ctx, oldCreate, before.Key())
	require.NoError(t, err)
	require.Equal(t, exitcode.Ok, allowed.MsgRct.ExitCode, allowed.Error)
	client.WaitTillChain(ctx, kit.HeightAtLeast(upgradeHeight+5))

	create, err := paych.Message(actorstypes.Version20, from).Create(to, abi.NewTokenAmount(1000))
	require.NoError(t, err)
	denied, err := client.StateCall(ctx, create, types.EmptyTSK)
	require.NoError(t, err)
	require.Equal(t, exitcode.ErrForbidden, denied.MsgRct.ExitCode, denied.Error)
	allowed, err = client.StateCall(ctx, oldCreate, before.Key())
	require.NoError(t, err)
	require.Equal(t, exitcode.Ok, allowed.MsgRct.ExitCode, allowed.Error)

	newActor, err := client.StateGetActor(ctx, channel, types.EmptyTSK)
	require.NoError(t, err)
	require.NotEqual(t, oldActor.Code, newActor.Code)
	require.Equal(t, oldActor.Head, newActor.Head)
	require.Equal(t, oldActor.Balance, newActor.Balance)
	newState, err := paych.Load(store, newActor)
	require.NoError(t, err)
	require.Equal(t, actorstypes.Version20, newState.ActorVersion())
	newSettlingAt, err := newState.SettlingAt()
	require.NoError(t, err)
	require.Equal(t, settlingAt, newSettlingAt)
	toSend, err := newState.ToSend()
	require.NoError(t, err)
	require.Equal(t, abi.NewTokenAmount(300), toSend)
	lanes, err := newState.LaneCount()
	require.NoError(t, err)
	require.EqualValues(t, 1, lanes)
	require.NoError(t, newState.ForEachLaneState(func(index uint64, lane paych.LaneState) error {
		require.EqualValues(t, 42, index)
		nonce, err := lane.Nonce()
		require.NoError(t, err)
		require.EqualValues(t, 7, nonce)
		redeemed, err := lane.Redeemed()
		require.NoError(t, err)
		require.Equal(t, abi.NewTokenAmount(300), redeemed)
		return nil
	}))

	initBefore, err := client.StateGetActor(ctx, builtin.InitActorAddr, types.EmptyTSK)
	require.NoError(t, err)
	for _, tc := range []struct {
		name      string
		value     abi.TokenAmount
		malformed bool
	}{{"funded", abi.NewTokenAmount(1000), false}, {"zero", big.Zero(), false}, {"malformed constructor", big.Zero(), true}} {
		t.Run(tc.name, func(t *testing.T) {
			message, err := paych.Message(actorstypes.Version20, from).Create(to, tc.value)
			require.NoError(t, err)
			if tc.malformed {
				code, ok := actors.GetActorCodeID(actorstypes.Version20, "paymentchannel")
				require.True(t, ok)
				message.Params, err = actors.SerializeParams(&init20.ExecParams{CodeCID: code, ConstructorParams: []byte{0xff}})
				require.NoError(t, err)
			}
			// Explicit gas bypasses estimation so rejection is checked in a mined receipt.
			message.GasLimit = 10_000_000
			signed, err := client.MpoolPushMessage(ctx, message, nil)
			require.NoError(t, err)
			result, err := client.WaitMsgResult(ctx, signed.Cid(), 3)
			require.NoError(t, err)
			require.Equal(t, exitcode.ErrForbidden, result.Receipt.ExitCode)
		})
	}
	initAfter, err := client.StateGetActor(ctx, builtin.InitActorAddr, types.EmptyTSK)
	require.NoError(t, err)
	require.Equal(t, initBefore.Head, initAfter.Head)
	require.Equal(t, initBefore.Balance, initAfter.Balance)

	for _, fund := range []bool{false, true} {
		if fund {
			_, err = client.PaychFund(ctx, from, to, abi.NewTokenAmount(1000))
		} else {
			_, err = client.PaychGet(ctx, from, to, abi.NewTokenAmount(1000), api.PaychGetOpts{})
		}
		require.ErrorContains(t, err, "cannot exec actor type")
		available, err := client.PaychAvailableFundsByFromTo(ctx, from, to)
		require.NoError(t, err)
		require.Nil(t, available.Channel)
		require.Nil(t, available.PendingWaitSentinel)
	}
	channels, err := client.PaychList(ctx)
	require.NoError(t, err)
	require.Equal(t, []address.Address{channel}, channels)

	client.WaitTillChain(ctx, kit.HeightAtLeast(settlingAt+5))
	receiverBalance, err := client.WalletBalance(ctx, to)
	require.NoError(t, err)
	collectCID, err := client.PaychCollect(ctx, channel)
	require.NoError(t, err)
	collected, err := client.WaitMsgResult(ctx, collectCID, 3)
	require.NoError(t, err)
	kit.RequireMessageSuccess(t, collected)
	paidBalance, err := client.WalletBalance(ctx, to)
	require.NoError(t, err)
	require.Equal(t, abi.NewTokenAmount(300), big.Sub(paidBalance, receiverBalance))
	_, err = client.StateGetActor(ctx, channel, types.EmptyTSK)
	require.ErrorContains(t, err, "actor not found")
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

func TestPaymentChannelsAPIDisabled(t *testing.T) {
	ctx := context.Background()
	kit.QuietMiningLogs()
	full, _, _ := kit.EnsembleMinimal(t, kit.MockProofs())

	// Call APIs to confirm they return ErrPaymentChannelDisabled
	// Using errors.Is() to verify the error properly traverses the RPC boundary
	_, err := full.PaychGet(ctx, address.TestAddress, address.TestAddress2, abi.NewTokenAmount(0), api.PaychGetOpts{})
	require.ErrorIs(t, err, api.ErrPaymentChannelDisabled)
	_, err = full.PaychFund(ctx, address.TestAddress, address.TestAddress2, abi.NewTokenAmount(0))
	require.ErrorIs(t, err, api.ErrPaymentChannelDisabled)
	_, err = full.PaychAvailableFunds(ctx, address.TestAddress)
	require.ErrorIs(t, err, api.ErrPaymentChannelDisabled)
	_, err = full.PaychAvailableFundsByFromTo(ctx, address.TestAddress, address.TestAddress2)
	require.ErrorIs(t, err, api.ErrPaymentChannelDisabled)
	_, err = full.PaychGetWaitReady(ctx, cid.Undef)
	require.ErrorIs(t, err, api.ErrPaymentChannelDisabled)
	_, err = full.PaychAllocateLane(ctx, address.TestAddress)
	require.ErrorIs(t, err, api.ErrPaymentChannelDisabled)
	_, err = full.PaychNewPayment(ctx, address.TestAddress, address.TestAddress2, nil)
	require.ErrorIs(t, err, api.ErrPaymentChannelDisabled)
	_, err = full.PaychList(ctx)
	require.ErrorIs(t, err, api.ErrPaymentChannelDisabled)
	_, err = full.PaychStatus(ctx, address.TestAddress)
	require.ErrorIs(t, err, api.ErrPaymentChannelDisabled)
	_, err = full.PaychSettle(ctx, address.TestAddress)
	require.ErrorIs(t, err, api.ErrPaymentChannelDisabled)
	_, err = full.PaychCollect(ctx, address.TestAddress)
	require.ErrorIs(t, err, api.ErrPaymentChannelDisabled)
	err = full.PaychVoucherCheckValid(ctx, address.TestAddress, &paychtypes.SignedVoucher{})
	require.ErrorIs(t, err, api.ErrPaymentChannelDisabled)
	_, err = full.PaychVoucherCheckSpendable(ctx, address.TestAddress, &paychtypes.SignedVoucher{}, nil, nil)
	require.ErrorIs(t, err, api.ErrPaymentChannelDisabled)
	_, err = full.PaychVoucherAdd(ctx, address.TestAddress, &paychtypes.SignedVoucher{}, nil, abi.NewTokenAmount(0))
	require.ErrorIs(t, err, api.ErrPaymentChannelDisabled)
	_, err = full.PaychVoucherCreate(ctx, address.TestAddress, abi.NewTokenAmount(0), 0)
	require.ErrorIs(t, err, api.ErrPaymentChannelDisabled)
	_, err = full.PaychVoucherList(ctx, address.TestAddress)
	require.ErrorIs(t, err, api.ErrPaymentChannelDisabled)
	_, err = full.PaychVoucherSubmit(ctx, address.TestAddress, &paychtypes.SignedVoucher{}, nil, nil)
	require.ErrorIs(t, err, api.ErrPaymentChannelDisabled)
}
