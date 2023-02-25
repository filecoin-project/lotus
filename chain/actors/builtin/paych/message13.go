package paych

import (
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	actorstypes "github.com/filecoin-project/go-state-types/actors"
	builtin13 "github.com/filecoin-project/go-state-types/builtin"
	init13 "github.com/filecoin-project/go-state-types/builtin/v13/init"
	paych13 "github.com/filecoin-project/go-state-types/builtin/v13/paych"
	paychtypes "github.com/filecoin-project/go-state-types/builtin/v8/paych"

	"github.com/filecoin-project/lotus/chain/actors"
	init_ "github.com/filecoin-project/lotus/chain/actors/builtin/init"
	"github.com/filecoin-project/lotus/chain/types"
)

type message13 struct{ from address.Address }

func (m message13) Create(to address.Address, initialAmount abi.TokenAmount) (*types.Message, error) {

	actorCodeID, ok := actors.GetActorCodeID(actorstypes.Version13, "paymentchannel")
	if !ok {
		return nil, xerrors.Errorf("error getting actor paymentchannel code id for actor version %d", 13)
	}

	params, aerr := actors.SerializeParams(&paych13.ConstructorParams{From: m.from, To: to})
	if aerr != nil {
		return nil, aerr
	}
	enc, aerr := actors.SerializeParams(&init13.ExecParams{
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
		Method: builtin13.MethodsInit.Exec,
		Params: enc,
	}, nil
}

func (m message13) Update(paych address.Address, sv *paychtypes.SignedVoucher, secret []byte) (*types.Message, error) {
	params, aerr := actors.SerializeParams(&paych13.UpdateChannelStateParams{

		Sv: toV13SignedVoucher(*sv),

		Secret: secret,
	})
	if aerr != nil {
		return nil, aerr
	}

	return &types.Message{
		To:     paych,
		From:   m.from,
		Value:  abi.NewTokenAmount(0),
		Method: builtin13.MethodsPaych.UpdateChannelState,
		Params: params,
	}, nil
}

func toV13SignedVoucher(sv paychtypes.SignedVoucher) paych13.SignedVoucher {
	merges := make([]paych13.Merge, len(sv.Merges))
	for i := range sv.Merges {
		merges[i] = paych13.Merge{
			Lane:  sv.Merges[i].Lane,
			Nonce: sv.Merges[i].Nonce,
		}
	}

	return paych13.SignedVoucher{
		ChannelAddr:     sv.ChannelAddr,
		TimeLockMin:     sv.TimeLockMin,
		TimeLockMax:     sv.TimeLockMax,
		SecretHash:      sv.SecretHash,
		Extra:           (*paych13.ModVerifyParams)(sv.Extra),
		Lane:            sv.Lane,
		Nonce:           sv.Nonce,
		Amount:          sv.Amount,
		MinSettleHeight: sv.MinSettleHeight,
		Merges:          merges,
		Signature:       sv.Signature,
	}
}

func (m message13) Settle(paych address.Address) (*types.Message, error) {
	return &types.Message{
		To:     paych,
		From:   m.from,
		Value:  abi.NewTokenAmount(0),
		Method: builtin13.MethodsPaych.Settle,
	}, nil
}

func (m message13) Collect(paych address.Address) (*types.Message, error) {
	return &types.Message{
		To:     paych,
		From:   m.from,
		Value:  abi.NewTokenAmount(0),
		Method: builtin13.MethodsPaych.Collect,
	}, nil
}
