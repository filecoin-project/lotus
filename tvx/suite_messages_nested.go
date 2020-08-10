package main

import (
	"bytes"
	"os"

	address "github.com/filecoin-project/go-address"
	vtypes "github.com/filecoin-project/oni/tvx/chain/types"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/abi/big"
	builtin "github.com/filecoin-project/specs-actors/actors/builtin"
	init_ "github.com/filecoin-project/specs-actors/actors/builtin/init"
	"github.com/filecoin-project/specs-actors/actors/builtin/multisig"
	"github.com/filecoin-project/specs-actors/actors/builtin/paych"
	"github.com/filecoin-project/specs-actors/actors/builtin/reward"
	"github.com/filecoin-project/specs-actors/actors/puppet"
	"github.com/filecoin-project/specs-actors/actors/runtime"
	exitcode_spec "github.com/filecoin-project/specs-actors/actors/runtime/exitcode"
	"github.com/filecoin-project/specs-actors/actors/util/adt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	typegen "github.com/whyrusleeping/cbor-gen"

	"github.com/filecoin-project/oni/tvx/chain"
	"github.com/filecoin-project/oni/tvx/drivers"
)

var PuppetAddress address.Address

func init() {
	var err error
	// the address before the burnt funds address
	PuppetAddress, err = address.NewIDAddress(builtin.FirstNonSingletonActorId - 2)
	if err != nil {
		panic(err)
	}
}

// Tests exercising messages sent internally from one actor to another.
// These use a multisig actor with approvers=1 as a convenient staging ground for arbitrary internal messages.
func MessageTest_NestedSends() error {
	var acctDefaultBalance = abi.NewTokenAmount(1_000_000_000_000)
	var multisigBalance = abi.NewTokenAmount(1_000_000_000)
	nonce := uint64(1)

	err := func(testname string) error {
		td := drivers.NewTestDriver()
		td.Vector.Meta.Desc = testname

		stage := prepareStage(td, acctDefaultBalance, multisigBalance)
		balanceBefore := td.GetBalance(stage.creator)

		// Multisig sends back to the creator.
		amtSent := abi.NewTokenAmount(1)
		result := stage.sendOk(stage.creator, amtSent, builtin.MethodSend, nil, nonce)

		td.AssertActor(stage.creator, big.Sub(big.Add(balanceBefore, amtSent), result.Receipt.GasUsed.Big()), nonce+1)

		td.MustSerialize(os.Stdout)

		return nil
	}("ok basic")
	if err != nil {
		return err
	}

	err = func(testname string) error {
		td := drivers.NewTestDriver()
		td.Vector.Meta.Desc = testname

		stage := prepareStage(td, acctDefaultBalance, multisigBalance)
		balanceBefore := td.GetBalance(stage.creator)

		// Multisig sends to new address.
		newAddr := td.Wallet().NewSECP256k1AccountAddress()
		amtSent := abi.NewTokenAmount(1)
		result := stage.sendOk(newAddr, amtSent, builtin.MethodSend, nil, nonce)

		td.AssertBalance(stage.msAddr, big.Sub(multisigBalance, amtSent))
		td.AssertBalance(stage.creator, big.Sub(balanceBefore, result.Receipt.GasUsed.Big()))
		td.AssertBalance(newAddr, amtSent)

		td.MustSerialize(os.Stdout)

		return nil
	}("ok to new actor")
	if err != nil {
		return err
	}

	err = func(testname string) error {
		td := drivers.NewTestDriver()
		td.Vector.Meta.Desc = testname

		stage := prepareStage(td, acctDefaultBalance, multisigBalance)
		balanceBefore := td.GetBalance(stage.creator)

		// Multisig sends to new address and invokes pubkey method at the same time.
		newAddr := td.Wallet().NewSECP256k1AccountAddress()
		amtSent := abi.NewTokenAmount(1)
		result := stage.sendOk(newAddr, amtSent, builtin.MethodsAccount.PubkeyAddress, nil, nonce)
		// TODO: use an explicit Approve() and check the return value is the correct pubkey address
		// when the multisig Approve() method plumbs through the inner exit code and value.
		// https://github.com/filecoin-project/specs-actors/issues/113
		//expected := bytes.Buffer{}
		//require.NoError(t, newAddr.MarshalCBOR(&expected))
		//assert.Equal(t, expected.Bytes(), result.Receipt.ReturnValue)

		td.AssertBalance(stage.msAddr, big.Sub(multisigBalance, amtSent))
		td.AssertBalance(stage.creator, big.Sub(balanceBefore, result.Receipt.GasUsed.Big()))
		td.AssertBalance(newAddr, amtSent)

		td.MustSerialize(os.Stdout)

		return nil
	}("ok to new actor with invoke")
	if err != nil {
		return err
	}

	err = func(testname string) error {
		td := drivers.NewTestDriver()
		td.Vector.Meta.Desc = testname

		_, anotherId := td.NewAccountActor(drivers.SECP, big.Zero())
		stage := prepareStage(td, acctDefaultBalance, multisigBalance)
		balanceBefore := td.GetBalance(stage.creator)

		// Multisig sends to itself.
		params := multisig.AddSignerParams{
			Signer:   anotherId,
			Increase: false,
		}
		result := stage.sendOk(stage.msAddr, big.Zero(), builtin.MethodsMultisig.AddSigner, &params, nonce)

		td.AssertBalance(stage.msAddr, multisigBalance)
		assert.Equal(drivers.T, big.Sub(balanceBefore, result.Receipt.GasUsed.Big()), td.GetBalance(stage.creator))
		var st multisig.State
		td.GetActorState(stage.msAddr, &st)
		assert.Equal(drivers.T, []address.Address{stage.creator, anotherId}, st.Signers)

		td.MustSerialize(os.Stdout)

		return nil
	}("ok recursive")
	if err != nil {
		return err
	}

	err = func(testname string) error {
		td := drivers.NewTestDriver()
		td.Vector.Meta.Desc = testname

		stage := prepareStage(td, acctDefaultBalance, multisigBalance)

		newAddr := td.Wallet().NewSECP256k1AccountAddress()
		amtSent := abi.NewTokenAmount(1)
		// So long as the parameters are not actually used by the method, a message can carry arbitrary bytes.
		params := typegen.Deferred{Raw: []byte{1, 2, 3, 4}}
		stage.sendOk(newAddr, amtSent, builtin.MethodSend, &params, nonce)

		td.AssertBalance(stage.msAddr, big.Sub(multisigBalance, amtSent))
		td.AssertBalance(newAddr, amtSent)

		td.MustSerialize(os.Stdout)

		return nil
	}("ok non-CBOR params with transfer")
	if err != nil {
		return err
	}

	//
	// TODO: Tests to exercise invalid "syntax" of the inner message.
	// These would fail message syntax validation if the message were top-level.
	//
	// Some of these require handcrafting the proposal params serialization.
	// - malformed address: zero-length, one-length, too-short pubkeys, invalid UVarints, ...
	// - negative method num
	//
	// Unfortunately the multisig actor can't be used to trigger a negative-value internal transfer because
	// it checks just before sending.
	// We need a custom actor for staging whackier messages.

	//
	// The following tests exercise invalid semantics of the inner message
	//

	err = func(testname string) error {
		td := drivers.NewTestDriver()
		td.Vector.Meta.Desc = testname

		stage := prepareStage(td, acctDefaultBalance, multisigBalance)

		newAddr := chain.MustNewIDAddr(1234)
		amtSent := abi.NewTokenAmount(1)
		stage.sendOk(newAddr, amtSent, builtin.MethodSend, nil, nonce)

		td.AssertBalance(stage.msAddr, multisigBalance) // No change.
		_, err := td.State().Actor(newAddr)
		assert.Error(drivers.T, err)

		td.MustSerialize(os.Stdout)

		return nil
	}("fail nonexistent ID address")
	if err != nil {
		return err
	}

	err = func(testname string) error {
		td := drivers.NewTestDriver()
		td.Vector.Meta.Desc = testname

		stage := prepareStage(td, acctDefaultBalance, multisigBalance)

		newAddr := chain.MustNewActorAddr("1234")
		amtSent := abi.NewTokenAmount(1)
		stage.sendOk(newAddr, amtSent, builtin.MethodSend, nil, nonce)

		td.AssertBalance(stage.msAddr, multisigBalance) // No change.
		_, err := td.State().Actor(newAddr)
		assert.Error(drivers.T, err)

		td.MustSerialize(os.Stdout)

		return nil
	}("fail nonexistent actor address")
	if err != nil {
		return err
	}

	err = func(testname string) error {
		td := drivers.NewTestDriver()
		td.Vector.Meta.Desc = testname

		stage := prepareStage(td, acctDefaultBalance, multisigBalance)

		newAddr := td.Wallet().NewSECP256k1AccountAddress()
		amtSent := abi.NewTokenAmount(1)
		stage.sendOk(newAddr, amtSent, abi.MethodNum(99), nil, nonce)

		td.AssertBalance(stage.msAddr, multisigBalance) // No change.
		td.AssertNoActor(newAddr)

		td.MustSerialize(os.Stdout)

		return nil
	}("fail invalid methodnum new actor")
	if err != nil {
		return err
	}

	err = func(testname string) error {
		td := drivers.NewTestDriver()
		td.Vector.Meta.Desc = testname

		stage := prepareStage(td, acctDefaultBalance, multisigBalance)
		balanceBefore := td.GetBalance(stage.creator)

		amtSent := abi.NewTokenAmount(1)
		result := stage.sendOk(stage.creator, amtSent, abi.MethodNum(99), nil, nonce)

		td.AssertBalance(stage.msAddr, multisigBalance)                                       // No change.
		td.AssertBalance(stage.creator, big.Sub(balanceBefore, result.Receipt.GasUsed.Big())) // Pay gas, don't receive funds.

		td.MustSerialize(os.Stdout)

		return nil
	}("fail invalid methodnum for actor")
	if err != nil {
		return err
	}

	// The multisig actor checks before attempting to transfer more than its balance, so we can't exercise that
	// the VM also checks this. Need a custome actor to exercise this.
	//t.Run("fail insufficient funds", func(t *testing.T) {
	//	td := builder.Build(t)
	//	defer td.Complete()
	//
	//	stage := prepareStage(td, acctDefaultBalance, multisigBalance)
	//	balanceBefore := td.GetBalance(stage.creator)
	//
	//	// Attempt to transfer from the multisig more than the balance it has.
	//	// The proposal to do should succeed, but the inner message fail.
	//	amtSent := big.Add(multisigBalance, abi.NewTokenAmount(1))
	//	result := stage.send(stage.creator, amtSent, builtin.MethodSend, nil, nonce)
	//	assert.Equal(t, exitcode_spec.Ok, result.Receipt.ExitCode)
	//
	//	td.AssertBalance(stage.msAddr, multisigBalance)                                       // No change.
	//	td.AssertBalance(stage.creator, big.Sub(balanceBefore, result.Receipt.GasUsed.Big())) // Pay gas, don't receive funds.
	//})

	err = func(testname string) error {
		td := drivers.NewTestDriver()
		td.Vector.Meta.Desc = testname

		stage := prepareStage(td, acctDefaultBalance, multisigBalance)
		balanceBefore := td.GetBalance(stage.creator)

		params := adt.Empty // Missing params required by AddSigner
		amtSent := abi.NewTokenAmount(1)
		result := stage.sendOk(stage.msAddr, amtSent, builtin.MethodsMultisig.AddSigner, params, nonce)

		td.AssertBalance(stage.creator, big.Sub(balanceBefore, result.Receipt.GasUsed.Big()))
		td.AssertBalance(stage.msAddr, multisigBalance)        // No change.
		assert.Equal(drivers.T, 1, len(stage.state().Signers)) // No new signers

		td.MustSerialize(os.Stdout)

		return nil
	}("fail missing params")
	if err != nil {
		return err
	}

	err = func(testname string) error {
		td := drivers.NewTestDriver()
		td.Vector.Meta.Desc = testname

		stage := prepareStage(td, acctDefaultBalance, multisigBalance)
		balanceBefore := td.GetBalance(stage.creator)

		// Wrong params for AddSigner
		params := multisig.ProposeParams{
			To:     stage.creator,
			Value:  big.Zero(),
			Method: builtin.MethodSend,
			Params: nil,
		}
		amtSent := abi.NewTokenAmount(1)
		result := stage.sendOk(stage.msAddr, amtSent, builtin.MethodsMultisig.AddSigner, &params, nonce)

		td.AssertBalance(stage.creator, big.Sub(balanceBefore, result.Receipt.GasUsed.Big()))
		td.AssertBalance(stage.msAddr, multisigBalance)        // No change.
		assert.Equal(drivers.T, 1, len(stage.state().Signers)) // No new signers

		td.MustSerialize(os.Stdout)

		return nil
	}("fail mismatched params")
	if err != nil {
		return err
	}

	err = func(testname string) error {
		td := drivers.NewTestDriver()
		td.Vector.Meta.Desc = testname

		stage := prepareStage(td, acctDefaultBalance, multisigBalance)
		prevHead := td.GetHead(builtin.RewardActorAddr)

		// AwardBlockReward will abort unless invoked by the system actor
		params := reward.AwardBlockRewardParams{
			Miner:     stage.creator,
			Penalty:   big.Zero(),
			GasReward: big.Zero(),
		}
		amtSent := abi.NewTokenAmount(1)
		stage.sendOk(builtin.RewardActorAddr, amtSent, builtin.MethodsReward.AwardBlockReward, &params, nonce)

		td.AssertBalance(stage.msAddr, multisigBalance) // No change.
		td.AssertHead(builtin.RewardActorAddr, prevHead)

		td.MustSerialize(os.Stdout)

		return nil
	}("fail inner abort")
	if err != nil {
		return err
	}

	err = func(testname string) error {
		td := drivers.NewTestDriver()
		td.Vector.Meta.Desc = testname

		stage := prepareStage(td, acctDefaultBalance, multisigBalance)
		prevHead := td.GetHead(builtin.InitActorAddr)

		// Illegal paych constructor params (addresses are not accounts)
		ctorParams := paych.ConstructorParams{
			From: builtin.SystemActorAddr,
			To:   builtin.SystemActorAddr,
		}
		execParams := init_.ExecParams{
			CodeCID:           builtin.PaymentChannelActorCodeID,
			ConstructorParams: chain.MustSerialize(&ctorParams),
		}

		amtSent := abi.NewTokenAmount(1)
		stage.sendOk(builtin.InitActorAddr, amtSent, builtin.MethodsInit.Exec, &execParams, nonce)

		td.AssertBalance(stage.msAddr, multisigBalance) // No change.
		td.AssertHead(builtin.InitActorAddr, prevHead)  // Init state unchanged.

		td.MustSerialize(os.Stdout)

		return nil
	}("fail aborted exec")
	if err != nil {
		return err
	}

	err = func(testname string) error {
		td := drivers.NewTestDriver()
		td.Vector.Meta.Desc = testname

		// puppet actor has zero funds
		puppetBalance := big.Zero()

		_, _, err := td.StateDriver.State().CreateActor(puppet.PuppetActorCodeID, PuppetAddress, puppetBalance, &puppet.State{})
		require.NoError(drivers.T, err)

		alice, _ := td.NewAccountActor(drivers.SECP, acctDefaultBalance)
		bob, _ := td.NewAccountActor(drivers.SECP, big.Zero())

		preroot := td.GetStateRoot()
		td.Vector.Pre.StateTree.RootCID = preroot

		// alice tells the puppet actor to send funds to bob, the puppet actor has 0 balance so the inner send will fail,
		// and alice will pay the gas cost.
		amtSent := abi.NewTokenAmount(1)
		result := td.ApplyMessage(td.MessageProducer.PuppetSend(alice, PuppetAddress, &puppet.SendParams{
			To:     bob,
			Value:  amtSent,
			Method: builtin.MethodSend,
			Params: nil,
		}))

		// the outer message should be applied successfully
		assert.Equal(drivers.T, exitcode_spec.Ok, result.Receipt.ExitCode)

		var puppetRet puppet.SendReturn
		chain.MustDeserialize(result.Receipt.ReturnValue, &puppetRet)

		// the inner message should fail
		assert.Equal(drivers.T, exitcode_spec.SysErrInsufficientFunds, puppetRet.Code)

		// alice should be charged for the gas cost and bob should have not received any funds.
		td.AssertBalance(alice, big.Sub(acctDefaultBalance, result.GasUsed().Big()))
		td.AssertBalance(bob, big.Zero())

		td.MustSerialize(os.Stdout)

		return nil
	}("fail insufficient funds for transfer in inner send")
	if err != nil {
		return err
	}

	return nil

	// TODO more tests:
	// fail send running out of gas on inner method
	// fail send when target method on multisig (recursive) aborts
}

// Wraps a multisig actor as a stage for nested sends.
type msStage struct {
	driver  *drivers.TestDriver
	creator address.Address // Address of the creator and sole signer of the multisig.
	msAddr  address.Address // Address of the multisig actor from which nested messages are sent.
}

// Creates a multisig actor with its creator as sole approver.
func prepareStage(td *drivers.TestDriver, creatorBalance, msBalance abi.TokenAmount) *msStage {
	_, creatorId := td.NewAccountActor(drivers.SECP, creatorBalance)

	msg := td.MessageProducer.CreateMultisigActor(creatorId, []address.Address{creatorId}, 0, 1, chain.Value(msBalance), chain.Nonce(0))

	td.UpdatePreStateRoot()

	result := td.ApplyMessage(msg)
	require.Equal(drivers.T, exitcode_spec.Ok, result.Receipt.ExitCode)

	var ret init_.ExecReturn
	err := ret.UnmarshalCBOR(bytes.NewReader(result.Receipt.ReturnValue))
	require.NoError(drivers.T, err)

	return &msStage{
		driver:  td,
		creator: creatorId,
		msAddr:  ret.IDAddress,
	}
}

func (s *msStage) sendOk(to address.Address, value abi.TokenAmount, method abi.MethodNum, params runtime.CBORMarshaler, approverNonce uint64) vtypes.ApplyMessageResult {
	buf := bytes.Buffer{}
	if params != nil {
		err := params.MarshalCBOR(&buf)
		require.NoError(drivers.T, err)
	}
	pparams := multisig.ProposeParams{
		To:     to,
		Value:  value,
		Method: method,
		Params: buf.Bytes(),
	}
	msg := s.driver.MessageProducer.MultisigPropose(s.creator, s.msAddr, &pparams, chain.Nonce(approverNonce))
	result := s.driver.ApplyMessage(msg)
	require.Equal(drivers.T, exitcode_spec.Ok, result.Receipt.ExitCode)
	return result
}

func (s *msStage) state() *multisig.State {
	var msState multisig.State
	s.driver.GetActorState(s.msAddr, &msState)
	return &msState
}
