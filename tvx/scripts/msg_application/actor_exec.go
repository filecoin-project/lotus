package main

import (
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/abi/big"
	"github.com/filecoin-project/specs-actors/actors/builtin/paych"
	"github.com/filecoin-project/specs-actors/actors/crypto"
	"github.com/filecoin-project/specs-actors/actors/runtime/exitcode"

	. "github.com/filecoin-project/oni/tvx/builders"
)

func failActorExecutionAborted(v *Builder) {
	v.Messages.SetDefaults(GasLimit(1_000_000_000), GasPremium(1), GasFeeCap(200))

	// Set up sender and receiver accounts.
	var sender, receiver AddressHandle
	var paychAddr AddressHandle

	v.Actors.AccountN(address.SECP256K1, balance1T, &sender, &receiver)
	paychAddr = AddressHandle{
		ID:     MustNewIDAddr(MustIDFromAddress(receiver.ID) + 1),
		Robust: sender.NextActorAddress(0, 0),
	}
	v.CommitPreconditions()

	// Construct the payment channel.
	createMsg := v.Messages.Sugar().CreatePaychActor(sender.Robust, receiver.Robust, Value(abi.NewTokenAmount(10_000)))

	// Update the payment channel.
	updateMsg := v.Messages.Typed(sender.Robust, paychAddr.Robust, PaychUpdateChannelState(&paych.UpdateChannelStateParams{
		Sv: paych.SignedVoucher{
			ChannelAddr: paychAddr.Robust,
			TimeLockMin: abi.ChainEpoch(10),
			Lane:        123,
			Nonce:       1,
			Amount:      big.NewInt(10),
			Signature: &crypto.Signature{
				Type: crypto.SigTypeBLS,
				Data: []byte("Grrr im an invalid signature, I cause panics in the payment channel actor"),
			},
		}}), Nonce(1), Value(big.Zero()))

	v.CommitApplies()

	v.Assert.Equal(exitcode.Ok, createMsg.Result.ExitCode)
	v.Assert.Equal(exitcode.ErrIllegalArgument, updateMsg.Result.ExitCode)
}
