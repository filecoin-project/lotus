package main

import (
	"bytes"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/abi/big"
	"github.com/filecoin-project/specs-actors/actors/builtin"
	init_ "github.com/filecoin-project/specs-actors/actors/builtin/init"
	"github.com/filecoin-project/specs-actors/actors/builtin/multisig"
	"github.com/filecoin-project/specs-actors/actors/builtin/paych"
	"github.com/filecoin-project/specs-actors/actors/builtin/reward"
	"github.com/filecoin-project/specs-actors/actors/puppet"
	"github.com/filecoin-project/specs-actors/actors/runtime"
	"github.com/filecoin-project/specs-actors/actors/runtime/exitcode"
	"github.com/filecoin-project/specs-actors/actors/util/adt"
	typegen "github.com/whyrusleeping/cbor-gen"

	. "github.com/filecoin-project/oni/tvx/builders"
)

var (
	acctDefaultBalance = abi.NewTokenAmount(1_000_000_000_000)
	multisigBalance    = abi.NewTokenAmount(1_000_000_000)
	nonce              = uint64(1)
	PuppetAddress      address.Address
)

func init() {
	var err error
	// the address before the burnt funds address
	PuppetAddress, err = address.NewIDAddress(builtin.FirstNonSingletonActorId - 2)
	if err != nil {
		panic(err)
	}
}

func nestedSends_OkBasic(v *Builder) {
	v.Messages.SetDefaults(GasLimit(1_000_000_000), GasPremium(1), GasFeeCap(200))

	stage := prepareStage(v, acctDefaultBalance, multisigBalance)
	balanceBefore := v.Actors.Balance(stage.creator)

	// Multisig sends back to the creator.
	amtSent := abi.NewTokenAmount(1)
	result := stage.sendOk(stage.creator, amtSent, builtin.MethodSend, nil, nonce)

	//td.AssertActor(stage.creator, big.Sub(big.Add(balanceBefore, amtSent), result.Result.Receipt.GasUsed.Big()), nonce+1)
	v.Assert.NonceEq(stage.creator, nonce+1)
	v.Assert.BalanceEq(stage.creator, big.Sub(big.Add(balanceBefore, amtSent), CalculateDeduction(result)))
}

func nestedSends_OkToNewActor(v *Builder) {
	v.Messages.SetDefaults(GasLimit(1_000_000_000), GasPremium(1), GasFeeCap(200))

	stage := prepareStage(v, acctDefaultBalance, multisigBalance)
	balanceBefore := v.Actors.Balance(stage.creator)

	// Multisig sends to new address.
	newAddr := v.Wallet.NewSECP256k1Account()
	amtSent := abi.NewTokenAmount(1)
	result := stage.sendOk(newAddr, amtSent, builtin.MethodSend, nil, nonce)

	v.Assert.BalanceEq(stage.msAddr, big.Sub(multisigBalance, amtSent))
	v.Assert.BalanceEq(stage.creator, big.Sub(balanceBefore, CalculateDeduction(result)))
	v.Assert.BalanceEq(newAddr, amtSent)
}

func nestedSends_OkToNewActorWithInvoke(v *Builder) {
	v.Messages.SetDefaults(GasLimit(1_000_000_000), GasPremium(1), GasFeeCap(200))

	stage := prepareStage(v, acctDefaultBalance, multisigBalance)
	balanceBefore := v.Actors.Balance(stage.creator)

	// Multisig sends to new address and invokes pubkey method at the same time.
	newAddr := v.Wallet.NewSECP256k1Account()
	amtSent := abi.NewTokenAmount(1)
	result := stage.sendOk(newAddr, amtSent, builtin.MethodsAccount.PubkeyAddress, nil, nonce)
	// TODO: use an explicit Approve() and check the return value is the correct pubkey address
	// when the multisig Approve() method plumbs through the inner exit code and value.
	// https://github.com/filecoin-project/specs-actors/issues/113
	//expected := bytes.Buffer{}
	//require.NoError(t, newAddr.MarshalCBOR(&expected))
	//assert.Equal(t, expected.Bytes(), result.Result.Receipt.ReturnValue)

	v.Assert.BalanceEq(stage.msAddr, big.Sub(multisigBalance, amtSent))
	v.Assert.BalanceEq(stage.creator, big.Sub(balanceBefore, CalculateDeduction(result)))
	v.Assert.BalanceEq(newAddr, amtSent)
}

func nestedSends_OkRecursive(v *Builder) {
	v.Messages.SetDefaults(GasLimit(1_000_000_000), GasPremium(1), GasFeeCap(200))

	another := v.Actors.Account(address.SECP256K1, big.Zero())
	stage := prepareStage(v, acctDefaultBalance, multisigBalance)
	balanceBefore := v.Actors.Balance(stage.creator)

	// Multisig sends to itself.
	params := multisig.AddSignerParams{
		Signer:   another.ID,
		Increase: false,
	}
	result := stage.sendOk(stage.msAddr, big.Zero(), builtin.MethodsMultisig.AddSigner, &params, nonce)

	v.Assert.BalanceEq(stage.msAddr, multisigBalance)
	v.Assert.Equal(big.Sub(balanceBefore, CalculateDeduction(result)), v.Actors.Balance(stage.creator))

	var st multisig.State
	v.Actors.ActorState(stage.msAddr, &st)
	v.Assert.Equal([]address.Address{stage.creator, another.ID}, st.Signers)
}

func nestedSends_OKNonCBORParamsWithTransfer(v *Builder) {
	v.Messages.SetDefaults(GasLimit(1_000_000_000), GasPremium(1), GasFeeCap(200))

	stage := prepareStage(v, acctDefaultBalance, multisigBalance)

	newAddr := v.Wallet.NewSECP256k1Account()
	amtSent := abi.NewTokenAmount(1)
	// So long as the parameters are not actually used by the method, a message can carry arbitrary bytes.
	params := typegen.Deferred{Raw: []byte{1, 2, 3, 4}}
	stage.sendOk(newAddr, amtSent, builtin.MethodSend, &params, nonce)

	v.Assert.BalanceEq(stage.msAddr, big.Sub(multisigBalance, amtSent))
	v.Assert.BalanceEq(newAddr, amtSent)
}

func nestedSends_FailNonexistentIDAddress(v *Builder) {
	v.Messages.SetDefaults(GasLimit(1_000_000_000), GasPremium(1), GasFeeCap(200))

	stage := prepareStage(v, acctDefaultBalance, multisigBalance)

	newAddr := MustNewIDAddr(1234)
	amtSent := abi.NewTokenAmount(1)
	stage.sendOk(newAddr, amtSent, builtin.MethodSend, nil, nonce)

	v.Assert.BalanceEq(stage.msAddr, multisigBalance) // No change.
	v.Assert.ActorMissing(newAddr)
}

func nestedSends_FailNonexistentActorAddress(v *Builder) {
	v.Messages.SetDefaults(GasLimit(1_000_000_000), GasPremium(1), GasFeeCap(200))

	stage := prepareStage(v, acctDefaultBalance, multisigBalance)

	newAddr := MustNewActorAddr("1234")
	amtSent := abi.NewTokenAmount(1)
	stage.sendOk(newAddr, amtSent, builtin.MethodSend, nil, nonce)

	v.Assert.BalanceEq(stage.msAddr, multisigBalance) // No change.
	v.Assert.ActorMissing(newAddr)
}

func nestedSends_FailInvalidMethodNumNewActor(v *Builder) {
	v.Messages.SetDefaults(GasLimit(1_000_000_000), GasPremium(1), GasFeeCap(200))

	stage := prepareStage(v, acctDefaultBalance, multisigBalance)

	newAddr := v.Wallet.NewSECP256k1Account()
	amtSent := abi.NewTokenAmount(1)
	stage.sendOk(newAddr, amtSent, abi.MethodNum(99), nil, nonce)

	v.Assert.BalanceEq(stage.msAddr, multisigBalance) // No change.
	v.Assert.ActorMissing(newAddr)
}

func nestedSends_FailInvalidMethodNumForActor(v *Builder) {
	v.Messages.SetDefaults(GasLimit(1_000_000_000), GasPremium(1), GasFeeCap(200))

	stage := prepareStage(v, acctDefaultBalance, multisigBalance)
	balanceBefore := v.Actors.Balance(stage.creator)

	amtSent := abi.NewTokenAmount(1)
	result := stage.sendOk(stage.creator, amtSent, abi.MethodNum(99), nil, nonce)

	v.Assert.BalanceEq(stage.msAddr, multisigBalance)                                     // No change.
	v.Assert.BalanceEq(stage.creator, big.Sub(balanceBefore, CalculateDeduction(result))) // Pay gas, don't receive funds.
}

func nestedSends_FailMissingParams(v *Builder) {
	v.Messages.SetDefaults(GasLimit(1_000_000_000), GasPremium(1), GasFeeCap(200))

	stage := prepareStage(v, acctDefaultBalance, multisigBalance)
	balanceBefore := v.Actors.Balance(stage.creator)

	params := adt.Empty // Missing params required by AddSigner
	amtSent := abi.NewTokenAmount(1)
	result := stage.sendOk(stage.msAddr, amtSent, builtin.MethodsMultisig.AddSigner, params, nonce)

	v.Assert.BalanceEq(stage.creator, big.Sub(balanceBefore, CalculateDeduction(result)))
	v.Assert.BalanceEq(stage.msAddr, multisigBalance) // No change.
	v.Assert.Equal(1, len(stage.state().Signers))     // No new signers
}

func nestedSends_FailMismatchParams(v *Builder) {
	v.Messages.SetDefaults(GasLimit(1_000_000_000), GasPremium(1), GasFeeCap(200))

	stage := prepareStage(v, acctDefaultBalance, multisigBalance)
	balanceBefore := v.Actors.Balance(stage.creator)

	// Wrong params for AddSigner
	params := multisig.ProposeParams{
		To:     stage.creator,
		Value:  big.Zero(),
		Method: builtin.MethodSend,
		Params: nil,
	}
	amtSent := abi.NewTokenAmount(1)
	result := stage.sendOk(stage.msAddr, amtSent, builtin.MethodsMultisig.AddSigner, &params, nonce)

	v.Assert.BalanceEq(stage.creator, big.Sub(balanceBefore, CalculateDeduction(result)))
	v.Assert.BalanceEq(stage.msAddr, multisigBalance) // No change.
	v.Assert.Equal(1, len(stage.state().Signers))     // No new signers
}

func nestedSends_FailInnerAbort(v *Builder) {
	v.Messages.SetDefaults(GasLimit(1_000_000_000), GasPremium(1), GasFeeCap(200))

	stage := prepareStage(v, acctDefaultBalance, multisigBalance)
	prevHead := v.Actors.Head(builtin.RewardActorAddr)

	// AwardBlockReward will abort unless invoked by the system actor
	params := reward.AwardBlockRewardParams{
		Miner:     stage.creator,
		Penalty:   big.Zero(),
		GasReward: big.Zero(),
	}
	amtSent := abi.NewTokenAmount(1)
	stage.sendOk(builtin.RewardActorAddr, amtSent, builtin.MethodsReward.AwardBlockReward, &params, nonce)

	v.Assert.BalanceEq(stage.msAddr, multisigBalance) // No change.
	v.Assert.HeadEq(builtin.RewardActorAddr, prevHead)
}

func nestedSends_FailAbortedExec(v *Builder) {
	v.Messages.SetDefaults(GasLimit(1_000_000_000), GasPremium(1), GasFeeCap(200))

	stage := prepareStage(v, acctDefaultBalance, multisigBalance)
	prevHead := v.Actors.Head(builtin.InitActorAddr)

	// Illegal paych constructor params (addresses are not accounts)
	ctorParams := paych.ConstructorParams{
		From: builtin.SystemActorAddr,
		To:   builtin.SystemActorAddr,
	}
	execParams := init_.ExecParams{
		CodeCID:           builtin.PaymentChannelActorCodeID,
		ConstructorParams: MustSerialize(&ctorParams),
	}

	amtSent := abi.NewTokenAmount(1)
	stage.sendOk(builtin.InitActorAddr, amtSent, builtin.MethodsInit.Exec, &execParams, nonce)

	v.Assert.BalanceEq(stage.msAddr, multisigBalance) // No change.
	v.Assert.HeadEq(builtin.InitActorAddr, prevHead)  // Init state unchanged.
}

func nestedSends_FailInsufficientFundsForTransferInInnerSend(v *Builder) {
	v.Messages.SetDefaults(GasLimit(1_000_000_000), GasPremium(1), GasFeeCap(200))

	// puppet actor has zero funds
	puppetBalance := big.Zero()
	_ = v.Actors.CreateActor(puppet.PuppetActorCodeID, PuppetAddress, puppetBalance, &puppet.State{})

	alice := v.Actors.Account(address.SECP256K1, acctDefaultBalance)
	bob := v.Actors.Account(address.SECP256K1, big.Zero())

	v.CommitPreconditions()

	// alice tells the puppet actor to send funds to bob, the puppet actor has 0 balance so the inner send will fail,
	// and alice will pay the gas cost.
	amtSent := abi.NewTokenAmount(1)
	msg := v.Messages.Typed(alice.ID, PuppetAddress, PuppetSend(&puppet.SendParams{
		To:     bob.ID,
		Value:  amtSent,
		Method: builtin.MethodSend,
		Params: nil,
	}), Nonce(0), Value(big.Zero()))

	v.Messages.ApplyOne(msg)

	v.CommitApplies()

	// the outer message should be applied successfully
	v.Assert.Equal(exitcode.Ok, msg.Result.ExitCode)

	var puppetRet puppet.SendReturn
	MustDeserialize(msg.Result.MessageReceipt.Return, &puppetRet)

	// the inner message should fail
	v.Assert.Equal(exitcode.SysErrInsufficientFunds, puppetRet.Code)

	// alice should be charged for the gas cost and bob should have not received any funds.
	v.Assert.MessageSendersSatisfy(BalanceUpdated(big.Zero()), msg)
	v.Assert.BalanceEq(bob.ID, big.Zero())
}

type msStage struct {
	v       *Builder
	creator address.Address // Address of the creator and sole signer of the multisig.
	msAddr  address.Address // Address of the multisig actor from which nested messages are sent.
}

// Creates a multisig actor with its creator as sole approver.
func prepareStage(v *Builder, creatorBalance, msBalance abi.TokenAmount) *msStage {
	// Set up sender and receiver accounts.
	creator := v.Actors.Account(address.SECP256K1, creatorBalance)
	v.CommitPreconditions()

	msg := v.Messages.Sugar().CreateMultisigActor(creator.ID, &multisig.ConstructorParams{
		Signers:               []address.Address{creator.ID},
		NumApprovalsThreshold: 1,
		UnlockDuration:        0,
	}, Value(msBalance), Nonce(0))
	v.Messages.ApplyOne(msg)

	v.Assert.Equal(msg.Result.ExitCode, exitcode.Ok)

	// Verify init actor return.
	var ret init_.ExecReturn
	MustDeserialize(msg.Result.Return, &ret)

	return &msStage{
		v:       v,
		creator: creator.ID,
		msAddr:  ret.IDAddress,
	}
}

func (s *msStage) sendOk(to address.Address, value abi.TokenAmount, method abi.MethodNum, params runtime.CBORMarshaler, approverNonce uint64) *ApplicableMessage {
	buf := bytes.Buffer{}
	if params != nil {
		err := params.MarshalCBOR(&buf)
		if err != nil {
			panic(err)
		}
	}
	pparams := multisig.ProposeParams{
		To:     to,
		Value:  value,
		Method: method,
		Params: buf.Bytes(),
	}
	msg := s.v.Messages.Typed(s.creator, s.msAddr, MultisigPropose(&pparams), Nonce(approverNonce), Value(big.NewInt(0)))
	s.v.CommitApplies()

	// all messages succeeded.
	s.v.Assert.EveryMessageResultSatisfies(ExitCode(exitcode.Ok))

	return msg
}

func (s *msStage) state() *multisig.State {
	var msState multisig.State
	s.v.Actors.ActorState(s.msAddr, &msState)
	return &msState
}
