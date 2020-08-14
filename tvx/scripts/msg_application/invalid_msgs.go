package main

import (
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/specs-actors/actors/abi/big"
	"github.com/filecoin-project/specs-actors/actors/runtime/exitcode"

	. "github.com/filecoin-project/oni/tvx/builders"
)

func failInvalidActorNonce(v *Builder) {
	v.Messages.SetDefaults(GasLimit(1_000_000_000), GasPrice(1))

	alice := v.Actors.Account(address.SECP256K1, balance1T)
	v.CommitPreconditions()

	// invalid nonce from known account.
	msg1 := v.Messages.Sugar().Transfer(alice.ID, alice.ID, Value(transferAmnt), Nonce(1))

	// invalid nonce from an unknown account.
	msg2 := v.Messages.Sugar().Transfer(unknown, alice.ID, Value(transferAmnt), Nonce(1))
	v.CommitApplies()

	v.Assert.Equal(msg1.Result.ExitCode, exitcode.SysErrSenderStateInvalid)
	v.Assert.Equal(msg2.Result.ExitCode, exitcode.SysErrSenderInvalid)
}

func failInvalidReceiverMethod(v *Builder) {
	v.Messages.SetDefaults(GasLimit(1_000_000_000), GasPrice(1))

	alice := v.Actors.Account(address.SECP256K1, balance1T)
	v.CommitPreconditions()

	v.Messages.Typed(alice.ID, alice.ID, MarketComputeDataCommitment(nil), Nonce(0), Value(big.Zero()))
	v.CommitApplies()

	v.Assert.EveryMessageResultSatisfies(ExitCode(exitcode.SysErrInvalidMethod))
}
