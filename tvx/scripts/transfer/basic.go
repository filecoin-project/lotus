package main

import (
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/runtime/exitcode"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/specs-actors/actors/abi/big"

	. "github.com/filecoin-project/oni/tvx/builders"
)

type basicTransferParams struct {
	senderType   address.Protocol
	senderBal    abi.TokenAmount
	receiverType address.Protocol
	amount       abi.TokenAmount
	exitCode     exitcode.ExitCode
}

func basicTransfer(params basicTransferParams) func(v *Builder) {
	return func(v *Builder) {
		v.Messages.SetDefaults(GasLimit(gasLimit), GasPremium(1), GasFeeCap(gasFeeCap))

		// Set up sender and receiver accounts.
		var sender, receiver AddressHandle
		sender = v.Actors.Account(params.senderType, params.senderBal)
		receiver = v.Actors.Account(params.receiverType, big.Zero())
		v.CommitPreconditions()

		// Perform the transfer.
		v.Messages.Sugar().Transfer(sender.ID, receiver.ID, Value(params.amount), Nonce(0))
		v.CommitApplies()

		v.Assert.EveryMessageResultSatisfies(ExitCode(params.exitCode))
		v.Assert.EveryMessageSenderSatisfies(BalanceUpdated(big.Zero()))

		if params.exitCode.IsSuccess() {
			v.Assert.EveryMessageSenderSatisfies(NonceUpdated())
			v.Assert.BalanceEq(receiver.ID, params.amount)
		}
	}
}
