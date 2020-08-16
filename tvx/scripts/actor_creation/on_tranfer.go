package main

import (
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/runtime/exitcode"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/specs-actors/actors/abi/big"

	. "github.com/filecoin-project/oni/tvx/builders"
)

type actorCreationOnTransferParams struct {
	senderType   address.Protocol
	senderBal    abi.TokenAmount
	receiverAddr address.Address
	amount       abi.TokenAmount
	exitCode     exitcode.ExitCode
}

func actorCreationOnTransfer(params actorCreationOnTransferParams) func(v *Builder) {
	return func(v *Builder) {
		v.Messages.SetDefaults(GasLimit(1_000_000_000), GasPremium(1), GasFeeCap(200))

		// Set up sender account.
		sender := v.Actors.Account(params.senderType, params.senderBal)
		v.CommitPreconditions()

		// Perform the transfer.
		v.Messages.Sugar().Transfer(sender.ID, params.receiverAddr, Value(params.amount), Nonce(0))
		v.CommitApplies()

		v.Assert.EveryMessageResultSatisfies(ExitCode(params.exitCode))
		v.Assert.EveryMessageSenderSatisfies(BalanceUpdated(big.Zero()))

		if params.exitCode.IsSuccess() {
			v.Assert.EveryMessageSenderSatisfies(NonceUpdated())
			v.Assert.BalanceEq(params.receiverAddr, params.amount)
		}
	}
}
