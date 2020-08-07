package main

import (
	"fmt"
	"os"

	abi_spec "github.com/filecoin-project/specs-actors/actors/abi"
	big_spec "github.com/filecoin-project/specs-actors/actors/abi/big"
	paych_spec "github.com/filecoin-project/specs-actors/actors/builtin/paych"
	crypto_spec "github.com/filecoin-project/specs-actors/actors/crypto"
	exitcode_spec "github.com/filecoin-project/specs-actors/actors/runtime/exitcode"

	"github.com/filecoin-project/oni/tvx/chain"
	"github.com/filecoin-project/oni/tvx/drivers"
)

func MessageTest_MessageApplicationEdgecases() error {
	var aliceBal = abi_spec.NewTokenAmount(1_000_000_000_000)
	var transferAmnt = abi_spec.NewTokenAmount(10)

	err := func(testname string) error {
		td := drivers.NewTestDriver()
		td.Vector.Meta.Desc = testname

		alice, _ := td.NewAccountActor(drivers.SECP, aliceBal)

		preroot := td.GetStateRoot()

		msg := td.MessageProducer.Transfer(alice, alice, chain.Value(transferAmnt), chain.Nonce(0), chain.GasPrice(1), chain.GasLimit(8))

		td.ApplyFailure(
			msg,
			exitcode_spec.SysErrOutOfGas)

		postroot := td.GetStateRoot()

		td.Vector.CAR = td.MustMarshalGzippedCAR(preroot, postroot)
		td.Vector.Pre.StateTree.RootCID = preroot
		td.Vector.Post.StateTree.RootCID = postroot

		// encode and output
		fmt.Fprintln(os.Stdout, string(td.Vector.MustMarshalJSON()))

		return nil
	}("fail to cover gas cost for message receipt on chain")
	if err != nil {
		return err
	}

	err = func(testname string) error {
		td := drivers.NewTestDriver()
		td.Vector.Meta.Desc = testname

		alice, _ := td.NewAccountActor(drivers.SECP, aliceBal)

		preroot := td.GetStateRoot()
		msg := td.MessageProducer.Transfer(alice, alice, chain.Value(transferAmnt), chain.Nonce(0), chain.GasPrice(10), chain.GasLimit(1))

		// Expect Message application to fail due to lack of gas
		td.ApplyFailure(
			msg,
			exitcode_spec.SysErrOutOfGas)

		unknown := chain.MustNewIDAddr(10000000)
		msg = td.MessageProducer.Transfer(unknown, alice, chain.Value(transferAmnt), chain.Nonce(0), chain.GasPrice(10), chain.GasLimit(1))

		// Expect Message application to fail due to lack of gas when sender is unknown
		td.ApplyFailure(
			msg,
			exitcode_spec.SysErrOutOfGas)

		postroot := td.GetStateRoot()

		td.Vector.CAR = td.MustMarshalGzippedCAR(preroot, postroot)
		td.Vector.Pre.StateTree.RootCID = preroot
		td.Vector.Post.StateTree.RootCID = postroot

		// encode and output
		fmt.Fprintln(os.Stdout, string(td.Vector.MustMarshalJSON()))

		return nil
	}("not enough gas to pay message on-chain-size cost")
	if err != nil {
		return err
	}

	err = func(testname string) error {
		td := drivers.NewTestDriver()
		td.Vector.Meta.Desc = testname

		alice, _ := td.NewAccountActor(drivers.SECP, aliceBal)
		preroot := td.GetStateRoot()

		aliceNonce := uint64(0)
		aliceNonceF := func() uint64 {
			defer func() { aliceNonce++ }()
			return aliceNonce
		}
		newAccountA := chain.MustNewSECP256K1Addr("1")

		msg := td.MessageProducer.Transfer(alice, newAccountA, chain.Value(transferAmnt), chain.Nonce(aliceNonceF()))

		// get the "true" gas cost of applying the message
		result := td.ApplyOk(msg)

		// decrease the gas cost by `gasStep` for each apply and ensure `SysErrOutOfGas` is always returned.
		trueGas := int64(result.GasUsed())
		gasStep := int64(trueGas / 100)
		newAccountB := chain.MustNewSECP256K1Addr("2")
		for tryGas := trueGas - gasStep; tryGas > 0; tryGas -= gasStep {
			msg := td.MessageProducer.Transfer(alice, newAccountB, chain.Value(transferAmnt), chain.Nonce(aliceNonceF()), chain.GasPrice(1), chain.GasLimit(tryGas))

			td.ApplyFailure(
				msg,
				exitcode_spec.SysErrOutOfGas,
			)
		}

		postroot := td.GetStateRoot()

		td.Vector.CAR = td.MustMarshalGzippedCAR(preroot, postroot)
		td.Vector.Pre.StateTree.RootCID = preroot
		td.Vector.Post.StateTree.RootCID = postroot

		// encode and output
		fmt.Fprintln(os.Stdout, string(td.Vector.MustMarshalJSON()))

		return nil
	}("fail not enough gas to cover account actor creation")
	if err != nil {
		return err
	}

	err = func(testname string) error {
		td := drivers.NewTestDriver()
		td.Vector.Meta.Desc = testname

		alice, _ := td.NewAccountActor(drivers.SECP, aliceBal)

		preroot := td.GetStateRoot()

		msg := td.MessageProducer.Transfer(alice, alice, chain.Value(transferAmnt), chain.Nonce(1))

		// Expect Message application to fail due to callseqnum being invalid: 1 instead of 0
		td.ApplyFailure(
			msg,
			exitcode_spec.SysErrSenderStateInvalid)

		unknown := chain.MustNewIDAddr(10000000)
		msg = td.MessageProducer.Transfer(unknown, alice, chain.Value(transferAmnt), chain.Nonce(1))

		// Expect message application to fail due to unknow actor when call seq num is also incorrect
		td.ApplyFailure(
			msg,
			exitcode_spec.SysErrSenderInvalid)

		postroot := td.GetStateRoot()

		td.Vector.CAR = td.MustMarshalGzippedCAR(preroot, postroot)
		td.Vector.Pre.StateTree.RootCID = preroot
		td.Vector.Post.StateTree.RootCID = postroot

		// encode and output
		fmt.Fprintln(os.Stdout, string(td.Vector.MustMarshalJSON()))

		return nil
	}("invalid actor nonce")
	if err != nil {
		return err
	}

	err = func(testname string) error {
		td := drivers.NewTestDriver()
		td.Vector.Meta.Desc = testname

		const pcTimeLock = abi_spec.ChainEpoch(10)
		const pcLane = uint64(123)
		const pcNonce = uint64(1)
		var pcAmount = big_spec.NewInt(10)
		var initialBal = abi_spec.NewTokenAmount(200_000_000_000)
		var toSend = abi_spec.NewTokenAmount(10_000)
		var pcSig = &crypto_spec.Signature{
			Type: crypto_spec.SigTypeBLS,
			Data: []byte("Grrr im an invalid signature, I cause panics in the payment channel actor"),
		}

		// will create and send on payment channel
		sender, _ := td.NewAccountActor(drivers.SECP, initialBal)
		// will be receiver on paych
		receiver, receiverID := td.NewAccountActor(drivers.SECP, initialBal)

		// the _expected_ address of the payment channel
		paychAddr := chain.MustNewIDAddr(chain.MustIDFromAddress(receiverID) + 1)
		createRet := td.ComputeInitActorExecReturn(sender, 0, 0, paychAddr)

		preroot := td.GetStateRoot()

		msg := td.MessageProducer.CreatePaymentChannelActor(sender, receiver, chain.Value(toSend), chain.Nonce(0))

		td.ApplyExpect(
			msg,
			chain.MustSerialize(&createRet))

		msg = td.MessageProducer.PaychUpdateChannelState(sender, paychAddr, &paych_spec.UpdateChannelStateParams{
			Sv: paych_spec.SignedVoucher{
				ChannelAddr:     paychAddr,
				TimeLockMin:     pcTimeLock,
				TimeLockMax:     pcTimeLock,
				SecretPreimage:  nil,
				Extra:           nil,
				Lane:            pcLane,
				Nonce:           pcNonce,
				Amount:          pcAmount,
				MinSettleHeight: 0,
				Merges:          nil,
				Signature:       pcSig, // construct with invalid signature
			},
		}, chain.Nonce(1), chain.Value(big_spec.Zero()))

		// message application fails due to invalid argument (signature).
		td.ApplyFailure(
			msg,
			exitcode_spec.ErrIllegalArgument)

		postroot := td.GetStateRoot()

		td.Vector.CAR = td.MustMarshalGzippedCAR(preroot, postroot)
		td.Vector.Pre.StateTree.RootCID = preroot
		td.Vector.Post.StateTree.RootCID = postroot

		// encode and output
		fmt.Fprintln(os.Stdout, string(td.Vector.MustMarshalJSON()))

		return nil
	}("abort during actor execution")
	if err != nil {
		return err
	}

	err = func(testname string) error {
		td := drivers.NewTestDriver()
		td.Vector.Meta.Desc = testname

		alice, _ := td.NewAccountActor(drivers.SECP, aliceBal)

		preroot := td.GetStateRoot()

		msg := td.MessageProducer.MarketComputeDataCommitment(alice, alice, nil, chain.Nonce(0))

		// message application fails because ComputeDataCommitment isn't defined
		// on the recipient actor
		td.ApplyFailure(
			msg,
			exitcode_spec.SysErrInvalidMethod)

		postroot := td.GetStateRoot()

		td.Vector.CAR = td.MustMarshalGzippedCAR(preroot, postroot)
		td.Vector.Pre.StateTree.RootCID = preroot
		td.Vector.Post.StateTree.RootCID = postroot

		// encode and output
		fmt.Fprintln(os.Stdout, string(td.Vector.MustMarshalJSON()))

		return nil
	}("invalid method for receiver")
	if err != nil {
		return err
	}

	err = func(testname string) error {
		td := drivers.NewTestDriver()
		td.Vector.Meta.Desc = testname
		td.Vector.Meta.Comment = "Note that this test is not a valid message, since it is using an unknown actor. However in the event that an invalid message isn't filtered by block validation we need to ensure behaviour is consistent across VM implementations."

		alice, _ := td.NewAccountActor(drivers.SECP, aliceBal)

		preroot := td.GetStateRoot()

		// Sending a message to non-existent ID address must produce an error.
		unknownA := chain.MustNewIDAddr(10000000)
		msg := td.MessageProducer.Transfer(alice, unknownA, chain.Value(transferAmnt), chain.Nonce(0))

		td.ApplyFailure(
			msg,
			exitcode_spec.SysErrInvalidReceiver)

		// Sending a message to non-existing actor address must produce an error.
		unknownB := chain.MustNewActorAddr("1234")
		msg = td.MessageProducer.Transfer(alice, unknownB, chain.Value(transferAmnt), chain.Nonce(1))

		td.ApplyFailure(
			msg,
			exitcode_spec.SysErrInvalidReceiver)

		postroot := td.GetStateRoot()

		td.Vector.CAR = td.MustMarshalGzippedCAR(preroot, postroot)
		td.Vector.Pre.StateTree.RootCID = preroot
		td.Vector.Post.StateTree.RootCID = postroot

		// encode and output
		fmt.Fprintln(os.Stdout, string(td.Vector.MustMarshalJSON()))

		return nil
	}("receiver ID/Actor address does not exist")
	if err != nil {
		return err
	}

	return nil

	// TODO more tests:
	// - missing/mismatched params for receiver
	// - various out-of-gas cases
}
