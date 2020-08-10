package main

import (
	"os"

	abi_spec "github.com/filecoin-project/specs-actors/actors/abi"
	big_spec "github.com/filecoin-project/specs-actors/actors/abi/big"
	paych_spec "github.com/filecoin-project/specs-actors/actors/builtin/paych"
	crypto_spec "github.com/filecoin-project/specs-actors/actors/crypto"
	"github.com/stretchr/testify/assert"

	"github.com/filecoin-project/oni/tvx/chain"
	"github.com/filecoin-project/oni/tvx/drivers"
)

func MessageTest_Paych() error {
	var initialBal = abi_spec.NewTokenAmount(200_000_000_000)
	var toSend = abi_spec.NewTokenAmount(10_000)

	err := func(testname string) error {
		td := drivers.NewTestDriver()
		td.Vector.Meta.Desc = testname

		// will create and send on payment channel
		sender, senderID := td.NewAccountActor(drivers.SECP, initialBal)

		// will be receiver on paych
		receiver, receiverID := td.NewAccountActor(drivers.SECP, initialBal)

		td.UpdatePreStateRoot()

		// the _expected_ address of the payment channel
		paychAddr := chain.MustNewIDAddr(chain.MustIDFromAddress(receiverID) + 1)
		createRet := td.ComputeInitActorExecReturn(sender, 0, 0, paychAddr)

		msg := td.MessageProducer.CreatePaymentChannelActor(sender, receiver, chain.Value(toSend), chain.Nonce(0))

		// init actor creates the payment channel
		td.ApplyExpect(
			msg,
			chain.MustSerialize(&createRet))

		var pcState paych_spec.State
		td.GetActorState(paychAddr, &pcState)
		assert.Equal(drivers.T, senderID, pcState.From)
		assert.Equal(drivers.T, receiverID, pcState.To)
		td.AssertBalance(paychAddr, toSend)

		td.MustSerialize(os.Stdout)

		return nil
	}("happy path constructor")
	if err != nil {
		return err
	}

	err = func(testname string) error {
		td := drivers.NewTestDriver()
		td.Vector.Meta.Desc = testname

		//const pcTimeLock = abi_spec.ChainEpoch(1)
		const pcTimeLock = abi_spec.ChainEpoch(0)
		const pcLane = uint64(123)
		const pcNonce = uint64(1)
		var pcAmount = big_spec.NewInt(10)
		var pcSig = &crypto_spec.Signature{
			Type: crypto_spec.SigTypeBLS,
			Data: []byte("signature goes here"), // TODO may need to generate an actual signature
		}

		// will create and send on payment channel
		sender, _ := td.NewAccountActor(drivers.SECP, initialBal)

		// will be receiver on paych
		receiver, receiverID := td.NewAccountActor(drivers.SECP, initialBal)

		td.UpdatePreStateRoot()

		// the _expected_ address of the payment channel
		paychAddr := chain.MustNewIDAddr(chain.MustIDFromAddress(receiverID) + 1)
		createRet := td.ComputeInitActorExecReturn(sender, 0, 0, paychAddr)

		msg := td.MessageProducer.CreatePaymentChannelActor(sender, receiver, chain.Value(toSend), chain.Nonce(0))
		td.ApplyExpect(
			msg,
			chain.MustSerialize(&createRet))

		msg = td.MessageProducer.PaychUpdateChannelState(sender, paychAddr, &paych_spec.UpdateChannelStateParams{
			Sv: paych_spec.SignedVoucher{
				ChannelAddr:     paychAddr,
				TimeLockMin:     pcTimeLock,
				TimeLockMax:     0, // TimeLockMax set to 0 means no timeout
				SecretPreimage:  nil,
				Extra:           nil,
				Lane:            pcLane,
				Nonce:           pcNonce,
				Amount:          pcAmount,
				MinSettleHeight: 0,
				Merges:          nil,
				Signature:       pcSig,
			},
		}, chain.Nonce(1), chain.Value(big_spec.Zero()))
		td.ApplyOk(msg)

		var pcState paych_spec.State
		td.GetActorState(paychAddr, &pcState)
		assert.Equal(drivers.T, 1, len(pcState.LaneStates))
		ls := pcState.LaneStates[0]
		assert.Equal(drivers.T, pcAmount, ls.Redeemed)
		assert.Equal(drivers.T, pcNonce, ls.Nonce)
		assert.Equal(drivers.T, pcLane, ls.ID)

		td.MustSerialize(os.Stdout)

		return nil
	}("happy path update")
	if err != nil {
		return err
	}

	err = func(testname string) error {
		td := drivers.NewTestDriver()
		td.Vector.Meta.Desc = testname

		// create the payment channel
		sender, _ := td.NewAccountActor(drivers.SECP, initialBal)
		receiver, receiverID := td.NewAccountActor(drivers.SECP, initialBal)
		paychAddr := chain.MustNewIDAddr(chain.MustIDFromAddress(receiverID) + 1)
		initRet := td.ComputeInitActorExecReturn(sender, 0, 0, paychAddr)

		td.UpdatePreStateRoot()

		msg := td.MessageProducer.CreatePaymentChannelActor(sender, receiver, chain.Value(toSend), chain.Nonce(0))
		td.ApplyExpect(
			msg,
			chain.MustSerialize(&initRet))
		td.AssertBalance(paychAddr, toSend)

		msg = td.MessageProducer.PaychUpdateChannelState(sender, paychAddr, &paych_spec.UpdateChannelStateParams{
			Sv: paych_spec.SignedVoucher{
				ChannelAddr:     paychAddr,
				TimeLockMin:     abi_spec.ChainEpoch(0),
				TimeLockMax:     0, // TimeLockMax set to 0 means no timeout
				SecretPreimage:  nil,
				Extra:           nil,
				Lane:            1,
				Nonce:           1,
				Amount:          toSend, // the amount that can be redeemed by receiver,
				MinSettleHeight: 0,
				Merges:          nil,
				Signature: &crypto_spec.Signature{
					Type: crypto_spec.SigTypeBLS,
					Data: []byte("signature goes here"),
				},
			},
		}, chain.Nonce(1), chain.Value(big_spec.Zero()))

		td.ApplyOk(msg)

		// settle the payment channel so it may be collected

		msg = td.MessageProducer.PaychSettle(receiver, paychAddr, nil, chain.Value(big_spec.Zero()), chain.Nonce(0))
		settleResult := td.ApplyOk(msg)

		// advance the epoch so the funds may be redeemed.
		td.ExeCtx.Epoch += paych_spec.SettleDelay

		msg = td.MessageProducer.PaychCollect(receiver, paychAddr, nil, chain.Nonce(1), chain.Value(big_spec.Zero()))
		collectResult := td.ApplyOk(msg)

		// receiver_balance = initial_balance + paych_send - settle_paych_msg_gas - collect_paych_msg_gas
		td.AssertBalance(receiver, big_spec.Sub(big_spec.Sub(big_spec.Add(toSend, initialBal), settleResult.Receipt.GasUsed.Big()), collectResult.Receipt.GasUsed.Big()))
		// the paych actor should have been deleted after the collect
		td.AssertNoActor(paychAddr)

		td.MustSerialize(os.Stdout)

		return nil
	}("happy path collect")
	if err != nil {
		return err
	}

	return nil
}
