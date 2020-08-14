package main

import (
	"os"
	"bytes"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/chain/vm"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/abi/big"
	init_ "github.com/filecoin-project/specs-actors/actors/builtin/init"
	builtin "github.com/filecoin-project/specs-actors/actors/builtin"
	"github.com/filecoin-project/specs-actors/actors/runtime"
	//"github.com/filecoin-project/specs-actors/actors/builtin/paych"
	"github.com/filecoin-project/specs-actors/actors/builtin/multisig"
	//"github.com/filecoin-project/specs-actors/actors/crypto"
	"github.com/filecoin-project/specs-actors/actors/runtime/exitcode"

	. "github.com/filecoin-project/oni/tvx/builders"
	"github.com/filecoin-project/oni/tvx/schema"
	//"github.com/davecgh/go-spew/spew"
)

func main() {
	nestedSends_OkBasic()
}

func nestedSends_OkBasic() {
	var acctDefaultBalance = abi.NewTokenAmount(1_000_000_000_000)
	var multisigBalance = abi.NewTokenAmount(1_000_000_000)
	nonce := uint64(1)

	metadata := &schema.Metadata{ID: "nested-sends-ok-basic", Version: "v1", Desc: ""}

	v := MessageVector(metadata)
	v.Messages.SetDefaults(GasLimit(1_000_000_000), GasPrice(1))

	stage := prepareStage(v, acctDefaultBalance, multisigBalance)
	balanceBefore := v.Actors.Balance(stage.creator)

	// Multisig sends back to the creator.
	amtSent := abi.NewTokenAmount(1)
	result := stage.sendOk(stage.creator, amtSent, builtin.MethodSend, nil, nonce)

	//td.AssertActor(stage.creator, big.Sub(big.Add(balanceBefore, amtSent), result.Receipt.GasUsed.Big()), nonce+1)
	v.Assert.NonceEq(stage.creator, nonce+1)
	v.Assert.BalanceEq(stage.creator, big.Sub(big.Add(balanceBefore, amtSent), big.NewInt(result.MessageReceipt.GasUsed)))

	v.Finish(os.Stdout)
}


type msStage struct {
	v  *Builder
	creator address.Address // Address of the creator and sole signer of the multisig.
	msAddr  address.Address // Address of the multisig actor from which nested messages are sent.
}

// Creates a multisig actor with its creator as sole approver.
func prepareStage(v *Builder, creatorBalance, msBalance abi.TokenAmount) *msStage {
	// Set up sender and receiver accounts.
	creator := v.Actors.Account(address.SECP256K1, creatorBalance)
	v.CommitPreconditions()

	msg := v.Messages.Sugar().CreateMultisigActor(creator.ID, []address.Address{creator.ID}, 0, 1, Value(msBalance), Nonce(0))
	v.Messages.ApplyOne(msg)

	v.Assert.Equal(msg.Result.ExitCode, exitcode.Ok)

	// Verify init actor return.
	var ret init_.ExecReturn
	MustDeserialize(msg.Result.Return, &ret)


	return &msStage{
		v:  v,
		creator: creator.ID,
		msAddr:  ret.IDAddress,
	}
}

//func (s *msStage) sendOk(to address.Address, value abi.TokenAmount, method abi.MethodNum, params runtime.CBORMarshaler, approverNonce uint64) vtypes.ApplyMessageResult {
func (s *msStage) sendOk(to address.Address, value abi.TokenAmount, method abi.MethodNum, params runtime.CBORMarshaler, approverNonce uint64) *vm.ApplyRet {
	buf := bytes.Buffer{}
	if params != nil {
		err := params.MarshalCBOR(&buf)
		if err != nil {
			panic(err)
		}
		//require.NoError(drivers.T, err)
	}
	pparams := multisig.ProposeParams{
		To:     to,
		Value:  value,
		Method: method,
		Params: buf.Bytes(),
	}
	msg := s.v.Messages.Typed(s.creator, s.msAddr, MultisigPropose(&pparams), Nonce(approverNonce), Value(big.NewInt(0)))
	//result := s.driver.ApplyMessage(msg)
	s.v.CommitApplies()
	//s.v.Assert.Equal(exitcode_spec.Ok, result.Receipt.ExitCode)

	// all messages succeeded.
	s.v.Assert.EveryMessageResultSatisfies(ExitCode(exitcode.Ok))

	return msg.Result
}

//func (s *msStage) state() *multisig.State {
	//var msState multisig.State
	//s.driver.GetActorState(s.msAddr, &msState)
	//return &msState
//}
