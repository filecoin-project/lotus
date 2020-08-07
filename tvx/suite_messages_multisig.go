package main

import (
	"fmt"
	"os"

	address "github.com/filecoin-project/go-address"
	"github.com/filecoin-project/oni/tvx/chain"
	"github.com/filecoin-project/oni/tvx/drivers"
	abi_spec "github.com/filecoin-project/specs-actors/actors/abi"
	big_spec "github.com/filecoin-project/specs-actors/actors/abi/big"
	builtin_spec "github.com/filecoin-project/specs-actors/actors/builtin"
	multisig_spec "github.com/filecoin-project/specs-actors/actors/builtin/multisig"
	exitcode_spec "github.com/filecoin-project/specs-actors/actors/runtime/exitcode"
	"github.com/minio/blake2b-simd"
)

func MessageTest_MultiSigActor() error {
	err := func(testname string) error {
		const numApprovals = 1
		const unlockDuration = 10
		var valueSend = abi_spec.NewTokenAmount(10)
		var initialBal = abi_spec.NewTokenAmount(200000000000)

		td := drivers.NewTestDriver()
		td.Vector.Meta.Desc = testname

		// creator of the multisig actor
		alice, aliceID := td.NewAccountActor(drivers.SECP, initialBal)

		// expected address of the actor
		multisigAddr := chain.MustNewIDAddr(1 + chain.MustIDFromAddress(aliceID))

		preroot := td.GetStateRoot()

		createRet := td.ComputeInitActorExecReturn(alice, 0, 0, multisigAddr)
		td.MustCreateAndVerifyMultisigActor(0, valueSend, multisigAddr, alice,
			&multisig_spec.ConstructorParams{
				Signers:               []address.Address{aliceID},
				NumApprovalsThreshold: numApprovals,
				UnlockDuration:        unlockDuration,
			},
			exitcode_spec.Ok, chain.MustSerialize(&createRet))

		postroot := td.GetStateRoot()

		td.Vector.CAR = td.MustMarshalGzippedCAR(preroot, postroot)
		td.Vector.Pre.StateTree.RootCID = preroot
		td.Vector.Post.StateTree.RootCID = postroot

		// encode and output
		fmt.Fprintln(os.Stdout, string(td.Vector.MustMarshalJSON()))

		return nil
	}("constructor test")
	if err != nil {
		return err
	}

	err = func(testname string) error {
		const numApprovals = 2
		const unlockDuration = 10
		var valueSend = abi_spec.NewTokenAmount(10)
		var initialBal = abi_spec.NewTokenAmount(200000000000)

		td := drivers.NewTestDriver()
		td.Vector.Meta.Desc = testname

		alice, aliceID := td.NewAccountActor(drivers.SECP, initialBal)

		bob, bobID := td.NewAccountActor(drivers.SECP, initialBal)
		outsider, outsiderID := td.NewAccountActor(drivers.SECP, initialBal)

		multisigAddr := chain.MustNewIDAddr(1 + chain.MustIDFromAddress(outsiderID))

		preroot := td.GetStateRoot()

		createRet := td.ComputeInitActorExecReturn(alice, 0, 0, multisigAddr)
		// create the multisig actor
		td.MustCreateAndVerifyMultisigActor(0, valueSend, multisigAddr, alice,
			&multisig_spec.ConstructorParams{
				Signers:               []address.Address{aliceID, bobID},
				NumApprovalsThreshold: numApprovals,
				UnlockDuration:        unlockDuration,
			},
			exitcode_spec.Ok, chain.MustSerialize(&createRet))
		td.AssertBalance(multisigAddr, valueSend)

		// alice proposes that outsider should receive 'valueSend' FIL.
		pparams := multisig_spec.ProposeParams{
			To:     outsider,
			Value:  valueSend,
			Method: builtin_spec.MethodSend,
			Params: nil,
		}

		// propose the transaction and assert it exists in the actor state
		txID0 := multisig_spec.TxnID(0)
		expected := multisig_spec.ProposeReturn{
			TxnID:   0,
			Applied: false,
			Code:    0,
			Ret:     nil,
		}
		td.ApplyExpect(
			td.MessageProducer.MultisigPropose(alice, multisigAddr, &pparams, chain.Nonce(1)),
			chain.MustSerialize(&expected))

		txn0 := multisig_spec.Transaction{
			To:       pparams.To,
			Value:    pparams.Value,
			Method:   pparams.Method,
			Params:   pparams.Params,
			Approved: []address.Address{aliceID},
		}
		ph := mustMakeProposalHash(&txn0)
		td.AssertMultisigTransaction(multisigAddr, txID0, txn0)

		// bob cancels alice's transaction. This fails as bob did not create alice's transaction.
		td.ApplyFailure(
			td.MessageProducer.MultisigCancel(bob, multisigAddr, &multisig_spec.TxnIDParams{ID: txID0, ProposalHash: ph}, chain.Nonce(0)),
			exitcode_spec.ErrForbidden)

		// alice cancels their transaction. The outsider doesn't receive any FIL, the multisig actor's balance is empty, and the
		// transaction is canceled.
		td.ApplyOk(
			td.MessageProducer.MultisigCancel(alice, multisigAddr, &multisig_spec.TxnIDParams{ID: txID0, ProposalHash: ph}, chain.Nonce(2)),
		)
		td.AssertMultisigState(multisigAddr, multisig_spec.State{
			Signers:               []address.Address{aliceID, bobID},
			NumApprovalsThreshold: numApprovals,
			NextTxnID:             1,
			InitialBalance:        valueSend,
			StartEpoch:            1,
			UnlockDuration:        unlockDuration,
		})
		td.AssertBalance(multisigAddr, valueSend)

		postroot := td.GetStateRoot()

		td.Vector.CAR = td.MustMarshalGzippedCAR(preroot, postroot)
		td.Vector.Pre.StateTree.RootCID = preroot
		td.Vector.Post.StateTree.RootCID = postroot

		// encode and output
		fmt.Fprintln(os.Stdout, string(td.Vector.MustMarshalJSON()))

		return nil
	}("propose and cancel")
	if err != nil {
		return err
	}

	err = func(testname string) error {
		td := drivers.NewTestDriver()
		td.Vector.Meta.Desc = testname

		var initialBal = abi_spec.NewTokenAmount(200000000000)
		const numApprovals = 2
		const unlockDuration = 1
		var valueSend = abi_spec.NewTokenAmount(10)

		// Signers
		alice, aliceID := td.NewAccountActor(drivers.SECP, initialBal)
		bob, bobID := td.NewAccountActor(drivers.SECP, initialBal)

		// Not Signer
		outsider, outsiderID := td.NewAccountActor(drivers.SECP, initialBal)

		preroot := td.GetStateRoot()

		// Multisig actor address
		multisigAddr := chain.MustNewIDAddr(1 + chain.MustIDFromAddress(outsiderID))
		createRet := td.ComputeInitActorExecReturn(alice, 0, 0, multisigAddr)

		// create the multisig actor
		td.MustCreateAndVerifyMultisigActor(0, valueSend, multisigAddr, alice,
			&multisig_spec.ConstructorParams{
				Signers:               []address.Address{aliceID, bobID},
				NumApprovalsThreshold: numApprovals,
				UnlockDuration:        unlockDuration,
			},
			exitcode_spec.Ok, chain.MustSerialize(&createRet))

		// setup propose expected values and params
		pparams := multisig_spec.ProposeParams{
			To:     outsider,
			Value:  valueSend,
			Method: builtin_spec.MethodSend,
			Params: nil,
		}

		// propose the transaction and assert it exists in the actor state
		txID0 := multisig_spec.TxnID(0)
		expectedPropose := multisig_spec.ProposeReturn{
			TxnID:   0,
			Applied: false,
			Code:    0,
			Ret:     nil,
		}
		td.ApplyExpect(
			td.MessageProducer.MultisigPropose(alice, multisigAddr, &pparams, chain.Nonce(1)),
			chain.MustSerialize(&expectedPropose))

		txn0 := multisig_spec.Transaction{
			To:       pparams.To,
			Value:    pparams.Value,
			Method:   pparams.Method,
			Params:   pparams.Params,
			Approved: []address.Address{aliceID},
		}
		ph := mustMakeProposalHash(&txn0)
		td.AssertMultisigTransaction(multisigAddr, txID0, txn0)

		// outsider proposes themselves to receive 'valueSend' FIL. This fails as they are not a signer.
		td.ApplyFailure(
			td.MessageProducer.MultisigPropose(outsider, multisigAddr, &pparams, chain.Nonce(0)),
			exitcode_spec.ErrForbidden)

		// outsider approves the value transfer alice sent. This fails as they are not a signer.
		td.ApplyFailure(
			td.MessageProducer.MultisigApprove(outsider, multisigAddr, &multisig_spec.TxnIDParams{ID: txID0, ProposalHash: ph}, chain.Nonce(1)),
			exitcode_spec.ErrForbidden)

		// increment the epoch to unlock the funds
		td.ExeCtx.Epoch += unlockDuration
		balanceBefore := td.GetBalance(outsider)

		// bob approves transfer of 'valueSend' FIL to outsider.
		expectedApprove := multisig_spec.ApproveReturn{
			Applied: true,
			Code:    0,
			Ret:     nil,
		}
		td.ApplyExpect(
			td.MessageProducer.MultisigApprove(bob, multisigAddr, &multisig_spec.TxnIDParams{ID: txID0, ProposalHash: ph}, chain.Nonce(0)),
			chain.MustSerialize(&expectedApprove))

		txID1 := multisig_spec.TxnID(1)
		td.AssertMultisigState(multisigAddr, multisig_spec.State{
			Signers:               []address.Address{aliceID, bobID},
			NumApprovalsThreshold: numApprovals,
			NextTxnID:             txID1,
			InitialBalance:        valueSend,
			StartEpoch:            1,
			UnlockDuration:        unlockDuration,
		})
		td.AssertMultisigContainsTransaction(multisigAddr, txID0, false)
		// Multisig balance has been transferred to outsider.
		td.AssertBalance(multisigAddr, big_spec.Zero())
		td.AssertBalance(outsider, big_spec.Add(balanceBefore, valueSend))

		postroot := td.GetStateRoot()

		td.Vector.CAR = td.MustMarshalGzippedCAR(preroot, postroot)
		td.Vector.Pre.StateTree.RootCID = preroot
		td.Vector.Post.StateTree.RootCID = postroot

		// encode and output
		fmt.Fprintln(os.Stdout, string(td.Vector.MustMarshalJSON()))

		return nil
	}("propose and approve")
	if err != nil {
		return err
	}

	err = func(testname string) error {
		const initialNumApprovals = 1
		var msValue = abi_spec.NewTokenAmount(100000000000)
		var initialBal = abi_spec.NewTokenAmount(200000000000)

		td := drivers.NewTestDriver()
		td.Vector.Meta.Desc = testname

		alice, aliceID := td.NewAccountActor(drivers.SECP, initialBal) // 101
		_, bobID := td.NewAccountActor(drivers.SECP, initialBal)       // 102
		var initialSigners = []address.Address{aliceID}

		multisigAddr := chain.MustNewIDAddr(1 + chain.MustIDFromAddress(bobID))

		preroot := td.GetStateRoot()

		createRet := td.ComputeInitActorExecReturn(alice, 0, 0, multisigAddr)

		td.MustCreateAndVerifyMultisigActor(0, msValue, multisigAddr, alice,
			&multisig_spec.ConstructorParams{
				Signers:               initialSigners,
				NumApprovalsThreshold: initialNumApprovals,
				UnlockDuration:        0,
			},
			exitcode_spec.Ok,
			chain.MustSerialize(&createRet),
		)

		addSignerParams := multisig_spec.AddSignerParams{
			Signer:   bobID,
			Increase: false,
		}

		// alice fails to call directly since AddSigner
		td.ApplyFailure(
			td.MessageProducer.MultisigAddSigner(alice, multisigAddr, &addSignerParams, chain.Nonce(1)),
			exitcode_spec.SysErrForbidden,
		)

		// AddSigner must be staged through the multisig itself
		// Alice proposes the AddSigner.
		// Since approvals = 1 this auto-approves the transaction.
		expected := multisig_spec.ProposeReturn{
			TxnID:   0,
			Applied: true,
			Code:    0,
			Ret:     nil,
		}
		td.ApplyExpect(
			td.MessageProducer.MultisigPropose(alice, multisigAddr, &multisig_spec.ProposeParams{
				To:     multisigAddr,
				Value:  big_spec.Zero(),
				Method: builtin_spec.MethodsMultisig.AddSigner,
				Params: chain.MustSerialize(&addSignerParams),
			}, chain.Nonce(2)),
			chain.MustSerialize(&expected),
		)

		// TODO also exercise the approvals = 2 case with explicit approval.

		// Check that bob is now a signer
		td.AssertMultisigState(multisigAddr, multisig_spec.State{
			Signers:               append(initialSigners, bobID),
			NumApprovalsThreshold: initialNumApprovals,
			NextTxnID:             multisig_spec.TxnID(1),
			InitialBalance:        big_spec.Zero(),
			StartEpoch:            0,
			UnlockDuration:        0,
		})

		postroot := td.GetStateRoot()

		td.Vector.CAR = td.MustMarshalGzippedCAR(preroot, postroot)
		td.Vector.Pre.StateTree.RootCID = preroot
		td.Vector.Post.StateTree.RootCID = postroot

		// encode and output
		fmt.Fprintln(os.Stdout, string(td.Vector.MustMarshalJSON()))

		return nil
	}("add signer")
	if err != nil {
		return err
	}

	return nil
}

func mustMakeProposalHash(txn *multisig_spec.Transaction) []byte {
	txnHash, err := multisig_spec.ComputeProposalHash(txn, blake2b.Sum256)
	if err != nil {
		panic(err)
	}
	return txnHash
}
