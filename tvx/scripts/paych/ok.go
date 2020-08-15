package main

import (
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/abi/big"
	init_ "github.com/filecoin-project/specs-actors/actors/builtin/init"
	"github.com/filecoin-project/specs-actors/actors/builtin/paych"
	"github.com/filecoin-project/specs-actors/actors/crypto"
	"github.com/filecoin-project/specs-actors/actors/runtime/exitcode"
	"github.com/filecoin-project/specs-actors/actors/util/adt"

	. "github.com/filecoin-project/oni/tvx/builders"
)

func happyPathCreate(v *Builder) {
	v.Messages.SetDefaults(GasLimit(1_000_000_000), GasPremium(1), GasFeeCap(200))

	// Set up sender and receiver accounts.
	var sender, receiver AddressHandle
	v.Actors.AccountN(address.SECP256K1, initialBal, &sender, &receiver)
	v.CommitPreconditions()

	// Add the constructor message.
	createMsg := v.Messages.Sugar().CreatePaychActor(sender.Robust, receiver.Robust, Value(toSend))
	v.CommitApplies()

	expectedActorAddr := AddressHandle{
		ID:     MustNewIDAddr(MustIDFromAddress(receiver.ID) + 1),
		Robust: sender.NextActorAddress(0, 0),
	}

	// Verify init actor return.
	var ret init_.ExecReturn
	MustDeserialize(createMsg.Result.Return, &ret)
	v.Assert.Equal(expectedActorAddr.Robust, ret.RobustAddress)
	v.Assert.Equal(expectedActorAddr.ID, ret.IDAddress)

	// Verify the paych state.
	var state paych.State
	actor := v.Actors.ActorState(ret.IDAddress, &state)
	v.Assert.Equal(sender.ID, state.From)
	v.Assert.Equal(receiver.ID, state.To)
	v.Assert.Equal(toSend, actor.Balance)

	v.Assert.EveryMessageSenderSatisfies(NonceUpdated())
}

func happyPathUpdate(v *Builder) {
	v.Messages.SetDefaults(GasLimit(1_000_000_000), GasPremium(1), GasFeeCap(200))

	var (
		timelock = abi.ChainEpoch(0)
		lane     = uint64(123)
		nonce    = uint64(1)
		amount   = big.NewInt(10)
	)

	// Set up sender and receiver accounts.
	var sender, receiver AddressHandle
	var paychAddr AddressHandle

	v.Actors.AccountN(address.SECP256K1, initialBal, &sender, &receiver)
	paychAddr = AddressHandle{
		ID:     MustNewIDAddr(MustIDFromAddress(receiver.ID) + 1),
		Robust: sender.NextActorAddress(0, 0),
	}
	v.CommitPreconditions()

	// Construct the payment channel.
	createMsg := v.Messages.Sugar().CreatePaychActor(sender.Robust, receiver.Robust, Value(toSend))

	// Update the payment channel.
	v.Messages.Typed(sender.Robust, paychAddr.Robust, PaychUpdateChannelState(&paych.UpdateChannelStateParams{
		Sv: paych.SignedVoucher{
			ChannelAddr:     paychAddr.Robust,
			TimeLockMin:     timelock,
			TimeLockMax:     0, // TimeLockMax set to 0 means no timeout
			Lane:            lane,
			Nonce:           nonce,
			Amount:          amount,
			MinSettleHeight: 0,
			Signature: &crypto.Signature{
				Type: crypto.SigTypeBLS,
				Data: []byte("signature goes here"), // TODO may need to generate an actual signature
			},
		}}), Nonce(1), Value(big.Zero()))

	v.CommitApplies()

	// all messages succeeded.
	v.Assert.EveryMessageResultSatisfies(ExitCode(exitcode.Ok))

	// Verify init actor return.
	var ret init_.ExecReturn
	MustDeserialize(createMsg.Result.Return, &ret)

	// Verify the paych state.
	var state paych.State
	v.Actors.ActorState(ret.RobustAddress, &state)

	arr, err := adt.AsArray(v.Stores.ADTStore, state.LaneStates)
	v.Assert.NoError(err)
	v.Assert.EqualValues(1, arr.Length())

	var ls paych.LaneState
	found, err := arr.Get(lane, &ls)
	v.Assert.NoError(err)
	v.Assert.True(found)

	v.Assert.Equal(amount, ls.Redeemed)
	v.Assert.Equal(nonce, ls.Nonce)

	v.Assert.EveryMessageSenderSatisfies(NonceUpdated())
}

func happyPathCollect(v *Builder) {
	v.Messages.SetDefaults(GasLimit(1_000_000_000), GasPremium(1), GasFeeCap(200))

	// Set up sender and receiver accounts.
	var sender, receiver AddressHandle
	var paychAddr AddressHandle
	v.Actors.AccountN(address.SECP256K1, initialBal, &sender, &receiver)
	paychAddr = AddressHandle{
		ID:     MustNewIDAddr(MustIDFromAddress(receiver.ID) + 1),
		Robust: sender.NextActorAddress(0, 0),
	}

	v.CommitPreconditions()

	// Construct the payment channel.
	createMsg := v.Messages.Sugar().CreatePaychActor(sender.Robust, receiver.Robust, Value(toSend))

	// Update the payment channel.
	updateMsg := v.Messages.Typed(sender.Robust, paychAddr.Robust, PaychUpdateChannelState(&paych.UpdateChannelStateParams{
		Sv: paych.SignedVoucher{
			ChannelAddr:     paychAddr.Robust,
			TimeLockMin:     0,
			TimeLockMax:     0, // TimeLockMax set to 0 means no timeout
			Lane:            1,
			Nonce:           1,
			Amount:          toSend,
			MinSettleHeight: 0,
			Signature: &crypto.Signature{
				Type: crypto.SigTypeBLS,
				Data: []byte("signature goes here"), // TODO may need to generate an actual signature
			},
		}}), Nonce(1), Value(big.Zero()))

	settleMsg := v.Messages.Typed(receiver.Robust, paychAddr.Robust, PaychSettle(nil), Value(big.Zero()), Nonce(0))

	// advance the epoch so the funds may be redeemed.
	collectMsg := v.Messages.Typed(receiver.Robust, paychAddr.Robust, PaychCollect(nil), Value(big.Zero()), Nonce(1), Epoch(paych.SettleDelay))

	v.CommitApplies()

	// all messages succeeded.
	v.Assert.EveryMessageResultSatisfies(ExitCode(exitcode.Ok))

	v.Assert.MessageSendersSatisfy(BalanceUpdated(big.Zero()), createMsg, updateMsg)
	v.Assert.MessageSendersSatisfy(BalanceUpdated(toSend), settleMsg, collectMsg)
	v.Assert.EveryMessageSenderSatisfies(NonceUpdated())

	// the paych actor should have been deleted after the collect
	v.Assert.ActorMissing(paychAddr.Robust)
	v.Assert.ActorMissing(paychAddr.ID)
}
