package paych

import (
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"

	builtin8 "github.com/filecoin-project/specs-actors/v8/actors/builtin"
	init8 "github.com/filecoin-project/specs-actors/v8/actors/builtin/init"
	paych8 "github.com/filecoin-project/specs-actors/v8/actors/builtin/paych"

	"github.com/filecoin-project/lotus/chain/actors"
	init_ "github.com/filecoin-project/lotus/chain/actors/builtin/init"
	"github.com/filecoin-project/lotus/chain/types"
)

type message8 struct{ from address.Address }

func (m message8) Create(to address.Address, initialAmount abi.TokenAmount) (*types.Message, error) {
	params, aerr := actors.SerializeParams(&paych8.ConstructorParams{From: m.from, To: to})
	if aerr != nil {
		return nil, aerr
	}
	enc, aerr := actors.SerializeParams(&init8.ExecParams{
		CodeCID:           builtin8.PaymentChannelActorCodeID,
		ConstructorParams: params,
	})
	if aerr != nil {
		return nil, aerr
	}

	return &types.Message{
		To:     init_.Address,
		From:   m.from,
		Value:  initialAmount,
		Method: builtin8.MethodsInit.Exec,
		Params: enc,
	}, nil
}

func (m message8) Update(paych address.Address, sv *SignedVoucher, secret []byte) (*types.Message, error) {
	params, aerr := actors.SerializeParams(&paych8.UpdateChannelStateParams{

		Sv: toV8SignedVoucher(*sv),

		Secret: secret,
	})
	if aerr != nil {
		return nil, aerr
	}

	return &types.Message{
		To:     paych,
		From:   m.from,
		Value:  abi.NewTokenAmount(0),
		Method: builtin8.MethodsPaych.UpdateChannelState,
		Params: params,
	}, nil
}

func (m message8) Settle(paych address.Address) (*types.Message, error) {
	return &types.Message{
		To:     paych,
		From:   m.from,
		Value:  abi.NewTokenAmount(0),
		Method: builtin8.MethodsPaych.Settle,
	}, nil
}

func (m message8) Collect(paych address.Address) (*types.Message, error) {
	return &types.Message{
		To:     paych,
		From:   m.from,
		Value:  abi.NewTokenAmount(0),
		Method: builtin8.MethodsPaych.Collect,
	}, nil
}
