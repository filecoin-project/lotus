package main

import (
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/runtime/exitcode"

	. "github.com/filecoin-project/oni/tvx/builders"
)

var (
	initial  = abi.NewTokenAmount(1_000_000_000_000)
	transfer = Value(abi.NewTokenAmount(10))
)

func failTransferUnknownSenderKnownReceiver(v *Builder) {
	v.Messages.SetDefaults(GasLimit(1_000_000_000), GasPremium(1), GasFeeCap(200))

	// Set up receiver account.
	receiver := v.Actors.Account(address.SECP256K1, initial)
	v.CommitPreconditions()

	// create a new random sender.
	sender := v.Wallet.NewSECP256k1Account()

	// perform the transfer.
	v.Messages.Sugar().Transfer(sender, receiver.Robust, transfer, Nonce(0))
	v.CommitApplies()

	v.Assert.EveryMessageResultSatisfies(ExitCode(exitcode.SysErrSenderInvalid))
	v.Assert.ActorMissing(sender)
	v.Assert.ActorExists(receiver.Robust)
	v.Assert.BalanceEq(receiver.Robust, initial)
}

func failTransferUnknownSenderUnknownReceiver(v *Builder) {
	v.Messages.SetDefaults(GasLimit(1_000_000_000), GasPremium(1), GasFeeCap(200))

	// no accounts in the system.
	v.CommitPreconditions()

	// create new random senders and resceivers.
	sender, receiver := v.Wallet.NewSECP256k1Account(), v.Wallet.NewSECP256k1Account()

	// perform the transfer.
	v.Messages.Sugar().Transfer(sender, receiver, transfer, Nonce(0))
	v.CommitApplies()

	v.Assert.EveryMessageResultSatisfies(ExitCode(exitcode.SysErrSenderInvalid))
	v.Assert.ActorMissing(sender)
	v.Assert.ActorMissing(receiver)
}
