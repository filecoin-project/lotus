package paych

import (
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"

	builtin2 "github.com/filecoin-project/specs-actors/actors/builtin"
	init2 "github.com/filecoin-project/specs-actors/actors/builtin/init"
	paych2 "github.com/filecoin-project/specs-actors/actors/builtin/paych"

	"github.com/filecoin-project/lotus/chain/actors"
	init_ "github.com/filecoin-project/lotus/chain/actors/builtin/init"
	"github.com/filecoin-project/lotus/chain/types"
)

type message2 struct{}

func (message2) Create(from, to address.Address, initialAmount abi.TokenAmount) (*types.Message, error) {
	params, aerr := actors.SerializeParams(&paych2.ConstructorParams{From: from, To: to})
	if aerr != nil {
		return nil, aerr
	}
	enc, aerr := actors.SerializeParams(&init2.ExecParams{
		CodeCID:           builtin2.PaymentChannelActorCodeID,
		ConstructorParams: params,
	})
	if aerr != nil {
		return nil, aerr
	}

	return &types.Message{
		To:     init_.Address,
		From:   from,
		Value:  initialAmount,
		Method: builtin2.MethodsInit.Exec,
		Params: enc,
	}, nil
}

func (message2) Update(from, paych address.Address, sv *SignedVoucher, secret []byte) (*types.Message, error) {
	params, aerr := actors.SerializeParams(&paych2.UpdateChannelStateParams{
		Sv:     *sv,
		Secret: secret,
	})
	if aerr != nil {
		return nil, aerr
	}

	return &types.Message{
		To:     paych,
		From:   from,
		Value:  abi.NewTokenAmount(0),
		Method: builtin2.MethodsPaych.UpdateChannelState,
		Params: params,
	}, nil
}

func (message2) Settle(from, paych address.Address) (*types.Message, error) {
	return &types.Message{
		To:     paych,
		From:   from,
		Value:  abi.NewTokenAmount(0),
		Method: builtin2.MethodsPaych.Settle,
	}, nil
}

func (message2) Collect(from, paych address.Address) (*types.Message, error) {
	return &types.Message{
		To:     paych,
		From:   from,
		Value:  abi.NewTokenAmount(0),
		Method: builtin2.MethodsPaych.Collect,
	}, nil
}
