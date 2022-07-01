package paych

import (
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	paychtypes "github.com/filecoin-project/go-state-types/builtin/v8/paych"
	builtin7 "github.com/filecoin-project/specs-actors/v7/actors/builtin"
	init7 "github.com/filecoin-project/specs-actors/v7/actors/builtin/init"
	paych7 "github.com/filecoin-project/specs-actors/v7/actors/builtin/paych"

	"github.com/filecoin-project/lotus/chain/actors"
	init_ "github.com/filecoin-project/lotus/chain/actors/builtin/init"
	"github.com/filecoin-project/lotus/chain/types"
)

type message7 struct{ from address.Address }

func (m message7) Create(to address.Address, initialAmount abi.TokenAmount) (*types.Message, error) {

	actorCodeID := builtin7.PaymentChannelActorCodeID

	params, aerr := actors.SerializeParams(&paych7.ConstructorParams{From: m.from, To: to})
	if aerr != nil {
		return nil, aerr
	}
	enc, aerr := actors.SerializeParams(&init7.ExecParams{
		CodeCID:           actorCodeID,
		ConstructorParams: params,
	})
	if aerr != nil {
		return nil, aerr
	}

	return &types.Message{
		To:     init_.Address,
		From:   m.from,
		Value:  initialAmount,
		Method: builtin7.MethodsInit.Exec,
		Params: enc,
	}, nil
}

func (m message7) Update(paych address.Address, sv *paychtypes.SignedVoucher, secret []byte) (*types.Message, error) {
	params, aerr := actors.SerializeParams(&paych7.UpdateChannelStateParams{

		Sv: *sv,

		Secret: secret,
	})
	if aerr != nil {
		return nil, aerr
	}

	return &types.Message{
		To:     paych,
		From:   m.from,
		Value:  abi.NewTokenAmount(0),
		Method: builtin7.MethodsPaych.UpdateChannelState,
		Params: params,
	}, nil
}

func (m message7) Settle(paych address.Address) (*types.Message, error) {
	return &types.Message{
		To:     paych,
		From:   m.from,
		Value:  abi.NewTokenAmount(0),
		Method: builtin7.MethodsPaych.Settle,
	}, nil
}

func (m message7) Collect(paych address.Address) (*types.Message, error) {
	return &types.Message{
		To:     paych,
		From:   m.from,
		Value:  abi.NewTokenAmount(0),
		Method: builtin7.MethodsPaych.Collect,
	}, nil
}
