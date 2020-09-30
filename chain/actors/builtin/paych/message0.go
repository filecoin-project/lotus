package paych

import (
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"

	builtin0 "github.com/filecoin-project/specs-actors/actors/builtin"
	init0 "github.com/filecoin-project/specs-actors/actors/builtin/init"
	paych0 "github.com/filecoin-project/specs-actors/actors/builtin/paych"

	"github.com/filecoin-project/lotus/chain/actors"
	init_ "github.com/filecoin-project/lotus/chain/actors/builtin/init"
	"github.com/filecoin-project/lotus/chain/types"
)

type message0 struct{}

func (message0) Create(from, to address.Address, initialAmount abi.TokenAmount) (*types.Message, error) {
	params, aerr := actors.SerializeParams(&paych0.ConstructorParams{From: from, To: to})
	if aerr != nil {
		return nil, aerr
	}
	enc, aerr := actors.SerializeParams(&init0.ExecParams{
		CodeCID:           builtin0.PaymentChannelActorCodeID,
		ConstructorParams: params,
	})
	if aerr != nil {
		return nil, aerr
	}

	return &types.Message{
		To:     init_.Address,
		From:   from,
		Value:  initialAmount,
		Method: builtin0.MethodsInit.Exec,
		Params: enc,
	}, nil
}

func (message0) Update(from, paych address.Address, sv *SignedVoucher, secret []byte) (*types.Message, error) {
	params, aerr := actors.SerializeParams(&paych0.UpdateChannelStateParams{
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
		Method: builtin0.MethodsPaych.UpdateChannelState,
		Params: params,
	}, nil
}

func (message0) Settle(from, paych address.Address) (*types.Message, error) {
	return &types.Message{
		To:     paych,
		From:   from,
		Value:  abi.NewTokenAmount(0),
		Method: builtin0.MethodsPaych.Settle,
	}, nil
}

func (message0) Collect(from, paych address.Address) (*types.Message, error) {
	return &types.Message{
		To:     paych,
		From:   from,
		Value:  abi.NewTokenAmount(0),
		Method: builtin0.MethodsPaych.Collect,
	}, nil
}
