package paych

import (
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	actorstypes "github.com/filecoin-project/go-state-types/actors"
	builtin8 "github.com/filecoin-project/go-state-types/builtin"
	init8 "github.com/filecoin-project/go-state-types/builtin/v8/init"
	paych8 "github.com/filecoin-project/go-state-types/builtin/v8/paych"
	paychtypes "github.com/filecoin-project/go-state-types/builtin/v8/paych"

	"github.com/filecoin-project/lotus/chain/actors"
	init_ "github.com/filecoin-project/lotus/chain/actors/builtin/init"
	"github.com/filecoin-project/lotus/chain/types"
)

type message8 struct{ from address.Address }

func (m message8) Create(to address.Address, initialAmount abi.TokenAmount) (*types.Message, error) {

	actorCodeID, ok := actors.GetActorCodeID(actorstypes.Version8, "paymentchannel")
	if !ok {
		return nil, xerrors.Errorf("error getting actor paymentchannel code id for actor version %d", 8)
	}

	params, aerr := actors.SerializeParams(&paych8.ConstructorParams{From: m.from, To: to})
	if aerr != nil {
		return nil, aerr
	}
	enc, aerr := actors.SerializeParams(&init8.ExecParams{
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
		Method: builtin8.MethodsInit.Exec,
		Params: enc,
	}, nil
}

func (m message8) Update(paych address.Address, sv *paychtypes.SignedVoucher, secret []byte) (*types.Message, error) {
	params, aerr := actors.SerializeParams(&paych8.UpdateChannelStateParams{

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
