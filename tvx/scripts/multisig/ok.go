package main

import (
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/abi/big"
	"github.com/filecoin-project/specs-actors/actors/builtin"
	init_ "github.com/filecoin-project/specs-actors/actors/builtin/init"
	"github.com/filecoin-project/specs-actors/actors/builtin/multisig"
	"github.com/filecoin-project/specs-actors/actors/runtime/exitcode"
	"github.com/filecoin-project/specs-actors/actors/util/adt"

	"github.com/filecoin-project/go-address"
	"github.com/minio/blake2b-simd"

	. "github.com/filecoin-project/oni/tvx/builders"
)

func constructor(v *Builder) {
	var balance = abi.NewTokenAmount(1_000_000_000_000)
	var amount = abi.NewTokenAmount(10)

	v.Messages.SetDefaults(GasLimit(gasLimit), GasPremium(1), GasFeeCap(gasFeeCap))

	// Set up one account.
	alice := v.Actors.Account(address.SECP256K1, balance)
	v.CommitPreconditions()

	createMultisig(v, alice, []address.Address{alice.ID}, 1, Value(amount), Nonce(0))
	v.CommitApplies()
}

func proposeAndCancelOk(v *Builder) {
	var (
		initial        = abi.NewTokenAmount(1_000_000_000_000)
		amount         = abi.NewTokenAmount(10)
		unlockDuration = abi.ChainEpoch(10)
	)

	v.Messages.SetDefaults(Value(big.Zero()), Epoch(1), GasLimit(gasLimit), GasPremium(1), GasFeeCap(gasFeeCap))

	// Set up three accounts: alice and bob (signers), and charlie (outsider).
	var alice, bob, charlie AddressHandle
	v.Actors.AccountN(address.SECP256K1, initial, &alice, &bob, &charlie)
	v.CommitPreconditions()

	// create the multisig actor; created by alice.
	multisigAddr := createMultisig(v, alice, []address.Address{alice.ID, bob.ID}, 2, Value(amount), Nonce(0))

	// alice proposes that charlie should receive 'amount' FIL.
	hash := proposeOk(v, proposeOpts{
		multisigAddr: multisigAddr,
		sender:       alice.ID,
		recipient:    charlie.ID,
		amount:       amount,
	}, Nonce(1))

	// bob cancels alice's transaction. This fails as bob did not create alice's transaction.
	bobCancelMsg := v.Messages.Typed(bob.ID, multisigAddr, MultisigCancel(&multisig.TxnIDParams{
		ID:           multisig.TxnID(0),
		ProposalHash: hash,
	}), Nonce(0))
	v.Messages.ApplyOne(bobCancelMsg)
	v.Assert.Equal(bobCancelMsg.Result.ExitCode, exitcode.ErrForbidden)

	// alice cancels their transaction; charlie doesn't receive any FIL,
	// the multisig actor's balance is empty, and the transaction is canceled.
	aliceCancelMsg := v.Messages.Typed(alice.ID, multisigAddr, MultisigCancel(&multisig.TxnIDParams{
		ID:           multisig.TxnID(0),
		ProposalHash: hash,
	}), Nonce(2))
	v.Messages.ApplyOne(aliceCancelMsg)
	v.Assert.Equal(exitcode.Ok, aliceCancelMsg.Result.ExitCode)

	v.CommitApplies()

	// verify balance is untouched.
	v.Assert.BalanceEq(multisigAddr, amount)

	// reload the multisig state and verify
	var multisigState multisig.State
	v.Actors.ActorState(multisigAddr, &multisigState)
	v.Assert.Equal(&multisig.State{
		Signers:               []address.Address{alice.ID, bob.ID},
		NumApprovalsThreshold: 2,
		NextTxnID:             1,
		InitialBalance:        amount,
		StartEpoch:            1,
		UnlockDuration:        unlockDuration,
		PendingTxns:           EmptyMapCid,
	}, &multisigState)
}

func proposeAndApprove(v *Builder) {
	var (
		initial        = abi.NewTokenAmount(1_000_000_000_000)
		amount         = abi.NewTokenAmount(10)
		unlockDuration = abi.ChainEpoch(10)
	)

	v.Messages.SetDefaults(Value(big.Zero()), Epoch(1), GasLimit(gasLimit), GasPremium(1), GasFeeCap(gasFeeCap))

	// Set up three accounts: alice and bob (signers), and charlie (outsider).
	var alice, bob, charlie AddressHandle
	v.Actors.AccountN(address.SECP256K1, initial, &alice, &bob, &charlie)
	v.CommitPreconditions()

	// create the multisig actor; created by alice.
	multisigAddr := createMultisig(v, alice, []address.Address{alice.ID, bob.ID}, 2, Value(amount), Nonce(0))

	// alice proposes that charlie should receive 'amount' FIL.
	hash := proposeOk(v, proposeOpts{
		multisigAddr: multisigAddr,
		sender:       alice.ID,
		recipient:    charlie.ID,
		amount:       amount,
	}, Nonce(1))

	// charlie proposes himself -> fails.
	charliePropose := v.Messages.Typed(charlie.ID, multisigAddr,
		MultisigPropose(&multisig.ProposeParams{
			To:     charlie.ID,
			Value:  amount,
			Method: builtin.MethodSend,
			Params: nil,
		}), Nonce(0))
	v.Messages.ApplyOne(charliePropose)
	v.Assert.Equal(exitcode.ErrForbidden, charliePropose.Result.ExitCode)

	// charlie attempts to accept the pending transaction -> fails.
	charlieApprove := v.Messages.Typed(charlie.ID, multisigAddr,
		MultisigApprove(&multisig.TxnIDParams{
			ID:           multisig.TxnID(0),
			ProposalHash: hash,
		}), Nonce(1))
	v.Messages.ApplyOne(charlieApprove)
	v.Assert.Equal(exitcode.ErrForbidden, charlieApprove.Result.ExitCode)

	// bob approves transfer of 'amount' FIL to charlie.
	// epoch is unlockDuration + 1
	bobApprove := v.Messages.Typed(bob.ID, multisigAddr,
		MultisigApprove(&multisig.TxnIDParams{
			ID:           multisig.TxnID(0),
			ProposalHash: hash,
		}), Nonce(0), Epoch(unlockDuration+1))
	v.Messages.ApplyOne(bobApprove)
	v.Assert.Equal(exitcode.Ok, bobApprove.Result.ExitCode)

	v.CommitApplies()

	var approveRet multisig.ApproveReturn
	MustDeserialize(bobApprove.Result.Return, &approveRet)
	v.Assert.Equal(multisig.ApproveReturn{
		Applied: true,
		Code:    0,
		Ret:     nil,
	}, approveRet)

	// assert that the multisig balance has been drained, and charlie's incremented.
	v.Assert.BalanceEq(multisigAddr, big.Zero())
	v.Assert.MessageSendersSatisfy(BalanceUpdated(amount), charliePropose, charlieApprove)

	// reload the multisig state and verify
	var multisigState multisig.State
	v.Actors.ActorState(multisigAddr, &multisigState)
	v.Assert.Equal(&multisig.State{
		Signers:               []address.Address{alice.ID, bob.ID},
		NumApprovalsThreshold: 2,
		NextTxnID:             1,
		InitialBalance:        amount,
		StartEpoch:            1,
		UnlockDuration:        unlockDuration,
		PendingTxns:           EmptyMapCid,
	}, &multisigState)
}

func addSigner(v *Builder) {
	var (
		initial = abi.NewTokenAmount(1_000_000_000_000)
		amount  = abi.NewTokenAmount(10)
	)

	v.Messages.SetDefaults(Value(big.Zero()), Epoch(1), GasLimit(gasLimit), GasPremium(1), GasFeeCap(gasFeeCap))

	// Set up three accounts: alice and bob (signers), and charlie (outsider).
	var alice, bob, charlie AddressHandle
	v.Actors.AccountN(address.SECP256K1, initial, &alice, &bob, &charlie)
	v.CommitPreconditions()

	// create the multisig actor; created by alice.
	multisigAddr := createMultisig(v, alice, []address.Address{alice.ID}, 1, Value(amount), Nonce(0))

	addParams := &multisig.AddSignerParams{
		Signer:   bob.ID,
		Increase: false,
	}

	// attempt to add bob as a signer; this fails because the addition needs to go through
	// the multisig flow, as it is subject to the same approval policy.
	v.Messages.Typed(alice.ID, multisigAddr, MultisigAddSigner(addParams), Nonce(1))

	// go through the multisig wallet.
	// since approvals = 1, this auto-approves the transaction.
	v.Messages.Typed(alice.ID, multisigAddr, MultisigPropose(&multisig.ProposeParams{
		To:     multisigAddr,
		Value:  big.Zero(),
		Method: builtin.MethodsMultisig.AddSigner,
		Params: MustSerialize(addParams),
	}), Nonce(2))

	// TODO also exercise the approvals = 2 case with explicit approval.

	v.CommitApplies()

	// reload the multisig state and verify that bob is now a signer.
	var multisigState multisig.State
	v.Actors.ActorState(multisigAddr, &multisigState)
	v.Assert.Equal(&multisig.State{
		Signers:               []address.Address{alice.ID, bob.ID},
		NumApprovalsThreshold: 1,
		NextTxnID:             1,
		InitialBalance:        amount,
		StartEpoch:            1,
		UnlockDuration:        10,
		PendingTxns:           EmptyMapCid,
	}, &multisigState)
}

type proposeOpts struct {
	multisigAddr address.Address
	sender       address.Address
	recipient    address.Address
	amount       abi.TokenAmount
}

func proposeOk(v *Builder, proposeOpts proposeOpts, opts ...MsgOpt) []byte {
	propose := &multisig.ProposeParams{
		To:     proposeOpts.recipient,
		Value:  proposeOpts.amount,
		Method: builtin.MethodSend,
		Params: nil,
	}
	proposeMsg := v.Messages.Typed(proposeOpts.sender, proposeOpts.multisigAddr, MultisigPropose(propose), opts...)

	v.Messages.ApplyOne(proposeMsg)

	// verify that the multisig state contains the outstanding TX.
	var multisigState multisig.State
	v.Actors.ActorState(proposeOpts.multisigAddr, &multisigState)

	id := multisig.TxnID(0)
	actualTxn := loadMultisigTxn(v, multisigState, id)
	v.Assert.Equal(&multisig.Transaction{
		To:       propose.To,
		Value:    propose.Value,
		Method:   propose.Method,
		Params:   propose.Params,
		Approved: []address.Address{proposeOpts.sender},
	}, actualTxn)

	return makeProposalHash(v, actualTxn)
}

func createMultisig(v *Builder, creator AddressHandle, approvers []address.Address, threshold uint64, opts ...MsgOpt) address.Address {
	const unlockDuration = abi.ChainEpoch(10)
	// create the multisig actor.
	params := &multisig.ConstructorParams{
		Signers:               approvers,
		NumApprovalsThreshold: threshold,
		UnlockDuration:        unlockDuration,
	}
	msg := v.Messages.Sugar().CreateMultisigActor(creator.ID, params, opts...)
	v.Messages.ApplyOne(msg)

	// verify ok
	v.Assert.EveryMessageResultSatisfies(ExitCode(exitcode.Ok))

	// verify the assigned addess is as expected.
	var ret init_.ExecReturn
	MustDeserialize(msg.Result.Return, &ret)
	v.Assert.Equal(creator.NextActorAddress(msg.Message.Nonce, 0), ret.RobustAddress)
	handles := v.Actors.Handles()
	v.Assert.Equal(MustNewIDAddr(MustIDFromAddress(handles[len(handles)-1].ID)+1), ret.IDAddress)

	// the multisig address's balance is incremented by the value sent to it.
	v.Assert.BalanceEq(ret.IDAddress, msg.Message.Value)

	return ret.IDAddress
}

func loadMultisigTxn(v *Builder, state multisig.State, id multisig.TxnID) *multisig.Transaction {
	pending, err := adt.AsMap(v.Stores.ADTStore, state.PendingTxns)
	v.Assert.NoError(err)

	var actualTxn multisig.Transaction
	found, err := pending.Get(id, &actualTxn)
	v.Assert.True(found)
	v.Assert.NoError(err)
	return &actualTxn
}

func makeProposalHash(v *Builder, txn *multisig.Transaction) []byte {
	ret, err := multisig.ComputeProposalHash(txn, blake2b.Sum256)
	v.Assert.NoError(err)
	return ret
}
