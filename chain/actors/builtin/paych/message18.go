package paych

import (
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	actorstypes "github.com/filecoin-project/go-state-types/actors"
	builtin18 "github.com/filecoin-project/go-state-types/builtin"
	init18 "github.com/filecoin-project/go-state-types/builtin/v18/init"
	paych18 "github.com/filecoin-project/go-state-types/builtin/v18/paych"
	paychtypes "github.com/filecoin-project/go-state-types/builtin/v8/paych"

	"github.com/filecoin-project/lotus/chain/actors"
	init_ "github.com/filecoin-project/lotus/chain/actors/builtin/init"
	"github.com/filecoin-project/lotus/chain/types"
)

type message18 struct{ from address.Address }

func (m message18) Create(to address.Address, initialAmount abi.TokenAmount) (*types.Message, error) {

	actorCodeID, ok := actors.GetActorCodeID(actorstypes.Version18, "paymentchannel")
	if !ok {
		return nil, xerrors.Errorf("error getting actor paymentchannel code id for actor version %d", 18)
	}

	params, aerr := actors.SerializeParams(&paych18.ConstructorParams{From: m.from, To: to})
	if aerr != nil {
		return nil, aerr
	}
	enc, aerr := actors.SerializeParams(&init18.ExecParams{
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
		Method: builtin18.MethodsInit.Exec,
		Params: enc,
	}, nil
}

func (m message18) Update(paych address.Address, sv *paychtypes.SignedVoucher, secret []byte) (*types.Message, error) {
	params, aerr := actors.SerializeParams(&paych18.UpdateChannelStateParams{

		Sv: toV18SignedVoucher(*sv),

		Secret: secret,
	})
	if aerr != nil {
		return nil, aerr
	}

	return &types.Message{
		To:     paych,
		From:   m.from,
		Value:  abi.NewTokenAmount(0),
		Method: builtin18.MethodsPaych.UpdateChannelState,
		Params: params,
	}, nil
}

func toV18SignedVoucher(sv paychtypes.SignedVoucher) paych18.SignedVoucher {
	merges := make([]paych18.Merge, len(sv.Merges))
	for i := range sv.Merges {
		merges[i] = paych18.Merge{
			Lane:  sv.Merges[i].Lane,
			Nonce: sv.Merges[i].Nonce,
		}
	}

	return paych18.SignedVoucher{
		ChannelAddr:     sv.ChannelAddr,
		TimeLockMin:     sv.TimeLockMin,
		TimeLockMax:     sv.TimeLockMax,
		SecretHash:      sv.SecretHash,
		Extra:           (*paych18.ModVerifyParams)(sv.Extra),
		Lane:            sv.Lane,
		Nonce:           sv.Nonce,
		Amount:          sv.Amount,
		MinSettleHeight: sv.MinSettleHeight,
		Merges:          merges,
		Signature:       sv.Signature,
	}
}

func (m message18) Settle(paych address.Address) (*types.Message, error) {
	return &types.Message{
		To:     paych,
		From:   m.from,
		Value:  abi.NewTokenAmount(0),
		Method: builtin18.MethodsPaych.Settle,
	}, nil
}

func (m message18) Collect(paych address.Address) (*types.Message, error) {
	return &types.Message{
		To:     paych,
		From:   m.from,
		Value:  abi.NewTokenAmount(0),
		Method: builtin18.MethodsPaych.Collect,
	}, nil
}
