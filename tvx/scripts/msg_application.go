package main

import (
	"os"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/abi/big"
	"github.com/filecoin-project/specs-actors/actors/builtin/paych"
	"github.com/filecoin-project/specs-actors/actors/crypto"
	"github.com/filecoin-project/specs-actors/actors/runtime/exitcode"

	. "github.com/filecoin-project/oni/tvx/builders"
	"github.com/filecoin-project/oni/tvx/schema"
)

var (
	unknown      = MustNewIDAddr(10000000)
	balance1T    = abi.NewTokenAmount(1_000_000_000_000)
	transferAmnt = abi.NewTokenAmount(10)
)

func main() {
	failCoverReceiptGasCost()
	failCoverOnChainSizeGasCost()
	failUnknownSender()
	failInvalidActorNonce()
	failInvalidReceiverMethod()
	failInexistentReceiver()
	failCoverTransferAccountCreationGasStepwise()
	failActorExecutionAborted()
}

func failCoverReceiptGasCost() {
	metadata := &schema.Metadata{
		ID:      "msg-apply-fail-receipt-gas",
		Version: "v1",
		Desc:    "fail to cover gas cost for message receipt on chain",
	}

	v := MessageVector(metadata)
	v.Messages.SetDefaults(GasLimit(1_000_000_000), GasPrice(1))

	alice := v.Actors.Account(address.SECP256K1, balance1T)
	v.CommitPreconditions()

	v.Messages.Sugar().Transfer(alice.ID, alice.ID, Value(transferAmnt), Nonce(0), GasLimit(8))
	v.CommitApplies()

	v.Assert.EveryMessageResultSatisfies(ExitCode(exitcode.SysErrOutOfGas))
	v.Finish(os.Stdout)
}

func failCoverOnChainSizeGasCost() {
	metadata := &schema.Metadata{
		ID:      "msg-apply-fail-onchainsize-gas",
		Version: "v1",
		Desc:    "not enough gas to pay message on-chain-size cost",
	}

	v := MessageVector(metadata)
	v.Messages.SetDefaults(GasLimit(1_000_000_000), GasPrice(10))

	alice := v.Actors.Account(address.SECP256K1, balance1T)
	v.CommitPreconditions()

	v.Messages.Sugar().Transfer(alice.ID, alice.ID, Value(transferAmnt), Nonce(0), GasLimit(1))
	v.CommitApplies()

	v.Assert.EveryMessageResultSatisfies(ExitCode(exitcode.SysErrOutOfGas))
	v.Finish(os.Stdout)
}

func failUnknownSender() {
	metadata := &schema.Metadata{
		ID:      "msg-apply-fail-unknown-sender",
		Version: "v1",
		Desc:    "fail due to lack of gas when sender is unknown",
	}

	v := MessageVector(metadata)
	v.Messages.SetDefaults(GasLimit(1_000_000_000), GasPrice(1))

	alice := v.Actors.Account(address.SECP256K1, balance1T)
	v.CommitPreconditions()

	v.Messages.Sugar().Transfer(unknown, alice.ID, Value(transferAmnt), Nonce(0))
	v.CommitApplies()

	v.Assert.EveryMessageResultSatisfies(ExitCode(exitcode.SysErrSenderInvalid))
	v.Finish(os.Stdout)
}

func failInvalidActorNonce() {
	metadata := &schema.Metadata{
		ID:      "msg-apply-fail-invalid-nonce",
		Version: "v1",
		Desc:    "invalid actor nonce",
	}

	v := MessageVector(metadata)
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

	v.Finish(os.Stdout)
}

func failInvalidReceiverMethod() {
	metadata := &schema.Metadata{
		ID:      "msg-apply-fail-invalid-receiver-method",
		Version: "v1",
		Desc:    "invalid receiver method",
	}

	v := MessageVector(metadata)
	v.Messages.SetDefaults(GasLimit(1_000_000_000), GasPrice(1))

	alice := v.Actors.Account(address.SECP256K1, balance1T)
	v.CommitPreconditions()

	v.Messages.Typed(alice.ID, alice.ID, MarketComputeDataCommitment(nil), Nonce(0), Value(big.Zero()))
	v.CommitApplies()

	v.Assert.EveryMessageResultSatisfies(ExitCode(exitcode.SysErrInvalidMethod))

	v.Finish(os.Stdout)
}

func failInexistentReceiver() {
	metadata := &schema.Metadata{
		ID:      "msg-apply-fail-inexistent-receiver",
		Version: "v1",
		Desc:    "inexistent receiver",
		Comment: `Note that this test is not a valid message, since it is using
an unknown actor. However in the event that an invalid message isn't filtered by
block validation we need to ensure behaviour is consistent across VM implementations.`,
	}

	v := MessageVector(metadata)
	v.Messages.SetDefaults(GasLimit(1_000_000_000), GasPrice(1))

	alice := v.Actors.Account(address.SECP256K1, balance1T)
	v.CommitPreconditions()

	// Sending a message to non-existent ID address must produce an error.
	unknownID := MustNewIDAddr(10000000)
	v.Messages.Sugar().Transfer(alice.ID, unknownID, Value(transferAmnt), Nonce(0))

	unknownActor := MustNewActorAddr("1234")
	v.Messages.Sugar().Transfer(alice.ID, unknownActor, Value(transferAmnt), Nonce(1))
	v.CommitApplies()

	v.Assert.EveryMessageResultSatisfies(ExitCode(exitcode.SysErrInvalidReceiver))
	v.Finish(os.Stdout)
}

func failCoverTransferAccountCreationGasStepwise() {
	metadata := &schema.Metadata{
		ID:      "msg-apply-fail-transfer-accountcreation-gas",
		Version: "v1",
		Desc:    "fail not enough gas to cover account actor creation on transfer",
	}

	v := MessageVector(metadata)
	v.Messages.SetDefaults(GasLimit(1_000_000_000), GasPrice(1))

	var alice, bob, charlie AddressHandle
	alice = v.Actors.Account(address.SECP256K1, balance1T)
	bob.Robust, charlie.Robust = MustNewSECP256K1Addr("1"), MustNewSECP256K1Addr("2")
	v.CommitPreconditions()

	var nonce uint64
	ref := v.Messages.Sugar().Transfer(alice.Robust, bob.Robust, Value(transferAmnt), Nonce(nonce))
	nonce++
	v.Messages.ApplyOne(ref)
	v.Assert.EveryMessageResultSatisfies(ExitCode(exitcode.Ok))

	// decrease the gas cost by `gasStep` for each apply and ensure `SysErrOutOfGas` is always returned.
	trueGas := ref.Result.GasUsed
	gasStep := trueGas / 100
	for tryGas := trueGas - gasStep; tryGas > 0; tryGas -= gasStep {
		v.Messages.Sugar().Transfer(alice.Robust, charlie.Robust, Value(transferAmnt), Nonce(nonce), GasPrice(1), GasLimit(tryGas))
		nonce++
	}
	v.CommitApplies()

	v.Assert.EveryMessageResultSatisfies(ExitCode(exitcode.SysErrOutOfGas), ref)
	v.Finish(os.Stdout)
}

func failActorExecutionAborted() {
	metadata := &schema.Metadata{
		ID:      "msg-apply-fail-actor-execution-illegal-arg",
		Version: "v1",
		Desc:    "abort during actor execution due to illegal argument",
	}

	// Set up sender and receiver accounts.
	var sender, receiver AddressHandle
	var paychAddr AddressHandle

	v := MessageVector(metadata)
	v.Messages.SetDefaults(GasLimit(1_000_000_000), GasPrice(1))

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

	v.Finish(os.Stdout)
}
