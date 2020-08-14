package main

import (
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/abi/big"
	init_ "github.com/filecoin-project/specs-actors/actors/builtin/init"
	"github.com/filecoin-project/specs-actors/actors/builtin/paych"
	"github.com/filecoin-project/specs-actors/actors/crypto"
	"github.com/filecoin-project/specs-actors/actors/runtime/exitcode"

	. "github.com/filecoin-project/oni/tvx/builders"
)

func happyPathCreate(v *Builder) {
	v.Messages.SetDefaults(GasLimit(1_000_000_000), GasPrice(1))

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
}

func happyPathUpdate(v *Builder) {
	v.Messages.SetDefaults(GasLimit(1_000_000_000), GasPrice(1))

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
	v.Assert.Len(state.LaneStates, 1)

	ls := state.LaneStates[0]
	v.Assert.Equal(amount, ls.Redeemed)
	v.Assert.Equal(nonce, ls.Nonce)
	v.Assert.Equal(lane, ls.ID)
}

func happyPathCollect(v *Builder) {
	v.Messages.SetDefaults(GasLimit(1_000_000_000), GasPrice(1))

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
	v.Messages.Sugar().CreatePaychActor(sender.Robust, receiver.Robust, Value(toSend))

	// Update the payment channel.
	v.Messages.Typed(sender.Robust, paychAddr.Robust, PaychUpdateChannelState(&paych.UpdateChannelStateParams{
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

	// receiver_balance = initial_balance + paych_send - settle_paych_msg_gas - collect_paych_msg_gas
	gasUsed := big.Add(big.NewInt(settleMsg.Result.MessageReceipt.GasUsed), big.NewInt(collectMsg.Result.MessageReceipt.GasUsed))
	v.Assert.BalanceEq(receiver.Robust, big.Sub(big.Add(toSend, initialBal), gasUsed))

	// the paych actor should have been deleted after the collect
	v.Assert.ActorMissing(paychAddr.Robust)
	v.Assert.ActorMissing(paychAddr.ID)
}
