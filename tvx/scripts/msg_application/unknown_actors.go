package main

import (
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/specs-actors/actors/runtime/exitcode"

	. "github.com/filecoin-project/oni/tvx/builders"
)

func failUnknownSender(v *Builder) {
	v.Messages.SetDefaults(GasLimit(1_000_000_000), GasPremium(1), GasFeeCap(200))

	alice := v.Actors.Account(address.SECP256K1, balance1T)
	v.CommitPreconditions()

	v.Messages.Sugar().Transfer(unknown, alice.ID, Value(transferAmnt), Nonce(0))
	v.CommitApplies()

	v.Assert.EveryMessageResultSatisfies(ExitCode(exitcode.SysErrSenderInvalid))
}

func failUnknownReceiver(v *Builder) {
	v.Messages.SetDefaults(GasLimit(1_000_000_000), GasPremium(1), GasFeeCap(200))

	alice := v.Actors.Account(address.SECP256K1, balance1T)
	v.CommitPreconditions()

	// Sending a message to non-existent ID address must produce an error.
	unknownID := MustNewIDAddr(10000000)
	v.Messages.Sugar().Transfer(alice.ID, unknownID, Value(transferAmnt), Nonce(0))

	unknownActor := MustNewActorAddr("1234")
	v.Messages.Sugar().Transfer(alice.ID, unknownActor, Value(transferAmnt), Nonce(1))
	v.CommitApplies()

	v.Assert.EveryMessageResultSatisfies(ExitCode(exitcode.SysErrInvalidReceiver))
}
